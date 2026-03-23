#!/usr/bin/with-contenv bashio
# ==============================================================================
# Home Assistant Add-on: Dockge
# Runs some initializations for Dockge
# ==============================================================================
bashio::require.unprotected

# Create data directory structure
bashio::log.info "Creating data directories..."
mkdir -p /data/dockge
mkdir -p "$(bashio::config 'stacks_dir')"

# Ensure proper permissions
chmod 755 /data/dockge
chmod 755 "$(bashio::config 'stacks_dir')"

# Check Docker socket access
if [[ -S /var/run/docker.sock ]]; then
    bashio::log.info "Docker socket found at /var/run/docker.sock"
elif [[ -S /run/docker.sock ]]; then
    bashio::log.info "Docker socket found at /run/docker.sock"
else
    bashio::log.error "Docker socket not found! Dockge will not be able to connect to Docker."
    bashio::log.error "This addon requires access to the Docker socket to function."
    bashio::log.error "Please ensure the addon has the proper permissions."
fi

# Track hide_hassio_containers setting changes
HIDE_SETTING_FILE="/data/dockge/.hide_hassio_containers"
CURRENT_SETTING=$(bashio::config 'hide_hassio_containers')

if bashio::fs.file_exists "${HIDE_SETTING_FILE}"; then
    PREVIOUS_SETTING=$(cat "${HIDE_SETTING_FILE}")
    if [[ "${CURRENT_SETTING}" != "${PREVIOUS_SETTING}" ]]; then
        bashio::log.warning "hide_hassio_containers setting changed from ${PREVIOUS_SETTING} to ${CURRENT_SETTING}"
        bashio::log.info "Note: You may need to manually show/hide containers in Dockge UI."
    fi
fi

echo "${CURRENT_SETTING}" > "${HIDE_SETTING_FILE}"

# Check if Dockge is available
if command -v node >/dev/null 2>&1; then
    bashio::log.info "Node.js is available"
else
    bashio::log.warning "Node.js not found in PATH, but Dockge should have it"
fi

# Generate Traefik reverse proxy configuration
if bashio::config.true 'traefik_enable'; then
    /usr/local/bin/generate-traefik-config.sh
else
    rm -f /share/traefik/dynamic/dockge.yml
fi

bashio::log.info "Dockge initialization complete"
