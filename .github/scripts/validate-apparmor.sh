#!/bin/bash
# Validates every app's AppArmor profile against the repo's hard-won rules
# (see "AppArmor Profile Rules" in the root CLAUDE.md; incident record in
# huly/CHANGELOG.md 0.7.426 maintenance notes, PRs #165/#166):
#
#   1. Profile must COMPILE (apparmor_parser -QK).
#   2. Profile must be FLAT — exactly one `profile` declaration, no `cx ->`
#      transitions. HAOS 18.1's kernel (6.18) denies AF_UNIX socket connects
#      from processes confined by nested child profiles regardless of the
#      rules the child contains.
#   3. If the profile references docker.sock, it must allow the RESOLVED
#      path (`/run/docker.sock rw,` — /var/run is a symlink to /run and
#      AppArmor matches resolved paths) and carry a bare `network,` rule
#      (AF_UNIX is required for the socket).
#
# Validates ALL profiles, not just changed ones — the rules are global and a
# violation anywhere ships broken on the next rebuild of that app.
#
# Usage: validate-apparmor.sh

set -euo pipefail

EXIT_CODE=0
ERRORS=""
CHECKED=0

err() {
    ERRORS="${ERRORS}\n- $1"
    EXIT_CODE=1
}

if ! command -v apparmor_parser > /dev/null 2>&1; then
    echo "apparmor_parser not found — install the 'apparmor' package" >&2
    exit 1
fi

for profile_file in */apparmor.txt; do
    [ -f "${profile_file}" ] || continue
    app_dir=$(dirname "${profile_file}")
    CHECKED=$((CHECKED + 1))
    echo "Validating ${profile_file}..."

    # Rule 1: must compile (dry run, skip kernel load and cache)
    if ! PARSE_OUT=$(apparmor_parser -QK "${profile_file}" 2>&1); then
        err "${profile_file}: does not compile: $(echo "${PARSE_OUT}" | head -1)"
        continue
    fi

    # Rule 2a: exactly one profile declaration (no nested child profiles)
    PROFILE_COUNT=$(grep -cE '^[[:space:]]*profile[[:space:]]' "${profile_file}" || true)
    if [ "${PROFILE_COUNT}" -ne 1 ]; then
        err "${profile_file}: ${PROFILE_COUNT} profile declarations found — profiles must be FLAT (nested child profiles break AF_UNIX sockets on HAOS 18.1+)"
    fi

    # Rule 2b: no cx (child-profile) transitions
    if grep -qE '\bcx\b' "${profile_file}"; then
        err "${profile_file}: contains a 'cx ->' child-profile transition — profiles must be FLAT (child profiles break AF_UNIX sockets on HAOS 18.1+)"
    fi

    # Rule 3: docker.sock apps need the resolved path + bare network rule
    if grep -q 'docker\.sock' "${profile_file}"; then
        if ! grep -qE '^[[:space:]]*/(\{,var/\})?run/docker\.sock[[:space:]]+rw,' "${profile_file}"; then
            err "${profile_file}: references docker.sock but never allows the RESOLVED path — add '/run/docker.sock rw,' (/var/run is a symlink; AppArmor matches resolved paths)"
        fi
        if ! grep -qE '^[[:space:]]*network,[[:space:]]*$' "${profile_file}"; then
            err "${profile_file}: docker-socket profile without a bare 'network,' rule — AF_UNIX is required for the socket"
        fi
    fi
done

echo ""
if [ ${EXIT_CODE} -eq 0 ]; then
    echo "✓ All ${CHECKED} AppArmor profiles valid"
else
    echo -e "✗ AppArmor validation errors:${ERRORS}"
fi

# Expose errors for the workflow's PR comment step
if [ -n "${GITHUB_OUTPUT:-}" ]; then
    {
        echo "ERRORS<<EOF"
        echo -e "${ERRORS}"
        echo "EOF"
    } >> "${GITHUB_OUTPUT}"
fi

exit ${EXIT_CODE}
