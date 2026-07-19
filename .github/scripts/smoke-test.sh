#!/bin/bash
# Smoke test for Home Assistant apps
# Usage: smoke-test.sh <app-directory> <image-name>
#
# Spins up a mock HA Supervisor so the app's S6 init boots normally,
# then checks health and runs app-specific validations.

set -e

APP_DIR="${1:?Usage: smoke-test.sh <app-dir> <image-name>}"
IMAGE_NAME="${2:?Usage: smoke-test.sh <app-dir> <image-name>}"
NETWORK_NAME="smoke-test-net"
SUPERVISOR_NAME="smoke-test-supervisor"
MQTT_NAME="smoke-test-mqtt"
CONTAINER_NAME="smoke-test-$(basename "${APP_DIR}")"
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

APPARMOR_PROFILE=""

cleanup() {
    # Clean up compose containers if this is a compose-based app
    if [ "${NEEDS_DOCKER}" = "true" ]; then
        docker ps -a --filter "label=com.docker.compose.project=huly_ha" \
            --format '{{.ID}}' 2>/dev/null | xargs -r docker rm -f 2>/dev/null || true
        docker network rm huly_ha_huly_net 2>/dev/null || true
    fi
    docker rm -f "${CONTAINER_NAME}" 2>/dev/null || true
    docker rm -f "${SUPERVISOR_NAME}" 2>/dev/null || true
    docker rm -f "${MQTT_NAME}" 2>/dev/null || true
    docker network rm "${NETWORK_NAME}" 2>/dev/null || true
    # Clean up host-side data directory for Docker-in-Docker apps
    [ -n "${SMOKE_DATA_DIR}" ] && rm -rf "${SMOKE_DATA_DIR}" 2>/dev/null || true
    # Unload the app's AppArmor profile if we loaded it
    if [ -n "${APPARMOR_PROFILE}" ]; then
        sudo apparmor_parser -R "${APP_DIR}/apparmor.txt" 2>/dev/null || true
    fi
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Parse app config
# ---------------------------------------------------------------------------
CONFIG="${APP_DIR}/config.yaml"
if [ ! -f "${CONFIG}" ]; then
    echo "Error: ${CONFIG} not found"
    exit 1
fi

SLUG=$(grep "^slug:" "${CONFIG}" | sed 's/slug: *"\(.*\)"/\1/')
NEEDS_DOCKER=$(grep -q "^docker_api: true" "${CONFIG}" && echo "true" || echo "false")
# Apps that declare an mqtt service (`- mqtt:want` / `- mqtt:need`) get a real
# broker so their MQTT discovery/publish path is exercised, not just REST.
NEEDS_MQTT=$(grep -qE '^[[:space:]]*-[[:space:]]*mqtt:(want|need)' "${CONFIG}" && echo "true" || echo "false")

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
docker network create "${NETWORK_NAME}" > /dev/null 2>&1

# Start a real MQTT broker for apps that declare an mqtt service, then tell the
# mock Supervisor to advertise it via /services/mqtt. Without this the app's
# MQTT auto-detection fails and it silently uses the REST fallback, leaving the
# entire discovery/publish path untested (how two hay_cm5_fan regressions
# shipped green in Jun 2026).
SUPERVISOR_ENV=()
if [ "${NEEDS_MQTT}" = "true" ]; then
    echo "==> Starting MQTT broker (app declares an mqtt service)..."
    docker run -d \
        --name "${MQTT_NAME}" \
        --network "${NETWORK_NAME}" \
        --network-alias mqtt \
        eclipse-mosquitto:2 \
        sh -c 'printf "listener 1883\nallow_anonymous true\n" > /mosquitto/config/mosquitto.conf && exec mosquitto -c /mosquitto/config/mosquitto.conf' \
        > /dev/null
    for _ in 1 2 3 4 5 6 7 8 9 10; do
        docker exec "${MQTT_NAME}" mosquitto_pub -h localhost -t smoke/ping -m up 2>/dev/null && break
        sleep 1
    done
    if docker exec "${MQTT_NAME}" mosquitto_pub -h localhost -t smoke/ping -m up 2>/dev/null; then
        pass "MQTT broker running"
    else
        docker logs "${MQTT_NAME}" 2>&1 | tail -10
        fail "MQTT broker did not start"
    fi
    SUPERVISOR_ENV=(-e "MOCK_MQTT_HOST=mqtt" -e "MOCK_MQTT_PORT=1883")
fi

echo "==> Starting mock HA Supervisor..."
docker run -d \
    --name "${SUPERVISOR_NAME}" \
    --network "${NETWORK_NAME}" \
    --network-alias supervisor \
    "${SUPERVISOR_ENV[@]}" \
    -v "${SCRIPT_DIR}/mock-supervisor.py:/mock-supervisor.py:ro" \
    -v "$(pwd)/${APP_DIR}:/app:ro" \
    python:3-slim \
    python3 /mock-supervisor.py /app 80 > /dev/null

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

# Run the app CONFINED by its real AppArmor profile when the runner supports
# it. On HAOS the Supervisor always applies apparmor.txt, so an unconfined
# smoke test can ship a profile that blocks the app in production (how the
# Jul 2026 Huly docker.sock outage stayed invisible to CI). The runner kernel
# is not HAOS's kernel, so this can't catch every kernel-specific behavior
# (validate-apparmor.sh's static rules cover the known ones) — but it does
# catch missing file/network rules for anything the app touches during boot.
if [ -f "${APP_DIR}/apparmor.txt" ] \
    && [ "$(cat /sys/module/apparmor/parameters/enabled 2>/dev/null)" = "Y" ] \
    && command -v sudo > /dev/null 2>&1; then
    APPARMOR_PROFILE=$(grep -m1 -E '^[[:space:]]*profile[[:space:]]' "${APP_DIR}/apparmor.txt" | awk '{print $2}')
    if sudo apparmor_parser -r "${APP_DIR}/apparmor.txt" 2>/dev/null; then
        pass "AppArmor profile '${APPARMOR_PROFILE}' loaded — app will run confined"
        DOCKER_ARGS+=("--security-opt" "apparmor=${APPARMOR_PROFILE}")
    else
        info "AppArmor profile failed to load on this runner — running unconfined"
        APPARMOR_PROFILE=""
    fi
else
    info "AppArmor unavailable on runner (or no apparmor.txt) — running unconfined"
fi

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
        # Hardware-specific app (aarch64-only) — on CI it runs emulated under
        # qemu (slow) and has no /dev/gpiochip0 or hwmon. The init warns (not
        # fails) on missing hardware and the daemon stays up, so we POLL for its
        # log lines with a generous timeout (a fixed short sleep is too short
        # under emulation) and confirm the container stays running.
        wait_for_log "Initializing HAY CM5 Fan Controller" "Init script executed" 90
        if docker logs "${CONTAINER_NAME}" 2>&1 | grep -q "libgpiod tools found"; then
            pass "libgpiod installed"
        else
            fail "libgpiod not found in image"
        fi
        if docker logs "${CONTAINER_NAME}" 2>&1 | grep -q "GPIO chip device.*not found"; then
            info "GPIO device not available (expected on CI)"
        fi
        # The daemon must reach its run loop and stay up despite missing hardware.
        wait_for_log "Starting HAY CM5 Fan Controller daemon" "Fan daemon started" 60
        if docker exec "${CONTAINER_NAME}" vcgencmd --version > /dev/null 2>&1; then
            pass "vcgencmd installed"
        else
            info "vcgencmd not testable without /dev/vcio"
        fi

        # --- MQTT path assertions (app declares mqtt:want) ---
        # The mock Supervisor advertises a broker, so the daemon MUST take the
        # MQTT discovery path. Both Jun-2026 regressions (discovery
        # device_class/unit, and the set -u crash in mqtt_pub) lived here and
        # were invisible to CI until this path was actually exercised.
        # Poll (don't single-shot grep): "MQTT connected" is logged a beat after
        # the daemon banner, and a timeout here correctly flags a real
        # REST-fallback regression (e.g. the Supervisor stopped advertising MQTT).
        wait_for_log "MQTT connected" "MQTT discovery path exercised (not REST fallback)" 45

        # Crash-loop detection. Under bashio strict mode a bad publish aborts the
        # daemon and S6 silently respawns it; the container stays 'Running', so a
        # liveness check can't see it. We poll for S6's crash banner (and any 2nd
        # daemon start) and fail the instant either appears. The window must
        # exceed one full emulated start->crash cycle (~20s under QEMU, since the
        # daemon enumerates every hwmon sensor before the crashing publish).
        echo "==> Watching for crash-loop (up to 50s)..."
        CRASH_LOGS=""
        for _ in 1 2 3 4 5 6 7 8 9 10; do
            CRASH_LOGS=$(docker logs "${CONTAINER_NAME}" 2>&1)
            STARTS=$(echo "${CRASH_LOGS}" | grep -c "Starting HAY CM5 Fan Controller daemon" || true)
            if echo "${CRASH_LOGS}" | grep -q "crashed with exit code" || [ "${STARTS}" -gt 1 ]; then
                echo "${CRASH_LOGS}" | sed 's/\x1b\[[0-9;]*m//g' | grep -E "unbound variable|crashed with exit|Starting HAY" | tail -8
                fail "Daemon crash-loop detected (S6 respawn) — it aborted before reaching steady state"
            fi
            sleep 5
        done
        pass "Daemon stable for 50s (single start, no respawn)"

        # Validate the retained MQTT discovery configs the daemon published:
        # every unit_of_measurement must be valid for its device_class, and the
        # degree sign must be real UTF-8 (catches the '°C' double-escape).
        echo "==> Validating retained MQTT discovery configs..."
        CONFIGS=$(docker exec "${MQTT_NAME}" mosquitto_sub -h localhost \
            -t "homeassistant/+/${SLUG}/+/config" -v -W 4 2>/dev/null || true)
        if [ -z "${CONFIGS}" ]; then
            fail "No retained discovery configs found on broker"
        fi
        CONFIG_COUNT=0
        while IFS= read -r line; do
            [ -z "${line}" ] && continue
            topic="${line%% *}"; payload="${line#* }"
            echo "${payload}" | jq -e . > /dev/null 2>&1 || fail "Invalid JSON config on ${topic}"
            dc=$(echo "${payload}" | jq -r '.device_class // ""')
            unit=$(echo "${payload}" | jq -r '.unit_of_measurement // ""')
            # device_class <-> unit consistency, the pairing HA now hard-enforces.
            # Accepted temperature units are built with printf so the real UTF-8
            # degree bytes (C2 B0) are what gets compared; a double-escaped unit
            # (the 7-char backslash-u-00b0-C string) fails here instead of
            # silently passing the way it did before this check existed.
            case "${dc}" in
                temperature)
                    if [ "${unit}" != "$(printf '\302\260C')" ] \
                    && [ "${unit}" != "$(printf '\302\260F')" ] \
                    && [ "${unit}" != "K" ]; then
                        fail "${topic}: device_class=temperature with invalid unit '${unit}'"
                    fi ;;
                frequency)
                    case "${unit}" in Hz|kHz|MHz|GHz) ;; *) fail "${topic}: device_class=frequency with invalid unit '${unit}'" ;; esac ;;
            esac
            CONFIG_COUNT=$((CONFIG_COUNT + 1))
        done <<< "${CONFIGS}"
        pass "All ${CONFIG_COUNT} discovery configs valid (unit/device_class + UTF-8 degree sign)"

        if docker inspect "${CONTAINER_NAME}" --format='{{.State.Running}}' 2>/dev/null | grep -q "true"; then
            pass "Container running (daemon stable without hardware)"
        else
            docker logs "${CONTAINER_NAME}" 2>&1 | tail -20
            fail "Container exited"
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
