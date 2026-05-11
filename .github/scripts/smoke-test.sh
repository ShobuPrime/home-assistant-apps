#!/bin/bash
# Smoke test for Home Assistant apps
# Usage: smoke-test.sh <app-directory> <image-name>
#
# Spins up a mock HA Supervisor so the app's S6 init boots normally,
# then checks health and runs app-specific validations.

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
    # Clean up compose containers if this is a compose-based app
    if [ "${NEEDS_DOCKER}" = "true" ]; then
        docker ps -a --filter "label=com.docker.compose.project=huly_ha" \
            --format '{{.ID}}' 2>/dev/null | xargs -r docker rm -f 2>/dev/null || true
        docker network rm huly_ha_huly_net 2>/dev/null || true
    fi
    docker rm -f "${CONTAINER_NAME}" 2>/dev/null || true
    docker rm -f "${SUPERVISOR_NAME}" 2>/dev/null || true
    docker network rm "${NETWORK_NAME}" 2>/dev/null || true
    # Clean up host-side data directory for Docker-in-Docker apps
    [ -n "${SMOKE_DATA_DIR}" ] && rm -rf "${SMOKE_DATA_DIR}" 2>/dev/null || true
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Parse app config
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
# Start the app container
# ---------------------------------------------------------------------------
DOCKER_ARGS=(
    "--network" "${NETWORK_NAME}"
    "-e" "SUPERVISOR_TOKEN=smoke-test-token"
)

if [ "${NEEDS_DOCKER}" = "true" ]; then
    # Create a host-side data directory for Docker-in-Docker bind mounts
    # Apps like Huly run docker-compose inside the container, which needs
    # host-side paths for volume mounts (container /data != host /data)
    SMOKE_DATA_DIR=$(mktemp -d "/tmp/smoke-test-data-XXXXXX")
    chmod 777 "${SMOKE_DATA_DIR}"
    DOCKER_ARGS+=("--privileged" "-v" "/var/run/docker.sock:/var/run/docker.sock" "-v" "${SMOKE_DATA_DIR}:/data")
fi

echo "==> Starting app container..."
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
# App-specific test flow
# ---------------------------------------------------------------------------
case "${SLUG}" in
    huly)
        # Huly is a 14-service Docker Compose stack (CockroachDB, Elasticsearch,
        # Kafka, MinIO, etc.) requiring 8+ GB RAM. CI runners have ~7 GB, so
        # services get OOM-killed before the stack is fully healthy.
        #
        # Smoke test strategy: verify the IMAGE and INIT work correctly.
        # Full stack health is an integration test, not a CI smoke test.

        # Phase 1: Init completes (config generation, path resolution, secrets)
        wait_for_log "Huly initialization complete" "Init completed" 120

        # Phase 2: Run script started
        wait_for_log "Huly stack starting" "Run script started" 30

        # Phase 3: Verify docker-compose created containers (images pulled, stack launched)
        echo "==> Waiting for compose containers to start..."
        WAITED=0
        COMPOSE_TIMEOUT=300
        RUNNING=0
        PEAK_RUNNING=0
        while [ ${WAITED} -lt ${COMPOSE_TIMEOUT} ]; do
            if ! docker inspect "${CONTAINER_NAME}" --format='{{.State.Running}}' 2>/dev/null | grep -q "true"; then
                fail "App container exited while waiting for compose stack"
            fi

            RUNNING=$(docker ps --filter "label=com.docker.compose.project=huly_ha" \
                --format '{{.Names}}' 2>/dev/null | wc -l || echo 0)
            [ "${RUNNING}" -gt "${PEAK_RUNNING}" ] && PEAK_RUNNING="${RUNNING}"

            if [ "${RUNNING}" -ge 10 ]; then
                pass "Compose stack launched (${RUNNING} containers, ${WAITED}s)"
                break
            fi

            if [ $((WAITED % 30)) -eq 0 ] && [ ${WAITED} -gt 0 ]; then
                PULL_STATUS=""
                if docker logs "${CONTAINER_NAME}" 2>&1 | grep -q "Pulling\|Downloading\|Extracting"; then
                    PULL_STATUS=" (images still pulling)"
                fi
                info "${RUNNING} containers running (peak: ${PEAK_RUNNING})${PULL_STATUS} (${WAITED}s)"
            fi

            sleep 5
            WAITED=$((WAITED + 5))
        done
        # Pass if we saw 10+ containers at any point (they may get OOM-killed on CI)
        if [ "${RUNNING}" -lt 10 ] && [ "${PEAK_RUNNING}" -ge 10 ]; then
            pass "Compose stack launched (peak: ${PEAK_RUNNING} containers, currently ${RUNNING} — OOM expected on CI)"
        elif [ "${RUNNING}" -lt 10 ]; then
            docker ps -a --filter "label=com.docker.compose.project=huly_ha" \
                --format "table {{.Names}}\t{{.Status}}" 2>/dev/null || true
            fail "Only ${PEAK_RUNNING} containers ever started (expected 10+)"
        fi

        # Phase 4: Verify key init artifacts were created
        LOGS=$(docker logs "${CONTAINER_NAME}" 2>&1)

        if echo "${LOGS}" | grep -q "Generated secrets"; then
            pass "Secrets generated"
        elif echo "${LOGS}" | grep -q "Using existing secrets"; then
            pass "Secrets loaded"
        fi

        if echo "${LOGS}" | grep -q "docker-compose.*up"; then
            pass "Docker Compose invoked"
        fi

        if echo "${LOGS}" | grep -q "Connected to compose network\|Waiting for Huly nginx"; then
            pass "Network bridge initialized"
        fi

        # Phase 5: Check infrastructure services (non-fatal — they may be OOM-killed)
        KAFKA_HEALTHY=$(docker ps --filter "name=kafka" --filter "health=healthy" \
            --format '{{.Names}}' 2>/dev/null | head -1)
        if [ -n "${KAFKA_HEALTHY}" ]; then
            pass "Kafka healthy"
        else
            info "Kafka not yet healthy (expected on CI — insufficient RAM)"
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

    sonuntius)
        # Phase 3b + Phase 4: ma-bridge, yt-cast, AND cast-receiver all
        # run as Go binaries now. We verify init, each service boots and
        # stays alive without a real network of senders (the smoke test
        # has no AirReceiver cert provisioned), and IPC round-trip works.
        sleep 8
        LOGS=$(docker logs "${CONTAINER_NAME}" 2>&1)
        if echo "${LOGS}" | grep -q "Sonuntius: preparing runtime environment"; then
            pass "Init script executed"
        else
            fail "Init script did not run"
        fi
        if echo "${LOGS}" | grep -q "Starting Sonuntius cast-receiver"; then
            pass "cast-receiver S6 service started"
        else
            fail "cast-receiver S6 service did not start"
        fi
        if echo "${LOGS}" | grep -q "cast-receiver: starting"; then
            pass "cast-receiver Go binary started"
        else
            fail "cast-receiver Go binary did not start"
        fi
        # CI does not ship an AirReceiver cert, so the binary must log
        # the no-cert path cleanly rather than crashing. We accept either
        # the cert-not-configured banner or the cont-init warning to
        # confirm the missing-cert path was exercised.
        if echo "${LOGS}" | grep -Eq "TLS server disabled \(cert not configured\)|AirReceiver cert (missing|not found)"; then
            pass "cast-receiver handled missing AirReceiver cert gracefully"
        else
            fail "cast-receiver did not log the no-cert path"
        fi
        if echo "${LOGS}" | grep -q "Starting Sonuntius yt-cast"; then
            pass "yt-cast S6 service started"
        else
            fail "yt-cast S6 service did not start"
        fi
        if echo "${LOGS}" | grep -q "yt-cast: starting (yt-cast-receiver port"; then
            pass "yt-cast Go binary started"
        else
            fail "yt-cast Go binary did not start"
        fi
        # Verify the upstream commit pin is alive in the binary's banner.
        if echo "${LOGS}" | grep -q "yt-cast-receiver port @ 83d61fa"; then
            pass "yt-cast banner reports pinned upstream commit"
        else
            fail "yt-cast banner missing pinned upstream commit hash"
        fi
        if echo "${LOGS}" | grep -q "Starting Sonuntius ma-bridge"; then
            pass "ma-bridge service started"
        else
            fail "ma-bridge service did not start"
        fi
        if echo "${LOGS}" | grep -q "ma-bridge online"; then
            pass "ma-bridge reached online state"
        else
            fail "ma-bridge never reached online state"
        fi
        if docker exec "${CONTAINER_NAME}" test -S /run/sonuntius/events.sock; then
            pass "IPC socket exists"
        else
            fail "IPC socket missing at /run/sonuntius/events.sock"
        fi
        # Phase 6b — health endpoint hosted by ma-bridge on 127.0.0.1:8099.
        HEALTH_BODY=$(docker exec "${CONTAINER_NAME}" \
            curl -sf --max-time 3 http://127.0.0.1:8099/health 2>/dev/null || true)
        if [ -n "${HEALTH_BODY}" ] && echo "${HEALTH_BODY}" | grep -q '"components"'; then
            pass "health endpoint responding on :8099/health"
        else
            fail "health endpoint not reachable or malformed at :8099/health"
        fi
        # The CI environment does not configure ma_player_id, so the
        # dispatcher component should report degraded — that confirms
        # status aggregation actually works rather than blindly returning
        # "ok" for everything.
        if echo "${HEALTH_BODY}" | grep -q '"status": "degraded"'; then
            pass "health endpoint correctly aggregates degraded components"
        else
            info "health endpoint reported ok (dispatcher may be configured)"
        fi
        if docker exec "${CONTAINER_NAME}" /usr/local/bin/sonuntius-ctl play \
                --provider ytmusic --track-id smoketest >/dev/null 2>&1; then
            pass "sonuntius-ctl successfully sent PlayIntent"
        else
            fail "sonuntius-ctl could not send PlayIntent"
        fi
        sleep 2
        DISPATCH_LOGS=$(docker logs "${CONTAINER_NAME}" 2>&1)
        if echo "${DISPATCH_LOGS}" | grep -q "ha: play_media"; then
            pass "dispatcher invoked play_media on HA REST"
        else
            info "play_media call not logged — dispatcher may be idle (ma_player_id unset)"
        fi
        # Every receiver must remain alive after 10s even without
        # senders on the network. A premature exit would mean S6 had to
        # restart it, which would manifest as repeated "Starting ..."
        # lines for that service.
        YT_STARTS=$(echo "${DISPATCH_LOGS}" | grep -c "Starting Sonuntius yt-cast" || echo 0)
        if [ "${YT_STARTS}" -le 1 ]; then
            pass "yt-cast service stable (no S6 restart loop)"
        else
            fail "yt-cast service restarted ${YT_STARTS} times — crash loop"
        fi
        CAST_STARTS=$(echo "${DISPATCH_LOGS}" | grep -c "Starting Sonuntius cast-receiver" || echo 0)
        if [ "${CAST_STARTS}" -le 1 ]; then
            pass "cast-receiver service stable (no S6 restart loop)"
        else
            fail "cast-receiver service restarted ${CAST_STARTS} times — crash loop"
        fi
        # Phase 5 — Tidal Connect binary fallback (opt-in). CI runs with
        # tidal_fallback.enabled = false, so the cont-init step must skip
        # extraction and the two new services must log "idle" and stay
        # asleep instead of crash-looping looking for a missing binary.
        if echo "${DISPATCH_LOGS}" | grep -q "tidal_fallback.enabled = false — skipping iFi"; then
            pass "Phase 5 cont-init correctly skipped (fallback disabled)"
        else
            fail "Phase 5 cont-init did not log the disabled path"
        fi
        if echo "${DISPATCH_LOGS}" | grep -q "tidal-connect: tidal_fallback.enabled = false"; then
            pass "tidal-connect service idle (fallback disabled)"
        else
            fail "tidal-connect service did not log the disabled path"
        fi
        if echo "${DISPATCH_LOGS}" | grep -q "alsa-to-sendspin: tidal_fallback.enabled = false"; then
            pass "alsa-to-sendspin service idle (fallback disabled)"
        else
            fail "alsa-to-sendspin service did not log the disabled path"
        fi
        # grep -c prints "0" on no-match AND exits 1, so `|| echo 0`
        # would double-up to "0\n0". Use `|| true` instead — grep's own
        # "0" is the count we want.
        TIDAL_STARTS=$(echo "${DISPATCH_LOGS}" | grep -c "Starting Sonuntius tidal-connect" || true)
        ALSA_STARTS=$(echo "${DISPATCH_LOGS}" | grep -c "Starting Sonuntius alsa-to-sendspin" || true)
        if [ "${TIDAL_STARTS}" -eq 0 ] && [ "${ALSA_STARTS}" -eq 0 ]; then
            pass "Phase 5 services never attempted exec (correct disabled-state behavior)"
        else
            fail "Phase 5 services attempted exec while disabled (tidal=${TIDAL_STARTS}, alsa=${ALSA_STARTS})"
        fi
        if docker inspect "${CONTAINER_NAME}" --format='{{.State.Running}}' 2>/dev/null | grep -q "true"; then
            pass "Container still running"
        else
            fail "Container exited"
        fi
        ;;

    hay_cm5_fan)
        # Hardware-specific app — no /dev/gpiochip0 on CI runners.
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
        info "No app-specific tests for '${SLUG}'"
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
    info "Exit code ${EXIT_CODE} (SIGTERM exit is normal for some apps)"
fi

echo ""
echo -e "${GREEN}==> Smoke test passed: ${SLUG}${NC}"
