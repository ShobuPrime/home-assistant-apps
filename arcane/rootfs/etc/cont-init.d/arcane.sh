#!/usr/bin/with-contenv bashio
# ==============================================================================
# Home Assistant App: Arcane Docker Manager
# Runs initialization for Arcane
# ==============================================================================
bashio::require.unprotected

# Resolve PUID/PGID (Arcane >=1.19 drops privileges before opening
# its sqlite DB, so the data dir must be writable by that user).
if bashio::config.has_value 'puid'; then
    PUID=$(bashio::config 'puid')
else
    PUID=1000
fi
if bashio::config.has_value 'pgid'; then
    PGID=$(bashio::config 'pgid')
else
    PGID=1000
fi

# Create data directory structure
bashio::log.info "Creating data directories (owner ${PUID}:${PGID})..."
mkdir -p /data/arcane
mkdir -p /data/projects

# Hand the data dirs to the runtime user; Arcane v1.19+ otherwise
# fails with "create sqlite file /data/arcane/arcane.db: permission denied".
chown -R "${PUID}:${PGID}" /data/arcane /data/projects
chmod 755 /data/arcane /data/projects

# Check if Arcane binary exists and is executable
if [[ ! -f /opt/arcane/arcane ]]; then
    bashio::log.error "Arcane binary not found at /opt/arcane/arcane!"
    exit 1
fi

if [[ ! -x /opt/arcane/arcane ]]; then
    bashio::log.warning "Arcane binary not executable, fixing permissions..."
    chmod +x /opt/arcane/arcane
fi

# Log Arcane version
bashio::log.info "Checking Arcane installation..."
if /opt/arcane/arcane --version 2>/dev/null; then
    bashio::log.info "Arcane binary is working correctly"
else
    bashio::log.warning "Could not get Arcane version, but continuing..."
fi

# Check Docker socket access
if [[ -S /var/run/docker.sock ]]; then
    bashio::log.info "Docker socket found at /var/run/docker.sock"
elif [[ -S /run/docker.sock ]]; then
    bashio::log.info "Docker socket found at /run/docker.sock"
else
    bashio::log.error "Docker socket not found! Arcane will not be able to connect to Docker."
    bashio::log.error "This app requires access to the Docker socket to function."
    bashio::log.error "Please ensure the app has the proper permissions."
fi

# Generate secrets if not already generated
SECRETS_FILE="/data/arcane/.secrets"
if [[ ! -f "${SECRETS_FILE}" ]]; then
    bashio::log.info "Generating encryption keys..."
    ENCRYPTION_KEY=$(openssl rand -base64 32)
    JWT_SECRET=$(openssl rand -base64 32)
    echo "ENCRYPTION_KEY=${ENCRYPTION_KEY}" > "${SECRETS_FILE}"
    echo "JWT_SECRET=${JWT_SECRET}" >> "${SECRETS_FILE}"
    chmod 600 "${SECRETS_FILE}"
    bashio::log.info "Encryption keys generated and stored"
else
    bashio::log.info "Using existing encryption keys"
fi

# Generate Traefik reverse proxy configuration
if bashio::config.true 'traefik_enable'; then
    /usr/local/bin/generate-traefik-config.sh
else
    rm -f /share/traefik/dynamic/arcane.yml
fi

bashio::log.info "Arcane initialization complete"
