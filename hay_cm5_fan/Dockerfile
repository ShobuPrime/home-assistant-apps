ARG BUILD_FROM

# fan-uart-hold: holds a serial break for "fan off" when the kernel owns the
# fan's pin (rootfs/usr/share/hay_cm5_fan/fan-control.sh). Built in a stage of
# the target platform, like the other apps' Go helpers: no build argument has
# to name the architecture.
FROM mirror.gcr.io/library/golang:1.27.3-alpine AS helper-builder
WORKDIR /src
COPY fan-uart-hold/go.mod fan-uart-hold/*.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /fan-uart-hold . \
    && test -x /fan-uart-hold

FROM $BUILD_FROM

# S6-overlay shutdown timing.
#
# cleanup() has to kill gpioset, set the fan to its shutdown state, and publish
# the retained MQTT "offline" status. That last call has no timeout, and during
# a HAOS shutdown the MQTT broker add-on may already be stopping — so the
# publish can block. S6's 3s default grace is not enough headroom for that, and
# being killed partway through leaves a stale "available" entity in Home
# Assistant, which is the condition #148 fixed.
#
# Must stay below config.yaml's `timeout: 30`.
ENV S6_SERVICES_GRACETIME=10000 \
    S6_KILL_FINISH_MAXTIME=10000

# Install minimal dependencies
# Upgrade first to resolve base image package version mismatches (e.g. libcrypto3/libssl3)
RUN apk upgrade --no-cache \
    && apk add --no-cache \
        curl \
        jq \
        libgpiod \
        mosquitto-clients \
        raspberrypi-utils-vcgencmd

COPY --from=helper-builder /fan-uart-hold /usr/bin/fan-uart-hold

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
    org.opencontainers.image.vendor="Home Assistant Apps" \
    org.opencontainers.image.authors="ShobuPrime" \
    org.opencontainers.image.licenses="MIT" \
    org.opencontainers.image.url="https://github.com/ShobuPrime/home-assistant-apps" \
    org.opencontainers.image.source="https://github.com/ShobuPrime/home-assistant-apps" \
    org.opencontainers.image.created=${BUILD_DATE} \
    org.opencontainers.image.revision=${BUILD_REF} \
    org.opencontainers.image.version=${BUILD_VERSION}
