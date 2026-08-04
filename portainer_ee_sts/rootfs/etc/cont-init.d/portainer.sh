#!/usr/bin/with-contenv bashio
# ==============================================================================
# Home Assistant App: Portainer EE
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

# Check Docker socket access and route it through the shim
#
# Portainer must not talk to dockerd directly: dockerd rejects any
# POST /containers/{id}/start whose body length is unknown, which is what
# Portainer's proxy sends, so every start from the UI fails with "non-empty
# request body was deprecated since API v1.22 and removed in v1.24". The shim
# (nginx, see /etc/nginx/docker-shim.conf) strips those bodies.
#
# Getting Portainer to use it is the awkward part. Its environment address is
# stored in the database and `--host` only seeds it on FIRST init, so pointing
# the flag at a new socket would fix nothing for an existing install — every
# user would have to re-create their environment by hand, orphaning their
# stacks in the process.
#
# So instead of moving Portainer, move the address it already has. /var/run is
# a symlink to /run in the base image, which is why `unix:///var/run/docker.sock`
# currently resolves to the socket the Supervisor mounts at /run/docker.sock.
# Replacing that symlink with a real directory holding a single entry —
# docker.sock -> /run/docker-shim/docker.sock — puts the shim in the path for
# both new and existing installs with no user action. The real socket stays
# exactly where it was for anything that wants it. Two properties of the base
# image make this safe, and BOTH have to hold:
#   1. s6-overlay's preinit runs before stage0 on every boot and treats
#      /var/run == /run as an invariant: it s6-rmrf's this directory and
#      re-links it. So the swap never leaks into a later boot — but an in-place
#      container restart does log "warning: /var/run is not a symlink to /run,
#      fixing it". That line is expected here, not a fault.
#   2. The one base-image writer under /var/run — base-addon-log-level, which
#      writes /var/run/s6/container_environment/LOG_LEVEL — is ordered ahead of
#      legacy-cont-init (legacy-cont-init/dependencies.d/base-addon-log-level),
#      so it always runs while /var/run is still the symlink.
# The base image sets S6_BEHAVIOUR_IF_STAGE2_FAILS=2, so breaking either one is
# fatal rather than cosmetic: do NOT add another cont-init script or service to
# this app that writes under /var/run/. (/var/lock is its own symlink to
# ../run/lock and is untouched.)
if bashio::fs.socket_exists '/run/docker.sock'; then
    bashio::log.info "Docker socket found at /run/docker.sock"

    # nginx chmods its listen socket to 0666, so the 0700 directory is what
    # keeps the proxied Docker API root-only inside the container.
    mkdir -p /run/docker-shim
    chmod 700 /run/docker-shim

    if [[ -L /var/run ]]; then
        rm -f /var/run
        mkdir -p /var/run
    fi

    if [[ ! -L /var/run ]] && [[ -d /var/run ]] \
        && { [[ ! -e /var/run/docker.sock ]] || [[ -L /var/run/docker.sock ]]; }; then
        ln -sfn /run/docker-shim/docker.sock /var/run/docker.sock
        bashio::log.info "Docker API routed through the shim (/var/run/docker.sock -> /run/docker-shim/docker.sock)"
    else
        bashio::log.warning "/var/run/docker.sock is not a symlink this app can redirect."
        bashio::log.warning "Portainer will talk to dockerd directly, and starting a container"
        bashio::log.warning "from the UI may fail with 'non-empty request body was deprecated'."
    fi
else
    bashio::log.error "Docker socket not found at /run/docker.sock!"
    bashio::log.error "Portainer will not be able to connect to Docker."
    bashio::log.error "This app requires access to the Docker socket to function."
    bashio::log.error "Please ensure the app has the proper permissions."
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
