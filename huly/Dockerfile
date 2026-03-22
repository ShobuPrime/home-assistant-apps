ARG BUILD_FROM
FROM $BUILD_FROM

# Build arguments for version
ARG HULY_VERSION=0.7.382
ENV HULY_VERSION=v${HULY_VERSION}

# S6-overlay shutdown timeouts (in milliseconds).
# The Huly stack has 14+ containers that need graceful shutdown.
# S6_SERVICES_GRACETIME: time to wait for docker-compose to exit after SIGTERM
# S6_KILL_FINISH_MAXTIME: time allowed for cont-finish.d scripts
# These must fit within the config.yaml timeout (120s) sent by the Supervisor.
ENV S6_SERVICES_GRACETIME=90000 \
    S6_KILL_FINISH_MAXTIME=90000

# Install requirements
# Upgrade first to resolve base image package version mismatches (e.g. libcrypto3/libssl3)
RUN \
    apk upgrade --no-cache \
    && apk add --no-cache \
        bash \
        ca-certificates \
        curl \
        docker-cli \
        jq \
        openssl \
        socat

# Install Docker Compose as a standalone binary (NOT as a CLI plugin)
# Alpine's docker-cli plugin loader segfaults on aarch64 (docker/cli#4900).
# Installing to /usr/local/bin and calling it directly bypasses the plugin system entirely.
RUN \
    ARCH="$(uname -m)" \
    && if [ "${ARCH}" = "aarch64" ]; then COMPOSE_ARCH="aarch64"; \
       elif [ "${ARCH}" = "x86_64" ]; then COMPOSE_ARCH="x86_64"; \
       else echo "Unsupported architecture: ${ARCH}"; exit 1; fi \
    && curl -fsSL -o /usr/local/bin/docker-compose \
        "https://github.com/docker/compose/releases/latest/download/docker-compose-linux-${COMPOSE_ARCH}" \
    && chmod +x /usr/local/bin/docker-compose

# Copy root filesystem
COPY rootfs /

# Ensure scripts are executable
RUN chmod a+x /etc/cont-init.d/*.sh \
    && chmod a+x /etc/services.d/*/run \
    && chmod a+x /etc/services.d/*/finish

# Build arguments
ARG BUILD_ARCH
ARG BUILD_DATE
ARG BUILD_DESCRIPTION
ARG BUILD_NAME
ARG BUILD_REF
ARG BUILD_REPOSITORY
ARG BUILD_VERSION

# Labels
LABEL \
    io.hass.name="${BUILD_NAME}" \
    io.hass.description="${BUILD_DESCRIPTION}" \
    io.hass.arch="${BUILD_ARCH}" \
    io.hass.type="addon" \
    io.hass.version=${BUILD_VERSION} \
    maintainer="Huly for Home Assistant" \
    huly.version="${HULY_VERSION}" \
    org.opencontainers.image.title="${BUILD_NAME}" \
    org.opencontainers.image.description="${BUILD_DESCRIPTION}" \
    org.opencontainers.image.vendor="Home Assistant Add-ons" \
    org.opencontainers.image.authors="Huly for Home Assistant" \
    org.opencontainers.image.licenses="EPL-2.0" \
    org.opencontainers.image.url="https://huly.io/" \
    org.opencontainers.image.source="https://github.com/hcengineering/huly-selfhost" \
    org.opencontainers.image.documentation="https://docs.huly.io/" \
    org.opencontainers.image.created=${BUILD_DATE} \
    org.opencontainers.image.revision=${BUILD_REF} \
    org.opencontainers.image.version=${BUILD_VERSION}
