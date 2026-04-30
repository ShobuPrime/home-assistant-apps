ARG BUILD_FROM
FROM $BUILD_FROM

# Build arguments for version
ARG PORTAINER_VERSION=2.41.0

# Install requirements
# Upgrade first to resolve base image package version mismatches (e.g. libcrypto3/libssl3)
RUN \
    apk upgrade --no-cache \
    && apk add --no-cache \
        ca-certificates \
        curl \
        jq

# Download and install Portainer EE STS
RUN \
    mkdir -p /opt/portainer \
    && ARCH="$(uname -m)" \
    && if [ "${ARCH}" = "aarch64" ]; then \
        PORTAINER_ARCH="arm64"; \
    elif [ "${ARCH}" = "x86_64" ]; then \
        PORTAINER_ARCH="amd64"; \
    elif [ "${ARCH}" = "armv7l" ]; then \
        PORTAINER_ARCH="arm"; \
    else \
        PORTAINER_ARCH="arm"; \
    fi \
    && echo "Downloading Portainer ${PORTAINER_VERSION} for ${PORTAINER_ARCH}..." \
    && curl -L -f -S -o /tmp/portainer.tar.gz \
        "https://github.com/portainer/portainer/releases/download/${PORTAINER_VERSION}/portainer-${PORTAINER_VERSION}-linux-${PORTAINER_ARCH}.tar.gz" \
    && echo "Download complete. File size:" && ls -lh /tmp/portainer.tar.gz \
    && curl -L -f -S -o /tmp/checksum.txt \
        "https://github.com/portainer/portainer/releases/download/${PORTAINER_VERSION}/portainer-${PORTAINER_VERSION}-linux-${PORTAINER_ARCH}-checksum.txt" \
    && echo "Verifying checksum..." \
    && EXPECTED_SHA256=$(cat /tmp/checksum.txt | sed -n 's/^SHA256 (.*.tar.gz) = \(.*\)$/\1/p') \
    && ACTUAL_SHA256=$(sha256sum /tmp/portainer.tar.gz | cut -d' ' -f1) \
    && echo "Expected: ${EXPECTED_SHA256}" \
    && echo "Actual:   ${ACTUAL_SHA256}" \
    && if [ "${ACTUAL_SHA256}" != "${EXPECTED_SHA256}" ]; then \
        echo "ERROR: SHA256 checksum mismatch!"; \
        exit 1; \
    fi \
    && tar -xzf /tmp/portainer.tar.gz -C /opt \
    && rm /tmp/portainer.tar.gz /tmp/checksum.txt \
    && chmod +x /opt/portainer/portainer

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
    maintainer="Portainer EE for Home Assistant" \
    portainer.version="${PORTAINER_VERSION}" \
    org.opencontainers.image.title="${BUILD_NAME}" \
    org.opencontainers.image.description="${BUILD_DESCRIPTION}" \
    org.opencontainers.image.vendor="Home Assistant Add-ons" \
    org.opencontainers.image.authors="Portainer EE for Home Assistant" \
    org.opencontainers.image.licenses="MIT" \
    org.opencontainers.image.url="https://www.portainer.io" \
    org.opencontainers.image.source="https://github.com/portainer/portainer" \
    org.opencontainers.image.documentation="https://docs.portainer.io" \
    org.opencontainers.image.created=${BUILD_DATE} \
    org.opencontainers.image.revision=${BUILD_REF} \
    org.opencontainers.image.version=${BUILD_VERSION}

