ARG BUILD_FROM
FROM louislam/dockge:1.5.0 as dockge

FROM $BUILD_FROM

# Build arguments for version
ARG DOCKGE_VERSION=1.5.0

# Install runtime requirements (NO nodejs — we install Node 22 LTS manually)
# Upgrade first to resolve base image package version mismatches
RUN \
    apk upgrade --no-cache \
    && apk add --no-cache \
        bash \
        ca-certificates \
        curl \
        jq

# Install Node.js 22 LTS (musl build) to match upstream Dockge's ABI
# Alpine 3.23 ships Node 24 which breaks native module compatibility.
# Node 22 musl build from nodejs.org gives us ABI v127 on musl,
# matching the prebuilt native modules (node-pty, better-sqlite3).
ARG NODE_VERSION=22.14.0
RUN ARCH="$(uname -m)" \
    && if [ "${ARCH}" = "x86_64" ]; then NODE_ARCH="x64"; \
    elif [ "${ARCH}" = "aarch64" ]; then NODE_ARCH="arm64"; \
    else echo "Unsupported: ${ARCH}"; exit 1; fi \
    && curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}-musl.tar.xz" \
       | tar -xJ -C /usr/local --strip-components=1 \
    && node --version && npm --version

# Copy Dockge application from official image
COPY --from=dockge /app /app

# Install tsx and rebuild native modules for musl
RUN apk add --no-cache --virtual .build-deps python3 make g++ linux-headers \
    && npm install -g tsx \
    && cd /app \
    && npm rebuild \
    && apk del .build-deps

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
