#!/bin/bash
# Update script for Dockhand addon
# Checks for new Dockhand releases via changelog.json and updates version files
#
# Dockhand publishes version info in their changelog.json file:
# https://raw.githubusercontent.com/Finsys/dockhand/main/src/lib/data/changelog.json

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Flags
CHECK_ONLY=false
AUTO_YES=false
JSON_OUTPUT=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --check-only)
            CHECK_ONLY=true
            shift
            ;;
        --yes|-y)
            AUTO_YES=true
            shift
            ;;
        --json)
            JSON_OUTPUT=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [--check-only] [--yes] [--json]"
            exit 1
            ;;
    esac
done

# Get the directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

# Check if we're in the right directory
if [ ! -f "config.yaml" ] || [ ! -f "build.yaml" ]; then
    echo -e "${RED}Error: config.yaml or build.yaml not found!${NC}"
    exit 1
fi

# Get current version from config.yaml
CURRENT_VERSION=$(grep "^version:" config.yaml | cut -d'"' -f2)

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

# Fetch changelog.json and find latest version with published Docker image
CHANGELOG_JSON=$(curl -s "https://raw.githubusercontent.com/Finsys/dockhand/main/src/lib/data/changelog.json")
ALL_VERSIONS=$(echo "${CHANGELOG_JSON}" | jq -r '.[].version')

LATEST_VERSION=""
for version in ${ALL_VERSIONS}; do
    if docker_tag_exists "$version"; then
        LATEST_VERSION="$version"
        break
    fi
    echo -e "${YELLOW}Version $version not yet published to Docker Hub, checking older...${NC}"
done

if [ -z "${LATEST_VERSION}" ]; then
    echo -e "${RED}Error: Could not find any published Dockhand version from changelog.json${NC}"
    exit 1
fi

# JSON output mode
if [ "${JSON_OUTPUT}" = true ]; then
    echo "{\"current_version\":\"${CURRENT_VERSION}\",\"latest_version\":\"${LATEST_VERSION}\",\"update_available\":$([ "${CURRENT_VERSION}" != "${LATEST_VERSION}" ] && echo "true" || echo "false")}"
    exit 0
fi

echo -e "${BLUE}=== Dockhand Version Checker ===${NC}"
echo -e "Current version: ${YELLOW}${CURRENT_VERSION}${NC}"
echo -e "Latest version:  ${GREEN}${LATEST_VERSION}${NC}"

# Check if update is needed
if [ "${CURRENT_VERSION}" = "${LATEST_VERSION}" ]; then
    echo -e "${GREEN}✓ Already up to date!${NC}"
    exit 0
fi

echo -e "${YELLOW}Update available!${NC}"

# Get changelog entry for the version we're updating to
RELEASE_DATE=$(echo "${CHANGELOG_JSON}" | jq -r --arg v "${LATEST_VERSION}" '.[] | select(.version == $v) | .date')
CHANGES=$(echo "${CHANGELOG_JSON}" | jq -r --arg v "${LATEST_VERSION}" '.[] | select(.version == $v) | .changes[]?' | head -10)

echo ""
echo -e "Release date: ${RELEASE_DATE}"
if [ -n "${CHANGES}" ]; then
    echo "Changes:"
    echo "${CHANGES}" | while read -r change; do
        echo "  - ${change}"
    done
fi

if [ "${CHECK_ONLY}" = true ]; then
    echo ""
    echo "Run without --check-only to apply the update."
    exit 0
fi

# Confirm update
if [ "${AUTO_YES}" != true ]; then
    echo ""
    read -p "Apply update to version ${LATEST_VERSION}? [y/N] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Update cancelled."
        exit 0
    fi
fi

echo -e "${BLUE}Updating to version ${LATEST_VERSION}...${NC}"

# Update config.yaml
sed -i "s/^version: \".*\"/version: \"${LATEST_VERSION}\"/" config.yaml
echo -e "  ${GREEN}✓${NC} Updated config.yaml"

# Update build.yaml
sed -i "s/DOCKHAND_VERSION: .*/DOCKHAND_VERSION: ${LATEST_VERSION}/" build.yaml
echo -e "  ${GREEN}✓${NC} Updated build.yaml"

# Update Dockerfile
sed -i "s/ARG DOCKHAND_VERSION=.*/ARG DOCKHAND_VERSION=${LATEST_VERSION}/" Dockerfile
echo -e "  ${GREEN}✓${NC} Updated Dockerfile"

# Update CHANGELOG.md
CHANGELOG_ENTRY="## [${LATEST_VERSION}] - $(date +%Y-%m-%d)

### Changed
- Updated Dockhand to version ${LATEST_VERSION}

Release date: ${RELEASE_DATE}
"

if [ -n "${CHANGES}" ]; then
    CHANGELOG_ENTRY="${CHANGELOG_ENTRY}
Changes from upstream:
$(echo "${CHANGES}" | while read -r change; do echo "- ${change}"; done)
"
fi

# Create temp file with new changelog entry
TEMP_FILE=$(mktemp)
echo "${CHANGELOG_ENTRY}" > "${TEMP_FILE}"

# Append existing changelog (skip first lines)
tail -n +3 CHANGELOG.md >> "${TEMP_FILE}"

# Add header back
echo -e "# Changelog\n\nAll notable changes to this project will be documented in this file.\n" | cat - "${TEMP_FILE}" > CHANGELOG.md
rm "${TEMP_FILE}"

echo -e "  ${GREEN}✓${NC} Updated CHANGELOG.md"

echo ""
echo -e "${GREEN}✓ Update complete!${NC}"
echo ""
echo "Next steps:"
echo "  1. Review the changes: git diff"
echo "  2. Test the build: ./build.sh"
echo "  3. Commit the changes: git add -A && git commit -m 'Update Dockhand to ${LATEST_VERSION}'"
