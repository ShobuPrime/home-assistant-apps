#!/usr/bin/with-contenv bashio
# ==============================================================================
# Home Assistant Add-on: Sonuntius
# Phase 5 — iFi Tidal Connect binary fallback (opt-in).
#
# When tidal_fallback.enabled = true:
#   1. Extract /share/sonuntius/ifi-tidal-release.tar.gz to /opt/ifi-tidal/
#   2. Locate the tidal_connect_application binary inside the extracted tree
#   3. Symlink it to /usr/local/bin/tidal_connect_application for the S6
#      service to exec
#   4. Verify the user-supplied cert exists at the configured path
#   5. Touch /run/sonuntius/tidal-fallback-ready as the green-light marker
#      the per-service run scripts check
#
# This script NEVER exits non-zero. If anything goes wrong it logs a clear
# warning and leaves the marker file absent — the tidal-connect and
# alsa-to-sendspin run scripts then log "disabled" and sleep, so the
# container stays healthy and S6 does not enter a restart loop.
#
# Architecture caveat: the iFi binary is ARMv7 (see DOCS.md provenance
# disclosure). On aarch64 it runs via the kernel's compat layer plus
# libc6-compat (installed in the Dockerfile when BUILD_ARCH=aarch64).
# On amd64 it would require qemu-user-static; we explicitly skip the
# fallback there and recommend the proxy path (Phase 3).
# ==============================================================================

set -euo pipefail

bashio::log.info "Phase 5: evaluating Tidal Connect fallback..."

if ! bashio::config.true 'tidal_fallback.enabled'; then
    bashio::log.info "tidal_fallback.enabled = false — skipping iFi binary setup."
    exit 0
fi

if [[ "$(uname -m)" != "aarch64" ]]; then
    bashio::log.warning "Phase 5: tidal_fallback enabled but host arch is $(uname -m)."
    bashio::log.warning "Phase 5: the iFi binary is ARMv7 and requires aarch64 + libc6-compat to run."
    bashio::log.warning "Phase 5: fallback services will stay idle. Use the Cast proxy (Phase 3) instead."
    exit 0
fi

TARBALL="$(bashio::config 'tidal_fallback.binary_tarball_path')"
CERT_FILENAME="$(bashio::config 'tidal_fallback.cert_filename')"

if [[ -z "${TARBALL}" || ! -f "${TARBALL}" ]]; then
    bashio::log.warning "Phase 5: iFi tarball not found at '${TARBALL}'."
    bashio::log.warning "Phase 5: place the iFi Tidal Connect release tarball there and restart the addon."
    bashio::log.warning "Phase 5: see DOCS.md → Tidal Connect fallback for provenance and sourcing instructions."
    exit 0
fi

bashio::log.info "Phase 5: extracting ${TARBALL} → /opt/ifi-tidal/"
mkdir -p /opt/ifi-tidal
if ! tar -xzf "${TARBALL}" -C /opt/ifi-tidal --strip-components=0; then
    bashio::log.warning "Phase 5: tarball extraction failed (corrupt archive?)."
    exit 0
fi

# Locate the binary anywhere under /opt/ifi-tidal. Different community
# tarball layouts nest it differently; we find it by name rather than
# hard-coding a path.
BINARY=""
while IFS= read -r -d '' candidate; do
    BINARY="${candidate}"
    break
done < <(find /opt/ifi-tidal -type f -name 'tidal_connect_application' -print0 2>/dev/null)

if [[ -z "${BINARY}" ]]; then
    bashio::log.warning "Phase 5: tidal_connect_application not found in extracted tree."
    bashio::log.warning "Phase 5: contents:"
    find /opt/ifi-tidal -maxdepth 3 -type f | head -20 | while read -r f; do
        bashio::log.warning "  ${f}"
    done
    exit 0
fi

chmod +x "${BINARY}"
ln -sf "${BINARY}" /usr/local/bin/tidal_connect_application
bashio::log.info "Phase 5: binary linked at /usr/local/bin/tidal_connect_application → ${BINARY}"

# Locate the cert. It may live next to the binary in id_certificate/, or
# at the path conventionally documented in the plan
# (/opt/ifi-tidal/id_certificate/<filename>), or anywhere under the
# extracted tree under a directory named id_certificate.
CERT_PATH=""
for candidate in \
    "/opt/ifi-tidal/id_certificate/${CERT_FILENAME}" \
    "$(dirname "${BINARY}")/id_certificate/${CERT_FILENAME}"; do
    if [[ -f "${candidate}" ]]; then
        CERT_PATH="${candidate}"
        break
    fi
done
if [[ -z "${CERT_PATH}" ]]; then
    while IFS= read -r -d '' candidate; do
        CERT_PATH="${candidate}"
        break
    done < <(find /opt/ifi-tidal -type f -name "${CERT_FILENAME}" -print0 2>/dev/null)
fi

if [[ -z "${CERT_PATH}" ]]; then
    bashio::log.warning "Phase 5: cert '${CERT_FILENAME}' not found in extracted tree."
    bashio::log.warning "Phase 5: services will remain idle until the cert is provided."
    exit 0
fi

bashio::log.info "Phase 5: cert located at ${CERT_PATH}"
ln -sf "${CERT_PATH}" /opt/ifi-tidal/id_certificate.dat

# ALSA loopback advisory. We can't load snd-aloop from inside the
# container; the user has to load it on the host. Document and warn.
if [[ ! -d /dev/snd ]]; then
    bashio::log.warning "Phase 5: /dev/snd not present in container — host audio access is not exposed."
    bashio::log.warning "Phase 5: see DOCS.md for ALSA loopback setup."
elif ! ls /dev/snd/pcm* >/dev/null 2>&1; then
    bashio::log.warning "Phase 5: no ALSA PCM devices visible in /dev/snd."
    bashio::log.warning "Phase 5: ensure snd-aloop is loaded on the host (modprobe snd-aloop)."
fi

# Marker file the per-service run scripts gate on.
mkdir -p /run/sonuntius
: > /run/sonuntius/tidal-fallback-ready
bashio::log.info "Phase 5: Tidal Connect fallback ready."
