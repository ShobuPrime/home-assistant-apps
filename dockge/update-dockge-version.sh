#!/bin/bash
# Script to check and update Dockge version
# Supports --check-only mode for automations

set -e

# Script modes
CHECK_ONLY=false
JSON_OUTPUT=false
SILENT=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --check-only)
            CHECK_ONLY=true
            shift
            ;;
        --json)
            JSON_OUTPUT=true
            SILENT=true
            shift
            ;;
        --silent)
            SILENT=true
            shift
            ;;
        -y|--yes)
            AUTO_MODE=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS]"
            echo "Options:"
            echo "  --check-only  Only check for updates, don't apply them"
            echo "  --json        Output results in JSON format"
            echo "  --silent      Suppress all output except errors"
            echo "  -y, --yes     Auto-confirm update without prompting"
            echo "  -h, --help    Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

[[ "$SILENT" != "true" ]] && echo "=== Dockge Version Updater ==="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to get latest version with retry logic
get_latest_version() {
    local retries=3
    local delay=2
    local version=""
    
    for i in $(seq 1 $retries); do
        # Get latest release from Dockge GitHub
        version=$(curl -s --connect-timeout 10 https://api.github.com/repos/louislam/dockge/releases/latest 2>/dev/null | \
            jq -r '.tag_name // empty' 2>/dev/null)
        
        if [ -n "$version" ]; then
            echo "$version"
            return 0
        fi
        
        [[ "$SILENT" != "true" ]] && [[ $i -lt $retries ]] && echo "Retry $i/$retries..." >&2
        sleep $delay
    done
    
    return 1
}

# Function to get changelog for a specific version
get_changelog() {
    local version="$1"
    local changelog=""
    
    # Fetch release info
    local release_info=$(curl -s --connect-timeout 10 "https://api.github.com/repos/louislam/dockge/releases/tags/${version}" 2>/dev/null)
    
    if [ -n "$release_info" ]; then
        # Extract and format changelog
        changelog=$(echo "$release_info" | jq -r '.body // "No changelog available"' 2>/dev/null)
        
        # Limit to first 1000 characters and clean up
        changelog=$(echo "$changelog" | head -c 1000 | sed 's///g')
        
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
    if [ ! -f "config.yaml" ]; then
        echo -e "${RED}Error: config.yaml not found!${NC}"
        exit 1
    fi
    grep "^version:" config.yaml | cut -d'"' -f2
}

# Set default for AUTO_MODE if not set by arguments
AUTO_MODE=${AUTO_MODE:-false}

# Check if we're in the right directory
if [ ! -f "config.yaml" ] || [ ! -f "apparmor.txt" ]; then
    echo -e "${RED}Error: This script must be run from the addon directory!${NC}"
    echo "Expected files not found: config.yaml, apparmor.txt"
    exit 1
fi

# Check current version
[[ "$SILENT" != "true" ]] && echo "Checking current version..."
CURRENT_VERSION=$(get_current_version)
[[ "$SILENT" != "true" ]] && echo -e "Current version: ${YELLOW}$CURRENT_VERSION${NC}"

# Check latest version
[[ "$SILENT" != "true" ]] && echo "Checking for latest release..."
LATEST_VERSION=$(get_latest_version)

if [ -z "$LATEST_VERSION" ]; then
    if [ "$JSON_OUTPUT" = "true" ]; then
        echo '{"error": "Could not fetch latest version from GitHub"}'
    else
        echo -e "${RED}Error: Could not fetch latest version from GitHub${NC}" >&2
        echo "Please check your internet connection and try again" >&2
    fi
    exit 1
fi

[[ "$SILENT" != "true" ]] && echo -e "Latest version: ${GREEN}$LATEST_VERSION${NC}"

# Compare versions
if [ "$CURRENT_VERSION" = "$LATEST_VERSION" ]; then
    if [ "$JSON_OUTPUT" = "true" ]; then
        echo "{\"current\": \"$CURRENT_VERSION\", \"latest\": \"$LATEST_VERSION\", \"update_available\": false}"
    else
        [[ "$SILENT" != "true" ]] && echo -e "${GREEN}✓ Already on latest version!${NC}"
    fi
    exit 0
fi

# If check-only mode, output result and exit
if [ "$CHECK_ONLY" = "true" ]; then
    if [ "$JSON_OUTPUT" = "true" ]; then
        # Get changelog for JSON output
        CHANGELOG=$(get_changelog "$LATEST_VERSION")
        # Escape for JSON
        CHANGELOG_JSON=$(echo "$CHANGELOG" | jq -Rs . 2>/dev/null || echo '""')
        echo "{\"current\": \"$CURRENT_VERSION\", \"latest\": \"$LATEST_VERSION\", \"update_available\": true, \"changelog\": $CHANGELOG_JSON}"
    else
        echo -e "${YELLOW}Update available: $CURRENT_VERSION -> $LATEST_VERSION${NC}"
    fi
    exit 0
fi

[[ "$SILENT" != "true" ]] && echo ""
[[ "$SILENT" != "true" ]] && echo -e "${YELLOW}Update available: $CURRENT_VERSION -> $LATEST_VERSION${NC}"
[[ "$SILENT" != "true" ]] && echo ""

# Ask for confirmation if not in auto mode
if [ "$AUTO_MODE" = false ]; then
    read -p "Do you want to update to $LATEST_VERSION? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Update cancelled"
        exit 0
    fi
fi

[[ "$SILENT" != "true" ]] && echo ""
[[ "$SILENT" != "true" ]] && echo "Updating to $LATEST_VERSION..."

# Create backups with timestamp
BACKUP_SUFFIX=".bak.$(date +%Y%m%d_%H%M%S)"
cp config.yaml "config.yaml${BACKUP_SUFFIX}"

# Update config.yaml
sed -i "s/version: \".*\"/version: \"$LATEST_VERSION\"/" config.yaml
echo -e "${GREEN}✓${NC} Updated config.yaml"

# Update README.md if it exists
if [ -f README.md ]; then
    cp README.md "README.md${BACKUP_SUFFIX}"
    sed -i "s/version-v[0-9.]*/version-v$LATEST_VERSION/g" README.md
    echo -e "${GREEN}✓${NC} Updated README.md"
fi

# Create/update CHANGELOG.md
[[ "$SILENT" != "true" ]] && echo "Fetching changelog..."
CHANGELOG_CONTENT=$(get_changelog "$LATEST_VERSION")
if [ -f CHANGELOG.md ]; then
    cp CHANGELOG.md "CHANGELOG.md${BACKUP_SUFFIX}"
fi
cat > CHANGELOG.md << EOF
# Changelog

## Version $LATEST_VERSION

$CHANGELOG_CONTENT

---

For full release notes, see: https://github.com/louislam/dockge/releases/tag/$LATEST_VERSION
EOF
echo -e "${GREEN}✓${NC} Updated CHANGELOG.md"

if [ "$JSON_OUTPUT" = "true" ]; then
    echo "{\"success\": true, \"version\": \"$LATEST_VERSION\", \"backups_created\": true}"
else
    [[ "$SILENT" != "true" ]] && echo ""
    [[ "$SILENT" != "true" ]] && echo -e "${GREEN}Update complete!${NC} Version updated to ${GREEN}$LATEST_VERSION${NC}"
    [[ "$SILENT" != "true" ]] && echo ""
    [[ "$SILENT" != "true" ]] && echo "Backup files created with timestamp: ${BACKUP_SUFFIX}"
fi

# If running in Home Assistant, offer to reload supervisor
if command -v ha &> /dev/null; then
    echo ""
    if [ "$AUTO_MODE" = true ]; then
        echo "Reloading Home Assistant Supervisor..."
        ha supervisor reload
        echo -e "${GREEN}✓${NC} Supervisor reloaded"
    else
        read -p "Reload Home Assistant Supervisor now? (y/N) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            echo "Reloading supervisor..."
            ha supervisor reload
            echo -e "${GREEN}✓${NC} Supervisor reloaded"
        fi
    fi
fi

echo ""
echo -e "${YELLOW}Note:${NC} The addon will show as having an update available in Home Assistant."
echo "You can now update it through the Home Assistant UI."

# Clean up old backups (keep only the 3 most recent for each file)
for base in config.yaml README.md CHANGELOG.md; do
    ls -t "${base}.bak."* 2>/dev/null | tail -n +4 | xargs -r rm 2>/dev/null || true
done

exit 0