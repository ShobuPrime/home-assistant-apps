ARG BUILD_FROM
FROM louislam/dockge:1.5.0 as dockge

FROM $BUILD_FROM

# Build arguments for version
ARG DOCKGE_VERSION=1.5.0

# Install runtime requirements
# Upgrade first to resolve base image package version mismatches (e.g. libcrypto3/libssl3)
RUN \
    apk upgrade --no-cache \
    && apk add --no-cache \
        bash \
        ca-certificates \
        curl \
        jq \
        nodejs \
        npm

# Copy Dockge application from official image
COPY --from=dockge /app /app

# Copy tsx from upstream (needed to run TypeScript backend)
COPY --from=dockge /usr/local/bin/tsx /usr/local/bin/tsx

# Rebuild ALL native Node.js modules for Alpine (musl + Node 24)
# Upstream Dockge is Debian (glibc + Node 22) — native addons
# (node-pty, better-sqlite3) must be recompiled from source.
# nodejs-dev provides the Node.js headers needed by node-gyp.
RUN apk add --no-cache --virtual .build-deps \
        python3 make g++ linux-headers \
    && cd /app \
    && npm rebuild 2>&1 || true \
    && apk del .build-deps

# Install tsx globally (the copied binary needs its npm deps)
RUN npm install -g tsx

WORKDIR /app

# Create required directories
RUN mkdir -p /app/data /app/data/db /app/data/stacks

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
    maintainer="Dockge for Home Assistant" \
    dockge.version="${DOCKGE_VERSION}" \
    org.opencontainers.image.title="${BUILD_NAME}" \
    org.opencontainers.image.description="${BUILD_DESCRIPTION}" \
    org.opencontainers.image.vendor="Home Assistant Add-ons" \
    org.opencontainers.image.authors="Dockge for Home Assistant" \
    org.opencontainers.image.licenses="MIT" \
    org.opencontainers.image.url="https://github.com/louislam/dockge" \
    org.opencontainers.image.source="https://github.com/louislam/dockge" \
    org.opencontainers.image.documentation="https://github.com/louislam/dockge" \
    org.opencontainers.image.created=${BUILD_DATE} \
    org.opencontainers.image.revision=${BUILD_REF} \
    org.opencontainers.image.version=${BUILD_VERSION}
