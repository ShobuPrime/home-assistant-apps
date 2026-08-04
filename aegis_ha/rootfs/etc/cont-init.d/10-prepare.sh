#!/usr/bin/with-contenv bashio
# ==============================================================================
# Home Assistant App: AegisHA
# Prepare the runtime environment: persistent data directory.
# ==============================================================================

bashio::log.info "AegisHA: preparing runtime environment"

mkdir -p /data/aegis_ha
chmod 700 /data/aegis_ha

if [[ ! -x /usr/local/bin/aegis_ha ]]; then
    bashio::log.error "AegisHA binary missing or not executable at /usr/local/bin/aegis_ha"
    exit 1
fi

bashio::log.info "AegisHA: runtime environment ready"
