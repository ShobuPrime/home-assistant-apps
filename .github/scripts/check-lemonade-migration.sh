#!/bin/bash
# Does upstream's Migration wiki page document a migration that upgrading
# lemonade from <current> to <target> would cross?
#
# Usage: check-lemonade-migration.sh <current> <target> [page-file]
#   page-file: read the page from a file instead of fetching (for tests)
#
# Exit codes: 0 = clean, 1 = flagged (matched tokens on stdout), 2 = fetch failed
#
# A migration documented at version X applies when the upgrade CROSSES X, so
# the check is against the half-open range (current, target] — not the target
# alone. Target-only matching has a real hole: with the pipeline down long
# enough to skip a release, a PR can jump e.g. 11.7.0 -> 11.8.2 and cross the
# documented 11.8.0 config-dir migration while "11.8.2" appears nowhere on the
# page.
#
# The page writes versions as vA.B.C, and series as vA.B.x / vA.x / vA.B. A
# series token is treated as its whole interval ([A.B.0, A.B+1.0) etc.) and
# flags when that interval intersects the range. This deliberately over-flags
# the "from"-side series of a heading like "v11.7.x / v11.8.0 - v11.8.1"
# (upgrading TO 11.7.0 flags even though that migration sits at 11.8.0):
# a spurious needs-review costs one human glance, a missed migration costs a
# broken device.

set -euo pipefail

CURRENT="${1:?usage: check-lemonade-migration.sh <current> <target> [page-file]}"
TARGET="${2:?usage: check-lemonade-migration.sh <current> <target> [page-file]}"
PAGE_FILE="${3:-}"

URL="https://raw.githubusercontent.com/wiki/lemonade-sdk/lemonade/Migration.md"

if [ -n "${PAGE_FILE}" ]; then
    PAGE="$(cat "${PAGE_FILE}")" || exit 2
else
    PAGE="$(curl -fsSL --retry 3 "${URL}")" || exit 2
fi
[ -n "${PAGE}" ] || exit 2

# $1 <= $2 in version order
ver_le() { [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | head -1)" = "$1" ]; }
# $1 < $2
ver_lt() { [ "$1" != "$2" ] && ver_le "$1" "$2"; }

# Every version-shaped token on the page. Non-Lemonade numbers (RAI 1.7,
# Ubuntu releases, ...) come along too; the range check filters them out.
TOKENS="$(printf '%s' "${PAGE}" \
    | grep -oE 'v?[0-9]+(\.[0-9]+)+(\.x)?|v?[0-9]+\.x' \
    | sed 's/^v//' | sort -uV || true)"

HITS=""
for t in ${TOKENS}; do
    case "${t}" in
        *.x)
            # Series: [smin, supper). 10# guards octal on zero-padded parts.
            p="${t%.x}"
            case "${p}" in
                *.*) stem="${p%.*}"; last="${p##*.}"
                     smin="${p}.0"; supper="${stem}.$((10#${last} + 1)).0" ;;
                *)   smin="${p}.0.0"; supper="$((10#${p} + 1)).0.0" ;;
            esac
            if ver_le "${smin}" "${TARGET}" && ver_lt "${CURRENT}" "${supper}"; then
                HITS="${HITS} ${t}"
            fi
            ;;
        *.*.*)
            # Exact: crossed iff current < t <= target.
            if ver_lt "${CURRENT}" "${t}" && ver_le "${t}" "${TARGET}"; then
                HITS="${HITS} ${t}"
            fi
            ;;
        *)
            # Two-component (v10.1): treat as the A.B.* series.
            stem="${t%.*}"; last="${t##*.}"
            smin="${t}.0"; supper="${stem}.$((10#${last} + 1)).0"
            if ver_le "${smin}" "${TARGET}" && ver_lt "${CURRENT}" "${supper}"; then
                HITS="${HITS} ${t}"
            fi
            ;;
    esac
done

if [ -n "${HITS}" ]; then
    echo "${HITS# }"
    exit 1
fi
exit 0
