# Global ARGs must be declared before any FROM
ARG DOCKHAND_VERSION=1.0.59
ARG BUILD_FROM

# Stage 1: Extract Dockhand from official image
# Dockhand publishes versioned tags as fnsys/dockhand:vX.Y.Z
FROM mirror.gcr.io/fnsys/dockhand:v${DOCKHAND_VERSION} AS dockhand-source

# ha-ingress-adapter: makes Dockhand's web UI work under Home Assistant ingress
# (ha-ingress-adapter/main.go). Built in a stage of the target platform.
FROM mirror.gcr.io/library/golang:1.27.3-alpine AS ingress-builder
WORKDIR /src
COPY ha-ingress-adapter/go.mod ha-ingress-adapter/*.go ha-ingress-adapter/shim.js ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /ha-ingress-adapter . \
    && test -x /ha-ingress-adapter

# ha-docker-filter: keeps Home Assistant's containers out of what Dockhand
# lists (ha-docker-filter/main.go). Other apps carry the same copy; CI checks
# that the copies are identical.
FROM mirror.gcr.io/library/golang:1.27.3-alpine AS filter-builder
WORKDIR /src
COPY ha-docker-filter/go.mod ha-docker-filter/*.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /ha-docker-filter . \
    && test -x /ha-docker-filter

# Stage 2: Build final image
FROM ${BUILD_FROM}

# Re-declare ARGs needed in this stage
ARG DOCKHAND_VERSION

# Install runtime requirements
# Upgrade first to resolve base image package version mismatches (e.g. libcrypto3/libssl3)
# The base image pins musl in /etc/apk/world, which `apk upgrade` respects;
# naming musl and musl-utils here lifts the pin so g++ (musl-dev) resolves below.
RUN \
    apk upgrade --no-cache \
    && apk add --no-cache \
        ca-certificates \
        curl \
        jq \
        docker-cli \
        docker-cli-buildx \
        docker-cli-compose \
        nodejs \
        npm \
        sqlite \
        git \
        musl \
        musl-utils \
        openssh-client \
        su-exec \
        tini

# Alpine's Node already trusts the system CA bundle, which `apk upgrade` keeps
# current. Set explicitly so it still holds if the Node build changes.
ENV NODE_OPTIONS="--use-openssl-ca"

# Copy Dockhand application from source image, with the script upstream starts
# it by. That script names /app, where upstream's image keeps the application.
COPY --from=dockhand-source /app /opt/dockhand
COPY --from=dockhand-source /usr/local/bin/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN ln -s /opt/dockhand /app \
    && mkdir -p /home/dockhand \
    && if ! grep -q '/app/server.js' /usr/local/bin/docker-entrypoint.sh || [ ! -f /app/server.js ]; then \
        echo "Upstream no longer starts /app/server.js: read its docker-entrypoint.sh and adjust services.d/dockhand/run" >&2; exit 1; \
    fi

# Rebuild native Node.js modules for Alpine (musl)
# Upstream Dockhand uses Wolfi (glibc) — better-sqlite3 native addon
# compiled against glibc won't load on Alpine's musl libc
RUN apk add --no-cache --virtual .build-deps python3 make g++ \
    && cd /opt/dockhand \
    && npm rebuild better-sqlite3 \
    && apk del .build-deps

# Dockhand appends `tls.rootCertificates` (Node's compiled-in roots, which
# NODE_OPTIONS does not change) to a remote environment's custom CA. Use the
# system store instead; fail if any use is left unpatched.
# BusyBox grep has no --include, so find picks the files.
# server.js, the entry point, imports the name instead of reading it off `tls`.
RUN find /opt/dockhand/build -name '*.js' -exec sed -i -E \
        's/([A-Za-z_$][A-Za-z0-9_$]*)\.rootCertificates\b/\1.getCACertificates("system")/g' {} + \
    && sed -i -E \
        -e "s/\brootCertificates( *\} *from 'node:tls')/getCACertificates\1/" \
        -e 's/\.\.\.rootCertificates\b/...getCACertificates("system")/g' /opt/dockhand/server.js \
    && left="$(find /opt/dockhand/build /opt/dockhand/server.js -name '*.js' \
        -exec grep -lE "rootCertificates|getCACertificates\\([\"']bundled" {} + || true)" \
    && if [ -n "${left}" ]; then echo "Unpatched Node root store use: ${left}" >&2; exit 1; fi

# Create required directories
RUN mkdir -p /opt/dockhand/data /opt/dockhand/data/db /opt/dockhand/data/stacks

COPY --from=ingress-builder /ha-ingress-adapter /usr/bin/ha-ingress-adapter
COPY --from=filter-builder /ha-docker-filter /usr/bin/ha-docker-filter

# Copy root filesystem
COPY rootfs /

# Ensure scripts are executable
RUN chmod a+x /etc/cont-init.d/*.sh \
    && chmod a+x /etc/services.d/*/run \
    && chmod a+x /etc/services.d/*/finish \
    && chmod a+x /usr/bin/dockhand-health

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
    CMD /usr/bin/dockhand-health

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
    org.opencontainers.image.vendor="Home Assistant Apps" \
    org.opencontainers.image.authors="Dockhand for Home Assistant" \
    org.opencontainers.image.licenses="BSL-1.1" \
    org.opencontainers.image.url="https://dockhand.pro" \
    org.opencontainers.image.source="https://github.com/Finsys/dockhand" \
    org.opencontainers.image.documentation="https://dockhand.pro/manual" \
    org.opencontainers.image.created=${BUILD_DATE} \
    org.opencontainers.image.revision=${BUILD_REF} \
    org.opencontainers.image.version=${BUILD_VERSION}
