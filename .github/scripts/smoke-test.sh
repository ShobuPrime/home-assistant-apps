#!/bin/bash
# Smoke test for Home Assistant addons
# Usage: smoke-test.sh <addon-directory> <image-name>
#
# Spins up a mock HA Supervisor so the addon's S6 init boots normally,
# then checks health and runs addon-specific validations.

set -e

ADDON_DIR="${1:?Usage: smoke-test.sh <addon-dir> <image-name>}"
IMAGE_NAME="${2:?Usage: smoke-test.sh <addon-dir> <image-name>}"
NETWORK_NAME="smoke-test-net"
SUPERVISOR_NAME="smoke-test-supervisor"
CONTAINER_NAME="smoke-test-$(basename "${ADDON_DIR}")"
MAX_WAIT=120
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

pass() { echo -e "  ${GREEN}PASS${NC}: $1"; }
fail() { echo -e "  ${RED}FAIL${NC}: $1"; echo "--- Container logs ---"; docker logs "${CONTAINER_NAME}" 2>&1 | tail -30; exit 1; }
info() { echo -e "  ${YELLOW}INFO${NC}: $1"; }

cleanup() {
    # Clean up compose containers if this is a compose-based addon
    if [ "${NEEDS_DOCKER}" = "true" ]; then
        docker ps -a --filter "label=com.docker.compose.project=huly_ha" \
            --format '{{.ID}}' 2>/dev/null | xargs -r docker rm -f 2>/dev/null || true
        docker network rm huly_ha_huly_net 2>/dev/null || true
    fi
    docker rm -f "${CONTAINER_NAME}" 2>/dev/null || true
    docker rm -f "${SUPERVISOR_NAME}" 2>/dev/null || true
    docker network rm "${NETWORK_NAME}" 2>/dev/null || true
    # Clean up host-side data directory for Docker-in-Docker addons
    [ -n "${SMOKE_DATA_DIR}" ] && rm -rf "${SMOKE_DATA_DIR}" 2>/dev/null || true
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Parse addon config
# ---------------------------------------------------------------------------
CONFIG="${ADDON_DIR}/config.yaml"
if [ ! -f "${CONFIG}" ]; then
    echo "Error: ${CONFIG} not found"
    exit 1
fi

SLUG=$(grep "^slug:" "${CONFIG}" | sed 's/slug: *"\(.*\)"/\1/')
NEEDS_DOCKER=$(grep -q "^docker_api: true" "${CONFIG}" && echo "true" || echo "false")

# Extract health check port from watchdog config
WATCHDOG=$(grep "^watchdog:" "${CONFIG}" | sed 's/watchdog: *//')
HEALTH_PORT=$(echo "${WATCHDOG}" | sed -n 's/.*PORT:\([0-9]*\).*/\1/p')
HEALTH_PATH=$(echo "${WATCHDOG}" | sed 's|.*\]||; s|^/||; s|/$||')

if [ -z "${HEALTH_PORT}" ]; then
    HEALTH_PORT=$(grep "^ingress_port:" "${CONFIG}" | awk '{print $2}')
fi

echo "=== Smoke Test: ${SLUG} ==="
echo "  Image: ${IMAGE_NAME}"
echo "  Health: port ${HEALTH_PORT}/${HEALTH_PATH:-(root)}"
echo "  Docker API: ${NEEDS_DOCKER}"
echo ""

# ---------------------------------------------------------------------------
# Create network and start mock Supervisor
# ---------------------------------------------------------------------------
echo "==> Starting mock HA Supervisor..."
docker network create "${NETWORK_NAME}" > /dev/null 2>&1

docker run -d \
    --name "${SUPERVISOR_NAME}" \
    --network "${NETWORK_NAME}" \
    --network-alias supervisor \
    -v "${SCRIPT_DIR}/mock-supervisor.py:/mock-supervisor.py:ro" \
    -v "$(pwd)/${ADDON_DIR}:/addon:ro" \
    python:3-slim \
    python3 /mock-supervisor.py /addon 80 > /dev/null

# Wait for mock supervisor to be ready
for i in 1 2 3 4 5 6 7 8 9 10; do
    if docker exec "${SUPERVISOR_NAME}" python3 -c "
import urllib.request
urllib.request.urlopen('http://localhost/supervisor/info').read()
" > /dev/null 2>&1; then
        break
    fi
    sleep 1
done

if ! docker exec "${SUPERVISOR_NAME}" python3 -c "
import urllib.request
urllib.request.urlopen('http://localhost/supervisor/info').read()
" > /dev/null 2>&1; then
    docker logs "${SUPERVISOR_NAME}" 2>&1 | tail -10
    fail "Mock Supervisor did not start"
fi
pass "Mock Supervisor running"

# ---------------------------------------------------------------------------
# Start the addon container
# ---------------------------------------------------------------------------
DOCKER_ARGS=(
    "--network" "${NETWORK_NAME}"
    "-e" "SUPERVISOR_TOKEN=smoke-test-token"
)

if [ "${NEEDS_DOCKER}" = "true" ]; then
    # Create a host-side data directory for Docker-in-Docker bind mounts
    # Addons like Huly run docker-compose inside the container, which needs
    # host-side paths for volume mounts (container /data != host /data)
    SMOKE_DATA_DIR=$(mktemp -d "/tmp/smoke-test-data-XXXXXX")
    chmod 777 "${SMOKE_DATA_DIR}"
    DOCKER_ARGS+=("--privileged" "-v" "/var/run/docker.sock:/var/run/docker.sock" "-v" "${SMOKE_DATA_DIR}:/data")
fi

echo "==> Starting addon container..."
docker run -d \
    --name "${CONTAINER_NAME}" \
    "${DOCKER_ARGS[@]}" \
    "${IMAGE_NAME}" > /dev/null

# ---------------------------------------------------------------------------
# Helper: wait for a log pattern with timeout
# ---------------------------------------------------------------------------
wait_for_log() {
    local pattern="$1" label="$2" timeout="${3:-${MAX_WAIT}}"
    local waited=0
    echo "==> Waiting for: ${label} (max ${timeout}s)..."
    while [ ${waited} -lt ${timeout} ]; do
        if ! docker inspect "${CONTAINER_NAME}" --format='{{.State.Running}}' 2>/dev/null | grep -q "true"; then
            fail "Container exited while waiting for: ${label}"
        fi
        if docker logs "${CONTAINER_NAME}" 2>&1 | grep -q "${pattern}"; then
            pass "${label} (${waited}s)"
            return 0
        fi
        sleep 3
        waited=$((waited + 3))
    done
    fail "${label} — not detected within ${timeout}s"
}

# ---------------------------------------------------------------------------
# Helper: wait for HTTP health check with timeout
# ---------------------------------------------------------------------------
wait_for_health() {
    local port="$1" timeout="${2:-${MAX_WAIT}}"
    local waited=0
    echo "==> Waiting for health on port ${port} (max ${timeout}s)..."
    while [ ${waited} -lt ${timeout} ]; do
        if ! docker inspect "${CONTAINER_NAME}" --format='{{.State.Running}}' 2>/dev/null | grep -q "true"; then
            fail "Container exited while waiting for health"
        fi
        for path in "${HEALTH_PATH}" "api/health" ""; do
            if docker exec "${CONTAINER_NAME}" \
                curl -sf --max-time 3 "http://127.0.0.1:${port}/${path}" > /dev/null 2>&1; then
                pass "Health endpoint responded at :${port}/${path:-} (${waited}s)"
                return 0
            fi
        done
        sleep 3
        waited=$((waited + 3))
    done
    fail "Health endpoint on port ${port} not reachable within ${timeout}s"
}

# ---------------------------------------------------------------------------
# Addon-specific test flow
# ---------------------------------------------------------------------------
case "${SLUG}" in
    huly)
        # Phase 1: Init completes (config generation, path resolution, secrets)
        wait_for_log "Huly initialization complete" "Init completed" 120

        # Phase 2: Run script started (banner appears immediately, before image pull)
        wait_for_log "Huly stack starting" "Run script started" 30

        # Phase 3: Wait for compose containers to be running.
        # Tracks progress by counting containers via docker ps — no fixed timer.
        # Image pulls happen before docker-compose up -d; once containers appear
        # they come up quickly. Timeout is a safety net, not the expected wait.
        echo "==> Waiting for compose stack..."
        WAITED=0
        COMPOSE_TIMEOUT=600
        RUNNING=0
        while [ ${WAITED} -lt ${COMPOSE_TIMEOUT} ]; do
            if ! docker inspect "${CONTAINER_NAME}" --format='{{.State.Running}}' 2>/dev/null | grep -q "true"; then
                fail "Addon container exited while waiting for compose stack"
            fi

            RUNNING=$(docker ps --filter "label=com.docker.compose.project=huly_ha" \
                --format '{{.Names}}' 2>/dev/null | wc -l || echo 0)

            if [ "${RUNNING}" -ge 10 ]; then
                pass "Compose stack running (${RUNNING} containers, ${WAITED}s)"
                break
            fi

            # Log pull progress if images are still downloading
            if [ $((WAITED % 30)) -eq 0 ] && [ ${WAITED} -gt 0 ]; then
                PULL_STATUS=""
                if docker logs "${CONTAINER_NAME}" 2>&1 | grep -q "Pulling\|Downloading\|Extracting"; then
                    PULL_STATUS=" (images still pulling)"
                fi
                info "${RUNNING} containers running${PULL_STATUS} (${WAITED}s)"
            fi

            sleep 5
            WAITED=$((WAITED + 5))
        done
        if [ "${RUNNING}" -lt 10 ]; then
            docker ps -a --filter "label=com.docker.compose.project=huly_ha" \
                --format "table {{.Names}}\t{{.Status}}" 2>/dev/null || true
            fail "Only ${RUNNING} compose containers running (expected 10+)"
        fi

        # Phase 4: Bridge connects and forwards
        wait_for_log "Starting port bridge" "Bridge forwarding" 120

        # Phase 5: Web UI reachable end-to-end
        wait_for_health 4859 60

        # Phase 6: Verify key infrastructure services
        LOGS=$(docker logs "${CONTAINER_NAME}" 2>&1)

        # Kafka healthy
        KAFKA_HEALTHY=$(docker ps --filter "name=kafka" --filter "health=healthy" \
            --format '{{.Names}}' 2>/dev/null | head -1)
        if [ -n "${KAFKA_HEALTHY}" ]; then
            pass "Kafka healthy"
        else
            info "Kafka not yet healthy"
        fi

        # Elasticsearch healthy
        ES_HEALTHY=$(docker ps --filter "name=huly_ha-elastic" --filter "health=healthy" \
            --format '{{.Names}}' 2>/dev/null | head -1)
        if [ -n "${ES_HEALTHY}" ]; then
            pass "Elasticsearch healthy"
        else
            info "Elasticsearch not yet healthy"
        fi

        # MinIO healthy
        MINIO_HEALTHY=$(docker ps --filter "name=huly_ha-minio" --filter "health=healthy" \
            --format '{{.Names}}' 2>/dev/null | head -1)
        if [ -n "${MINIO_HEALTHY}" ]; then
            pass "MinIO healthy"
        else
            info "MinIO not yet healthy"
        fi
        ;;

    muninndb)
        wait_for_health "${HEALTH_PORT}" 120

        LOGS=$(docker logs "${CONTAINER_NAME}" 2>&1)
        if echo "${LOGS}" | grep -q "local embed provider initialized"; then
            pass "Local ONNX embedder initialized"
        elif echo "${LOGS}" | grep -q "no embedder configured"; then
            fail "Local embedder not available"
        fi

        LOGIN_CODE=$(docker exec "${CONTAINER_NAME}" curl -s -o /dev/null -w "%{http_code}" \
            -X POST http://127.0.0.1:8476/api/auth/login \
            -H 'Content-Type: application/json' \
            -d '{"username":"root","password":"password"}' 2>/dev/null)
        if [ "${LOGIN_CODE}" = "200" ]; then
            pass "Admin login works"
        else
            fail "Admin login returned HTTP ${LOGIN_CODE}"
        fi

        if echo "${LOGS}" | grep -q "MuninnDB provisioning complete"; then
            pass "Provisioning completed"
        else
            info "Provisioning may still be running"
        fi
        ;;

    arcane|dockhand)
        wait_for_health "${HEALTH_PORT}" 120
        ;;

    portainer_ee_lts|portainer_ee_sts)
        wait_for_health "${HEALTH_PORT}" 120
        ;;

    hay_cm5_fan)
        # Hardware-specific addon — no /dev/gpiochip0 on CI runners.
        # Verify the image built successfully and the init script runs
        # (it will fail at GPIO check, which is expected).
        sleep 5
        LOGS=$(docker logs "${CONTAINER_NAME}" 2>&1)
        if echo "${LOGS}" | grep -q "Initializing HAY CM5 Fan Controller"; then
            pass "Init script executed"
        else
            fail "Init script did not run"
        fi
        if echo "${LOGS}" | grep -q "libgpiod tools found"; then
            pass "libgpiod installed"
        else
            fail "libgpiod not found in image"
        fi
        if echo "${LOGS}" | grep -q "GPIO chip device.*not found"; then
            info "GPIO device not available (expected on CI)"
        fi
        if docker exec "${CONTAINER_NAME}" vcgencmd --version > /dev/null 2>&1; then
            pass "vcgencmd installed"
        else
            info "vcgencmd not testable without /dev/vcio"
        fi
        ;;

    *)
        wait_for_health "${HEALTH_PORT}" 120
        info "No addon-specific tests for '${SLUG}'"
        ;;
esac

# ---------------------------------------------------------------------------
# Test clean shutdown
# ---------------------------------------------------------------------------
echo "==> Testing shutdown..."
STOP_TIMEOUT=15
if [ "${SLUG}" = "huly" ]; then
    STOP_TIMEOUT=90
fi
docker stop -t "${STOP_TIMEOUT}" "${CONTAINER_NAME}" > /dev/null 2>&1

EXIT_CODE=$(docker inspect "${CONTAINER_NAME}" --format='{{.State.ExitCode}}' 2>/dev/null || echo "unknown")
if [ "${EXIT_CODE}" = "0" ]; then
    pass "Clean shutdown (exit code 0)"
else
    info "Exit code ${EXIT_CODE} (SIGTERM exit is normal for some addons)"
fi

echo ""
echo -e "${GREEN}==> Smoke test passed: ${SLUG}${NC}"
