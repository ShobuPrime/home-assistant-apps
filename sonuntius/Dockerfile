ARG BUILD_FROM
ARG GO_VERSION=1.26-alpine

# ---------------------------------------------------------------------------
# Stage 1: build the Go binaries (ma-bridge, sonuntius-ctl)
# ---------------------------------------------------------------------------
# hadolint ignore=DL3007
FROM golang:${GO_VERSION} AS builder
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN go build -ldflags="-s -w" -o /out/ma-bridge ./cmd/ma-bridge \
    && go build -ldflags="-s -w" -o /out/sonuntius-ctl ./cmd/sonuntius-ctl \
    && go build -ldflags="-s -w" -o /out/yt-cast ./cmd/yt-cast \
    && go build -ldflags="-s -w" -o /out/cast-receiver ./cmd/cast-receiver \
    && go build -ldflags="-s -w" -o /out/alsa-to-sendspin ./cmd/alsa-to-sendspin

# ---------------------------------------------------------------------------
# Stage 2: runtime image on top of hassio-addons base
# ---------------------------------------------------------------------------
# hadolint ignore=DL3006
FROM ${BUILD_FROM}

# Upgrade first to resolve base image package version mismatches (e.g. libcrypto3/libssl3)
RUN apk upgrade --no-cache \
    && apk add --no-cache \
        bash \
        ca-certificates \
        curl \
        jq \
        alsa-utils \
        tar \
        yt-dlp

# Phase 5 — iFi Tidal Connect binary fallback (opt-in). The ARMv7 iFi
# binary needs glibc-compatible shims on aarch64; libc6-compat provides
# them. Skipped on amd64 because qemu-user-static would be required to
# run an ARMv7 binary there and that's not a path we recommend.
ARG BUILD_ARCH
RUN if [ "${BUILD_ARCH}" = "aarch64" ]; then \
        apk add --no-cache libc6-compat ; \
    fi

COPY --from=builder /out/ma-bridge /usr/local/bin/ma-bridge
COPY --from=builder /out/sonuntius-ctl /usr/local/bin/sonuntius-ctl
COPY --from=builder /out/yt-cast /usr/local/bin/yt-cast
COPY --from=builder /out/cast-receiver /usr/local/bin/cast-receiver
COPY --from=builder /out/alsa-to-sendspin /usr/local/bin/alsa-to-sendspin

COPY rootfs /

RUN chmod a+x /etc/cont-init.d/*.sh \
    && chmod a+x /etc/services.d/*/run \
    && chmod a+x /etc/services.d/*/finish

ARG BUILD_DATE
ARG BUILD_DESCRIPTION
ARG BUILD_NAME
ARG BUILD_REF
ARG BUILD_REPOSITORY
ARG BUILD_VERSION

# Healthcheck — Supervisor reads this to gate the STARTUP -> STARTED state
# transition (yellow -> green dot in the HA UI) on the app actually being
# reachable, instead of just on the container being up.
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD curl -fs http://127.0.0.1:8099/health || exit 1

LABEL \
    io.hass.name="${BUILD_NAME}" \
    io.hass.description="${BUILD_DESCRIPTION}" \
    io.hass.arch="${BUILD_ARCH}" \
    io.hass.type="addon" \
    io.hass.version=${BUILD_VERSION} \
    maintainer="Sonuntius for Home Assistant" \
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
