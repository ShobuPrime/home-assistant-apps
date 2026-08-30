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
# Scratch container the Portainer apps start/stop through their Docker socket
# shim. Created inside the test, removed by cleanup() either way.
SHIM_PROBE="smoke-test-shim-probe"
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
    docker rm -f "${SHIM_PROBE}" "${SHIM_PROBE}-big" 2>/dev/null || true
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
        mirror.gcr.io/library/eclipse-mosquitto:2 \
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
    mirror.gcr.io/library/python:3-slim \
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

    lemonade)
        # Lemonade downloads its llama.cpp backend and the default model on
        # first start, so this test does real network I/O and needs headroom.
        wait_for_health "${HEALTH_PORT}" 120
        wait_for_log "Lemonade provisioning complete" "Model provisioning finished" 420

        LOGS=$(docker logs "${CONTAINER_NAME}" 2>&1)

        # The bridge is always in the path now: it is the origin gate, not just
        # the memory proxy. See lemonade/CLAUDE.md.
        if docker exec "${CONTAINER_NAME}" test -d /etc/services.d/lemonade-bridge; then
            pass "Bridge service present and supervised (it is the origin gate, not just the memory proxy)"
        else
            fail "lemonade-bridge service dir missing — nothing would enforce browser origins"
        fi

        # lemond on the public port would be reachable without the gate — and it
        # runs with a wildcard allowlist, so it would accept every origin.
        if docker exec "${CONTAINER_NAME}" grep -q "^LEMOND_PORT=13306$" /run/lemonade/runtime.env \
            && docker exec "${CONTAINER_NAME}" grep -q "^LEMOND_BIND_HOST=127.0.0.1$" /run/lemonade/runtime.env; then
            pass "lemond bound to loopback 13306, behind the bridge"
        else
            docker exec "${CONTAINER_NAME}" cat /run/lemonade/runtime.env 2>&1 | sed 's/^/    /'
            fail "lemond is not on loopback 13306 — it would be reachable without the origin gate"
        fi

        # https://ha.example.com is the mock's external_url and is NOT in the
        # static list, so allowing it proves the runtime fetch happened. Testing
        # only that a bad origin 403s would pass with the gate hard-wired shut.
        ORIGIN_WAIT=0
        while [ "${ORIGIN_WAIT}" -lt 30 ]; do
            docker logs "${CONTAINER_NAME}" 2>&1 | grep -q "origins: allowlist now" && break
            ORIGIN_WAIT=$((ORIGIN_WAIT + 2)); sleep 2
        done

        HA_ORIGIN_CODE=$(docker exec "${CONTAINER_NAME}" curl -s -o /dev/null -w "%{http_code}" \
            --max-time 15 -H "Origin: https://ha.example.com" \
            "http://127.0.0.1:${HEALTH_PORT}/api/v1/health" 2>/dev/null || echo "000")
        if [ "${HA_ORIGIN_CODE}" = "200" ]; then
            pass "Home Assistant's own origin allowed — learned from Core at runtime, not at container start"
        else
            docker logs "${CONTAINER_NAME}" 2>&1 | grep -i "origins:" | tail -5 | sed 's/^/    /'
            fail "Home Assistant's external_url got HTTP ${HA_ORIGIN_CODE}, want 200 — the ingress panel would 403 on every action"
        fi

        # Home Assistant is commonly served over TLS, so reaching it directly at
        # https://<ip>:8123 makes the panel carry THAT origin. Emitting only the
        # http:// form 403'd every direct-IP user while the configured URL worked.
        HOST_IP=$(docker exec "${CONTAINER_NAME}" sh -c \
            'curl -sf -H "Authorization: Bearer $SUPERVISOR_TOKEN" http://supervisor/network/info \
             | sed -n "s/.*\"address\": *\[\"\\([0-9.]*\\)\\/.*/\\1/p" | head -1' 2>/dev/null || echo "")
        if [ -n "${HOST_IP}" ]; then
            for SCHEME in http https; do
                CODE=$(docker exec "${CONTAINER_NAME}" curl -s -o /dev/null -w "%{http_code}" \
                    --max-time 15 -H "Origin: ${SCHEME}://${HOST_IP}:8123" \
                    "http://127.0.0.1:${HEALTH_PORT}/api/v1/health" 2>/dev/null || echo "000")
                if [ "${CODE}" = "200" ]; then
                    pass "Direct host access allowed over ${SCHEME} (${SCHEME}://${HOST_IP}:8123)"
                else
                    fail "${SCHEME}://${HOST_IP}:8123 got HTTP ${CODE}, want 200 — reaching Home Assistant by IP over ${SCHEME} would 403"
                fi
            done
        else
            fail "Could not read a host address from the mock Supervisor"
        fi

        EVIL_CODE=$(docker exec "${CONTAINER_NAME}" curl -s -o /tmp/evil.json -w "%{http_code}" \
            --max-time 15 -X POST -H "Origin: https://evil.example" \
            "http://127.0.0.1:${HEALTH_PORT}/api/v1/unload" 2>/dev/null || echo "000")
        if [ "${EVIL_CODE}" = "403" ] \
            && docker exec "${CONTAINER_NAME}" grep -q "Origin not allowed" /tmp/evil.json 2>/dev/null; then
            pass "Unknown origin refused with lemond's exact 403 (DNS-rebinding protection intact)"
        else
            fail "Unknown origin got HTTP ${EVIL_CODE}, want 403 — the API is open to any web page you visit"
        fi

        # HA's Ollama integration and curl send no Origin; gating them would
        # break the integration while protecting nothing.
        NO_ORIGIN_CODE=$(docker exec "${CONTAINER_NAME}" curl -s -o /dev/null -w "%{http_code}" \
            --max-time 15 "http://127.0.0.1:${HEALTH_PORT}/api/v1/health" 2>/dev/null || echo "000")
        if [ "${NO_ORIGIN_CODE}" = "200" ]; then
            pass "Requests with no Origin pass (Home Assistant's own API client sends none)"
        else
            fail "A request with no Origin got HTTP ${NO_ORIGIN_CODE}, want 200 — the Ollama integration would break"
        fi

        # Wildcard and loopback must hold together; either alone is a hole.
        if docker exec "${CONTAINER_NAME}" sh -c \
            'tr "\0" "\n" < /proc/$(pgrep -f "^/opt/lemonade/lemond" | head -1)/environ | grep -q "^LEMONADE_ALLOWED_ORIGINS=\*$"'; then
            pass "lemond runs with a wildcard allowlist, reachable only through the gate"
        else
            fail "lemond's LEMONADE_ALLOWED_ORIGINS is not '*' — the bridge and lemond would both enforce, and CORS headers would be dropped for allowed cross-origin clients"
        fi

        # The Web UI is what ingress serves, and it ships separately from the
        # binaries — the embeddable archive has no UI at all. A missing UI is
        # invisible from the API side: the server starts, answers every API
        # call, and only logs `Could not open index.html` when someone actually
        # opens the panel. Assert it here so that can't ship again.
        UI_CODE=$(docker exec "${CONTAINER_NAME}" \
            curl -s -o /tmp/ui.html -w "%{http_code}" --max-time 10 "http://127.0.0.1:${HEALTH_PORT}/" 2>/dev/null || echo "000")
        if [ "${UI_CODE}" = "200" ] && docker exec "${CONTAINER_NAME}" grep -qi "<html" /tmp/ui.html 2>/dev/null; then
            pass "Web UI served at / (HTTP 200, HTML)"
        else
            docker logs "${CONTAINER_NAME}" 2>&1 | grep -i "index.html" | tail -2
            fail "Web UI not served at / (HTTP ${UI_CODE}) — ingress would 404. The UI assets come from the 'webui' Dockerfile stage, not the embeddable archive."
        fi

        # ...and the check above is not sufficient on its own. `/` is only the
        # "Lemonade Server" landing page, which lives in resources/static/. The
        # actual chat application is a SEPARATE directory, resources/web-app/,
        # served at /web-app/ — and for the whole of 11.5.0 it was never copied
        # into the image. Every signal looked healthy: / returned 200 HTML, no
        # index.html error was ever logged, and nothing on the landing page
        # links to /web-app/, so the panel showed a real-looking page with the
        # application silently absent behind it. `ingress_entry: /web-app` now
        # points the panel here, so a 404 is a broken panel.
        APP_CODE=$(docker exec "${CONTAINER_NAME}" \
            curl -s -o /tmp/webapp.html -w "%{http_code}" --max-time 10 "http://127.0.0.1:${HEALTH_PORT}/web-app/" 2>/dev/null || echo "000")
        if [ "${APP_CODE}" = "200" ] && docker exec "${CONTAINER_NAME}" grep -qi "renderer.bundle.js" /tmp/webapp.html 2>/dev/null; then
            pass "Chat application served at /web-app/ (HTTP 200, bundle referenced)"
        else
            fail "Chat application not served at /web-app/ (HTTP ${APP_CODE}) — ingress_entry points here, so the panel would 404. Copy resources/web-app/ from the 'webui' stage, not just resources/static/."
        fi

        # The bundle itself must resolve, or the panel renders an empty page
        # with a 404 in the browser console and no server-side symptom at all.
        BUNDLE_CODE=$(docker exec "${CONTAINER_NAME}" \
            curl -s -o /dev/null -w "%{http_code}" --max-time 15 "http://127.0.0.1:${HEALTH_PORT}/web-app/renderer.bundle.js" 2>/dev/null || echo "000")
        if [ "${BUNDLE_CODE}" = "200" ]; then
            pass "Chat application bundle served (/web-app/renderer.bundle.js)"
        else
            fail "Chat application bundle 404s (HTTP ${BUNDLE_CODE}) — the panel would load an empty page."
        fi

        # Speech-to-text is off by default, so the Wyoming port must NOT be
        # listening — an add-on that advertises a speech engine it was not
        # asked to run would put a dead entry in Home Assistant's dropdown.
        # cont-init's line states the add-on's configuration; the bridge's
        # states its own. Either works now, but this one is the intent.
        if echo "${LOGS}" | grep -q "speech-to-text both off"; then
            pass "Speech-to-text off by default"
        else
            fail "Expected speech-to-text disabled by default"
        fi
        if docker exec "${CONTAINER_NAME}" sh -c \
            'curl -s -o /dev/null --max-time 3 telnet://127.0.0.1:10600' 2>/dev/null; then
            fail "Wyoming port 10600 is listening even though speech-to-text is disabled"
        else
            pass "Wyoming port closed while speech-to-text is disabled"
        fi

        # config.yaml must declare Wyoming discovery, or Home Assistant never
        # learns the engine exists no matter what the bridge serves.
        if grep -qE '^\s*-\s*wyoming\s*$' "${APP_DIR}/config.yaml"; then
            pass "config.yaml declares Wyoming discovery"
        else
            fail "config.yaml is missing 'discovery: - wyoming' — HA cannot auto-detect the speech engine"
        fi

        # glibc compat stubs. lemond does not reference these (glibc 2.34
        # merged them into libc), so they never enter the ldd closure — but the
        # backends Lemonade downloads at RUNTIME do: moonshine-server needs
        # libdl+libpthread, libonnxruntime needs libdl+libpthread+librt. Their
        # absence presents as exit 127 and "failed to start or become ready",
        # which names nothing useful. Assert them here because no build-time
        # check can see a binary that is fetched later.
        MISSING_STUBS=""
        for stub in libdl.so.2 libpthread.so.0 librt.so.1; do
            docker exec "${CONTAINER_NAME}" test -e "/opt/glibc/lib/${stub}" 2>/dev/null \
                || MISSING_STUBS="${MISSING_STUBS} ${stub}"
        done
        if [ -z "${MISSING_STUBS}" ]; then
            pass "glibc compat stubs bundled (Moonshine/ONNX backends can start)"
        else
            fail "Missing glibc compat stubs:${MISSING_STUBS} — runtime-downloaded backends would exit 127."
        fi

        # The selected llama.cpp build must reach config.json under the
        # backend-specific key lemond actually reads (`<backend>_bin`), not a
        # generic one — a wrong key name is silently ignored and the app just
        # keeps running the builtin build.
        if docker exec "${CONTAINER_NAME}" sh -c \
            'jq -e ".llamacpp.cpu_bin == \"builtin\"" /data/lemonade/.config/lemonade/config.json' >/dev/null 2>&1; then
            pass "llamacpp_bin written to config.json as cpu_bin"
        else
            docker exec "${CONTAINER_NAME}" sh -c 'jq ".llamacpp" /data/lemonade/.config/lemonade/config.json' 2>/dev/null | head -8
            fail "llamacpp_bin did not reach config.json as .llamacpp.cpu_bin"
        fi

        # lemond 11.8.0 moved its config dir to .config/lemonade and reads the
        # legacy .cache path only once, to migrate it. A config.json sitting
        # there after boot means something is still writing options where
        # lemond no longer looks — they would apply once, then never again.
        if docker exec "${CONTAINER_NAME}" test -f /data/lemonade/.cache/lemonade/config.json 2>/dev/null; then
            fail "config.json present at legacy .cache/lemonade path — options are being written where lemond 11.8.0+ does not read"
        else
            pass "No config.json at the legacy .cache/lemonade path"
        fi

        # The two patches applied to upstream's web app in the Dockerfile. The
        # build already fails loudly if an anchor stops matching, but that only
        # covers the patch being APPLIED — these check it is actually SERVED,
        # which is what the browser depends on.
        SHIM_CODE=$(docker exec "${CONTAINER_NAME}" \
            curl -s -o /tmp/shim.js -w "%{http_code}" --max-time 10 "http://127.0.0.1:${HEALTH_PORT}/web-app/ha-addon-shim.js" 2>/dev/null || echo "000")
        if [ "${SHIM_CODE}" = "200" ] && docker exec "${CONTAINER_NAME}" grep -q "__haLemonadeBase" /tmp/shim.js 2>/dev/null; then
            pass "Add-on web shim served (/web-app/ha-addon-shim.js)"
        else
            fail "Add-on web shim not served (HTTP ${SHIM_CODE}) — the ingress panel would sit on 'connecting' and Android would redirect to the Play Store."
        fi

        # Order matters: the shim must load BEFORE the deferred bundle, or the
        # bundle reads its API base before the shim has published one.
        if docker exec "${CONTAINER_NAME}" sh -c \
            'curl -s --max-time 10 "http://127.0.0.1:'"${HEALTH_PORT}"'/web-app/" | grep -q "ha-addon-shim\.js\"></script><script defer"' 2>/dev/null; then
            pass "Shim loads before the deferred bundle in web-app/index.html"
        else
            fail "Shim is not ordered before renderer.bundle.js — the API base would be read before the shim sets it."
        fi

        # The bundle must prefer the shim's base over window.location.origin,
        # which is what makes API calls and the log WebSocket resolve under the
        # ingress path prefix instead of hitting the Home Assistant root.
        if docker exec "${CONTAINER_NAME}" grep -q "__haLemonadeBase" \
            /opt/lemonade/resources/web-app/renderer.bundle.js 2>/dev/null; then
            pass "Bundle patched to honour the ingress-aware API base URL"
        else
            fail "renderer.bundle.js is unpatched — it would use window.location.origin and never reach the add-on through ingress."
        fi

        # The bridge derives the list at runtime; assert it logged one.
        if docker logs "${CONTAINER_NAME}" 2>&1 | grep -q "origins: allowlist now"; then
            pass "Origin allowlist derived and logged"
        else
            fail "No origin allowlist was derived — the Web UI would return 403 {\"error\": \"Origin not allowed\"} on chat."
        fi

        # The regression this test exists for: Lemonade's binaries are glibc
        # builds running on Alpine via a bundled glibc closure, and the
        # llama.cpp backend is fetched at RUNTIME so its dependencies can't be
        # checked at build time. A missing library shows up here — and ONLY
        # here — as the backend dying instantly while lemond looks healthy.
        if echo "${LOGS}" | grep -q "error while loading shared libraries"; then
            echo "${LOGS}" | grep -m3 "error while loading shared libraries"
            fail "Backend binary is missing a shared library — the bundled glibc closure in the Dockerfile needs updating"
        fi
        pass "No missing-shared-library errors from the runtime-downloaded backend"

        if echo "${LOGS}" | grep -q "loaded and ready"; then
            pass "Model downloaded and loaded"

            # Ollama API — this is the surface Home Assistant's built-in
            # Ollama integration talks to, so a break here breaks the whole
            # point of the app.
            TAGS=$(docker exec "${CONTAINER_NAME}" \
                curl -sf --max-time 10 "http://127.0.0.1:${HEALTH_PORT}/api/tags" 2>/dev/null || true)
            if echo "${TAGS}" | grep -q '"name"'; then
                pass "Ollama /api/tags lists models: $(echo "${TAGS}" | jq -r '[.models[].name] | join(", ")' 2>/dev/null)"
            else
                fail "Ollama /api/tags returned no models"
            fi

            # End-to-end inference through the OpenAI-compatible API
            COMPLETION=$(docker exec "${CONTAINER_NAME}" \
                curl -sf --max-time 120 "http://127.0.0.1:${HEALTH_PORT}/api/v1/chat/completions" \
                -H 'Content-Type: application/json' \
                -d '{"model":"user.LFM2.5-230M","messages":[{"role":"user","content":"Say OK"}],"max_tokens":16}' \
                2>/dev/null || true)
            if echo "${COMPLETION}" | jq -e '.choices[0].message.content' > /dev/null 2>&1; then
                pass "Chat completion returned: $(echo "${COMPLETION}" | jq -r '.choices[0].message.content' | head -c 60)"
            else
                echo "${COMPLETION}" | head -c 400
                fail "Chat completion did not return a message"
            fi
        else
            # Distinguish "our packaging is broken" (hard failure above) from
            # "Hugging Face was unreachable on this runner" (not our bug).
            info "Model did not finish downloading — likely a transient upstream/network issue, not a packaging fault"
        fi
        ;;

    arcane|dockhand)
        wait_for_health "${HEALTH_PORT}" 120
        ;;

    portainer_ee_lts|portainer_ee_sts)
        wait_for_health "${HEALTH_PORT}" 120

        # --- Docker socket shim ---
        # dockerd rejects any POST /containers/{id}/start whose body length is
        # unknown, which is exactly what Portainer's Docker-API proxy sends
        # through Home Assistant's streaming ingress. The error names no
        # container ("starting container with non-empty request body was
        # deprecated since API v1.22 and removed in v1.24") because the check
        # runs before dockerd looks at one — so it also masks whatever the real
        # start failure was. The app proxies Portainer through nginx to strip
        # those bodies. Everything below is the contract that fix depends on.
        SHIM_SOCK="/run/docker-shim/docker.sock"

        if docker exec "${CONTAINER_NAME}" test -S "${SHIM_SOCK}"; then
            pass "Shim socket present at ${SHIM_SOCK}"
        else
            docker logs "${CONTAINER_NAME}" 2>&1 | grep -i "shim\|nginx" | tail -5
            fail "Shim socket missing — nginx did not start, so Portainer is talking to dockerd directly"
        fi

        # Portainer's environment address lives in its database and --host only
        # seeds it on first init, so the fix reaches existing installs only by
        # making the address they already have resolve to the shim.
        SHIM_LINK=$(docker exec "${CONTAINER_NAME}" readlink /var/run/docker.sock 2>/dev/null || true)
        if [ "${SHIM_LINK}" = "${SHIM_SOCK}" ]; then
            pass "/var/run/docker.sock redirected to the shim (existing installs need no migration)"
        else
            fail "/var/run/docker.sock points at '${SHIM_LINK:-nothing}', not the shim — Portainer would bypass it"
        fi

        # A directory the shim socket can hide in: nginx chmods its listen
        # socket 0666 unconditionally, and the real socket is 0660.
        SHIM_DIR_MODE=$(docker exec "${CONTAINER_NAME}" stat -c '%a' /run/docker-shim 2>/dev/null || true)
        if [ "${SHIM_DIR_MODE}" = "700" ]; then
            pass "Shim socket directory is 0700 (Docker API not world-reachable in-container)"
        else
            fail "Shim socket directory is 0${SHIM_DIR_MODE:-???}, expected 0700"
        fi

        # The reproducer, against the raw socket. If this stops being a 400 the
        # shim is no longer load-bearing and this whole section can go — so
        # report it rather than asserting it.
        SHIM_CURL="curl -s -o /dev/null -w %{http_code} -X POST -H 'Content-Type: application/json' -H 'Transfer-Encoding: chunked' -d '{}'"
        RAW_CODE=$(docker exec "${CONTAINER_NAME}" sh -c \
            "${SHIM_CURL} --unix-socket /run/docker.sock http://localhost/v1.44/containers/${CONTAINER_NAME}/start" 2>/dev/null || echo "000")
        if [ "${RAW_CODE}" = "400" ]; then
            pass "Bug reproduced on the raw socket (HTTP 400) — the shim is still needed"
        else
            info "Raw socket returned HTTP ${RAW_CODE}, not 400 — this dockerd may no longer reject unknown-length start bodies"
        fi

        # ...and the same request through the shim must reach the real handler.
        # 304 = "already started", i.e. dockerd got far enough to look at the
        # container. Both the versioned and bare forms, because the location
        # regex has to match with and without the /v1.NN/ prefix.
        for API_PREFIX in "/v1.44" ""; do
            SHIM_CODE=$(docker exec "${CONTAINER_NAME}" sh -c \
                "${SHIM_CURL} --unix-socket ${SHIM_SOCK} http://localhost${API_PREFIX}/containers/${CONTAINER_NAME}/start" 2>/dev/null || echo "000")
            if [ "${SHIM_CODE}" = "304" ]; then
                pass "Chunked-body start via shim reached dockerd (HTTP 304, path '${API_PREFIX:-/}')"
            else
                fail "Chunked-body start via shim returned HTTP ${SHIM_CODE} for path '${API_PREFIX:-/}' (want 304). The body strip is not working."
            fi
        done

        # 304 only proves the request got past the body check. This proves the
        # button in the UI works: a genuinely stopped container has to start.
        docker exec "${CONTAINER_NAME}" sh -c \
            "curl -s -o /dev/null -X POST -H 'Content-Type: application/json' \
             -d '{\"Image\":\"${IMAGE_NAME}\",\"Entrypoint\":[\"/bin/sleep\"],\"Cmd\":[\"600\"]}' \
             --unix-socket ${SHIM_SOCK} 'http://localhost/containers/create?name=${SHIM_PROBE}'" 2>/dev/null || true
        START_CODE=$(docker exec "${CONTAINER_NAME}" sh -c \
            "${SHIM_CURL} --unix-socket ${SHIM_SOCK} http://localhost/containers/${SHIM_PROBE}/start" 2>/dev/null || echo "000")
        RUNNING=$(docker inspect -f '{{.State.Running}}' "${SHIM_PROBE}" 2>/dev/null || echo "missing")
        if [ "${START_CODE}" = "204" ] && [ "${RUNNING}" = "true" ]; then
            pass "A stopped container really starts through the shim (HTTP 204, running)"
        else
            fail "Stopped container did not start through the shim (HTTP ${START_CODE}, running=${RUNNING})"
        fi

        # The other half of the contract: the shim must strip bodies ONLY where
        # a body is illegal. /exec/{id}/start carries a real one ({"Detach":…,
        # "Tty":…}) and is hijacked mid-response, so this covers both "the body
        # survived" and "the upgrade tunnel works" — a broken console is how
        # this would otherwise ship unnoticed.
        EXEC_ID=$(docker exec "${CONTAINER_NAME}" sh -c \
            "curl -s -X POST -H 'Content-Type: application/json' \
             -d '{\"AttachStdout\":true,\"Cmd\":[\"/bin/echo\",\"shim-exec-ok\"]}' \
             --unix-socket ${SHIM_SOCK} http://localhost/containers/${SHIM_PROBE}/exec" 2>/dev/null \
            | sed 's/.*"Id":"\([^"]*\)".*/\1/')
        EXEC_OUT=$(docker exec "${CONTAINER_NAME}" sh -c \
            "curl -s -X POST -H 'Content-Type: application/json' -H 'Connection: Upgrade' -H 'Upgrade: tcp' \
             -d '{\"Detach\":false,\"Tty\":false}' \
             --unix-socket ${SHIM_SOCK} http://localhost/exec/${EXEC_ID}/start" 2>/dev/null | tr -dc '[:print:]')
        if echo "${EXEC_OUT}" | grep -q "shim-exec-ok"; then
            pass "Hijacked exec streams through the shim with its request body intact"
        else
            fail "Exec through the shim produced '${EXEC_OUT}' — the body was stripped or the upgrade tunnel is broken"
        fi

        # Stop last, so the probe is still running for the exec above.
        STOP_CODE=$(docker exec "${CONTAINER_NAME}" sh -c \
            "${SHIM_CURL} --unix-socket ${SHIM_SOCK} 'http://localhost/containers/${SHIM_PROBE}/stop?t=1'" 2>/dev/null || echo "000")
        STILL_RUNNING=$(docker inspect -f '{{.State.Running}}' "${SHIM_PROBE}" 2>/dev/null || echo "missing")
        if [ "${STOP_CODE}" = "204" ] && [ "${STILL_RUNNING}" = "false" ]; then
            pass "Stop verb also survives the body strip (HTTP 204, stopped)"
        else
            fail "Stop through the shim returned HTTP ${STOP_CODE}, running=${STILL_RUNNING} (want 204/false)"
        fi

        # Alpine's stock nginx.conf caps bodies at 1m, which would 413 image
        # builds and large container creates. The shim runs its own config with
        # no cap; this is what proves it.
        BIG_CODE=$(docker exec "${CONTAINER_NAME}" sh -c \
            "{ printf '{\"Image\":\"${IMAGE_NAME}\",\"Entrypoint\":[\"/bin/true\"],\"Labels\":{\"big\":\"'; \
               head -c 2000000 /dev/zero | tr '\\0' 'x'; printf '\"}}'; } > /tmp/big.json; \
             curl -s -o /dev/null -w %{http_code} -X POST -H 'Content-Type: application/json' \
             --data-binary @/tmp/big.json --unix-socket ${SHIM_SOCK} \
             'http://localhost/containers/create?name=${SHIM_PROBE}-big'" 2>/dev/null || echo "000")
        if [ "${BIG_CODE}" = "201" ]; then
            pass "2 MB request body passes the shim (no 1m cap)"
        else
            fail "2 MB create through the shim returned HTTP ${BIG_CODE} (want 201) — nginx is capping request bodies"
        fi

        # Passthrough must be byte-exact, not merely successful.
        RAW_LOGS=$(docker exec "${CONTAINER_NAME}" sh -c \
            "curl -s --unix-socket /run/docker.sock 'http://localhost/containers/${SUPERVISOR_NAME}/logs?stdout=1&stderr=1&tail=20' | md5sum" 2>/dev/null || true)
        SHIM_LOGS=$(docker exec "${CONTAINER_NAME}" sh -c \
            "curl -s --unix-socket ${SHIM_SOCK} 'http://localhost/containers/${SUPERVISOR_NAME}/logs?stdout=1&stderr=1&tail=20' | md5sum" 2>/dev/null || true)
        if [ -n "${RAW_LOGS}" ] && [ "${RAW_LOGS}" = "${SHIM_LOGS}" ]; then
            pass "Log stream is byte-identical through the shim"
        else
            fail "Log stream differs through the shim (raw ${RAW_LOGS:-none} vs shim ${SHIM_LOGS:-none})"
        fi

        if docker logs "${CONTAINER_NAME}" 2>&1 | grep -qE "nginx.*\[(error|alert|emerg)\]"; then
            docker logs "${CONTAINER_NAME}" 2>&1 | grep -E "nginx.*\[(error|alert|emerg)\]" | tail -5
            fail "nginx logged errors while proxying the Docker API"
        fi
        pass "Shim proxied every request without logging an error"

        # A shim that cannot come back is worse than no shim. nginx unlinks its
        # listen socket only on a CLEAN exit, so a SIGKILL/OOM/unclean container
        # stop leaves the path behind and every S6 respawn dies with
        # "bind() ... (98: Address in use)" — permanently, because /run here is
        # the image layer rather than a tmpfs, so it survives a restart too.
        # And it is invisible: Portainer keeps answering on :9000, so both the
        # HEALTHCHECK and the watchdog stay green while Portainer has no Docker
        # access whatsoever. Measured before the fix: nginx gone, _ping through
        # /var/run/docker.sock returning 000, container still reported healthy.
        docker exec "${CONTAINER_NAME}" sh -c 'kill -9 $(pgrep nginx) 2>/dev/null; true' > /dev/null 2>&1 || true
        SHIM_RECOVERED=""
        for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
            sleep 2
            if docker exec "${CONTAINER_NAME}" curl -sf -o /dev/null --max-time 3 \
                --unix-socket /var/run/docker.sock http://localhost/_ping > /dev/null 2>&1; then
                SHIM_RECOVERED="yes"
                break
            fi
        done
        if [ -n "${SHIM_RECOVERED}" ]; then
            pass "Shim recovers from a hard kill — Docker API reachable again"
        else
            docker logs "${CONTAINER_NAME}" 2>&1 | grep -i "bind()" | tail -3
            fail "Shim did not recover after SIGKILL. A stale listen socket is blocking every respawn, so Portainer has no Docker access — and the healthcheck and watchdog both stay green, so nothing else would ever report it."
        fi
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
        # The discovery node_id is NOT necessarily the app slug. hay_cm5_fan
        # publishes under "hay_cm5" as of 1.3.0, precisely so Home Assistant
        # derives short entity IDs. Read the namespace the app actually uses
        # instead of assuming it matches the slug — this check failed the 1.3.0
        # PR by looking under the old namespace and finding nothing.
        DISCOVERY_NS=$(grep -m1 '^DISCOVERY_NS=' \
            "${APP_DIR}/rootfs/etc/services.d/cm5-fan/run" 2>/dev/null | cut -d'"' -f2)
        [ -n "${DISCOVERY_NS}" ] || DISCOVERY_NS="${SLUG}"
        echo "    discovery namespace: ${DISCOVERY_NS}"
        CONFIGS=$(docker exec "${MQTT_NAME}" mosquitto_sub -h localhost \
            -t "homeassistant/+/${DISCOVERY_NS}/+/config" -v -W 4 2>/dev/null || true)
        if [ -z "${CONFIGS}" ]; then
            fail "No retained discovery configs found on broker under '${DISCOVERY_NS}'"
        fi

        # The pre-1.3.0 namespace must be left EMPTY: the app retires those
        # topics on start, and a leftover there means HA would keep the old
        # long-form entities alongside the new ones.
        LEGACY_NS=$(grep -m1 '^LEGACY_DISCOVERY_NS=' \
            "${APP_DIR}/rootfs/etc/services.d/cm5-fan/run" 2>/dev/null | cut -d'"' -f2)
        if [ -n "${LEGACY_NS}" ]; then
            LEFTOVER=$(docker exec "${MQTT_NAME}" mosquitto_sub -h localhost \
                -t "homeassistant/+/${LEGACY_NS}/+/config" -W 3 2>/dev/null || true)
            if [ -n "${LEFTOVER}" ]; then
                fail "Legacy discovery configs still retained under '${LEGACY_NS}' — the migration did not retire them, so HA would show duplicate entities"
            fi
            pass "Legacy discovery namespace '${LEGACY_NS}' is clear"
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

# The budget the real Supervisor applies is config.yaml's `timeout:` (default
# 10s). Read it per app rather than hardcoding, so this tracks whatever each
# app declares.
CONFIG_TIMEOUT=$(grep -E "^timeout:" "${CONFIG}" | awk '{print $2}')
[ -z "${CONFIG_TIMEOUT}" ] && CONFIG_TIMEOUT=10

# Deliberately give Docker far more than that, so DOCKER never kills the
# container: we want to observe the app's real stop behaviour and then judge it,
# rather than have a SIGKILL hide what actually happened.
STOP_START=$(date +%s)
docker stop -t $((CONFIG_TIMEOUT * 3 + 30)) "${CONTAINER_NAME}" > /dev/null 2>&1
STOP_ELAPSED=$(( $(date +%s) - STOP_START ))

# CI runners are slower than the hardware this runs on in production, so a stop
# that merely creeps over budget is reported rather than failed. Overshooting
# by more than 2x is a real regression, not runner noise.
if [ "${STOP_ELAPSED}" -gt $((CONFIG_TIMEOUT * 2)) ]; then
    fail "Stop took ${STOP_ELAPSED}s against a ${CONFIG_TIMEOUT}s budget — the Supervisor would SIGKILL this during a HAOS update or backup. Raise 'timeout:' in config.yaml (and S6_SERVICES_GRACETIME to match)."
elif [ "${STOP_ELAPSED}" -gt "${CONFIG_TIMEOUT}" ]; then
    info "Stop took ${STOP_ELAPSED}s, over the ${CONFIG_TIMEOUT}s budget — likely runner slowness, but worth watching"
else
    pass "Stopped in ${STOP_ELAPSED}s, within the ${CONFIG_TIMEOUT}s budget"
fi

# A truncated shutdown still exits 0, so the exit code alone cannot detect it.
# `s6-svwait: fatal: timed out` means S6 gave up waiting for a service to stop
# and killed it mid-cleanup — the app never finished flushing state. This
# matters because Home Assistant stops every app for HAOS updates and for
# backups that include app data, so a shutdown that silently truncates is a
# data-loss risk on a routine operation. Fix by raising S6_SERVICES_GRACETIME
# (and `timeout:` in config.yaml to stay above it), not by ignoring this.
if docker logs "${CONTAINER_NAME}" 2>&1 | grep -q "s6-svwait: fatal: timed out"; then
    docker logs "${CONTAINER_NAME}" 2>&1 | grep -B2 "s6-svwait: fatal: timed out" | tail -5
    fail "Shutdown was truncated — S6 timed out waiting for a service and killed it mid-cleanup (exit code still 0). Raise S6_SERVICES_GRACETIME in the Dockerfile and 'timeout:' in config.yaml."
fi
pass "Shutdown completed without S6 having to kill any service"

EXIT_CODE=$(docker inspect "${CONTAINER_NAME}" --format='{{.State.ExitCode}}' 2>/dev/null || echo "unknown")
if [ "${EXIT_CODE}" = "0" ]; then
    pass "Clean shutdown (exit code 0)"
elif [ "${EXIT_CODE}" = "137" ] && [ -n "${APPARMOR_PROFILE}" ]; then
    # 137 = SIGKILL, i.e. the app never shut itself down and s6 had to be
    # killed after the grace period. Under confinement this is the exact
    # signature of an AppArmor profile that doesn't grant `signal ... receive`
    # (validate-apparmor.sh rule 4) — SIGTERM never reaches the app, so
    # graceful shutdown is skipped and any on-shutdown work silently never
    # runs. Treated as a hard failure so the regression can't ship again.
    fail "SIGKILLed on stop (exit 137) while confined by '${APPARMOR_PROFILE}' — the app never received SIGTERM. Check the profile grants 'signal (send,receive),'"
else
    info "Exit code ${EXIT_CODE} (SIGTERM exit is normal for some apps)"
fi

echo ""
echo -e "${GREEN}==> Smoke test passed: ${SLUG}${NC}"
