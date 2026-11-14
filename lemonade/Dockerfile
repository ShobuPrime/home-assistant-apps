# Global ARGs must be declared before the first FROM
ARG BUILD_FROM
ARG LEMONADE_VERSION=2026.46.2

# ---------------------------------------------------------------------------
# Stage 1: Fetch Lemonade and collect the glibc runtime it needs
#
# Upstream ships no musl build of `lemond`, and the llama.cpp backends
# Lemonade downloads at runtime are glibc builds too, so a glibc runtime has to
# come along. Rather than switching this app to a Debian base image (which
# would cost ~45 MB of base alone and diverge from every other app in this
# repo), we bundle just the glibc closure into the standard Alpine base — the
# same approach muninndb already uses here.
#
# The donor's glibc must be at least what lemond was linked against, and that
# moves: upstream builds amd64 on `ubuntu-latest` and arm64 on `ubuntu-24.04-arm`.
# 2026.45.3's amd64 lemond needs GLIBC_2.43, which ruled out Debian trixie (2.41).
# Ubuntu 26.04 matches that runner and is a pinned LTS.
# ---------------------------------------------------------------------------
FROM mirror.gcr.io/library/ubuntu:26.04 AS glibc-provider

ARG LEMONADE_VERSION

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        libdrm-amdgpu1 \
        libgcc-s1 \
        libgomp1 \
        libssl3t64 \
        libstdc++6 \
        libzstd1 \
        zlib1g \
    && rm -rf /var/lib/apt/lists/*

# The "embeddable" archive is the right artifact: just `lemond`, the `lemonade`
# CLI and `resources/`, with no systemd unit and no distro package dependencies.
RUN \
    ARCH="$(uname -m)" \
    && if [ "${ARCH}" = "aarch64" ]; then \
        LEMONADE_ARCH="arm64"; \
    elif [ "${ARCH}" = "x86_64" ]; then \
        LEMONADE_ARCH="x64"; \
    else \
        echo "Unsupported architecture: ${ARCH}"; \
        exit 1; \
    fi \
    && echo "Downloading Lemonade v${LEMONADE_VERSION} for ${LEMONADE_ARCH}..." \
    && curl -L -f -S -o /tmp/lemonade.tar.gz \
        "https://github.com/lemonade-sdk/lemonade/releases/download/v${LEMONADE_VERSION}/lemonade-embeddable-${LEMONADE_VERSION}-ubuntu-${LEMONADE_ARCH}.tar.gz" \
    && mkdir -p /opt/lemonade \
    && tar -xzf /tmp/lemonade.tar.gz --strip-components=1 -C /opt/lemonade \
    && rm /tmp/lemonade.tar.gz \
    && chmod +x /opt/lemonade/lemond /opt/lemonade/lemonade \
    && test -f /opt/lemonade/resources/server_models.json

# Collect the closure with `ldd` rather than a hand-written library list, so a
# new upstream dependency can't silently go missing.
#
# Two categories have to be added by hand, because `ldd` here can only see the
# binaries present at BUILD time and Lemonade downloads its backends at RUNTIME:
#
#   libgomp    every llama.cpp build links OpenMP. Without it the backend dies
#              with "libgomp.so.1: cannot open shared object file" (exit 127)
#              the moment a model loads, while lemond itself looks healthy.
#
#   libdl,     glibc 2.34 merged these into libc and now ships them only as
#   libpthread tiny compatibility stubs. lemond is built against that glibc so
#   librt,     it does not reference them, and they never enter the ldd closure
#   libresolv, — but the runtime-downloaded backends ARE linked against them:
#   libutil
#                moonshine-server  -> libdl, libpthread
#                libonnxruntime.so -> libdl, libpthread, librt
#
#              Their absence presents as `moonshine-server: error while loading
#              shared libraries: libdl.so.2` and exit 127, i.e. "Failed to load
#              model: moonshine-server failed to start or become ready" with no
#              hint of the real cause. ~121 KB for all five, so they are copied
#              unconditionally rather than guessed at per backend.
#
# The general rule this encodes: anything Lemonade fetches at runtime may need
# a library `ldd` cannot discover here. When a new backend fails with exit 127,
# read its DT_NEEDED (`readelf -d`) rather than adding symbols one at a time.
RUN set -eu \
    # ldd reports a too-old glibc only as a message; the donor must satisfy it here.
    && if ldd /opt/lemonade/lemond /opt/lemonade/lemonade 2>&1 | grep 'not found'; then \
        echo "GLIBC DONOR TOO OLD: see the requirement above; update glibc-provider's FROM" ; exit 1 ; \
    fi \
    && mkdir -p /glibc/lib \
    && LIBGOMP="$(find /usr/lib -name 'libgomp.so.1' | head -1)" \
    && COMPAT="" \
    && for stub in libdl.so.2 libpthread.so.0 librt.so.1 libresolv.so.2 libutil.so.1 ; do \
        p="$(find /lib /usr/lib -name "${stub}" 2>/dev/null | head -1)" ; \
        test -n "${p}" || { echo "MISSING glibc compat stub: ${stub}" ; exit 1 ; } ; \
        COMPAT="${COMPAT} ${p}" ; \
    done \
    && for lib in \
        $(ldd /opt/lemonade/lemond /opt/lemonade/lemonade "${LIBGOMP}" \
            | awk '$3 ~ /^\// { print $3 }' | sort -u) \
        "${LIBGOMP}" ${COMPAT} ; do \
        cp -L "${lib}" /glibc/lib/ ; \
    done \
    # The ELF interpreter path is baked into every binary, including the
    # llama.cpp ones fetched at runtime, so the loader must end up at exactly
    # the path the donor used. Record it for the runtime stage.
    && LOADER="$(ldd /opt/lemonade/lemond | awk '/ld-linux/ { print $1 }' | head -1)" \
    && test -n "${LOADER}" \
    && cp -L "${LOADER}" /glibc/ld.so \
    && echo "${LOADER}" > /glibc/loader-path \
    && echo "Bundled glibc closure:" && ls -1 /glibc/lib

# ---------------------------------------------------------------------------
# Stage 1b: Web UI assets and the resources the embeddable archive omits
#
# web-app/ is the application, served at / and /web-app/. static/ is the status
# page at /api/v1. Without web-app/ the ingress panel is an empty page and
# nothing logs an error.
#
# Published for amd64 only. Everything copied from here is architecture
# independent and nothing in this stage is executed.
#
# Pinned to the binaries' version. update-lemonade.sh verifies this tag exists
# before proposing a bump.
# ---------------------------------------------------------------------------
FROM --platform=linux/amd64 ghcr.io/lemonade-sdk/lemonade-server:v${LEMONADE_VERSION} AS webui

# ---------------------------------------------------------------------------
# Stage 2: Build the Home Assistant bridge
#
# A small Go sidecar that fronts lemond and adds what Home Assistant needs and
# Lemonade does not provide: long-term memory in MuninnDB, Origin handling, and
# a Wyoming server so Home Assistant can discover Lemonade as a Speech-to-text
# engine. Built CGO-free so
# it is a static binary that runs on musl without touching the glibc closure
# above.
# ---------------------------------------------------------------------------
FROM mirror.gcr.io/library/golang:1.27.3-alpine AS bridge-builder

WORKDIR /src
COPY ha-lemonade-bridge/go.mod ./
COPY ha-lemonade-bridge/*.go ./

# Do NOT try to smoke-test the binary here by running it: the bridge takes no
# flags and goes straight to ListenAndServe, so any attempt to exec it during
# the build blocks forever. Verify it exists instead.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /ha-lemonade-bridge . \
    && test -x /ha-lemonade-bridge \
    && ls -lh /ha-lemonade-bridge

# ---------------------------------------------------------------------------
# Stage 3: Runtime image on the repo-standard Alpine base
# ---------------------------------------------------------------------------
FROM $BUILD_FROM

# S6-overlay shutdown timings.
#
# This matters more than it looks. Home Assistant stops the container for every
# HAOS update and every backup that includes app data, and the Supervisor's
# default stop timeout is only 10s (`ATTR_TIMEOUT, default=10`) before Docker
# SIGKILLs it.
#
# S6's own default grace is 3000ms, which is SHORTER than lemond needs: it has
# to unload the model and terminate the llama-server child, measured at ~2.1s
# for a 230M model and longer for bigger ones. The result was
# `s6-svwait: fatal: timed out` in the shutdown log — S6 giving up and killing
# lemond mid-cleanup, while the container still exited 0 and looked healthy.
#
# 20s comfortably covers a large model's unload while still fitting inside the
# 30s `timeout:` declared in config.yaml. Raising one without the other
# reintroduces the truncated shutdown.
ENV S6_SERVICES_GRACETIME=20000 \
    S6_KILL_FINISH_MAXTIME=20000

# Always upgrade first to resolve base image package version conflicts
# (libcrypto3/libssl3 vs openssl).
#
# These are the musl-side tools only. Lemonade's own TLS/compression libraries
# come from the bundled glibc closure, never from these packages.
RUN \
    apk upgrade --no-cache \
    && apk add --no-cache \
        ca-certificates \
        curl \
        jq \
        tar \
        unzip \
        xz

# Bundled glibc runtime + the Lemonade binaries
COPY --from=glibc-provider /glibc /opt/glibc
COPY --from=glibc-provider /opt/lemonade /opt/lemonade

# HA bridge (static, CGO-free — needs none of the glibc closure above)
COPY --from=bridge-builder /ha-lemonade-bridge /opt/ha-lemonade-bridge/ha-lemonade-bridge

# Absent from the embeddable archive (see the webui stage above).
#   architecture_defaults.json  per-model-family sampler defaults
#   docs/                       served at /api/v1/docs and to MCP clients
COPY --from=webui /opt/lemonade/resources/static /opt/lemonade/resources/static
COPY --from=webui /opt/lemonade/resources/web-app /opt/lemonade/resources/web-app
COPY --from=webui /opt/lemonade/resources/docs /opt/lemonade/resources/docs
COPY --from=webui /opt/lemonade/resources/architecture_defaults.json /opt/lemonade/resources/architecture_defaults.json

# Load ha-addon-shim.js (from rootfs) ahead of the app's bundles, and fail the
# build if the client stops reading the settings the shim sets. See
# UPDATE_GUIDE.md "If the Web UI breaks".
RUN set -e; \
    WEBAPP=/opt/lemonade/resources/web-app; \
    sed -i 's#<script defer="defer"#<script defer="defer" src="ha-addon-shim.js"></script>&#' "${WEBAPP}/index.html"; \
    [ "$(grep -o 'ha-addon-shim\.js"></script><script defer="defer"' "${WEBAPP}/index.html" | wc -l)" = "1" ] \
        || { echo "PATCH FAILED: expected the shim once, ahead of the deferred bundles in web-app/index.html"; exit 1; }; \
    for token in lemonade_base_url baseURL; do \
        grep -qF "${token}" "${WEBAPP}"/*.js \
            || { echo "PATCH FAILED: '${token}' not found in the web app; the shim no longer sets its API base"; exit 1; }; \
    done

# Put the loader where the binaries expect it (/lib64/ld-linux-x86-64.so.2 on
# amd64, /lib/ld-linux-aarch64.so.1 on aarch64). musl's own loader is named
# ld-musl-*.so.1, so there is no collision with Alpine's.
#
# LD_LIBRARY_PATH is deliberately NOT set globally: Alpine's own curl/jq/bashio
# would then resolve glibc's libz/libssl and break. It is applied per-exec by
# the `lemonade` wrapper and the S6 run script instead.
RUN set -eu \
    && LOADER="$(cat /opt/glibc/loader-path)" \
    && mkdir -p "$(dirname "${LOADER}")" \
    && cp /opt/glibc/ld.so "${LOADER}" \
    && chmod +x "${LOADER}" \
    && rm -f /opt/glibc/ld.so /opt/glibc/loader-path \
    && chmod +x /opt/lemonade/lemond /opt/lemonade/lemonade \
    && ls -lh /opt/lemonade \
    # Proves the bundled glibc actually runs the binary on musl.
    && LD_LIBRARY_PATH=/opt/glibc/lib /opt/lemonade/lemond --version

# Copy root filesystem
COPY rootfs /

RUN chmod a+x /etc/cont-init.d/*.sh \
    && chmod a+x /etc/services.d/*/run \
    && chmod a+x /etc/services.d/*/finish \
    && chmod a+x /usr/local/bin/lemonade \
    && chmod a+x /opt/ha-lemonade-bridge/ha-lemonade-bridge

# Healthcheck — Supervisor reads this to gate the STARTUP -> STARTED state
# transition (yellow -> green dot in the HA UI) on the server actually being
# reachable. Mirrors the `watchdog:` endpoint in config.yaml.
# start-period is generous: the first boot downloads the llama.cpp backend.
HEALTHCHECK --interval=30s --timeout=5s --start-period=120s --retries=3 \
    CMD curl -fs http://127.0.0.1:13305/live || exit 1

# Build arguments
ARG BUILD_ARCH
ARG BUILD_DATE
ARG BUILD_DESCRIPTION
ARG BUILD_NAME
ARG BUILD_REF
ARG BUILD_REPOSITORY
ARG BUILD_VERSION
# Re-declared: the global ARG was consumed by the glibc-provider stage.
ARG LEMONADE_VERSION

# Labels
LABEL \
    io.hass.name="${BUILD_NAME}" \
    io.hass.description="${BUILD_DESCRIPTION}" \
    io.hass.arch="${BUILD_ARCH}" \
    io.hass.type="addon" \
    io.hass.version=${BUILD_VERSION} \
    maintainer="Lemonade for Home Assistant" \
    lemonade.version="${LEMONADE_VERSION}" \
    org.opencontainers.image.title="${BUILD_NAME}" \
    org.opencontainers.image.description="${BUILD_DESCRIPTION}" \
    org.opencontainers.image.vendor="Home Assistant Apps" \
    org.opencontainers.image.authors="Lemonade for Home Assistant" \
    org.opencontainers.image.licenses="Apache-2.0" \
    org.opencontainers.image.url="https://lemonade-server.ai" \
    org.opencontainers.image.source="https://github.com/lemonade-sdk/lemonade" \
    org.opencontainers.image.documentation="https://lemonade-server.ai/docs/" \
    org.opencontainers.image.created=${BUILD_DATE} \
    org.opencontainers.image.revision=${BUILD_REF} \
    org.opencontainers.image.version=${BUILD_VERSION}
