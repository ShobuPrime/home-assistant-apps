# Global ARGs must be declared before the first FROM
ARG BUILD_FROM
ARG LEMONADE_VERSION=11.5.1

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
# Debian *trixie* is the donor, not bookworm: `lemond` needs GLIBC_2.38 and
# GLIBCXX_3.4.32, and bookworm only has glibc 2.36 / libstdc++ 12.
# ---------------------------------------------------------------------------
FROM mirror.gcr.io/library/debian:trixie-slim AS glibc-provider

ARG LEMONADE_VERSION

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        libdrm-amdgpu1 \
        libgcc-s1 \
        libgomp1 \
        libssl3 \
        libstdc++6 \
        libzstd1 \
        zlib1g \
    && rm -rf /var/lib/apt/lists/*

# The "embeddable" archive is the right artifact: just `lemond`, the `lemonade`
# CLI and `resources/`, with no systemd unit and no dependency on Debian
# backports (the .deb needs libcpp-httplib0.41 from trixie-backports).
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
    # the path Debian used. Record it for the runtime stage.
    && LOADER="$(ldd /opt/lemonade/lemond | awk '/ld-linux/ { print $1 }' | head -1)" \
    && test -n "${LOADER}" \
    && cp -L "${LOADER}" /glibc/ld.so \
    && echo "${LOADER}" > /glibc/loader-path \
    && echo "Bundled glibc closure:" && ls -1 /glibc/lib

# ---------------------------------------------------------------------------
# Stage 1b: Web UI assets
#
# The embeddable archive ships the binaries and JSON resources but NOT the web
# UI. Without this, lemond starts and serves its APIs perfectly well, then logs
# `Could not open index.html at: /opt/lemonade/resources/static/index.html` and
# the ingress panel 404s — the failure is invisible unless you open the UI.
#
# The UI lives in TWO directories, and both are needed:
#
#   resources/static/    ~132 KB  "Lemonade Server" landing page, served at /
#   resources/web-app/   ~4.5 MB  "Lemonade App" — the actual chat UI, served
#                                 at /web-app/ (routes `/web-app/?` and
#                                 `/web-app/(.+)` in the lemond binary)
#
# Copying only `static/` — as this did until 11.5.1 — yields a server whose
# root page loads fine, so the UI looks present, while the application itself
# 404s. Nothing on the landing page links to /web-app/, so there is no way to
# reach it and no error anywhere; it simply does not exist. Ingress lands on
# the landing page and that is all the user ever sees.
#
# web-app/ uses RELATIVE asset paths (`src="renderer.bundle.js"`), so its
# ASSETS load unchanged behind ingress path prefixing — which is what makes the
# `ingress_entry: web-app/` in config.yaml safe. Its API calls are a different
# matter and are patched below; assets being relative is what made the panel
# render while the app behind it could not reach anything.
#
# The UI exists only in upstream's container image, which is published for
# amd64 ONLY. That is fine here: both directories are HTML/JS/CSS/fonts with
# no native code (verified: no .so/.node/.bin), so they are architecture
# independent. Nothing from this stage is ever executed — files are copied out
# and that is all — so pulling an amd64 image while building for aarch64 is
# safe.
#
# Pinned to the same version as the binaries so the UI can never drift from the
# server it talks to. update-lemonade.sh verifies this tag exists before
# proposing a version bump.
# ---------------------------------------------------------------------------
FROM --platform=linux/amd64 ghcr.io/lemonade-sdk/lemonade-server:v${LEMONADE_VERSION} AS webui

# ---------------------------------------------------------------------------
# Stage 2: Build the Home Assistant bridge
#
# A small Go sidecar that fronts lemond and adds what Home Assistant needs and
# Lemonade does not provide: long-term memory in MuninnDB, the web-app fixes
# for ingress and Android, Origin handling, and a Wyoming server so Home
# Assistant can discover Lemonade as a Speech-to-text engine. Built CGO-free so
# it is a static binary that runs on musl without touching the glibc closure
# above.
# ---------------------------------------------------------------------------
FROM mirror.gcr.io/library/golang:1.26.5-alpine AS bridge-builder

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

# Web UI — absent from the embeddable archive (see the webui stage above).
# BOTH directories are required: static/ is the landing page at /, web-app/ is
# the actual application at /web-app/. Copying only static/ leaves the app
# silently missing.
COPY --from=webui /opt/lemonade/resources/static /opt/lemonade/resources/static
COPY --from=webui /opt/lemonade/resources/web-app /opt/lemonade/resources/web-app

# ---------------------------------------------------------------------------
# Two patches to upstream's web app, both required to make it usable inside
# Home Assistant. Each asserts its anchor matched and FAILS THE BUILD if not —
# upstream ships roughly weekly, and a silently-no-op patch would quietly
# reintroduce the bug it fixes.
#
# 1. ANDROID: the app's entry module redirects to the Play Store on any
#    Android user-agent:
#
#      /android/i.test(ua) ? location.href="https://play.google.com/..."
#      : /iPad|iPhone|iPod/.test(ua) ? location.href="https://apps.apple.com/..."
#      : createRoot(...).render(<App/>)
#
#    The else-branch is the app mount, so on Android the UI never renders at
#    all — the HA Companion app leaves for the Play Store the moment the panel
#    opens. Companion only supplies the user-agent; Chrome on Android does the
#    same thing, so no Companion setting can fix it. The shim rewrites the
#    "Android" token so the ternary falls through to mounting. "Mobile Safari"
#    is left intact, so the app still knows it is on a phone.
#
# 2. INGRESS: in browser mode the app sets its API base to
#    `window.location.origin` — origin ONLY, no path. Under ingress the page
#    lives at /api/hassio_ingress/<token>/web-app/, so every API call and the
#    log WebSocket go to the Home Assistant frontend root instead of the
#    add-on, and the UI sits on "connecting" with an empty log view forever.
#    The shim publishes the correct base (origin + the ingress prefix, empty on
#    direct access) and the bundle is patched to prefer it.
#
# The shim is an EXTERNAL script deliberately: it must run before the deferred
# bundle, and inline <script> tags cannot be deferred. lemond injects its own
# `window.api` shim immediately before </head> at serve time — i.e. AFTER
# everything in this file — so the shim only touches a plain global and never
# window.api, which lemond would overwrite.
RUN set -e; \
    WEBAPP=/opt/lemonade/resources/web-app; \
    printf '%s\n' \
        '// Injected by the Home Assistant add-on. See lemonade/Dockerfile.' \
        '(function () {' \
        '  try {' \
        '    // Ingress: expose origin + path prefix, minus the /web-app/ suffix.' \
        '    // Direct access yields the bare origin, so both paths work.' \
        '    var prefix = location.pathname.replace(/\/web-app(\/.*)?$/, "");' \
        '    window.__haLemonadeBase = location.origin + prefix;' \
        '  } catch (e) { /* leave the bundle to fall back to its own default */ }' \
        '  try {' \
        '    // Android: defeat the Play Store redirect by hiding the token.' \
        '    var d = Object.getOwnPropertyDescriptor(Navigator.prototype, "userAgent");' \
        '    if (d && d.get) {' \
        '      Object.defineProperty(navigator, "userAgent", {' \
        '        configurable: true,' \
        '        get: function () { return d.get.call(navigator).replace(/Android/gi, "Linux"); }' \
        '      });' \
        '    }' \
        '  } catch (e) { /* nothing to do; the redirect stays, the app still works elsewhere */ }' \
        '})();' \
        > "${WEBAPP}/ha-addon-shim.js"; \
    \
    grep -q '<script defer="defer" src="renderer.bundle.js">' "${WEBAPP}/index.html" \
        || { echo "PATCH FAILED: bundle <script> tag not found in web-app/index.html — upstream layout changed"; exit 1; }; \
    sed -i 's|<script defer="defer" src="renderer.bundle.js">|<script src="ha-addon-shim.js"></script><script defer="defer" src="renderer.bundle.js">|' \
        "${WEBAPP}/index.html"; \
    grep -q 'ha-addon-shim.js' "${WEBAPP}/index.html" \
        || { echo "PATCH FAILED: shim <script> not inserted into web-app/index.html"; exit 1; }; \
    \
    HITS="$(grep -c 'window\.location?\.origin' "${WEBAPP}/renderer.bundle.js" || true)"; \
    [ "${HITS}" = "1" ] \
        || { echo "PATCH FAILED: expected exactly 1 'window.location?.origin' in renderer.bundle.js, found ${HITS} — upstream changed how the API base URL is derived"; exit 1; }; \
    sed -i 's#window\.location?\.origin#(window.__haLemonadeBase||window.location?.origin)#' \
        "${WEBAPP}/renderer.bundle.js"; \
    grep -q '__haLemonadeBase' "${WEBAPP}/renderer.bundle.js" \
        || { echo "PATCH FAILED: base-URL patch did not apply to renderer.bundle.js"; exit 1; }; \
    echo "web-app patched: Android redirect defeated, ingress base URL made path-aware"

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
    # Proves the bundled glibc actually runs the binary on musl. Skipped (not
    # failed) when building a foreign arch without qemu binfmt; the smoke test
    # covers the native case.
    && ( LD_LIBRARY_PATH=/opt/glibc/lib /opt/lemonade/lemond --version \
         || echo "WARNING: could not exec lemond (cross-build without qemu?) — skipping version check" )

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
