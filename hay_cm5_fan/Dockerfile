ARG BUILD_FROM
FROM $BUILD_FROM

# Install minimal dependencies
# Upgrade first to resolve base image package version mismatches (e.g. libcrypto3/libssl3)
RUN apk upgrade --no-cache \
    && apk add --no-cache \
        curl \
        jq

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
    maintainer="CM5 Fan Controller for Home Assistant" \
    org.opencontainers.image.title="${BUILD_NAME}" \
    org.opencontainers.image.description="${BUILD_DESCRIPTION}" \
    org.opencontainers.image.vendor="Home Assistant Add-ons" \
    org.opencontainers.image.authors="ShobuPrime" \
    org.opencontainers.image.licenses="MIT" \
    org.opencontainers.image.url="https://github.com/ShobuPrime/home-assistant-apps" \
    org.opencontainers.image.source="https://github.com/ShobuPrime/home-assistant-apps" \
    org.opencontainers.image.created=${BUILD_DATE} \
    org.opencontainers.image.revision=${BUILD_REF} \
    org.opencontainers.image.version=${BUILD_VERSION}
