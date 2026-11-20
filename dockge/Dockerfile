ARG BUILD_FROM
FROM mirror.gcr.io/louislam/dockge:1.5.0 AS dockge

# ha-ingress-adapter: makes Dockge's web UI work under Home Assistant ingress,
# and ha-docker-filter: keeps Home Assistant's containers out of what Dockge
# lists. Other apps carry the same copies; CI checks that they are identical.
# Built in stages of the target platform.
FROM mirror.gcr.io/library/golang:1.27.3-alpine AS helpers-builder
WORKDIR /src
COPY ha-ingress-adapter/go.mod ha-ingress-adapter/*.go ha-ingress-adapter/shim.js ./ingress/
COPY ha-docker-filter/go.mod ha-docker-filter/*.go ./filter/
RUN cd /src/ingress && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /ha-ingress-adapter . \
    && cd /src/filter && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /ha-docker-filter . \
    && test -x /ha-ingress-adapter && test -x /ha-docker-filter

FROM $BUILD_FROM

# Build arguments for version
ARG DOCKGE_VERSION=1.5.0

# Install runtime requirements (NO nodejs — we install Node 22 LTS manually)
# Upgrade first to resolve base image package version mismatches
# The base image pins musl in /etc/apk/world, which `apk upgrade` respects;
# naming musl and musl-utils here lifts the pin so g++ (musl-dev) resolves below.
RUN \
    apk upgrade --no-cache \
    && apk add --no-cache \
        ca-certificates \
        bash \
        curl \
        jq \
        libstdc++ \
        libgcc \
        musl \
        musl-utils \
        docker-cli \
        docker-cli-buildx \
        docker-cli-compose

# Install Node.js 22 LTS (musl build) to match upstream Dockge's ABI
# Alpine 3.23 ships Node 24 which breaks native module compatibility.
# Node 22 musl build from nodejs.org gives us ABI v127 on musl,
# matching the prebuilt native modules (node-pty, better-sqlite3).
ARG NODE_VERSION=22.23.4
RUN ARCH="$(uname -m)" \
    && if [ "${ARCH}" = "x86_64" ]; then NODE_ARCH="x64"; \
    elif [ "${ARCH}" = "aarch64" ]; then NODE_ARCH="arm64"; \
    else echo "Unsupported: ${ARCH}"; exit 1; fi \
    && curl -fsSL "https://unofficial-builds.nodejs.org/download/release/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}-musl.tar.xz" \
       | tar -xJ -C /usr/local --strip-components=1 \
    && node --version && npm --version

# Trust the system CA bundle, which `apk upgrade` keeps current, rather than
# the root store compiled into this Node release.
ENV NODE_OPTIONS="--use-openssl-ca"

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

COPY --from=helpers-builder /ha-ingress-adapter /usr/bin/ha-ingress-adapter
COPY --from=helpers-builder /ha-docker-filter /usr/bin/ha-docker-filter

# Copy root filesystem
COPY rootfs /

# Ensure scripts are executable
RUN chmod a+x /etc/cont-init.d/*.sh \
    && chmod a+x /etc/services.d/*/run \
    && chmod a+x /etc/services.d/*/finish \
    && chmod a+x /usr/bin/dockge-health /usr/local/bin/docker

# Build arguments
ARG BUILD_ARCH
ARG BUILD_DATE
ARG BUILD_DESCRIPTION
ARG BUILD_NAME
ARG BUILD_REF
ARG BUILD_REPOSITORY
ARG BUILD_VERSION

# Labels
# Healthcheck — Supervisor reads this to gate the STARTUP -> STARTED state
# transition (yellow -> green dot in the HA UI) on the app actually being
# reachable, instead of just on the container being up.
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD /usr/bin/dockge-health

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
    org.opencontainers.image.vendor="Home Assistant Apps" \
    org.opencontainers.image.authors="Dockge for Home Assistant" \
    org.opencontainers.image.licenses="MIT" \
    org.opencontainers.image.url="https://github.com/louislam/dockge" \
    org.opencontainers.image.source="https://github.com/louislam/dockge" \
    org.opencontainers.image.documentation="https://github.com/louislam/dockge" \
    org.opencontainers.image.created=${BUILD_DATE} \
    org.opencontainers.image.revision=${BUILD_REF} \
    org.opencontainers.image.version=${BUILD_VERSION}
