ARG BUILD_FROM
FROM $BUILD_FROM

# Build arguments for version
ARG ARCANE_VERSION=1.16.4

# Install requirements
# Upgrade first to resolve base image package version mismatches (e.g. libcrypto3/libssl3)
RUN \
    apk upgrade --no-cache \
    && apk add --no-cache \
        ca-certificates \
        curl \
        jq \
        openssl

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
    maintainer="Arcane for Home Assistant" \
    arcane.version="${ARCANE_VERSION}" \
    org.opencontainers.image.title="${BUILD_NAME}" \
    org.opencontainers.image.description="${BUILD_DESCRIPTION}" \
    org.opencontainers.image.vendor="Home Assistant Add-ons" \
    org.opencontainers.image.authors="Arcane for Home Assistant" \
    org.opencontainers.image.licenses="BSD-3-Clause" \
    org.opencontainers.image.url="https://getarcane.app" \
    org.opencontainers.image.source="https://github.com/getarcaneapp/arcane" \
    org.opencontainers.image.documentation="https://getarcane.app/docs" \
    org.opencontainers.image.created=${BUILD_DATE} \
    org.opencontainers.image.revision=${BUILD_REF} \
    org.opencontainers.image.version=${BUILD_VERSION}
