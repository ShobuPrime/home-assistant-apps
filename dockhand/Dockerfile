# Global ARGs must be declared before any FROM
ARG DOCKHAND_VERSION=1.0.25
ARG BUILD_FROM

# Stage 1: Extract Dockhand from official image
# Dockhand publishes versioned tags as fnsys/dockhand:vX.Y.Z
FROM fnsys/dockhand:v${DOCKHAND_VERSION} AS dockhand-source

# Stage 2: Build final image
FROM ${BUILD_FROM}

# Re-declare ARGs needed in this stage
ARG DOCKHAND_VERSION

# Install runtime requirements
# Upgrade first to resolve base image package version mismatches (e.g. libcrypto3/libssl3)
RUN \
    apk upgrade --no-cache \
    && apk add --no-cache \
        ca-certificates \
        curl \
        jq \
        docker-cli \
        nodejs \
        npm \
        sqlite \
        git \
        openssh-client \
        tini

# Copy Dockhand application from source image
COPY --from=dockhand-source /app /opt/dockhand

# Rebuild native Node.js modules for Alpine (musl)
# Upstream Dockhand uses Wolfi (glibc) — better-sqlite3 native addon
# compiled against glibc won't load on Alpine's musl libc
RUN apk add --no-cache --virtual .build-deps python3 make g++ \
    && cd /opt/dockhand \
    && npm rebuild better-sqlite3 \
    && apk del .build-deps

# Create required directories
RUN mkdir -p /opt/dockhand/data /opt/dockhand/data/db /opt/dockhand/data/stacks

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
    maintainer="Dockhand for Home Assistant" \
    dockhand.version="${DOCKHAND_VERSION}" \
    org.opencontainers.image.title="${BUILD_NAME}" \
    org.opencontainers.image.description="${BUILD_DESCRIPTION}" \
    org.opencontainers.image.vendor="Home Assistant Add-ons" \
    org.opencontainers.image.authors="Dockhand for Home Assistant" \
    org.opencontainers.image.licenses="BSL-1.1" \
    org.opencontainers.image.url="https://dockhand.pro" \
    org.opencontainers.image.source="https://github.com/Finsys/dockhand" \
    org.opencontainers.image.documentation="https://dockhand.pro/manual" \
    org.opencontainers.image.created=${BUILD_DATE} \
    org.opencontainers.image.revision=${BUILD_REF} \
    org.opencontainers.image.version=${BUILD_VERSION}
