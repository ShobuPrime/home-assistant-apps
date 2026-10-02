#!/bin/bash
# Script to check and update Portainer version
# Supports both LTS and STS releases (detected by release name, not version pattern)

set -e

# gh_api (authenticated, retried, one diagnostic line on failure) and ver_lt.
source "$(dirname "${BASH_SOURCE[0]}")/upstream-lib.sh"

# Configuration
APP_PATH="${APP_PATH:-.}"
VERSION_TYPE="${VERSION_TYPE:-lts}" # lts or sts
CHECK_ONLY="${CHECK_ONLY:-false}"
JSON_OUTPUT="${JSON_OUTPUT:-false}"

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

# Function to get latest version based on type
get_latest_version() {
    local version_type="$1"
    local retries=3
    local delay=2
    local version=""

    for i in $(seq 1 $retries); do
        # Fetch all non-prerelease releases
        local releases
        releases=$(gh_api https://api.github.com/repos/portainer/portainer/releases) || releases=""

        if [ -z "$releases" ]; then
            [ $i -lt $retries ] && log "Retry $i/$retries..." >&2
            sleep $delay
            continue
        fi

        # LTS/STS releases are marked as such in the release NAME (not by any
        # version-number pattern). Take the HIGHEST matching version, not the
        # first in publish order: Portainer patches two LTS lines at once
        # (2.39.8 LTS was published two hours before 2.45.1 LTS on 2026-09-16),
        # and `head -1` on that list would have proposed a downgrade to 2.39.8.
        version=$(echo "$releases" | \
            jq -r --arg t "${version_type^^}" \
                '.[] | select(.prerelease == false) | select(.name | test($t; "i")) | .tag_name' 2>/dev/null | \
            grep -E '^[0-9]+\.[0-9]+\.[0-9]+$' | \
            sort -V | tail -1)

        if [ -n "$version" ]; then
            echo "$version"
            return 0
        fi

        [ $i -lt $retries ] && log "Retry $i/$retries..." >&2
        sleep $delay
    done

    return 1
}

# Function to get changelog for a specific version
get_changelog() {
    local version="$1"
    local changelog=""

    # Fetch release info
    local release_info
    release_info=$(gh_api "https://api.github.com/repos/portainer/portainer/releases/tags/${version}") || release_info=""

    if [ -n "$release_info" ]; then
        # Extract and format changelog
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
    sed -i "s/version: \".*\"/version: \"$new_version\"/" "$app_path/config.yaml"
    log "${GREEN}✓${NC} Updated config.yaml"

    # Update build.yaml
    if [ -f "$app_path/build.yaml" ]; then
        sed -i "s/PORTAINER_VERSION: .*/PORTAINER_VERSION: $new_version/" "$app_path/build.yaml"
        log "${GREEN}✓${NC} Updated build.yaml"
    fi

    # Update Dockerfile
    if [ -f "$app_path/Dockerfile" ]; then
        sed -i "s/ARG PORTAINER_VERSION=.*/ARG PORTAINER_VERSION=$new_version/" "$app_path/Dockerfile"
        log "${GREEN}✓${NC} Updated Dockerfile"
    fi

    # Update README.md - only update specific version references, not all occurrences
    if [ -f "$app_path/README.md" ]; then
        # Update "Currently running Portainer X.X.X" type statements
        sed -i "s/Currently running Portainer [0-9.]*/Currently running Portainer $new_version/g" "$app_path/README.md"
        # Update "running version X.X.X" type statements
        sed -i "s/running version [0-9.]*/running version $new_version/g" "$app_path/README.md"
        # Update version badges/shields if present
        sed -i "s/version-[0-9.]*-/version-$new_version-/g" "$app_path/README.md"
        log "${GREEN}✓${NC} Updated README.md"
    fi

    # Update DOCS.md - only update specific version references, not section headers
    if [ -f "$app_path/DOCS.md" ]; then
        # Update "running version X.X.X" type statements
        sed -i "s/running version [0-9.]*/running version $new_version/g" "$app_path/DOCS.md"
        # Update "Currently running Portainer X.X.X" type statements
        sed -i "s/Currently running Portainer [0-9.]*/Currently running Portainer $new_version/g" "$app_path/DOCS.md"
        log "${GREEN}✓${NC} Updated DOCS.md"
    fi
}

# Function to update changelog
update_changelog() {
    local new_version="$1"
    local app_path="$2"
    local changelog_content="$3"

    if [ -f "$app_path/CHANGELOG.md" ]; then
        # Prepend new version to existing changelog
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
        # Create new changelog
        cat > "$app_path/CHANGELOG.md" << EOF
# Changelog

## $new_version

_$(date +%Y-%m-%d)_

$changelog_content

---

For full release notes, see: https://github.com/portainer/portainer/releases/tag/$new_version
EOF
    fi
    log "${GREEN}✓${NC} Updated CHANGELOG.md"
}

# Main execution
main() {
    log "=== Portainer ${VERSION_TYPE^^} Version Updater ==="

    # Check if we're in the right directory
    if [ ! -f "$APP_PATH/config.yaml" ]; then
        log "${RED}Error: config.yaml not found at $APP_PATH!${NC}" >&2
        exit 1
    fi

    # Get current version
    log "Checking current version..."
    CURRENT_VERSION=$(get_current_version)
    log "Current version: ${YELLOW}$CURRENT_VERSION${NC}"

    # Get latest version
    log "Checking for latest ${VERSION_TYPE^^} release..."
    LATEST_VERSION=$(get_latest_version "$VERSION_TYPE") || LATEST_VERSION=""

    if [ -z "$LATEST_VERSION" ]; then
        if [ "$JSON_OUTPUT" = "true" ]; then
            echo "{\"error\": \"Could not fetch latest ${VERSION_TYPE^^} version from GitHub\"}"
        else
            log "${RED}Error: Could not fetch latest ${VERSION_TYPE^^} version from GitHub${NC}" >&2
        fi
        exit 1
    fi

    log "Latest ${VERSION_TYPE^^} version: ${GREEN}$LATEST_VERSION${NC}"

    # Compare versions
    if [ "$CURRENT_VERSION" = "$LATEST_VERSION" ]; then
        if [ "$JSON_OUTPUT" = "true" ]; then
            echo "{\"current\": \"$CURRENT_VERSION\", \"latest\": \"$LATEST_VERSION\", \"update_available\": false}"
        else
            log "${GREEN}✓ Already on latest ${VERSION_TYPE^^} version!${NC}"
        fi
        exit 0
    fi

    # Never propose a version below the one shipping — see ver_lt in upstream-lib.sh.
    if ver_lt "$LATEST_VERSION" "$CURRENT_VERSION"; then
        echo "Refusing downgrade: upstream's newest release is $LATEST_VERSION, this app ships $CURRENT_VERSION" >&2
        if [ "$JSON_OUTPUT" = "true" ]; then
            echo "{\"current\": \"$CURRENT_VERSION\", \"latest\": \"$LATEST_VERSION\", \"update_available\": false, \"error\": \"upstream's newest release $LATEST_VERSION is older than $CURRENT_VERSION\"}"
        fi
        exit 1
    fi

    # Get changelog
    log "Fetching changelog..."
    CHANGELOG=$(get_changelog "$LATEST_VERSION")

    # If check-only mode, output result and exit
    if [ "$CHECK_ONLY" = "true" ]; then
        if [ "$JSON_OUTPUT" = "true" ]; then
            CHANGELOG_JSON=$(echo "$CHANGELOG" | jq -Rs . 2>/dev/null || echo '""')
            echo "{\"current\": \"$CURRENT_VERSION\", \"latest\": \"$LATEST_VERSION\", \"update_available\": true, \"changelog\": $CHANGELOG_JSON}"
        else
            log "${YELLOW}Update available: $CURRENT_VERSION -> $LATEST_VERSION${NC}"
            log ""
            log "Changelog:"
            log "$CHANGELOG"
        fi
        exit 0
    fi

    # Perform update
    log ""
    log "${YELLOW}Updating from $CURRENT_VERSION to $LATEST_VERSION...${NC}"
    log ""

    update_files "$LATEST_VERSION" "$APP_PATH"
    update_changelog "$LATEST_VERSION" "$APP_PATH" "$CHANGELOG"

    if [ "$JSON_OUTPUT" = "true" ]; then
        echo "{\"success\": true, \"old_version\": \"$CURRENT_VERSION\", \"new_version\": \"$LATEST_VERSION\"}"
    else
        log ""
        log "${GREEN}Update complete!${NC} Version updated from ${YELLOW}$CURRENT_VERSION${NC} to ${GREEN}$LATEST_VERSION${NC}"
    fi
}

# Run main function
main
