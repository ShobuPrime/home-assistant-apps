#!/usr/bin/with-contenv bashio
# ==============================================================================
# Home Assistant App: Dockhand
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
    bashio::log.error "This app requires access to the Docker socket to function."
    bashio::log.error "Please ensure the app has the proper permissions."
fi

# With hide_hassio_containers on, Dockhand reaches Docker through
# ha-docker-filter (services.d/dockhand-filter). Dockhand keeps a socket path
# per environment, /var/run/docker.sock by default, and its start script, its
# stacks and its look at its own container use that path too. So the filter
# is put at that path: /var/run, a link to /run in the base image, becomes a
# directory holding docker.sock -> the filter's socket. The real socket stays
# at /run/docker.sock, for the filter.
# This holds because s6-overlay links /var/run to /run again at every boot
# (an in-place restart logs "/var/run is not a symlink to /run, fixing it"),
# and the base image's one writer under /var/run runs before this script. Do
# not add another: S6_BEHAVIOUR_IF_STAGE2_FAILS=2 stops the container on it.
if bashio::config.true 'hide_hassio_containers'; then
    if [[ -L /var/run ]]; then
        rm -f /var/run
        mkdir -p /var/run
    fi
    if [[ -d /var/run ]] && [[ ! -L /var/run ]] \
        && { [[ ! -e /var/run/docker.sock ]] || [[ -L /var/run/docker.sock ]]; }; then
        ln -sfn /run/docker-filter/docker.sock /var/run/docker.sock
        # Dockhand's start script reads the socket's group off this path
        # without following the link.
        chown -h "root:$(stat -c %g /run/docker.sock)" /var/run/docker.sock
        bashio::log.info "Home Assistant's containers are hidden: /var/run/docker.sock leads to ha-docker-filter"
    else
        # No way round the filter: Home Assistant's containers would be back
        # with no sign.
        bashio::exit.nok "/var/run/docker.sock cannot be pointed at ha-docker-filter. Turn hide_hassio_containers off to start Dockhand without it."
    fi

    # An environment with another socket path goes round the filter. The path
    # is the user's to choose, so it is reported, not changed.
    if [[ -f /data/dockhand/db/dockhand.db ]]; then
        bypassing="$(sqlite3 -readonly /data/dockhand/db/dockhand.db \
            "SELECT name || ' (' || socket_path || ')' FROM environments WHERE connection_type = 'socket' AND socket_path != '/var/run/docker.sock';" 2>/dev/null || true)"
        while IFS= read -r environment; do
            [[ -n "${environment}" ]] || continue
            bashio::log.warning "Dockhand's environment ${environment} goes round the filter and lists Home Assistant's containers. Set its socket path to /var/run/docker.sock under Settings > Environments."
        done <<< "${bypassing}"
    fi
fi

# Link data directory for persistence
if [[ -d /opt/dockhand/data ]]; then
    rm -rf /opt/dockhand/data
fi
ln -sf /data/dockhand /opt/dockhand/data

# Generate Traefik reverse proxy configuration
if bashio::config.true 'traefik_enable'; then
    # The route goes to Dockhand's own port as the host publishes it. The
    # script's default is the ingress port, which is the panel's adapter here
    # and is not published. S6_KEEP_ENV: the script starts through
    # with-contenv, which otherwise drops what is passed here.
    traefik_port="$(bashio::app.port 3000 2>/dev/null || true)"
    S6_KEEP_ENV=1 TRAEFIK_PORT="${traefik_port:-3000}" /usr/local/bin/generate-traefik-config.sh
else
    rm -f /share/traefik/dynamic/dockhand.yml
fi

bashio::log.info "Dockhand initialization complete"
