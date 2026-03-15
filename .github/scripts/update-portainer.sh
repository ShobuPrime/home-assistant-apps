#!/bin/bash
# Script to check and update Portainer version
# Supports both LTS and STS releases (detected by release name, not version pattern)

set -e

# Configuration
ADDON_PATH="${ADDON_PATH:-.}"
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
        local releases=$(curl -s --connect-timeout 10 https://api.github.com/repos/portainer/portainer/releases 2>/dev/null)

        if [ -z "$releases" ]; then
            [ $i -lt $retries ] && log "Retry $i/$retries..." >&2
            sleep $delay
            continue
        fi

        if [ "$version_type" = "lts" ]; then
            # LTS versions are explicitly marked with "LTS" in the release name
            # Filter by release name containing "LTS" (case-insensitive)
            version=$(echo "$releases" | \
                jq -r '.[] | select(.prerelease == false) | select(.name | test("LTS"; "i")) | .tag_name' 2>/dev/null | \
                grep -E '^[0-9]+\.[0-9]+\.[0-9]+$' | \
                head -1)
        else
            # STS versions are explicitly marked with "STS" in the release name
            # Filter by release name containing "STS" (case-insensitive)
            version=$(echo "$releases" | \
                jq -r '.[] | select(.prerelease == false) | select(.name | test("STS"; "i")) | .tag_name' 2>/dev/null | \
                grep -E '^[0-9]+\.[0-9]+\.[0-9]+$' | \
                head -1)
        fi

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
    local release_info=$(curl -s --connect-timeout 10 "https://api.github.com/repos/portainer/portainer/releases/tags/${version}" 2>/dev/null)

    if [ -n "$release_info" ]; then
        # Extract and format changelog
        changelog=$(echo "$release_info" | jq -r '.body // "No changelog available"' 2>/dev/null)

        # Limit to first 1000 characters and clean up
        changelog=$(echo "$changelog" | head -c 1000 | sed 's/\r//g' | sed 's/@\([a-zA-Z0-9_-]*\)/\1/g')

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
        sed -i "s/PORTAINER_VERSION: .*/PORTAINER_VERSION: $new_version/" "$addon_path/build.yaml"
        log "${GREEN}✓${NC} Updated build.yaml"
    fi

    # Update Dockerfile
    if [ -f "$addon_path/Dockerfile" ]; then
        sed -i "s/ARG PORTAINER_VERSION=.*/ARG PORTAINER_VERSION=$new_version/" "$addon_path/Dockerfile"
        log "${GREEN}✓${NC} Updated Dockerfile"
    fi

    # Update README.md - only update specific version references, not all occurrences
    if [ -f "$addon_path/README.md" ]; then
        # Update "Currently running Portainer X.X.X" type statements
        sed -i "s/Currently running Portainer [0-9.]*/Currently running Portainer $new_version/g" "$addon_path/README.md"
        # Update "running version X.X.X" type statements
        sed -i "s/running version [0-9.]*/running version $new_version/g" "$addon_path/README.md"
        # Update version badges/shields if present
        sed -i "s/version-[0-9.]*-/version-$new_version-/g" "$addon_path/README.md"
        log "${GREEN}✓${NC} Updated README.md"
    fi

    # Update DOCS.md - only update specific version references, not section headers
    if [ -f "$addon_path/DOCS.md" ]; then
        # Update "running version X.X.X" type statements
        sed -i "s/running version [0-9.]*/running version $new_version/g" "$addon_path/DOCS.md"
        # Update "Currently running Portainer X.X.X" type statements
        sed -i "s/Currently running Portainer [0-9.]*/Currently running Portainer $new_version/g" "$addon_path/DOCS.md"
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
    if [ ! -f "$ADDON_PATH/config.yaml" ]; then
        log "${RED}Error: config.yaml not found at $ADDON_PATH!${NC}" >&2
        exit 1
    fi

    # Get current version
    log "Checking current version..."
    CURRENT_VERSION=$(get_current_version)
    log "Current version: ${YELLOW}$CURRENT_VERSION${NC}"

    # Get latest version
    log "Checking for latest ${VERSION_TYPE^^} release..."
    LATEST_VERSION=$(get_latest_version "$VERSION_TYPE")

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
