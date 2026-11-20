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
# /app/data in the image is a symlink to /data/arcane/data, which must exist
# before Arcane starts.
mkdir -p /data/arcane/data

# Hand the data dirs to the runtime user; Arcane v1.19+ otherwise
# fails with "create sqlite file /data/arcane/arcane.db: permission denied".
chown -R "${PUID}:${PGID}" /data/arcane
chmod 755 /data/arcane

# Created by this app before 3.1.0. Arcane only uses it if its Projects
# Directory setting was pointed here by hand.
if [[ -d /data/projects ]]; then
    chown -R "${PUID}:${PGID}" /data/projects
fi

# /builds is Arcane's default place for build workspaces, as in its own image.
chown "${PUID}:${PGID}" /builds

# Directory settings an early release of this app left pointing where Arcane
# cannot write. Run as Arcane's user, so the database files stay its own.
while IFS= read -r line; do
    bashio::log.notice "${line}"
done < <(s6-setuidgid "${PUID}:${PGID}" /usr/share/arcane/migrate-settings.sh /data/arcane/arcane.db 2>&1)

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

# Generate the encryption key if not already generated. Files written before
# Arcane 3 also hold a JWT_SECRET line, which Arcane no longer reads.
SECRETS_FILE="/data/arcane/.secrets"
if [[ ! -f "${SECRETS_FILE}" ]]; then
    bashio::log.info "Generating encryption key..."
    ENCRYPTION_KEY=$(openssl rand -base64 32)
    echo "ENCRYPTION_KEY=${ENCRYPTION_KEY}" > "${SECRETS_FILE}"
    chmod 600 "${SECRETS_FILE}"
    bashio::log.info "Encryption key generated and stored"
else
    bashio::log.info "Using existing encryption key"
fi

# Generate Traefik reverse proxy configuration
if bashio::config.true 'traefik_enable'; then
    # The route goes to Arcane's own port as the host publishes it. The
    # script's default is the ingress port, which is the panel's proxy here
    # and is not published.
    traefik_port="$(bashio::app.port 3552 2>/dev/null || true)"
    TRAEFIK_PORT="${traefik_port:-3552}" /usr/local/bin/generate-traefik-config.sh
else
    rm -f /share/traefik/dynamic/arcane.yml
fi

bashio::log.info "Arcane initialization complete"
