#!/usr/bin/with-contenv bashio
# ==============================================================================
# Home Assistant Add-on: Portainer EE
# Runs some initializations for Portainer
# ==============================================================================
bashio::require.unprotected

# Create data directory structure
bashio::log.info "Creating data directories..."
mkdir -p /data/portainer
mkdir -p /data/tls

# Ensure proper permissions
chmod 755 /data/portainer
chmod 755 /data/tls

# Check if Portainer binary exists and is executable
if [[ ! -f /opt/portainer/portainer ]]; then
    bashio::log.error "Portainer binary not found at /opt/portainer/portainer!"
    exit 1
fi

if [[ ! -x /opt/portainer/portainer ]]; then
    bashio::log.warning "Portainer binary not executable, fixing permissions..."
    chmod +x /opt/portainer/portainer
fi

# Log Portainer version
bashio::log.info "Checking Portainer installation..."
if /opt/portainer/portainer --version; then
    bashio::log.info "Portainer binary is working correctly"
else
    bashio::log.warning "Could not get Portainer version, but continuing..."
fi

# Check Docker socket access
if [[ -S /var/run/docker.sock ]]; then
    bashio::log.info "Docker socket found at /var/run/docker.sock"
elif [[ -S /run/docker.sock ]]; then
    bashio::log.info "Docker socket found at /run/docker.sock"
else
    bashio::log.error "Docker socket not found! Portainer will not be able to connect to Docker."
    bashio::log.error "This addon requires access to the Docker socket to function."
    bashio::log.error "Please ensure the addon has the proper permissions."
fi

# Track hide_hassio_containers setting changes
HIDE_SETTING_FILE="/data/portainer/.hide_hassio_containers"
CURRENT_SETTING=$(bashio::config 'hide_hassio_containers')

if bashio::fs.file_exists "${HIDE_SETTING_FILE}"; then
    PREVIOUS_SETTING=$(cat "${HIDE_SETTING_FILE}")
    if [[ "${CURRENT_SETTING}" != "${PREVIOUS_SETTING}" ]]; then
        bashio::log.warning "hide_hassio_containers setting changed from ${PREVIOUS_SETTING} to ${CURRENT_SETTING}"
        bashio::log.info "Note: Portainer caches hidden labels. You may need to manually show/hide containers in Portainer UI."
        bashio::log.info "To reset: Go to Settings → Hidden containers in Portainer to manage visibility."
    fi
fi

echo "${CURRENT_SETTING}" > "${HIDE_SETTING_FILE}"

# Generate Traefik reverse proxy configuration
if bashio::config.true 'traefik_enable'; then
    # Use HTTPS backend if ssl is configured
    if bashio::config.true 'ssl'; then
        TRAEFIK_PORT=9443 TRAEFIK_SCHEME=https /usr/local/bin/generate-traefik-config.sh
    else
        TRAEFIK_PORT=9000 /usr/local/bin/generate-traefik-config.sh
    fi
else
    rm -f /share/traefik/dynamic/portainer_ee_sts.yml
fi

bashio::log.info "Portainer EE initialization complete"
