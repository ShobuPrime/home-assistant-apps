#!/bin/bash
# Does the upstream release's "Breaking Changes" section name anything this
# app actually depends on?
#
# Usage: check-lemonade-breaking.sh <release-notes-file> <path>...
#   release-notes-file: the GitHub release body (markdown)
#   path...:            files/directories to search for the named tokens
#
# Exit codes: 0 = clean, 1 = flagged (one "token: files | bullet" line per
#             match on stdout), 2 = notes unusable
#
# The Migration wiki page only documents what upstream chooses to write up.
# 11.9.0 deprecated LEMONADE_ALLOWED_ORIGINS — a variable this app set on every
# boot — with nothing on that page, so the PR body carried the breaking change
# and auto-merged anyway. Every upstream release lists breaking changes,
# though, so flagging on the section's presence would block every bump.
# Instead: pull the backtick-quoted identifiers out of the section (env vars,
# config keys, paths, flags) and flag those that occur verbatim in the paths
# given. Pass the files that DEPEND on lemond's behaviour — config.yaml,
# rootfs/, Dockerfile, apparmor.txt, the bridge, the smoke test — not the
# docs, which mention far more than the app relies on. Prose that names no
# identifier is not caught; the smoke test stays the backstop for that.
#
# Calibrated against 11.5.1..11.9.0 with that path set: 11.9.0 flags on
# LEMONADE_ALLOWED_ORIGINS, allowed_origins and extra_models_dir; 11.8.0 on
# --host/--port (this app passes both to lemond, so the glance was due);
# 11.8.1 is clean. Expect roughly every other bump to ask for a glance.

set -euo pipefail

NOTES="${1:?usage: check-lemonade-breaking.sh <release-notes-file> <path>...}"
shift
[ "$#" -gt 0 ] || { echo "usage: check-lemonade-breaking.sh <release-notes-file> <path>..." >&2; exit 2; }

[ -s "${NOTES}" ] || exit 2
case "$(head -c 200 "${NOTES}")" in
    "No changelog available"*|"Could not fetch changelog"*) exit 2 ;;
esac

# The section body: from a "Breaking Changes" heading to the next heading of
# the same or higher level. Several sections (rare) are concatenated.
SECTION="$(tr -d '\r' < "${NOTES}" | awk '
    /^#+[[:space:]]+Breaking Changes/ { lvl = length($1); on = 1; next }
    on && /^#+[[:space:]]/ && length($1) <= lvl { on = 0 }
    on { print }
')"
[ -n "${SECTION}" ] || exit 0

# Backtick-quoted spans, split on whitespace and "=", stripped of quotes and
# trailing punctuation. Keep only identifier-shaped tokens: something with an
# underscore, dot, slash or a leading dash — env vars, config keys, paths,
# flags. Plain words (`limit`, `latest`) and bare version numbers match too
# much of any tree to mean anything.
TOKENS="$(printf '%s\n' "${SECTION}" \
    | grep -oE '`[^`]+`' | tr -d '`' \
    | tr ' =' '\n\n' \
    | sed -E 's/^["'"'"'(]+//; s/["'"'"'),.;:]+$//' \
    | grep -E '^[A-Za-z0-9_./*-]+$' \
    | grep -E '[_./]|^-' \
    | grep -vE '^v?[0-9]+(\.[0-9]+)*$' \
    | grep -vE '^[-._/*]+$' \
    | awk 'length($0) >= 4' \
    | sort -u)"
[ -n "${TOKENS}" ] || exit 0

HITS=""
while IFS= read -r t; do
    [ -n "${t}" ] || continue
    # Fixed-string, whole-word: `chat/` must not match `/v1/chat/completions`.
    files="$(grep -rIlFw --exclude=CHANGELOG.md --exclude='*.png' -- "${t}" "$@" 2>/dev/null | sort | tr '\n' ' ')" || true
    [ -n "${files}" ] || continue
    # The bullet that named it, so the reviewer sees the claim next to the hit.
    bullet="$(printf '%s\n' "${SECTION}" | { grep -F -m1 -- "\`${t}" || grep -F -m1 -- "${t}" || true; } \
        | sed -E 's/^[[:space:]]*[-*][[:space:]]*//' | cut -c1-160)"
    HITS="${HITS}${t}: ${files% } | ${bullet}"$'\n'
done <<< "${TOKENS}"

if [ -n "${HITS}" ]; then
    printf '%s' "${HITS}"
    exit 1
fi
exit 0
