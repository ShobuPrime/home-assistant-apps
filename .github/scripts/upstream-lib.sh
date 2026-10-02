#!/bin/bash
# Shared by the update-*.sh scripts. Source it, do not run it:
#
#     source "$(dirname "${BASH_SOURCE[0]}")/upstream-lib.sh"
#
# Why this exists: on 2026-09-20 the Portainer LTS check exited 1 with no
# output at all. Its three unauthenticated calls to api.github.com had come
# back without a usable release list — a GitHub-hosted runner shares its
# egress IP, and the anonymous API budget is 60 requests/hour per IP — and
# `set -e` killed the script on `LATEST_VERSION=$(get_latest_version)`
# before the error branch that would have said so. Everything here is about
# making that impossible to repeat: authenticate, retry, and always leave a
# diagnostic.

# Fetch a GitHub REST API URL. Body on stdout; non-zero exit on any failure,
# with one diagnostic line on stderr that names the URL, the HTTP status and
# the API's own message — so a rate limit reads as
#   GET https://api.github.com/...: HTTP 403 API rate limit exceeded for 1.2.3.4
# in the job log rather than as nothing.
#
# Authenticated when GITHUB_TOKEN (or GH_TOKEN) is set: the workflows pass
# secrets.GITHUB_TOKEN, which is good for 1,000 requests/hour per repository
# instead of 60 per shared runner IP. A token that cannot see a public repo
# does not exist, so reading upstream releases with it is safe. Unset (a
# local run without a token) still works, just anonymously.
#
# `--retry` covers the transient class curl knows about: timeouts, 429 and
# 5xx, plus refused connections. It does not retry a 403 rate limit — that
# resets on the hour, not in seconds — which is what the token is for.
gh_api() {
    local url="$1"
    local -a auth=()
    local token="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
    if [ -n "${token}" ]; then
        auth=(-H "Authorization: Bearer ${token}")
    fi

    local out code body
    if ! out=$(curl -sS --connect-timeout 10 --max-time 60 \
            --retry 3 --retry-delay 5 --retry-connrefused \
            -H "Accept: application/vnd.github+json" \
            -H "X-GitHub-Api-Version: 2022-11-28" \
            "${auth[@]}" \
            -w '\n%{http_code}' "${url}" 2>&1); then
        # curl's own error (DNS, timeout, reset), one line per retry, and -w
        # still appended "\n000". Keep the last real line.
        echo "GET ${url}: $(printf '%s\n' "${out}" | grep -v '^[0-9]*$' | tail -1)" >&2
        return 1
    fi
    code="${out##*$'\n'}"
    body="${out%$'\n'*}"

    if [ "${code}" -ge 400 ] 2>/dev/null || [ -z "${code}" ]; then
        echo "GET ${url}: HTTP ${code:-?} $(printf '%s' "${body}" | jq -r '.message // empty' 2>/dev/null)" >&2
        printf '%s\n' "${body}"
        return 1
    fi

    printf '%s\n' "${body}"
}

# $1 sorts strictly below $2 in version order (GNU sort -V). Used to refuse
# downgrades: an update script must never propose a version below the one
# shipping. Portainer maintains two LTS lines at once (2.39.x and 2.45.x were
# both receiving patches in September 2026), so "the most recently published
# LTS" is not "the newest LTS", and a yanked upstream release makes the newest
# remaining one older than ours. Neither is a reason to move a device backwards.
ver_lt() {
    [ "$1" != "$2" ] && [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | head -1)" = "$1" ]
}
