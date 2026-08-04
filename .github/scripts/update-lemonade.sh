#!/bin/bash
# Script to check and update Lemonade version
# Used by GitHub Actions workflow

set -e

# Configuration
APP_PATH="${APP_PATH:-.}"
CHECK_ONLY="${CHECK_ONLY:-false}"
JSON_OUTPUT="${JSON_OUTPUT:-false}"

UPSTREAM_REPO="lemonade-sdk/lemonade"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to log messages
log() {
    if [ "$JSON_OUTPUT" != "true" ]; then
        echo -e "$@"
    fi
}

# Function to get latest version with retry logic
get_latest_version() {
    local retries=3
    local delay=2
    local version=""

    for i in $(seq 1 $retries); do
        version=$(curl -s --connect-timeout 10 \
            "https://api.github.com/repos/${UPSTREAM_REPO}/releases/latest" 2>/dev/null | \
            jq -r '.tag_name // empty' 2>/dev/null)

        if [ -n "$version" ]; then
            # Remove 'v' prefix if present
            version="${version#v}"
            echo "$version"
            return 0
        fi

        [ $i -lt $retries ] && log "Retry $i/$retries..." >&2
        sleep $delay
    done

    return 1
}

# Function to verify the release actually ships the assets this app installs.
#
# The app builds from the "embeddable" archives, one per architecture. Upstream
# occasionally publishes a release before every asset is uploaded, and has
# renamed assets across major versions. Bumping the version on a release that
# lacks them would produce a PR that only fails at image-build time, so check
# here instead.
assets_present() {
    local version="$1"
    local retries=3
    local delay=2
    local assets=""

    for i in $(seq 1 $retries); do
        assets=$(curl -s --connect-timeout 10 \
            "https://api.github.com/repos/${UPSTREAM_REPO}/releases/tags/v${version}" 2>/dev/null | \
            jq -r '.assets[]?.name // empty' 2>/dev/null)

        if [ -n "$assets" ]; then
            break
        fi

        [ $i -lt $retries ] && log "Retry $i/$retries (assets)..." >&2
        sleep $delay
    done

    if [ -z "$assets" ]; then
        return 1
    fi

    local arch
    for arch in arm64 x64; do
        if ! echo "$assets" | grep -qx "lemonade-embeddable-${version}-ubuntu-${arch}.tar.gz"; then
            log "${RED}Missing asset: lemonade-embeddable-${version}-ubuntu-${arch}.tar.gz${NC}" >&2
            return 1
        fi
    done

    # The web UI is not in the embeddable archive — it is copied out of
    # upstream's container image at the matching tag (see the Dockerfile's
    # `webui` stage). If that tag is missing the build would still succeed but
    # ship no UI, so check it here rather than discover it after merging.
    local token code
    token=$(curl -s "https://ghcr.io/token?scope=repository:lemonade-sdk/lemonade-server:pull&service=ghcr.io" 2>/dev/null | jq -r '.token // empty')
    if [ -z "$token" ]; then
        log "${YELLOW}Could not reach ghcr.io to verify the web UI image tag — continuing${NC}" >&2
        return 0
    fi
    code=$(curl -s -o /dev/null -w "%{http_code}" \
        -H "Authorization: Bearer ${token}" \
        -H "Accept: application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json" \
        "https://ghcr.io/v2/lemonade-sdk/lemonade-server/manifests/v${version}" 2>/dev/null)
    if [ "$code" != "200" ]; then
        log "${RED}No container image tagged v${version} — the web UI assets come from there${NC}" >&2
        return 1
    fi

    return 0
}

# Function to get changelog for a specific version
get_changelog() {
    local version="$1"
    local changelog=""

    local release_info=$(curl -s --connect-timeout 10 \
        "https://api.github.com/repos/${UPSTREAM_REPO}/releases/tags/v${version}" 2>/dev/null)

    if [ -z "$release_info" ] || [ "$(echo "$release_info" | jq -r '.message // empty')" = "Not Found" ]; then
        release_info=$(curl -s --connect-timeout 10 \
            "https://api.github.com/repos/${UPSTREAM_REPO}/releases/tags/${version}" 2>/dev/null)
    fi

    if [ -n "$release_info" ]; then
        changelog=$(echo "$release_info" | jq -r '.body // "No changelog available"' 2>/dev/null)

        # Not truncated. CHANGELOG.md carries the upstream release notes in
        # full — a byte cap here used to cut them mid-word (Portainer 2.39.5
        # ended at "...not seeing all of their te"). The PR body is clamped
        # separately in the workflow, because GitHub hard-caps that field.
        # @mentions become code spans so automated PRs never ping upstream.
        changelog=$(echo "$changelog" | sed 's/\r//g' | sed 's/@\([a-zA-Z0-9_-]*\)/`\1`/g')

        if [ -n "$changelog" ] && [ "$changelog" != "null" ]; then
            echo "$changelog"
        else
            echo "No changelog available for version $version"
        fi
    else
        echo "Could not fetch changelog for version $version"
    fi
}

# Function to get current version from config.yaml
get_current_version() {
    if [ ! -f "$APP_PATH/config.yaml" ]; then
        log "${RED}Error: config.yaml not found at $APP_PATH!${NC}" >&2
        exit 1
    fi
    grep "^version:" "$APP_PATH/config.yaml" | sed 's/version: *"\(.*\)"/\1/'
}

# Function to update files
update_files() {
    local new_version="$1"
    local app_path="$2"

    # Update config.yaml
    sed -i "s/^version: \".*\"/version: \"$new_version\"/" "$app_path/config.yaml"
    log "${GREEN}${NC} Updated config.yaml"

    # Update build.yaml
    if [ -f "$app_path/build.yaml" ]; then
        sed -i "s/LEMONADE_VERSION: .*/LEMONADE_VERSION: $new_version/" "$app_path/build.yaml"
        log "${GREEN}${NC} Updated build.yaml"
    fi

    # Update Dockerfile
    if [ -f "$app_path/Dockerfile" ]; then
        sed -i "s/ARG LEMONADE_VERSION=.*/ARG LEMONADE_VERSION=$new_version/" "$app_path/Dockerfile"
        log "${GREEN}${NC} Updated Dockerfile"
    fi

    # Update README.md - conservative: only the explicit version sentence, so
    # section headings and model names containing digits are left alone.
    if [ -f "$app_path/README.md" ]; then
        sed -i "s/Currently running Lemonade [0-9][0-9.]*/Currently running Lemonade $new_version/g" "$app_path/README.md"
        log "${GREEN}${NC} Updated README.md"
    fi
}

# Function to update changelog
update_changelog() {
    local new_version="$1"
    local app_path="$2"
    local changelog_content="$3"

    if [ -f "$app_path/CHANGELOG.md" ]; then
        local temp_file=$(mktemp)
        cat > "$temp_file" << EOF
# Changelog

## $new_version

_$(date +%Y-%m-%d)_

$changelog_content

---

$(tail -n +2 "$app_path/CHANGELOG.md")
EOF
        mv "$temp_file" "$app_path/CHANGELOG.md"
    else
        cat > "$app_path/CHANGELOG.md" << EOF
# Changelog

## $new_version

_$(date +%Y-%m-%d)_

$changelog_content

---

For full release notes, see: https://github.com/${UPSTREAM_REPO}/releases/tag/v$new_version
EOF
    fi
    log "${GREEN}${NC} Updated CHANGELOG.md"
}

# Main execution
main() {
    log "=== Lemonade Version Updater ==="

    if [ ! -f "$APP_PATH/config.yaml" ]; then
        log "${RED}Error: config.yaml not found at $APP_PATH!${NC}" >&2
        exit 1
    fi

    log "Checking current version..."
    CURRENT_VERSION=$(get_current_version)
    log "Current version: ${YELLOW}$CURRENT_VERSION${NC}"

    log "Checking for latest release..."
    LATEST_VERSION=$(get_latest_version)

    if [ -z "$LATEST_VERSION" ]; then
        if [ "$JSON_OUTPUT" = "true" ]; then
            echo "{\"error\": \"Could not fetch latest version from GitHub\", \"update_available\": false}"
        else
            log "${RED}Error: Could not fetch latest version from GitHub${NC}" >&2
        fi
        exit 1
    fi

    log "Latest version: ${GREEN}$LATEST_VERSION${NC}"

    if [ "$CURRENT_VERSION" = "$LATEST_VERSION" ]; then
        if [ "$JSON_OUTPUT" = "true" ]; then
            echo "{\"current\": \"$CURRENT_VERSION\", \"latest\": \"$LATEST_VERSION\", \"update_available\": false}"
        else
            log "${GREEN}Already up to date!${NC}"
        fi
        exit 0
    fi

    # Don't propose a bump to a release that doesn't ship the archives we build from
    if ! assets_present "$LATEST_VERSION"; then
        if [ "$JSON_OUTPUT" = "true" ]; then
            echo "{\"current\": \"$CURRENT_VERSION\", \"latest\": \"$LATEST_VERSION\", \"update_available\": false, \"error\": \"Release v$LATEST_VERSION is missing the embeddable archives this app installs\"}"
        else
            log "${YELLOW}Release v$LATEST_VERSION is missing the embeddable archives — skipping${NC}"
        fi
        exit 0
    fi

    log "${YELLOW}Update available: $CURRENT_VERSION -> $LATEST_VERSION${NC}"

    CHANGELOG=$(get_changelog "$LATEST_VERSION")

    if [ "$CHECK_ONLY" = "true" ]; then
        if [ "$JSON_OUTPUT" = "true" ]; then
            ESCAPED_CHANGELOG=$(echo "$CHANGELOG" | jq -Rs '.')
            echo "{\"current\": \"$CURRENT_VERSION\", \"latest\": \"$LATEST_VERSION\", \"update_available\": true, \"changelog\": $ESCAPED_CHANGELOG}"
        fi
        exit 0
    fi

    log "\n${YELLOW}Applying update...${NC}"
    update_files "$LATEST_VERSION" "$APP_PATH"
    update_changelog "$LATEST_VERSION" "$APP_PATH" "$CHANGELOG"

    log "\n${GREEN}Update complete: $CURRENT_VERSION -> $LATEST_VERSION${NC}"

    if command -v ha &> /dev/null; then
        log "Reloading Home Assistant Supervisor..."
        ha supervisor reload || true
    fi
}

main "$@"
