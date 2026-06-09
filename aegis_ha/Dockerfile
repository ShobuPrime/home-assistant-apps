ARG BUILD_FROM
ARG GO_VERSION=1.26-alpine

# ---------------------------------------------------------------------------
# Stage 1: build the Go binary
# ---------------------------------------------------------------------------
# hadolint ignore=DL3007
FROM golang:${GO_VERSION} AS builder
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG BUILD_VERSION=0.2.0
RUN go build -ldflags="-s -w -X main.version=${BUILD_VERSION}" -o /out/aegis_ha ./cmd/aegis_ha

# ---------------------------------------------------------------------------
# Stage 2: runtime image on top of the hassio-addons base
# ---------------------------------------------------------------------------
# hadolint ignore=DL3006
FROM ${BUILD_FROM}

# Upgrade first to resolve base image package version mismatches (e.g. libcrypto3/libssl3)
RUN apk upgrade --no-cache \
    && apk add --no-cache \
        ca-certificates \
        curl \
        jq

COPY --from=builder /out/aegis_ha /usr/local/bin/aegis_ha

COPY rootfs /

RUN chmod a+x /etc/cont-init.d/*.sh \
    && chmod a+x /etc/services.d/*/run \
    && chmod a+x /etc/services.d/*/finish

ARG BUILD_ARCH
ARG BUILD_DATE
ARG BUILD_DESCRIPTION
ARG BUILD_NAME
ARG BUILD_REF
ARG BUILD_REPOSITORY
ARG BUILD_VERSION

# Healthcheck — Supervisor reads this to gate the STARTUP -> STARTED state
# transition (yellow -> green dot in the HA UI) on the app actually being
# reachable, not just on the container being up.
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD curl -fs http://127.0.0.1:8099/health || exit 1

LABEL \
    io.hass.name="${BUILD_NAME}" \
    io.hass.description="${BUILD_DESCRIPTION}" \
    io.hass.arch="${BUILD_ARCH}" \
    io.hass.type="addon" \
    io.hass.version=${BUILD_VERSION} \
    maintainer="AegisHA for Home Assistant" \
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
