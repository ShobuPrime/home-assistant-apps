#!/usr/bin/with-contenv bashio
# ==============================================================================
# Home Assistant Add-on: Sonuntius
# Prepares the runtime environment for the cast-receiver, yt-cast, and
# ma-bridge services. The Go binaries read /data/options.json directly
# (with a Supervisor REST fallback), so no env wiring is needed.
# ==============================================================================

bashio::log.info "Sonuntius: preparing runtime environment..."

mkdir -p /data/sonuntius
chmod 755 /data/sonuntius

mkdir -p /run/sonuntius
chmod 755 /run/sonuntius

if [[ ! -x /usr/local/bin/ma-bridge ]]; then
    bashio::log.error "ma-bridge binary missing from /usr/local/bin/"
    exit 1
fi

# Cast (CASTV2 / AirReceiver) cert is optional — the cast-receiver
# binary degrades gracefully when it isn't present. We warn-and-continue
# so the user can see in the init log that proxy mode is unavailable
# until they drop the cert under /share/sonuntius/.
if bashio::config.has_value 'cast_cert_path'; then
    CAST_CERT_PATH=$(bashio::config 'cast_cert_path')
else
    CAST_CERT_PATH="/share/sonuntius/airreceiver_cert.pem"
fi

if [[ -n "${CAST_CERT_PATH}" && ! -f "${CAST_CERT_PATH}" ]]; then
    bashio::log.warning "Sonuntius: AirReceiver cert not found at ${CAST_CERT_PATH}"
    bashio::log.warning "Sonuntius: Tidal proxy mode will be disabled (cast-receiver stays alive for mDNS only)"
fi

bashio::log.info "Sonuntius: runtime environment ready."
