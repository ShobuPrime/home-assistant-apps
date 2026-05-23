#!/bin/bash
# Script to check and update Huly version
# Used by GitHub Actions workflow

set -e

# Configuration
ADDON_PATH="${ADDON_PATH:-.}"
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

# Function to get latest version with retry logic
get_latest_version() {
    local retries=3
    local delay=2
    local version=""

    for i in $(seq 1 $retries); do
        # Fetch .template.huly.conf from huly-selfhost repo and extract HULY_VERSION
        version=$(curl -s --connect-timeout 10 "https://raw.githubusercontent.com/hcengineering/huly-selfhost/main/.template.huly.conf" 2>/dev/null | \
            grep "^HULY_VERSION=" | sed 's/HULY_VERSION=//' | sed 's/^v//' | tr -d '[:space:]')

        if [ -n "$version" ]; then
            echo "$version"
            return 0
        fi

        [ $i -lt $retries ] && log "Retry $i/$retries..." >&2
        sleep $delay
    done

    return 1
}

# Function to get changelog from recent commits
get_changelog() {
    local version="$1"
    local changelog=""

    # Try to get recent commit messages from the huly-selfhost repo
    changelog=$(curl -s --connect-timeout 10 "https://api.github.com/repos/hcengineering/huly-selfhost/commits?per_page=5" 2>/dev/null | \
        jq -r '.[].commit.message' 2>/dev/null | head -20)

    if [ -n "$changelog" ] && [ "$changelog" != "null" ]; then
        echo "$changelog"
    else
        echo "Updated to Huly version $version"
    fi
}

# Function to get current version from config.yaml
get_current_version() {
    if [ ! -f "$ADDON_PATH/config.yaml" ]; then
        log "${RED}Error: config.yaml not found at $ADDON_PATH!${NC}" >&2
        exit 1
    fi
    grep "^version:" "$ADDON_PATH/config.yaml" | sed 's/version: *"\(.*\)"/\1/'
}

# Function to update files
update_files() {
    local new_version="$1"
    local addon_path="$2"

    # Update config.yaml
    sed -i "s/version: \".*\"/version: \"$new_version\"/" "$addon_path/config.yaml"
    log "${GREEN}${NC} Updated config.yaml"

    # Update build.yaml
    if [ -f "$addon_path/build.yaml" ]; then
        sed -i "s/HULY_VERSION: .*/HULY_VERSION: $new_version/" "$addon_path/build.yaml"
        log "${GREEN}${NC} Updated build.yaml"
    fi

    # Update Dockerfile
    if [ -f "$addon_path/Dockerfile" ]; then
        sed -i "s/ARG HULY_VERSION=.*/ARG HULY_VERSION=$new_version/" "$addon_path/Dockerfile"
        log "${GREEN}${NC} Updated Dockerfile"
    fi

    # Update README.md - only update specific version references
    if [ -f "$addon_path/README.md" ]; then
        sed -i "s/Currently running Huly [0-9.]*/Currently running Huly $new_version/g" "$addon_path/README.md"
        sed -i "s/running version [0-9.]*/running version $new_version/g" "$addon_path/README.md"
        sed -i "s/version-[0-9.]*-/version-$new_version-/g" "$addon_path/README.md"
        log "${GREEN}${NC} Updated README.md"
    fi

    # Update DOCS.md - only update specific version references
    if [ -f "$addon_path/DOCS.md" ]; then
        sed -i "s/running version [0-9.]*/running version $new_version/g" "$addon_path/DOCS.md"
        sed -i "s/Currently running Huly [0-9.]*/Currently running Huly $new_version/g" "$addon_path/DOCS.md"
        log "${GREEN}${NC} Updated DOCS.md"
    fi
}

# Function to update changelog
update_changelog() {
    local new_version="$1"
    local addon_path="$2"
    local changelog_content="$3"

    if [ -f "$addon_path/CHANGELOG.md" ]; then
        # Prepend new version to existing changelog
        local temp_file=$(mktemp)
        cat > "$temp_file" << EOF
# Changelog

## $new_version

_$(date +%Y-%m-%d)_

$changelog_content

---

$(tail -n +2 "$addon_path/CHANGELOG.md")
EOF
        mv "$temp_file" "$addon_path/CHANGELOG.md"
    else
        # Create new changelog
        cat > "$addon_path/CHANGELOG.md" << EOF
# Changelog

## $new_version

_$(date +%Y-%m-%d)_

$changelog_content

---

For more details, see: https://github.com/hcengineering/huly-selfhost
EOF
    fi
    log "${GREEN}${NC} Updated CHANGELOG.md"
}

# Main execution
main() {
    log "=== Huly Version Updater ==="

    # Check if we're in the right directory
    if [ ! -f "$ADDON_PATH/config.yaml" ]; then
        log "${RED}Error: config.yaml not found at $ADDON_PATH!${NC}" >&2
        exit 1
    fi

    # Get current version
    log "Checking current version..."
    CURRENT_VERSION=$(get_current_version)
    log "Current version: ${YELLOW}$CURRENT_VERSION${NC}"

    # Get latest version
    log "Checking for latest version..."
    LATEST_VERSION=$(get_latest_version)

    if [ -z "$LATEST_VERSION" ]; then
        if [ "$JSON_OUTPUT" = "true" ]; then
            echo "{\"error\": \"Could not fetch latest version from huly-selfhost\"}"
        else
            log "${RED}Error: Could not fetch latest version from huly-selfhost${NC}" >&2
        fi
        exit 1
    fi

    log "Latest version: ${GREEN}$LATEST_VERSION${NC}"

    # Compare versions
    if [ "$CURRENT_VERSION" = "$LATEST_VERSION" ]; then
        if [ "$JSON_OUTPUT" = "true" ]; then
            echo "{\"current\": \"$CURRENT_VERSION\", \"latest\": \"$LATEST_VERSION\", \"update_available\": false}"
        else
            log "${GREEN} Already on latest version!${NC}"
        fi
        exit 0
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

    update_files "$LATEST_VERSION" "$ADDON_PATH"
    update_changelog "$LATEST_VERSION" "$ADDON_PATH" "$CHANGELOG"

    if [ "$JSON_OUTPUT" = "true" ]; then
        echo "{\"success\": true, \"old_version\": \"$CURRENT_VERSION\", \"new_version\": \"$LATEST_VERSION\"}"
    else
        log ""
        log "${GREEN}Update complete!${NC} Version updated from ${YELLOW}$CURRENT_VERSION${NC} to ${GREEN}$LATEST_VERSION${NC}"
    fi
}

# Run main function
main
