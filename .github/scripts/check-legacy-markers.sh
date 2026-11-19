#!/usr/bin/env bash
# Lists "LEGACY UNTIL YYYY-MM-DD" markers whose date has passed.
#
# Code kept only for installs an old release made carries such a marker
# (CLAUDE.md, "Legacy handling has an end date"). The smoke test runs this for
# the app it builds, so once a date passes that app's next build fails until
# the code is removed, or the date is moved with a reason.
#
# Usage: check-legacy-markers.sh <directory>...
#        TODAY=YYYY-MM-DD overrides the date, for testing.
# Exit code 1 when a marker is past its date or has no date.
set -euo pipefail

today="${TODAY:-$(date -u +%F)}"
status=0

for dir in "$@"; do
    # Markdown is left out: the docs name the marker without being one.
    while IFS= read -r hit; do
        [ -n "${hit}" ] || continue
        file="${hit%%:*}"
        rest="${hit#*:}"
        line="${rest%%:*}"
        if [[ "${rest#*:}" =~ LEGACY\ UNTIL\ ([0-9]{4}-[0-9]{2}-[0-9]{2}) ]]; then
            due="${BASH_REMATCH[1]}"
            if [[ "${due}" < "${today}" ]]; then
                echo "  ${file}:${line}: legacy handling was due for removal after ${due}"
                status=1
            fi
        else
            echo "  ${file}:${line}: LEGACY UNTIL needs a date, as YYYY-MM-DD"
            status=1
        fi
    done < <(grep -rn --exclude='*.md' "LEGACY UNTIL" "${dir}" 2>/dev/null || true)
done

exit "${status}"
