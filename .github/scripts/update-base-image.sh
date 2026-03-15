#!/bin/bash
# Script to check and update hassio-addons base image version across all addons
# Used by GitHub Actions workflow

set -e

# Configuration
REPO_ROOT="${REPO_ROOT:-.}"
CHECK_ONLY="${CHECK_ONLY:-false}"
JSON_OUTPUT="${JSON_OUTPUT:-false}"

# Source repo for the base image
BASE_IMAGE_REPO="hassio-addons/app-base"
BASE_IMAGE_REGISTRY="ghcr.io/hassio-addons/base"

# All addon directories that use the base image
ADDON_DIRS="arcane dockge dockhand huly muninndb portainer_ee_lts portainer_ee_sts"

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
        # Get latest release from hassio-addons/app-base
        version=$(curl -s --connect-timeout 10 "https://api.github.com/repos/${BASE_IMAGE_REPO}/releases/latest" 2>/dev/null | \
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

# Function to get changelog for a specific version
get_changelog() {
    local version="$1"
    local changelog=""

    # Fetch release info (try with 'v' prefix first)
    local release_info=$(curl -s --connect-timeout 10 "https://api.github.com/repos/${BASE_IMAGE_REPO}/releases/tags/v${version}" 2>/dev/null)

    if [ -z "$release_info" ] || [ "$(echo "$release_info" | jq -r '.message // empty')" = "Not Found" ]; then
        release_info=$(curl -s --connect-timeout 10 "https://api.github.com/repos/${BASE_IMAGE_REPO}/releases/tags/${version}" 2>/dev/null)
    fi

    if [ -n "$release_info" ]; then
        changelog=$(echo "$release_info" | jq -r '.body // "No changelog available"' 2>/dev/null)
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

# Function to get current base image version from any addon's build.yaml
get_current_version() {
    local first_addon=""
    for addon in $ADDON_DIRS; do
        if [ -f "$REPO_ROOT/$addon/build.yaml" ]; then
            first_addon="$addon"
            break
        fi
    done

    if [ -z "$first_addon" ]; then
        log "${RED}Error: No addon build.yaml files found!${NC}" >&2
        exit 1
    fi

    # Extract version from base image reference (use | delimiter to avoid conflicts with / in URLs)
    grep "${BASE_IMAGE_REGISTRY}" "$REPO_ROOT/$first_addon/build.yaml" | head -1 | \
        sed "s|.*${BASE_IMAGE_REGISTRY}:\([0-9.]*\).*|\1|"
}

# Function to detect if this is a major version bump
is_major_bump() {
    local current="$1"
    local latest="$2"
    local current_major="${current%%.*}"
    local latest_major="${latest%%.*}"
    [ "$current_major" != "$latest_major" ]
}

# Function to update all addon files
update_files() {
    local current_version="$1"
    local new_version="$2"

    local old_image="${BASE_IMAGE_REGISTRY}:${current_version}"
    local new_image="${BASE_IMAGE_REGISTRY}:${new_version}"

    # Update build.yaml in all addon directories
    for addon in $ADDON_DIRS; do
        local build_file="$REPO_ROOT/$addon/build.yaml"
        if [ -f "$build_file" ]; then
            sed -i "s|${old_image}|${new_image}|g" "$build_file"
            log "${GREEN}${NC} Updated $addon/build.yaml"
        fi
    done

    # Update any Dockerfiles with inline BUILD_FROM defaults
    for addon in $ADDON_DIRS; do
        local dockerfile="$REPO_ROOT/$addon/Dockerfile"
        if [ -f "$dockerfile" ]; then
            if grep -q "ARG BUILD_FROM=${BASE_IMAGE_REGISTRY}" "$dockerfile"; then
                sed -i "s|ARG BUILD_FROM=${old_image}|ARG BUILD_FROM=${new_image}|g" "$dockerfile"
                log "${GREEN}${NC} Updated $addon/Dockerfile inline BUILD_FROM"
            fi
        fi
    done
}

# Main execution
main() {
    log "=== Hassio-Addons Base Image Updater ==="

    # Verify we can find at least one addon
    local found_addon=false
    for addon in $ADDON_DIRS; do
        if [ -f "$REPO_ROOT/$addon/build.yaml" ]; then
            found_addon=true
            break
        fi
    done

    if [ "$found_addon" = "false" ]; then
        if [ "$JSON_OUTPUT" = "true" ]; then
            echo "{\"error\": \"No addon build.yaml files found in $REPO_ROOT\"}"
        else
            log "${RED}Error: No addon build.yaml files found in $REPO_ROOT!${NC}" >&2
        fi
        exit 1
    fi

    # Get current version
    log "Checking current base image version..."
    CURRENT_VERSION=$(get_current_version)
    log "Current version: ${YELLOW}$CURRENT_VERSION${NC}"

    # Get latest version
    log "Checking for latest release..."
    LATEST_VERSION=$(get_latest_version)

    if [ -z "$LATEST_VERSION" ]; then
        if [ "$JSON_OUTPUT" = "true" ]; then
            echo "{\"error\": \"Could not fetch latest version from GitHub\"}"
        else
            log "${RED}Error: Could not fetch latest version from GitHub${NC}" >&2
        fi
        exit 1
    fi

    log "Latest version: ${GREEN}$LATEST_VERSION${NC}"

    # Compare versions
    if [ "$CURRENT_VERSION" = "$LATEST_VERSION" ]; then
        if [ "$JSON_OUTPUT" = "true" ]; then
            echo "{\"current\": \"$CURRENT_VERSION\", \"latest\": \"$LATEST_VERSION\", \"update_available\": false, \"major_bump\": false}"
        else
            log "${GREEN} Already on latest version!${NC}"
        fi
        exit 0
    fi

    # Check for major version bump
    MAJOR_BUMP="false"
    if is_major_bump "$CURRENT_VERSION" "$LATEST_VERSION"; then
        MAJOR_BUMP="true"
        log "${YELLOW}Major version bump detected: ${CURRENT_VERSION%%.*} -> ${LATEST_VERSION%%.*}${NC}"
    fi

    # Get changelog
    log "Fetching changelog..."
    CHANGELOG=$(get_changelog "$LATEST_VERSION")

    # If check-only mode, output result and exit
    if [ "$CHECK_ONLY" = "true" ]; then
        if [ "$JSON_OUTPUT" = "true" ]; then
            CHANGELOG_JSON=$(echo "$CHANGELOG" | jq -Rs . 2>/dev/null || echo '""')
            echo "{\"current\": \"$CURRENT_VERSION\", \"latest\": \"$LATEST_VERSION\", \"update_available\": true, \"major_bump\": $MAJOR_BUMP, \"changelog\": $CHANGELOG_JSON}"
        else
            log "${YELLOW}Update available: $CURRENT_VERSION -> $LATEST_VERSION${NC}"
            if [ "$MAJOR_BUMP" = "true" ]; then
                log "${YELLOW}This is a MAJOR version bump - review for breaking changes!${NC}"
            fi
            log ""
            log "Changelog:"
            log "$CHANGELOG"
        fi
        exit 0
    fi

    # Perform update
    log ""
    log "${YELLOW}Updating base image from $CURRENT_VERSION to $LATEST_VERSION...${NC}"
    log ""

    update_files "$CURRENT_VERSION" "$LATEST_VERSION"

    if [ "$JSON_OUTPUT" = "true" ]; then
        echo "{\"success\": true, \"old_version\": \"$CURRENT_VERSION\", \"new_version\": \"$LATEST_VERSION\", \"major_bump\": $MAJOR_BUMP}"
    else
        log ""
        log "${GREEN}Update complete!${NC} Base image updated from ${YELLOW}$CURRENT_VERSION${NC} to ${GREEN}$LATEST_VERSION${NC}"
        log ""
        log "Updated addons:"
        for addon in $ADDON_DIRS; do
            if [ -f "$REPO_ROOT/$addon/build.yaml" ]; then
                log "  - $addon"
            fi
        done
    fi
}

# Run main function
main
