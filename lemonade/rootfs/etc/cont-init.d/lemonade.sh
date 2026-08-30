#!/usr/bin/with-contenv bashio
# ==============================================================================
# Home Assistant App: Lemonade
# Prepares the persistent cache layout and seeds config.json from app options
# ==============================================================================

# Everything Lemonade persists hangs off $HOME:
#   $HOME/.config/lemonade/config.json - server configuration (also
#                                        user_models.json, recipe_options.json)
#   $HOME/.cache/lemonade/             - downloaded llama.cpp backends
#   $HOME/.cache/huggingface/          - downloaded GGUF weights
#   $HOME/llama/<backend>/             - llama.cpp binaries fetched at runtime
# Pointing HOME at /data keeps all of it inside Home Assistant backups.
LEMONADE_HOME="/data/lemonade"
CONFIG_DIR="${LEMONADE_HOME}/.config/lemonade"
CONFIG_FILE="${CONFIG_DIR}/config.json"
LEGACY_CONFIG_FILE="${LEMONADE_HOME}/.cache/lemonade/config.json"

bashio::log.info "Preparing Lemonade data directories..."
mkdir -p \
    "${CONFIG_DIR}" \
    "${LEMONADE_HOME}/.cache/lemonade" \
    "${LEMONADE_HOME}/.cache/huggingface" \
    "${LEMONADE_HOME}/llama"
chmod 755 "${LEMONADE_HOME}"

# lemond 11.8.0 split config out of the cache dir (.cache/lemonade ->
# .config/lemonade). Its migration only moves the legacy file when the new
# path is missing, and this script runs first — so move it here. Creating a
# fresh file at the new path instead would make lemond skip its migration and
# silently orphan the user's existing config (installed backends, UI-set
# options). lemond still migrates user_models.json and friends itself.
if [[ -f "${LEGACY_CONFIG_FILE}" && ! -f "${CONFIG_FILE}" ]]; then
    mv "${LEGACY_CONFIG_FILE}" "${CONFIG_FILE}"
    bashio::log.info "Migrated config.json to .config/lemonade (11.8.0 layout)"
fi

# Private runtime dir for lemond's get_runtime_dir()
mkdir -p /run/lemonade
chmod 700 /run/lemonade

if [[ ! -x /opt/lemonade/lemond ]]; then
    bashio::log.error "lemond not found or not executable at /opt/lemonade/lemond!"
    bashio::exit.nok
fi

# ------------------------------------------------------------------------------
# Seed config.json from the app options.
#
# lemond reads config.json on start and a user's config.json wins over the
# defaults baked into the release, so the add-on options have to be written
# there to take effect. We MERGE rather than overwrite: keys Lemonade or the
# user set through the web UI (installed backends, per-model recipe options,
# cloud providers, ...) are preserved, and only the keys this add-on exposes
# as options are asserted on every start.
# ------------------------------------------------------------------------------
CTX_SIZE="$(bashio::config 'ctx_size' '4096')"
# Lemonade uses -1 for "let the model decide"; expose that as 0 in the UI
# because the add-on schema only accepts non-negative integers.
if [[ "${CTX_SIZE}" -eq 0 ]]; then
    CTX_SIZE=-1
fi

LLAMACPP_BACKEND="$(bashio::config 'llamacpp_backend' 'cpu')"
LLAMACPP_BIN="$(bashio::config 'llamacpp_bin' 'builtin')"

# Which llama.cpp build to run for the selected backend.
#
# lemond stores this per backend as `<backend>_bin` and accepts "builtin"
# (the build shipped with this Lemonade release), "latest" (fetch the newest
# llama.cpp release at startup), a version tag such as "b8664", or a path to a
# pre-downloaded binary.
#
# "builtin" is the default deliberately: it needs no network at startup, and it
# is the build upstream tested against this server version. "latest" downloads
# on first use, so a start with no internet leaves the app without a backend.
if [[ -z "${LLAMACPP_BIN}" ]]; then
    LLAMACPP_BIN="builtin"
fi
MAX_LOADED="$(bashio::config 'max_loaded_models' '1')"
LOG_LEVEL="$(bashio::config 'log_level' 'info')"
TELEMETRY="$(bashio::config 'telemetry' 'false')"

# ------------------------------------------------------------------------------
# Extra models directory
#
# A second directory Lemonade scans recursively for .gguf files, on top of the
# Hugging Face cache. A file `my-model.gguf` is offered as `my-model`, and is
# read-only to Lemonade (it refuses to delete files it did not download).
#
# Note: upstream's Extra-Models-Dir-Spec.md documents an `extra.` name prefix.
# As of 11.5.0 that prefix is NOT applied to the listed name — verified on a
# real drop-in: the model lists as `my-custom-model` / `my-custom-model:latest`
# (the `extra.` form does still resolve as an alias). Don't "correct" the log
# message below to match the spec without re-checking the running server.
#
# Defaulting this under /share means you can drop a .gguf in via Samba, the File
# editor or `scp` and have it show up after a restart — no CLI, no Hugging Face
# round trip, and the file stays put when the app is updated or reinstalled.
# ------------------------------------------------------------------------------
EXTRA_MODELS_DIR=""
if bashio::config.has_value 'extra_models_dir'; then
    EXTRA_MODELS_DIR="$(bashio::config 'extra_models_dir')"

    if mkdir -p "${EXTRA_MODELS_DIR}" 2>/dev/null; then
        bashio::log.info "Extra models directory: ${EXTRA_MODELS_DIR}"
        # Count only what Lemonade would actually pick up.
        GGUF_COUNT="$(find "${EXTRA_MODELS_DIR}" -type f -name '*.gguf' 2>/dev/null | wc -l)"
        if [[ "${GGUF_COUNT}" -gt 0 ]]; then
            bashio::log.info "  Found ${GGUF_COUNT} .gguf file(s) — listed under their filename without the extension"
        else
            bashio::log.info "  No .gguf files yet — drop one in and restart to use it"
        fi
    else
        # A bad path shouldn't stop the server from serving models it already has.
        bashio::log.warning "Could not create ${EXTRA_MODELS_DIR} — extra models disabled"
        EXTRA_MODELS_DIR=""
    fi
fi

# Lemonade's own log levels are a subset of the HA app levels.
case "${LOG_LEVEL}" in
    trace|debug) LEMON_LOG_LEVEL="debug" ;;
    warning)     LEMON_LOG_LEVEL="warning" ;;
    error|fatal) LEMON_LOG_LEVEL="error" ;;
    *)           LEMON_LOG_LEVEL="info" ;;
esac

if [[ ! -f "${CONFIG_FILE}" ]]; then
    echo '{}' > "${CONFIG_FILE}"
    bashio::log.info "Created a fresh config.json"
fi

# A corrupt config.json would make lemond fail to boot on every restart, so
# fall back to an empty object rather than dying here.
if ! jq empty "${CONFIG_FILE}" > /dev/null 2>&1; then
    bashio::log.warning "config.json is not valid JSON — resetting it"
    echo '{}' > "${CONFIG_FILE}"
fi

MERGED="$(jq \
    --argjson ctx_size "${CTX_SIZE}" \
    --argjson max_loaded "${MAX_LOADED}" \
    --argjson telemetry "${TELEMETRY}" \
    --arg backend "${LLAMACPP_BACKEND}" \
    --arg bin "${LLAMACPP_BIN}" \
    --arg log_level "${LEMON_LOG_LEVEL}" \
    --arg extra_models_dir "${EXTRA_MODELS_DIR}" \
    '
    . as $existing
    | .host = "0.0.0.0"
    | .port = 13305
    | .log_level = $log_level
    | .ctx_size = $ctx_size
    | .max_loaded_models = $max_loaded
    | .extra_models_dir = $extra_models_dir
    | .llamacpp = (($existing.llamacpp // {}) + {backend: $backend}
                   + {("\($backend)_bin"): $bin})
    | .telemetry = (($existing.telemetry // {}) + {enabled: $telemetry})
    ' "${CONFIG_FILE}")"

echo "${MERGED}" > "${CONFIG_FILE}"

bashio::log.info "Lemonade configured:"
bashio::log.info "  Backend:          llamacpp/${LLAMACPP_BACKEND} (${LLAMACPP_BIN})"
bashio::log.info "  Context size:     ${CTX_SIZE}"
bashio::log.info "  Max loaded models: ${MAX_LOADED}"
bashio::log.info "  Log level:        ${LEMON_LOG_LEVEL}"
bashio::log.info "  Telemetry:        ${TELEMETRY}"
bashio::log.info "  Extra models dir: ${EXTRA_MODELS_DIR:-(disabled)}"

if bashio::config.has_value 'api_key'; then
    bashio::log.info "  API key:          set (clients must authenticate)"
else
    bashio::log.info "  API key:          not set (open on the local network)"
fi

# ------------------------------------------------------------------------------
# Service topology, decided once and written to runtime.env for every service:
#
#   Home Assistant ──► ha-lemonade-bridge :13305 ──► lemond 127.0.0.1:13306
#
# The bridge always runs, even with memory and speech-to-text off, because it
# is also the browser-Origin gate. Memory and STT stay gated inside it.
# See lemonade/CLAUDE.md.
# ------------------------------------------------------------------------------
RUNTIME_ENV="/run/lemonade/runtime.env"
LEMOND_BIND_HOST="127.0.0.1"
LEMOND_PORT="13306"
MEMORY_ENABLED="false"
MEMORY_URL=""
MEMORY_VAULT=""

if bashio::config.true 'memory_enabled'; then
    MEMORY_ENABLED="true"
    MEMORY_URL="$(bashio::config 'memory_url' '')"
    MEMORY_VAULT="$(bashio::config 'memory_vault' 'homeassistant')"

    if [[ -z "${MEMORY_URL}" ]]; then
        bashio::log.warning "Memory is enabled but 'memory_url' is empty — running as a plain pass-through"
        bashio::log.warning "Set it to your MuninnDB REST endpoint, e.g. http://<muninndb-hostname>:8475"
        MEMORY_ENABLED="false"
    fi
fi

STT_ENABLED="false"
if bashio::config.true 'stt_enabled'; then
    STT_ENABLED="true"
fi

if [[ "${MEMORY_ENABLED}" == "true" ]]; then
    # Reachability preflight. This never fails the boot — MuninnDB may simply
    # start after us, and the proxy retries with a circuit breaker anyway. It
    # exists so the log says plainly whether memory will actually work.
    if curl -sf --max-time 3 -o /dev/null "${MEMORY_URL}/api/vaults" 2>/dev/null \
        || curl -sf --max-time 3 -o /dev/null "${MEMORY_URL}/health" 2>/dev/null; then
        bashio::log.info "Memory: MuninnDB reachable at ${MEMORY_URL} (vault '${MEMORY_VAULT}')"
    else
        bashio::log.warning "Memory: MuninnDB not reachable at ${MEMORY_URL} yet"
        bashio::log.warning "  Chat still works — memory is skipped until it responds."
        bashio::log.warning "  Check the MuninnDB app is running and that memory_url matches its hostname."
    fi
else
    bashio::log.info "Memory: disabled — the bridge proxies without looking up anything"
fi

if [[ "${MEMORY_ENABLED}" != "true" && "${STT_ENABLED}" != "true" ]]; then
    bashio::log.info "Memory and speech-to-text both off — the bridge only enforces browser origins"
fi

# ------------------------------------------------------------------------------
# Origin allowlist — only what the user typed. Everything else (this device's
# addresses, Home Assistant's URLs, the published port) is discovered by the
# bridge at runtime, because none of it is reliably knowable here: Supervisor
# starts us before Core, and addresses change under DHCP.
# See ha-lemonade-bridge/origins.go.
# ------------------------------------------------------------------------------
ORIGINS="$(bashio::config 'allowed_origins' '')"
if [[ "${ORIGINS}" == *"*"* ]]; then
    bashio::log.warning "allowed_origins contains '*' — any website you visit can then drive this API from your browser."
    bashio::log.warning "  Set an 'api_key' as well if you really need this."
fi
# Strip whitespace; the bridge splits on commas.
ORIGINS="$(echo "${ORIGINS}" | tr -d '[:space:]')"

if [[ -n "${ORIGINS}" ]]; then
    bashio::log.info "Allowed browser origins (extra, from configuration): ${ORIGINS}"
fi
bashio::log.info "Allowed browser origins are discovered by the bridge; see its log for the full list"

{
    echo "BRIDGE_MEMORY_ENABLED=${MEMORY_ENABLED}"
    echo "STT_ENABLED=${STT_ENABLED}"
    echo "LEMOND_BIND_HOST=${LEMOND_BIND_HOST}"
    echo "LEMOND_PORT=${LEMOND_PORT}"
    echo "BRIDGE_ALLOWED_ORIGINS=${ORIGINS}"
} > "${RUNTIME_ENV}"

bashio::log.info "Lemonade initialization complete"
