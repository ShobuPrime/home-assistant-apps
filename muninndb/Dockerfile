# Global ARGs must be declared before the first FROM
ARG BUILD_FROM=ghcr.io/hassio-addons/base:20.0.1
ARG MUNINNDB_VERSION=0.4.2-alpha

# ---------------------------------------------------------------------------
# Stage 1: Extract glibc runtime libraries
# NO dependency on MUNINNDB_VERSION — cached permanently until Go base
# image changes.
# ---------------------------------------------------------------------------
FROM golang:1.26-bookworm AS glibc-provider

RUN mkdir -p /glibc-libs \
    && ARCH="$(uname -m)" \
    && if [ "${ARCH}" = "x86_64" ]; then \
        LIB_DIR="/lib/x86_64-linux-gnu"; \
        LD_PATH="/lib64/ld-linux-x86-64.so.2"; \
    elif [ "${ARCH}" = "aarch64" ]; then \
        LIB_DIR="/lib/aarch64-linux-gnu"; \
        LD_PATH="/lib/ld-linux-aarch64.so.1"; \
    fi \
    && cp "${LIB_DIR}/libc.so.6" /glibc-libs/ \
    && cp "${LIB_DIR}/libm.so.6" /glibc-libs/ \
    && cp "${LIB_DIR}/libdl.so.2" /glibc-libs/ \
    && cp "${LIB_DIR}/libpthread.so.0" /glibc-libs/ \
    && cp "${LIB_DIR}/librt.so.1" /glibc-libs/ \
    && cp "${LIB_DIR}/libgcc_s.so.1" /glibc-libs/ \
    && cp "${LIB_DIR}/libstdc++.so.6" /glibc-libs/ \
    && cp "${LD_PATH}" /glibc-libs/ld-linux.so \
    && echo "${LIB_DIR}" > /glibc-libs/.lib_dir \
    && echo "${LD_PATH}" > /glibc-libs/.ld_path

# ---------------------------------------------------------------------------
# Stage 2: Clone source and fetch ONNX assets
# Shared base for both web and Go build stages.
# ---------------------------------------------------------------------------
FROM golang:1.26-bookworm AS source

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl make ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

ARG MUNINNDB_VERSION

RUN git clone --depth 1 --branch "v${MUNINNDB_VERSION}" \
    https://github.com/scrypster/muninndb.git .

ARG TARGETARCH

# Fetch ONNX model and platform-specific ORT native library
RUN make fetch-model _ort-linux-${TARGETARCH}

# ---------------------------------------------------------------------------
# Stage 3: Build web assets (Tailwind CSS via Vite)
# Runs independently from the Go build. npm ci + vite build are isolated
# here so the Go compilation doesn't wait on them.
# ---------------------------------------------------------------------------
FROM node:20-bookworm-slim AS web-builder

WORKDIR /src/web

# Copy only package files first for npm cache efficiency
COPY --from=source /src/web/package.json /src/web/package-lock.json ./
RUN npm ci --ignore-scripts

# Copy rest of web source and build (output: static/dist/)
COPY --from=source /src/web ./
RUN npm run build

# ---------------------------------------------------------------------------
# Stage 4: Build MuninnDB Go binary with embedded ONNX model
# The release binary does NOT include the local embedder (-tags localassets).
# Building from source ensures semantic search works out of the box.
# ---------------------------------------------------------------------------
FROM source AS builder

# Copy built web assets from the web-builder stage
COPY --from=web-builder /src/web/static/dist ./web/static/dist

# Build the server binary WITH local assets embedded
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -tags localassets -ldflags="-s -w" -o /muninndb-server ./cmd/muninn/...

# ---------------------------------------------------------------------------
# Stage 5: Runtime image
# ---------------------------------------------------------------------------
FROM $BUILD_FROM

# Install requirements — cached across MuninnDB version updates
# Upgrade first to resolve base image package version mismatches
RUN \
    apk upgrade --no-cache \
    && apk add --no-cache \
        ca-certificates \
        curl \
        jq

# Copy glibc runtime from the glibc-provider stage (Debian bookworm).
# This stage is independent of MUNINNDB_VERSION, so this COPY is cached
# across MuninnDB updates — only invalidated if the Go base image changes.
COPY --from=glibc-provider /glibc-libs /tmp/glibc-libs
RUN LIB_DIR=$(cat /tmp/glibc-libs/.lib_dir) \
    && LD_PATH=$(cat /tmp/glibc-libs/.ld_path) \
    && mkdir -p "${LIB_DIR}" "$(dirname "${LD_PATH}")" \
    && cp /tmp/glibc-libs/libc.so.6 "${LIB_DIR}/" \
    && cp /tmp/glibc-libs/libm.so.6 "${LIB_DIR}/" \
    && cp /tmp/glibc-libs/libdl.so.2 "${LIB_DIR}/" \
    && cp /tmp/glibc-libs/libpthread.so.0 "${LIB_DIR}/" \
    && cp /tmp/glibc-libs/librt.so.1 "${LIB_DIR}/" \
    && cp /tmp/glibc-libs/libgcc_s.so.1 "${LIB_DIR}/" \
    && cp /tmp/glibc-libs/libstdc++.so.6 "${LIB_DIR}/" \
    && cp /tmp/glibc-libs/ld-linux.so "${LD_PATH}" \
    && rm -rf /tmp/glibc-libs

# Copy the built binary — only layer that changes on version update
RUN mkdir -p /opt/muninndb
COPY --from=builder /muninndb-server /opt/muninndb/muninn
RUN chmod +x /opt/muninndb/muninn \
    && echo "MuninnDB installed successfully" \
    && ls -lh /opt/muninndb/muninn

# Copy root filesystem — only rebuilds if rootfs scripts change
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
ARG MUNINNDB_VERSION

# Labels
LABEL \
    io.hass.name="${BUILD_NAME}" \
    io.hass.description="${BUILD_DESCRIPTION}" \
    io.hass.arch="${BUILD_ARCH}" \
    io.hass.type="addon" \
    io.hass.version=${BUILD_VERSION} \
    maintainer="MuninnDB for Home Assistant" \
    muninndb.version="${MUNINNDB_VERSION}" \
    org.opencontainers.image.title="${BUILD_NAME}" \
    org.opencontainers.image.description="${BUILD_DESCRIPTION}" \
    org.opencontainers.image.vendor="Home Assistant Add-ons" \
    org.opencontainers.image.authors="MuninnDB for Home Assistant" \
    org.opencontainers.image.licenses="BSL-1.1" \
    org.opencontainers.image.url="https://muninndb.com" \
    org.opencontainers.image.source="https://github.com/scrypster/muninndb" \
    org.opencontainers.image.documentation="https://muninndb.com/docs" \
    org.opencontainers.image.created=${BUILD_DATE} \
    org.opencontainers.image.revision=${BUILD_REF} \
    org.opencontainers.image.version=${BUILD_VERSION}
