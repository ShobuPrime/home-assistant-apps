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
    docker rm -f "${CONTAINER_NAME}" 2>/dev/null || true
    docker rm -f "${SUPERVISOR_NAME}" 2>/dev/null || true
    docker network rm "${NETWORK_NAME}" 2>/dev/null || true
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
HEALTH_PORT=$(echo "${WATCHDOG}" | grep -oP 'PORT:\K[0-9]+')
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

# Wait for mock supervisor to be ready (python:3-slim has no curl, use python)
sleep 2
for i in 1 2 3 4 5; do
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
    echo "--- Mock Supervisor logs ---"
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
    DOCKER_ARGS+=("--privileged" "-v" "/var/run/docker.sock:/var/run/docker.sock")
fi

echo "==> Starting addon container..."
docker run -d \
    --name "${CONTAINER_NAME}" \
    "${DOCKER_ARGS[@]}" \
    "${IMAGE_NAME}" > /dev/null

# ---------------------------------------------------------------------------
# Wait for health endpoint
# ---------------------------------------------------------------------------
# For compose-based addons (huly), the full stack won't be available in CI
# since Docker images aren't pre-pulled. Wait for the addon container's init
# to complete instead of a health endpoint.
if [ "${SLUG}" = "huly" ]; then
    echo "==> Waiting for addon init to complete (max ${MAX_WAIT}s)..."
    WAITED=0
    while [ ${WAITED} -lt ${MAX_WAIT} ]; do
        if ! docker inspect "${CONTAINER_NAME}" --format='{{.State.Running}}' 2>/dev/null | grep -q "true"; then
            fail "Container exited prematurely"
        fi
        if docker logs "${CONTAINER_NAME}" 2>&1 | grep -q "Huly initialization complete"; then
            pass "Addon init completed (${WAITED}s)"
            break
        fi
        sleep 2
        WAITED=$((WAITED + 2))
    done
    if [ ${WAITED} -ge ${MAX_WAIT} ]; then
        fail "Addon init did not complete within ${MAX_WAIT}s"
    fi
else
    echo "==> Waiting for service to be healthy (max ${MAX_WAIT}s)..."
    WAITED=0
    HEALTHY=false
    while [ ${WAITED} -lt ${MAX_WAIT} ]; do
        # Check container is still running
        if ! docker inspect "${CONTAINER_NAME}" --format='{{.State.Running}}' 2>/dev/null | grep -q "true"; then
            fail "Container exited prematurely"
        fi

        # Try health check — attempt common health paths, then fall back to TCP
        for path in "${HEALTH_PATH}" "api/health" ""; do
            if docker exec "${CONTAINER_NAME}" \
                curl -sf --max-time 3 "http://127.0.0.1:${HEALTH_PORT}/${path}" > /dev/null 2>&1; then
                pass "Health endpoint responded at /${path:-} (${WAITED}s)"
                HEALTHY=true
                break 2
            fi
        done

        sleep 2
        WAITED=$((WAITED + 2))
    done

    if [ "${HEALTHY}" != "true" ]; then
        fail "Service did not become healthy within ${MAX_WAIT}s"
    fi
fi

# ---------------------------------------------------------------------------
# Addon-specific tests
# ---------------------------------------------------------------------------
case "${SLUG}" in
    muninndb)
        # Verify local embedder initialized
        LOGS=$(docker logs "${CONTAINER_NAME}" 2>&1)
        if echo "${LOGS}" | grep -q "local embed provider initialized"; then
            pass "Local ONNX embedder initialized"
        elif echo "${LOGS}" | grep -q "no embedder configured"; then
            fail "Local embedder not available"
        fi

        # Verify admin login
        LOGIN_CODE=$(docker exec "${CONTAINER_NAME}" curl -s -o /dev/null -w "%{http_code}" \
            -X POST http://127.0.0.1:8476/api/auth/login \
            -H 'Content-Type: application/json' \
            -d '{"username":"root","password":"password"}' 2>/dev/null)
        if [ "${LOGIN_CODE}" = "200" ]; then
            pass "Admin login works"
        else
            fail "Admin login returned HTTP ${LOGIN_CODE}"
        fi

        # Verify provisioning ran
        if echo "${LOGS}" | grep -q "MuninnDB provisioning complete"; then
            pass "Provisioning completed"
        else
            info "Provisioning may still be running"
        fi
        ;;

    huly)
        # Huly is a compose orchestrator — full stack needs 14+ Docker images
        # pulled from Docker Hub which is too slow/flaky for CI smoke tests.
        # Validate the addon container itself starts, init runs, and the bridge
        # service attempts to connect. Full stack testing is manual.
        LOGS=$(docker logs "${CONTAINER_NAME}" 2>&1)

        if echo "${LOGS}" | grep -q "Huly initialization complete"; then
            pass "Init script completed"
        else
            fail "Init script did not complete"
        fi

        if echo "${LOGS}" | grep -q "Starting Huly services"; then
            pass "Compose startup initiated"
        else
            info "Compose startup not detected in logs yet"
        fi

        if echo "${LOGS}" | grep -q "Waiting for Huly compose network\|Starting port bridge"; then
            pass "Bridge service started"
        else
            info "Bridge service not detected yet (compose may still be pulling images)"
        fi

        info "Full stack validation skipped (requires 14+ Docker image pulls)"
        ;;

    arcane|dockhand)
        API_CODE=$(docker exec "${CONTAINER_NAME}" curl -s -o /dev/null -w "%{http_code}" \
            "http://127.0.0.1:${HEALTH_PORT}/${HEALTH_PATH}" 2>/dev/null)
        if [ "${API_CODE}" = "200" ]; then
            pass "API health endpoint returned 200"
        else
            info "API returned HTTP ${API_CODE} (may need configuration)"
        fi
        ;;

    portainer_ee_lts|portainer_ee_sts)
        API_CODE=$(docker exec "${CONTAINER_NAME}" curl -s -o /dev/null -w "%{http_code}" \
            "http://127.0.0.1:9000/api/status" 2>/dev/null)
        if [ "${API_CODE}" = "200" ]; then
            pass "Portainer API responded"
        else
            info "Portainer returned HTTP ${API_CODE} (initial setup may be needed)"
        fi
        ;;

    *)
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
