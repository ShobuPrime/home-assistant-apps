#!/usr/bin/with-contenv bashio
# ==============================================================================
# Home Assistant App: Dockge
# Runs some initializations for Dockge
# ==============================================================================
bashio::require.unprotected

bashio::log.info "Creating data directories..."
mkdir -p /data/dockge /data/stacks /run/dockge
chmod 755 /data/dockge /data/stacks

if [[ ! -S /run/docker.sock ]]; then
    bashio::log.error "Docker socket not found! Dockge will not be able to connect to Docker."
    bashio::log.error "This app requires access to the Docker socket to function."
fi

# Compose turns a stack's relative folders (./config) into absolute paths and
# hands them to the Docker daemon, which looks for them on the host. Upstream
# has the stacks folder mounted at the same path on both sides, which an app
# cannot ask for. So the host's path to this app's data folder is read from
# this container and made to lead to /data here. Dockge keeps its stacks
# under that name, and the docker wrapper (/usr/local/bin/docker) gives it to
# Compose as well.
rm -f /run/dockge/host-data
own_id="$(sed -n 's|.*docker/containers/\([a-f0-9]\{64\}\).*|\1|p' /proc/self/mountinfo 2>/dev/null | head -n 1)" || true
host_data="$(curl -s --max-time 5 --unix-socket /run/docker.sock "http://localhost/containers/${own_id:-none}/json" 2>/dev/null \
    | jq -r '.Mounts[]? | select(.Destination == "/data") | .Source' 2>/dev/null | head -n 1)" || true
if [[ "${host_data}" != /* ]]; then
    bashio::log.warning "This app's data folder on the host could not be found. A stack's relative folders (./config) will not be mounted from where Dockge keeps them."
elif [[ "${host_data}" != "/data" ]] && [[ -e "${host_data}" ]] && [[ ! -L "${host_data}" ]]; then
    bashio::log.warning "${host_data} exists in this container and cannot lead to /data. A stack's relative folders (./config) will not be mounted from where Dockge keeps them."
else
    if [[ "${host_data}" != "/data" ]]; then
        mkdir -p "$(dirname "${host_data}")"
        ln -sfn /data "${host_data}"
    fi
    echo "${host_data}" > /run/dockge/host-data
fi

# Generate Traefik reverse proxy configuration
if bashio::config.true 'traefik_enable'; then
    # The route goes to Dockge's own port as the host publishes it. The
    # script's default is the ingress port, which is the panel's adapter here
    # and is not published.
    traefik_port="$(bashio::app.port 5001 2>/dev/null || true)"
    TRAEFIK_PORT="${traefik_port:-5001}" /usr/local/bin/generate-traefik-config.sh
else
    rm -f /share/traefik/dynamic/dockge.yml
fi

bashio::log.info "Dockge initialization complete"
