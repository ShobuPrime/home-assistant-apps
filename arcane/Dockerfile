ARG BUILD_FROM

# ha-arcane-ingress: makes Arcane's web UI work under Home Assistant ingress
# (ha-arcane-ingress/main.go). Built in a stage of the target platform, like
# the other apps' Go helpers.
FROM mirror.gcr.io/library/golang:1.27.3-alpine AS ingress-builder
WORKDIR /src
COPY ha-arcane-ingress/go.mod ha-arcane-ingress/*.go ha-arcane-ingress/shim.js ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /ha-arcane-ingress . \
    && test -x /ha-arcane-ingress

# ha-docker-filter: keeps Home Assistant's containers out of what Arcane lists
# (ha-docker-filter/main.go).
FROM mirror.gcr.io/library/golang:1.27.3-alpine AS filter-builder
WORKDIR /src
COPY ha-docker-filter/go.mod ha-docker-filter/*.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /ha-docker-filter . \
    && test -x /ha-docker-filter

FROM $BUILD_FROM

# Build arguments for version
ARG ARCANE_VERSION=3.1.1

# Install requirements.
# The base image pins libcrypto3/libssl3 in apk "world" at an exact older
# revision (e.g. =3.5.6-r0). When the Alpine repo moves ahead — openssl 3.5.7-r0
# requires libcrypto3/libssl3=3.5.7-r0 — a plain `apk add openssl` can't resolve
# against the held libs. `apk add --upgrade libcrypto3 libssl3 openssl` rewrites
# those world entries and upgrades the whole TLS stack to the repo version in one
# transaction (no version pinning). Listing the libs explicitly is required:
# under apk-tools 3, `apk upgrade [--available]` does not override an exact pin.
#
# sqlite is here for usr/share/arcane/migrate-settings.sh only and goes with it
# (LEGACY UNTIL 2027-06-01).
RUN \
    apk upgrade --no-cache \
    && apk add --no-cache --upgrade \
        ca-certificates \
        curl \
        jq \
        openssl \
        libcrypto3 \
        libssl3 \
        sqlite

# Download and install Arcane
# Release assets are raw binaries named: arcane_linux_amd64, arcane_linux_arm64
RUN \
    mkdir -p /opt/arcane \
    && ARCH="$(uname -m)" \
    && if [ "${ARCH}" = "aarch64" ]; then \
        ARCANE_ARCH="arm64"; \
    elif [ "${ARCH}" = "x86_64" ]; then \
        ARCANE_ARCH="amd64"; \
    else \
        echo "Unsupported architecture: ${ARCH}"; \
        exit 1; \
    fi \
    && echo "Downloading Arcane v${ARCANE_VERSION} for ${ARCANE_ARCH}..." \
    && curl -L -f -S -o /opt/arcane/arcane \
        "https://github.com/getarcaneapp/arcane/releases/download/v${ARCANE_VERSION}/arcane_linux_${ARCANE_ARCH}" \
    && echo "Download complete. File size:" && ls -lh /opt/arcane/arcane \
    && chmod +x /opt/arcane/arcane \
    && echo "Arcane installed successfully"

# Upstream's image provides two directories and Arcane's defaults point at
# them: /app/data (its data volume: projects, templates, git known_hosts, its
# Docker config) and /builds (build workspaces). /app/data would be the
# container layer here, discarded on every restart, so it leads into /data.
# tests/upstream_paths.py reads the defaults out of the binary and fails the
# smoke test when a release adds a path this image does not provide.
RUN mkdir -p /app /builds && ln -s /data/arcane/data /app/data

COPY --from=ingress-builder /ha-arcane-ingress /usr/bin/ha-arcane-ingress
COPY --from=filter-builder /ha-docker-filter /usr/bin/ha-docker-filter

# Copy root filesystem
COPY rootfs /

# Ensure scripts are executable
RUN chmod a+x /etc/cont-init.d/*.sh \
    && chmod a+x /etc/services.d/*/run \
    && chmod a+x /etc/services.d/*/finish \
    && chmod a+x /usr/share/arcane/*.sh /usr/bin/arcane-health

# Build arguments
ARG BUILD_ARCH
ARG BUILD_DATE
ARG BUILD_DESCRIPTION
ARG BUILD_NAME
ARG BUILD_REF
ARG BUILD_REPOSITORY
ARG BUILD_VERSION

# Healthcheck — Supervisor reads this to gate the STARTUP -> STARTED state
# transition (yellow -> green dot in the HA UI) on the app actually being
# reachable, instead of just on the container being up.
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD /usr/bin/arcane-health

LABEL \
    io.hass.name="${BUILD_NAME}" \
    io.hass.description="${BUILD_DESCRIPTION}" \
    io.hass.arch="${BUILD_ARCH}" \
    io.hass.type="addon" \
    io.hass.version=${BUILD_VERSION} \
    maintainer="Arcane for Home Assistant" \
    arcane.version="${ARCANE_VERSION}" \
    org.opencontainers.image.title="${BUILD_NAME}" \
    org.opencontainers.image.description="${BUILD_DESCRIPTION}" \
    org.opencontainers.image.vendor="Home Assistant Apps" \
    org.opencontainers.image.authors="Arcane for Home Assistant" \
    org.opencontainers.image.licenses="BSD-3-Clause" \
    org.opencontainers.image.url="https://getarcane.app" \
    org.opencontainers.image.source="https://github.com/getarcaneapp/arcane" \
    org.opencontainers.image.documentation="https://getarcane.app/docs" \
    org.opencontainers.image.created=${BUILD_DATE} \
    org.opencontainers.image.revision=${BUILD_REF} \
    org.opencontainers.image.version=${BUILD_VERSION}
