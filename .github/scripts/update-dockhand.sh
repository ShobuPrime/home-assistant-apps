#!/bin/bash
# Script to check and update Dockhand version
# Used by GitHub Actions workflow
#
# Dockhand publishes version info in their changelog.json file:
# https://raw.githubusercontent.com/Finsys/dockhand/main/src/lib/data/changelog.json

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

# Function to check if Docker image tag exists
docker_tag_exists() {
    local tag="$1"
    local response

    response=$(curl -s --connect-timeout 10 \
        "https://hub.docker.com/v2/repositories/fnsys/dockhand/tags/v${tag}" 2>/dev/null)

    if echo "$response" | jq -e '.name' >/dev/null 2>&1; then
        return 0
    fi
    return 1
}

# Function to get latest version from changelog.json
# Only returns versions that have published Docker images
get_latest_version() {
    local retries=3
    local delay=2

    for i in $(seq 1 $retries); do
        # Fetch changelog.json and extract all versions
        local changelog_json=$(curl -s --connect-timeout 10 \
            "https://raw.githubusercontent.com/Finsys/dockhand/main/src/lib/data/changelog.json" 2>/dev/null)

        if [ -n "$changelog_json" ]; then
            # Get versions from changelog (newest first)
            local versions=$(echo "$changelog_json" | jq -r '.[].version' 2>/dev/null)

            # Find the first version that has a published Docker image
            for version in $versions; do
                if docker_tag_exists "$version"; then
                    echo "$version"
                    return 0
                fi
                log "  Version $version not yet published to Docker Hub, checking older..." >&2
            done
        fi

        [ $i -lt $retries ] && log "Retry $i/$retries..." >&2
        sleep $delay
    done

    return 1
}

# Function to get changelog for a specific version
# Prefers GitHub release notes (richer content, security advisories) over changelog.json
get_changelog() {
    local version="$1"
    local changelog=""

    # Try GitHub release notes first (has security advisories, detailed descriptions)
    local gh_release
    gh_release=$(curl -s --connect-timeout 10 \
        "https://api.github.com/repos/Finsys/dockhand/releases/tags/v${version}" 2>/dev/null)

    if [ -n "$gh_release" ] && echo "$gh_release" | jq -e '.body' >/dev/null 2>&1; then
        changelog=$(echo "$gh_release" | jq -r '.body // empty' 2>/dev/null)
        local pub_date=$(echo "$gh_release" | jq -r '.published_at // empty' 2>/dev/null | cut -dT -f1)
        if [ -n "$pub_date" ] && [ -n "$changelog" ]; then
            changelog="Released: ${pub_date}\n\n${changelog}"
        fi
    fi

    # Fallback to changelog.json if no GitHub release
    if [ -z "$changelog" ]; then
        local changelog_json
        changelog_json=$(curl -s --connect-timeout 10 \
            "https://raw.githubusercontent.com/Finsys/dockhand/main/src/lib/data/changelog.json" 2>/dev/null)

        if [ -n "$changelog_json" ]; then
            local entry
            entry=$(echo "$changelog_json" | jq -r --arg v "$version" '.[] | select(.version == $v)' 2>/dev/null)

            if [ -n "$entry" ]; then
                local date=$(echo "$entry" | jq -r '.date // "Unknown date"')
                local changes=$(echo "$entry" | jq -r '.changes[]? | "- \(.type): \(.text)"' 2>/dev/null | head -20)

                changelog="Release date: $date"
                if [ -n "$changes" ]; then
                    changelog="$changelog\n\n$changes"
                fi
            fi
        fi
    fi

    if [ -n "$changelog" ]; then
        echo -e "$changelog"
    else
        echo "No changelog available for version $version"
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
    log "${GREEN}✓${NC} Updated config.yaml"

    # Update build.yaml
    if [ -f "$addon_path/build.yaml" ]; then
        sed -i "s/DOCKHAND_VERSION: .*/DOCKHAND_VERSION: $new_version/" "$addon_path/build.yaml"
        log "${GREEN}✓${NC} Updated build.yaml"
    fi

    # Update Dockerfile
    if [ -f "$addon_path/Dockerfile" ]; then
        sed -i "s/ARG DOCKHAND_VERSION=.*/ARG DOCKHAND_VERSION=$new_version/" "$addon_path/Dockerfile"
        log "${GREEN}✓${NC} Updated Dockerfile"
    fi

    # Update README.md - only update specific version references
    if [ -f "$addon_path/README.md" ]; then
        sed -i "s/Currently running Dockhand [0-9.]*/Currently running Dockhand $new_version/g" "$addon_path/README.md"
        sed -i "s/running version [0-9.]*/running version $new_version/g" "$addon_path/README.md"
        sed -i "s/version-[0-9.]*-/version-$new_version-/g" "$addon_path/README.md"
        log "${GREEN}✓${NC} Updated README.md"
    fi

    # Update DOCS.md - only update specific version references
    if [ -f "$addon_path/DOCS.md" ]; then
        sed -i "s/running version [0-9.]*/running version $new_version/g" "$addon_path/DOCS.md"
        sed -i "s/Currently running Dockhand [0-9.]*/Currently running Dockhand $new_version/g" "$addon_path/DOCS.md"
        log "${GREEN}✓${NC} Updated DOCS.md"
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

## Version $new_version ($(date +%Y-%m-%d))

### Changed
- Updated Dockhand to version $new_version

$changelog_content

---

$(tail -n +2 "$addon_path/CHANGELOG.md")
EOF
        mv "$temp_file" "$addon_path/CHANGELOG.md"
    else
        # Create new changelog
        cat > "$addon_path/CHANGELOG.md" << EOF
# Changelog

## Version $new_version ($(date +%Y-%m-%d))

### Changed
- Updated Dockhand to version $new_version

$changelog_content

---

For full release notes, see: https://dockhand.pro/changelog
EOF
    fi
    log "${GREEN}✓${NC} Updated CHANGELOG.md"
}

# Main execution
main() {
    log "=== Dockhand Version Updater ==="

    # Check if we're in the right directory
    if [ ! -f "$ADDON_PATH/config.yaml" ]; then
        log "${RED}Error: config.yaml not found at $ADDON_PATH!${NC}" >&2
        exit 1
    fi

    # Get current version
    log "Checking current version..."
    CURRENT_VERSION=$(get_current_version)
    log "Current version: ${YELLOW}$CURRENT_VERSION${NC}"

    # Get latest version from changelog.json
    log "Checking for latest release from changelog.json..."
    LATEST_VERSION=$(get_latest_version)

    if [ -z "$LATEST_VERSION" ]; then
        if [ "$JSON_OUTPUT" = "true" ]; then
            echo "{\"error\": \"Could not fetch latest version from changelog.json\"}"
        else
            log "${RED}Error: Could not fetch latest version from changelog.json${NC}" >&2
        fi
        exit 1
    fi

    log "Latest version: ${GREEN}$LATEST_VERSION${NC}"

    # Compare versions
    if [ "$CURRENT_VERSION" = "$LATEST_VERSION" ]; then
        if [ "$JSON_OUTPUT" = "true" ]; then
            echo "{\"current\": \"$CURRENT_VERSION\", \"latest\": \"$LATEST_VERSION\", \"update_available\": false}"
        else
            log "${GREEN}✓ Already on latest version!${NC}"
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
