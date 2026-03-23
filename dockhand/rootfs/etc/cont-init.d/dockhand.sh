#!/usr/bin/with-contenv bashio
# ==============================================================================
# Home Assistant Add-on: Dockhand
# Runs initialization for Dockhand
# ==============================================================================
bashio::require.unprotected

# Create data directory structure
bashio::log.info "Creating data directories..."
mkdir -p /data/dockhand/db
mkdir -p /data/dockhand/stacks

# Ensure proper permissions
chmod 755 /data/dockhand
chmod 755 /data/dockhand/db
chmod 755 /data/dockhand/stacks

# Check if Dockhand app directory exists
if [[ ! -d /opt/dockhand ]]; then
    bashio::log.error "Dockhand application not found at /opt/dockhand!"
    exit 1
fi

bashio::log.info "Dockhand application directory found"

# Check Docker socket access
if [[ -S /var/run/docker.sock ]]; then
    bashio::log.info "Docker socket found at /var/run/docker.sock"
elif [[ -S /run/docker.sock ]]; then
    bashio::log.info "Docker socket found at /run/docker.sock"
else
    bashio::log.error "Docker socket not found! Dockhand will not be able to connect to Docker."
    bashio::log.error "This addon requires access to the Docker socket to function."
    bashio::log.error "Please ensure the addon has the proper permissions."
fi

# Link data directory for persistence
if [[ -d /opt/dockhand/data ]]; then
    rm -rf /opt/dockhand/data
fi
ln -sf /data/dockhand /opt/dockhand/data

# Generate Traefik reverse proxy configuration
if bashio::config.true 'traefik_enable'; then
    /usr/local/bin/generate-traefik-config.sh
else
    rm -f /share/traefik/dynamic/dockhand.yml
fi

bashio::log.info "Dockhand initialization complete"
