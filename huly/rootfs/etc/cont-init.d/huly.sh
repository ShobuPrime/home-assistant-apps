#!/usr/bin/with-contenv bashio
# ==============================================================================
# Home Assistant Add-on: Huly
# Initializes Huly configuration and data directories
# ==============================================================================
bashio::require.unprotected

bashio::log.info "========================================"
bashio::log.info "Huly init starting at $(date '+%Y-%m-%d %H:%M:%S %Z')"
bashio::log.info "========================================"
bashio::log.debug "Architecture: $(uname -m)"
bashio::log.debug "Architecture: $(uname -m)"
bashio::log.debug "Kernel: $(uname -r)"

# Create data directories for all Huly services
bashio::log.info "Creating data directories..."
for dir in /data/huly /data/huly/cockroach /data/huly/cockroach-certs \
           /data/huly/elastic /data/huly/minio /data/huly/kafka; do
    mkdir -p "${dir}"
    chmod 755 "${dir}"
    bashio::log.debug "Created directory: ${dir}"
done

# One-time migration: clean up old Redpanda data directory (replaced by Kafka)
if [[ -d /data/huly/kafka ]]; then
    bashio::log.info "Cleaning up legacy Redpanda data directory (replaced by Kafka)..."
    rm -rf /data/huly/kafka
fi

# ---------------------------------------------------------------------------
# Resolve credentials: config UI value > existing secrets file > auto-generate
# ---------------------------------------------------------------------------
SECRETS_FILE="/data/huly/.secrets"

# Read user-supplied credentials from the HA config UI (empty string = not set)
CFG_SECRET=$(bashio::config 'server_secret')
CFG_CR_PASSWORD=$(bashio::config 'cockroachdb_password')
CFG_MINIO_USER=$(bashio::config 'minio_root_user')
CFG_MINIO_PWD=$(bashio::config 'minio_root_password')

# Source existing secrets file (provides fallback values for anything not in config)
if [[ -f "${SECRETS_FILE}" ]]; then
    bashio::log.info "Loading existing secrets file"
    # shellcheck source=/dev/null
    source "${SECRETS_FILE}"
fi

# Check whether ANY config password was explicitly set by the user
HAS_CONFIG_PASSWORDS=false
for val in "${CFG_SECRET}" "${CFG_CR_PASSWORD}" \
           "${CFG_MINIO_USER}" "${CFG_MINIO_PWD}"; do
    if bashio::var.has_value "${val}"; then
        HAS_CONFIG_PASSWORDS=true
        break
    fi
done

# Stale-data safety net.
# Case 1: No secrets file + no config passwords + existing DB data → full wipe.
# Case 2: Secrets file exists + no config passwords + existing DB data but the
#          secrets file was written by a DIFFERENT init run than the one that
#          provisioned CockroachDB. We detect this with a canary file that is
#          created atomically with the secrets and deleted on data wipe.
NEEDS_DATA_WIPE=false

if [[ -d /data/huly/cockroach ]] && [[ -n "$(ls -A /data/huly/cockroach 2>/dev/null)" ]]; then
    if [[ ! -f "${SECRETS_FILE}" ]] && [[ "${HAS_CONFIG_PASSWORDS}" == "false" ]]; then
        bashio::log.warning "Secrets file missing, no config passwords set, but database data exists — credential mismatch detected."
        NEEDS_DATA_WIPE=true
    elif [[ -f "${SECRETS_FILE}" ]] && [[ ! -f /data/huly/.secrets_canary ]] && [[ "${HAS_CONFIG_PASSWORDS}" == "false" ]]; then
        bashio::log.warning "Secrets file exists but canary missing — secrets were regenerated without re-provisioning databases."
        NEEDS_DATA_WIPE=true
    fi
fi

if [[ "${NEEDS_DATA_WIPE}" == "true" ]]; then
    bashio::log.warning "Wiping stale service data for fresh initialization..."
    for dir in cockroach cockroach-certs elastic minio redpanda; do
        rm -rf "/data/huly/${dir:?}"/*
        bashio::log.debug "Cleared /data/huly/${dir}"
    done
    # Remove stale canary so a new one is created with fresh secrets
    rm -f /data/huly/.secrets_canary
fi

# Resolve each credential: config value > existing secret > auto-generate
resolve_credential() {
    local cfg_val="$1" existing_val="$2" generator="$3"
    if bashio::var.has_value "${cfg_val}"; then
        echo "${cfg_val}"
    elif bashio::var.has_value "${existing_val}"; then
        echo "${existing_val}"
    else
        eval "${generator}"
    fi
}

SECRET=$(resolve_credential "${CFG_SECRET}" "${SECRET:-}" "openssl rand -hex 32")
CR_USER_PASSWORD=$(resolve_credential "${CFG_CR_PASSWORD}" "${CR_USER_PASSWORD:-}" "openssl rand -hex 16")
# MinIO defaults to minioadmin/minioadmin (matching upstream huly-selfhost).
# Nginx proxies /files/ to MinIO unauthenticated, which works with the defaults
# since MinIO allows anonymous read access in its default configuration.
# Custom credentials break this unless a public bucket policy is also configured.
MINIO_ROOT_USER=$(resolve_credential "${CFG_MINIO_USER}" "${MINIO_ROOT_USER:-}" "echo minioadmin")
MINIO_ROOT_PASSWORD=$(resolve_credential "${CFG_MINIO_PWD}" "${MINIO_ROOT_PASSWORD:-}" "echo minioadmin")

# Persist resolved values so subsequent restarts (without config changes) reuse them
cat > "${SECRETS_FILE}" << EOF
SECRET=${SECRET}
CR_USER_PASSWORD=${CR_USER_PASSWORD}
MINIO_ROOT_USER=${MINIO_ROOT_USER}
MINIO_ROOT_PASSWORD=${MINIO_ROOT_PASSWORD}
EOF
chmod 600 "${SECRETS_FILE}"

# Write a canary file atomically with the secrets. When CockroachDB starts with
# an empty data dir it provisions COCKROACH_USER/PASSWORD from the environment.
# If the canary is missing but DB data exists, the secrets don't match the data.
if [[ ! -f /data/huly/.secrets_canary ]]; then
    date -Iseconds > /data/huly/.secrets_canary
fi
bashio::log.info "Credentials resolved and saved"

# Elasticsearch requires its data directory owned by uid 1000 (elasticsearch user).
# Run AFTER the stale-data wipe so a freshly-cleared directory gets correct ownership.
chown -R 1000:1000 /data/huly/elastic
bashio::log.debug "Set /data/huly/elastic ownership to 1000:1000"

# Check Docker socket
if [[ -S /var/run/docker.sock ]]; then
    bashio::log.info "Docker socket found at /var/run/docker.sock"
    bashio::log.debug "Docker socket permissions: $(ls -la /var/run/docker.sock)"
elif [[ -S /run/docker.sock ]]; then
    bashio::log.info "Docker socket found at /run/docker.sock"
    bashio::log.debug "Docker socket permissions: $(ls -la /run/docker.sock)"
else
    bashio::log.error "Docker socket not found! Huly requires Docker access."
    bashio::log.error "Please ensure the addon has the proper permissions."
fi

# Resolve the host-side path for /data.
# Inside the addon container /data is a bind mount from the host. Sub-containers
# spawned via Docker Compose are created by the HOST Docker daemon, so their
# bind-mount paths must reference the real host path, not the container path.
#
# In HAOS, docker inspect returns a path like /mnt/data/supervisor/addons/data/<slug>
# which IS the real host path. In some setups it may return /supervisor/addons/data/<slug>
# which needs a /mnt/data prefix.
#
# Strategy: get the inspect path, check if it already starts with DockerRootDir's
# parent (indicating it's already a full host path), then try prefixed candidates
# only if needed. Verify each candidate with a Docker volume bind test.
#
# NOTE: We avoid Alpine's docker-cli entirely (segfaults on aarch64, docker/cli#4900).
HOST_DATA_PATH=""
MOUNT_SOURCE=""

# Step 1: Get the /data mount source from container inspect.
# Try multiple container identification methods since the ID format varies.
# HAOS names addon containers addon_<hash>_<slug> but hostname is <hash>-<slug>.
CONTAINER_HOSTNAME="$(hostname)"
bashio::log.info "Resolving host data path (hostname: ${CONTAINER_HOSTNAME})..."

# Build candidate container IDs:
#   1. Full container ID from /proc/self/mountinfo (most reliable)
#   2. HAOS addon naming: addon_<hash>_<slug> (hostname with - replaced by _)
#   3. Plain hostname (fallback)
PROC_CID=$(sed -n 's|.*docker/containers/\([a-f0-9]\{64\}\).*|\1|p' /proc/self/mountinfo 2>/dev/null | head -1) || true
HAOS_CID="addon_$(echo "${CONTAINER_HOSTNAME}" | sed 's/-/_/g')"

for CONTAINER_ID in ${PROC_CID} ${HAOS_CID} ${CONTAINER_HOSTNAME}; do
    bashio::log.debug "Trying container ID: ${CONTAINER_ID}"
    INSPECT_JSON=$(curl -s --unix-socket /var/run/docker.sock \
        "http://localhost/containers/${CONTAINER_ID}/json" 2>/dev/null) || true
    if [[ -n "${INSPECT_JSON}" ]]; then
        MOUNT_SOURCE=$(echo "${INSPECT_JSON}" \
            | jq -r '.Mounts[] | select(.Destination == "/data") | .Source' 2>/dev/null) || true
        if [[ -n "${MOUNT_SOURCE}" ]]; then
            bashio::log.info "Identified container as: ${CONTAINER_ID}"
            bashio::log.debug "All mounts: $(echo "${INSPECT_JSON}" | jq -c '[.Mounts[] | {Source, Destination}]' 2>/dev/null)" || true
            bashio::log.debug "Extracted /data mount source: '${MOUNT_SOURCE}'"
            break
        fi
    fi
done

# Step 2: Get DockerRootDir to derive the data partition prefix.
DOCKER_ROOT_DIR=""
DOCKER_INFO=$(curl -s --unix-socket /var/run/docker.sock \
    "http://localhost/info" 2>/dev/null) || true
if [[ -n "${DOCKER_INFO}" ]]; then
    DOCKER_ROOT_DIR=$(echo "${DOCKER_INFO}" | jq -r '.DockerRootDir // empty' 2>/dev/null) || true
    bashio::log.debug "DockerRootDir: '${DOCKER_ROOT_DIR}'"
fi

# Step 3: Build candidate paths and test each by creating a temporary volume.
# A successful volume create with bind mount proves Docker daemon can access it.
#
# IMPORTANT: If MOUNT_SOURCE already starts with a known prefix (e.g. /mnt/data),
# skip prepending that prefix to avoid doubled paths like /mnt/data/mnt/data/...
if [[ -n "${MOUNT_SOURCE}" ]]; then
    # Derive the data partition prefix from DockerRootDir (e.g. /mnt/data/docker → /mnt/data)
    DATA_PREFIX=""
    if [[ "${DOCKER_ROOT_DIR}" == */docker ]]; then
        DATA_PREFIX="${DOCKER_ROOT_DIR%/docker}"
    fi

    # Build list of prefixes to try (most specific first), but skip any prefix
    # that MOUNT_SOURCE already starts with to prevent doubling.
    PREFIXES=""
    if [[ -n "${DATA_PREFIX}" ]] && [[ "${MOUNT_SOURCE}" != "${DATA_PREFIX}"* ]]; then
        PREFIXES="${DATA_PREFIX}"
    fi
    if [[ "${MOUNT_SOURCE}" != /mnt/data* ]]; then
        PREFIXES="${PREFIXES} /mnt/data"
    fi
    # Always try no prefix last (in case the path works directly)
    PREFIXES="${PREFIXES} "

    TEST_VOL_NAME="huly_path_test_$$"
    for prefix in ${PREFIXES}; do
        CANDIDATE="${prefix}${MOUNT_SOURCE}"
        bashio::log.debug "Testing candidate path: '${CANDIDATE}'"

        # Try to create a Docker volume bound to this path
        VOL_RESULT=$(curl -s --unix-socket /var/run/docker.sock \
            -X POST -H "Content-Type: application/json" \
            -d "$(jq -n --arg name "${TEST_VOL_NAME}" --arg dev "${CANDIDATE}" \
                '{Name: $name, Driver: "local", DriverOpts: {type: "none", o: "bind", device: $dev}}')" \
            "http://localhost/volumes/create" 2>/dev/null) || true

        # Check if the volume was created (no error message)
        VOL_ERROR=$(echo "${VOL_RESULT}" | jq -r '.message // empty' 2>/dev/null) || true
        if [[ -z "${VOL_ERROR}" ]]; then
            HOST_DATA_PATH="${CANDIDATE}"
            bashio::log.info "Verified host data path: ${HOST_DATA_PATH}"
            # Clean up test volume
            curl -s --unix-socket /var/run/docker.sock \
                -X DELETE "http://localhost/volumes/${TEST_VOL_NAME}" 2>/dev/null || true
            break
        else
            bashio::log.debug "Path '${CANDIDATE}' failed: ${VOL_ERROR}"
            # Clean up failed volume attempt just in case
            curl -s --unix-socket /var/run/docker.sock \
                -X DELETE "http://localhost/volumes/${TEST_VOL_NAME}" 2>/dev/null || true
        fi
    done

    # If prefixed candidates all failed, try MOUNT_SOURCE directly (it may already
    # be the correct full host path, as is common on recent HAOS).
    if [[ -z "${HOST_DATA_PATH}" ]]; then
        bashio::log.debug "Prefixed candidates failed, testing MOUNT_SOURCE directly: '${MOUNT_SOURCE}'"
        VOL_RESULT=$(curl -s --unix-socket /var/run/docker.sock \
            -X POST -H "Content-Type: application/json" \
            -d "$(jq -n --arg name "${TEST_VOL_NAME}" --arg dev "${MOUNT_SOURCE}" \
                '{Name: $name, Driver: "local", DriverOpts: {type: "none", o: "bind", device: $dev}}')" \
            "http://localhost/volumes/create" 2>/dev/null) || true
        VOL_ERROR=$(echo "${VOL_RESULT}" | jq -r '.message // empty' 2>/dev/null) || true
        if [[ -z "${VOL_ERROR}" ]]; then
            HOST_DATA_PATH="${MOUNT_SOURCE}"
            bashio::log.info "MOUNT_SOURCE works directly as host path: ${HOST_DATA_PATH}"
        fi
        curl -s --unix-socket /var/run/docker.sock \
            -X DELETE "http://localhost/volumes/${TEST_VOL_NAME}" 2>/dev/null || true
    fi
fi

if [[ -z "${HOST_DATA_PATH}" || "${HOST_DATA_PATH}" == "/" ]]; then
    bashio::log.error "Could not determine host path for /data — volume mounts will fail."
    bashio::log.error "Mount source from inspect: '${MOUNT_SOURCE}'"
    bashio::log.error "DockerRootDir: '${DOCKER_ROOT_DIR}'"
    bashio::log.error "Falling back to mount source path (may not work)."
    HOST_DATA_PATH="${MOUNT_SOURCE:-/data}"
else
    bashio::log.info "Resolved host data path: ${HOST_DATA_PATH}"
fi

# Verify docker compose is available
COMPOSE_VER=$(/usr/local/bin/docker-compose version 2>&1) || true
if [[ -n "${COMPOSE_VER}" ]]; then
    bashio::log.info "Docker Compose is available"
    bashio::log.debug "Docker Compose version: ${COMPOSE_VER}"
else
    bashio::log.error "Docker Compose not available!"
fi

# Read config values
bashio::log.info "Generating Huly configuration..."
HOST_ADDRESS=$(bashio::config 'host_address')
TITLE=$(bashio::config 'title')
DEFAULT_LANGUAGE=$(bashio::config 'default_language')
LAST_NAME_FIRST=$(bashio::config 'last_name_first')

# Determine HOST_ADDRESS:
#   1. User-configured value (domain name or IP for reverse proxy setups)
#   2. Auto-detected from HA Supervisor network API
#   3. Fallback to localhost:4859 (won't work from browser but prevents crash)
if bashio::var.has_value "${HOST_ADDRESS}"; then
    bashio::log.info "Using configured host address: ${HOST_ADDRESS}"
else
    # Auto-detect the HA host IP via Supervisor API
    DETECTED_IP=$(curl -s -H "Authorization: Bearer ${SUPERVISOR_TOKEN}" \
        http://supervisor/network/info 2>/dev/null \
        | jq -r '.data.interfaces[]?.ipv4.address[]? // empty' 2>/dev/null \
        | head -1 | cut -d'/' -f1)

    if [[ -n "${DETECTED_IP}" ]] && [[ "${DETECTED_IP}" != 172.* ]] && [[ "${DETECTED_IP}" != 127.* ]]; then
        HOST_ADDRESS="${DETECTED_IP}:4859"
        bashio::log.info "Auto-detected host address: ${HOST_ADDRESS}"
    else
        HOST_ADDRESS="localhost:4859"
        bashio::log.warning "Could not auto-detect host IP. Set host_address in addon config."
        bashio::log.warning "Using fallback: ${HOST_ADDRESS} (browser access may not work)"
    fi
fi

# Determine HULY_VERSION from build ENV or default (includes v prefix)
HULY_VERSION="${HULY_VERSION:-v0.7.375}"
# Desktop update channel uses the version without the v prefix
DESKTOP_CHANNEL="${HULY_VERSION#v}"

# Write the .env file for docker compose
bashio::log.info "Writing Huly environment configuration..."
cat > /data/huly/.env << EOF
# Huly version
HULY_VERSION=${HULY_VERSION}
DOCKER_NAME=huly_ha

# Network
HOST_ADDRESS=${HOST_ADDRESS}
HTTP_PORT=80
HTTP_BIND=

# Huly settings
TITLE=${TITLE:-Huly}
DEFAULT_LANGUAGE=${DEFAULT_LANGUAGE:-en}
LAST_NAME_FIRST=${LAST_NAME_FIRST:-true}
DESKTOP_CHANNEL=${DESKTOP_CHANNEL}

# CockroachDB
CR_DATABASE=defaultdb
CR_USERNAME=selfhost
CR_USER_PASSWORD=${CR_USER_PASSWORD}
CR_DB_URL=postgres://selfhost:${CR_USER_PASSWORD}@cockroach:26257/defaultdb

# Secret
SECRET=${SECRET}

# MinIO
MINIO_ROOT_USER=${MINIO_ROOT_USER}
MINIO_ROOT_PASSWORD=${MINIO_ROOT_PASSWORD}

# Volumes (host-side paths for bind mounts into sub-containers)
VOLUME_CR_DATA_PATH=${HOST_DATA_PATH}/huly/cockroach
VOLUME_CR_CERTS_PATH=${HOST_DATA_PATH}/huly/cockroach-certs
VOLUME_ELASTIC_PATH=${HOST_DATA_PATH}/huly/elastic
VOLUME_FILES_PATH=${HOST_DATA_PATH}/huly/minio
VOLUME_KAFKA_PATH=${HOST_DATA_PATH}/huly/kafka

# Nginx config (host-side path for bind mount)
NGINX_CONF_PATH=${HOST_DATA_PATH}/huly/nginx.conf
EOF

# Log the generated .env for debugging (redact secrets)
bashio::log.debug "Generated .env file (secrets redacted):"
bashio::log.debug "$(grep -v -E '(PASSWORD|SECRET|PWD)=' /data/huly/.env)" || true

# Copy compose file template
bashio::log.info "Setting up docker-compose configuration..."
cp /opt/huly/compose.yaml.tmpl /data/huly/compose.yml

# Clean up stale directory if Docker auto-created it from a failed bind mount
if [[ -d /data/huly/nginx.conf ]]; then
    bashio::log.warning "Removing stale nginx.conf directory (auto-created by Docker from failed mount)..."
    rm -rf /data/huly/nginx.conf
fi

# Generate nginx config for routing.
# Upstream uses a single location / that proxies everything to front:8080.
# The Huly front service handles all internal routing (/_accounts, /_transactor,
# /_collaborator, /files, etc.) and authenticates with backend services.
bashio::log.info "Generating nginx configuration..."
cat > /data/huly/nginx.conf << 'NGINXEOF'
server {
    listen 80;
    server_name _;

    client_max_body_size 250M;

    location / {
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_pass http://front:8080;
    }
}
NGINXEOF

bashio::log.info "Huly initialization complete"
