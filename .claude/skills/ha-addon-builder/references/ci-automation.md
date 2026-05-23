# CI/CD Automation Reference

This file contains templates for automated version update scripts and GitHub Actions workflows. These integrate the new addon into the repository's existing automation pipeline.

## Table of Contents

1. [Update Script Template](#update-script-template)
2. [Update Workflow Template](#update-workflow-template)
3. [Base Image Script Integration](#base-image-script-integration)
4. [Version Source Patterns](#version-source-patterns)

---

## Update Script Template

Save as `.github/scripts/update-<addon>.sh`. This script checks for upstream updates and modifies addon files when a new version is available.

The script operates in two modes:
- **Check mode** (`CHECK_ONLY=true`): Only checks if an update is available, outputs JSON
- **Update mode** (`CHECK_ONLY=false`): Actually modifies files in place

```bash
#!/bin/bash
# Update script for <Addon Name> addon
# Usage: CHECK_ONLY=true JSON_OUTPUT=true bash update-<addon>.sh

set -e

# Configuration
ADDON_PATH="${ADDON_PATH:-.}"
CHECK_ONLY="${CHECK_ONLY:-false}"
JSON_OUTPUT="${JSON_OUTPUT:-false}"
SILENT="${SILENT:-false}"

# Colors (only when not silent)
if [ "$SILENT" = "false" ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    NC='\033[0m'
else
    RED=''
    GREEN=''
    YELLOW=''
    NC=''
fi

log() {
    if [ "$SILENT" = "false" ]; then
        echo -e "$1"
    fi
}

# Get current version from config.yaml
CURRENT_VERSION=$(grep "^version:" "${ADDON_PATH}/config.yaml" | cut -d'"' -f2)
log "Current version: ${YELLOW}${CURRENT_VERSION}${NC}"

# Fetch latest version from upstream
# ===================================
# CUSTOMIZE THIS SECTION for each addon's version source.
# See "Version Source Patterns" section below for examples.
# ===================================

LATEST_VERSION=""
CHANGELOG=""
MAX_RETRIES=3
RETRY_DELAY=2

for i in $(seq 1 $MAX_RETRIES); do
    # --- REPLACE THIS BLOCK with addon-specific version detection ---
    RESPONSE=$(curl -s -f "https://api.github.com/repos/<owner>/<repo>/releases/latest" 2>/dev/null) && break
    log "${YELLOW}Retry $i/$MAX_RETRIES...${NC}"
    sleep $RETRY_DELAY
done

if [ -z "$RESPONSE" ]; then
    log "${RED}Failed to fetch latest version after $MAX_RETRIES attempts${NC}"
    if [ "$JSON_OUTPUT" = "true" ]; then
        echo '{"update_available": false, "error": "Failed to fetch version"}'
    fi
    exit 1
fi

# --- REPLACE with addon-specific version extraction ---
LATEST_VERSION=$(echo "$RESPONSE" | jq -r '.tag_name' | sed 's/^v//')
CHANGELOG=$(echo "$RESPONSE" | jq -r '.body // "No changelog available"')

log "Latest version: ${GREEN}${LATEST_VERSION}${NC}"

# Compare versions
if [ "$CURRENT_VERSION" = "$LATEST_VERSION" ]; then
    log "${GREEN}Already up to date${NC}"
    if [ "$JSON_OUTPUT" = "true" ]; then
        echo "{\"update_available\": false, \"current\": \"${CURRENT_VERSION}\", \"latest\": \"${LATEST_VERSION}\"}"
    fi
    exit 0
fi

log "${YELLOW}Update available: ${CURRENT_VERSION} -> ${LATEST_VERSION}${NC}"

# If check-only mode, output and exit
if [ "$CHECK_ONLY" = "true" ]; then
    if [ "$JSON_OUTPUT" = "true" ]; then
        # Escape changelog for JSON
        ESCAPED_CHANGELOG=$(echo "$CHANGELOG" | jq -Rs '.')
        echo "{\"update_available\": true, \"current\": \"${CURRENT_VERSION}\", \"latest\": \"${LATEST_VERSION}\", \"changelog\": ${ESCAPED_CHANGELOG}}"
    fi
    exit 0
fi

# Apply updates
log "\n${YELLOW}Applying update...${NC}"

# Update config.yaml - version field
sed -i "s/^version: \".*\"/version: \"${LATEST_VERSION}\"/" "${ADDON_PATH}/config.yaml"
log "Updated config.yaml"

# Update build.yaml - version arg
sed -i "s/<ADDON_UPPER>_VERSION: .*/<ADDON_UPPER>_VERSION: ${LATEST_VERSION}/" "${ADDON_PATH}/build.yaml"
log "Updated build.yaml"

# Update Dockerfile - ARG default
sed -i "s/ARG <ADDON_UPPER>_VERSION=.*/ARG <ADDON_UPPER>_VERSION=${LATEST_VERSION}/" "${ADDON_PATH}/Dockerfile"
log "Updated Dockerfile"

# Update README.md - version reference (conservative regex)
sed -i "s/Currently running <Addon Name> [0-9][0-9.]*/Currently running <Addon Name> ${LATEST_VERSION}/" "${ADDON_PATH}/README.md"
log "Updated README.md"

# Update DOCS.md (if it has version references)
# Use conservative regex - only update specific version patterns, never section headers

# Update CHANGELOG.md - prepend new entry
# Format: bare "## X.Y.Z" header (no "Version " prefix, no trailing date)
# so Core's release-notes regex ^#* {version}\n matches and only the
# new-version delta is shown in the HA UI.
DATE=$(date +%Y-%m-%d)
CHANGELOG_ENTRY="## ${LATEST_VERSION}

_${DATE}_

${CHANGELOG}

---

"

# Insert after the first "# Changelog" line
sed -i "/^# Changelog$/a\\
\\
${CHANGELOG_ENTRY}" "${ADDON_PATH}/CHANGELOG.md" 2>/dev/null || {
    # Fallback: prepend with temp file
    TEMP_FILE=$(mktemp)
    echo "# Changelog

${CHANGELOG_ENTRY}" > "$TEMP_FILE"
    tail -n +2 "${ADDON_PATH}/CHANGELOG.md" >> "$TEMP_FILE"
    mv "$TEMP_FILE" "${ADDON_PATH}/CHANGELOG.md"
}
log "Updated CHANGELOG.md"

log "\n${GREEN}Update complete: ${CURRENT_VERSION} -> ${LATEST_VERSION}${NC}"

# If running on Home Assistant, reload supervisor
if command -v ha &> /dev/null; then
    log "Reloading Home Assistant Supervisor..."
    ha supervisor reload || true
fi
```

### Customization Points

Replace these placeholders:
- `<addon>` - addon slug (lowercase, underscores)
- `<Addon Name>` - human-readable name
- `<ADDON_UPPER>` - uppercase version ARG name (e.g., `PORTAINER`, `ARCANE`)
- `<owner>/<repo>` - upstream GitHub repository
- The version detection block (see Version Source Patterns below)

---

## Update Workflow Template

Save as `.github/workflows/update-<addon>.yml`.

```yaml
name: Update <Addon Name>

on:
  schedule:
    # Pick a unique time slot - see SKILL.md for existing schedule
    - cron: '<MM> <HH> * * *'
  workflow_dispatch:
    # Allow manual triggering

permissions:
  contents: write
  pull-requests: write

jobs:
  check-update:
    runs-on: ubuntu-latest
    outputs:
      update_available: ${{ steps.check.outputs.update_available }}
      current_version: ${{ steps.check.outputs.current_version }}
      latest_version: ${{ steps.check.outputs.latest_version }}
      changelog: ${{ steps.check.outputs.changelog }}
    steps:
      - name: Checkout repository
        uses: actions/checkout@v4

      - name: Check for updates
        id: check
        run: |
          cd <addon_slug>
          result=$(ADDON_PATH=. CHECK_ONLY=true JSON_OUTPUT=true bash ../.github/scripts/update-<addon>.sh)
          echo "Result: $result"

          update_available=$(echo "$result" | jq -r '.update_available')
          current=$(echo "$result" | jq -r '.current')
          latest=$(echo "$result" | jq -r '.latest')
          changelog=$(echo "$result" | jq -r '.changelog')

          echo "update_available=$update_available" >> $GITHUB_OUTPUT
          echo "current_version=$current" >> $GITHUB_OUTPUT
          echo "latest_version=$latest" >> $GITHUB_OUTPUT
          echo "changelog<<EOF" >> $GITHUB_OUTPUT
          echo "$changelog" >> $GITHUB_OUTPUT
          echo "EOF" >> $GITHUB_OUTPUT

  update-version:
    needs: check-update
    if: needs.check-update.outputs.update_available == 'true'
    runs-on: ubuntu-latest
    steps:
      - name: Checkout repository
        uses: actions/checkout@v4

      - name: Configure Git
        run: |
          git config --global user.name "github-actions[bot]"
          git config --global user.email "github-actions[bot]@users.noreply.github.com"

      - name: Update <Addon Name>
        id: update
        run: |
          cd <addon_slug>
          ADDON_PATH=. CHECK_ONLY=false JSON_OUTPUT=false bash ../.github/scripts/update-<addon>.sh

      - name: Create Pull Request
        id: create-pr
        uses: peter-evans/create-pull-request@v6
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          sign-commits: true
          commit-message: |
            Update <Addon Name> to ${{ needs.check-update.outputs.latest_version }}

            Automated update from ${{ needs.check-update.outputs.current_version }} to ${{ needs.check-update.outputs.latest_version }}
          branch: update-<addon>-${{ needs.check-update.outputs.latest_version }}
          delete-branch: true
          title: "Update <Addon Name> to ${{ needs.check-update.outputs.latest_version }}"
          body: |
            ## <Addon Name> Update

            This automated PR updates <Addon Name> from `${{ needs.check-update.outputs.current_version }}` to `${{ needs.check-update.outputs.latest_version }}`.

            ### Changelog

            ${{ needs.check-update.outputs.changelog }}

            ### Changes

            - Updated `config.yaml` version
            - Updated `build.yaml` <ADDON_UPPER>_VERSION
            - Updated `Dockerfile` <ADDON_UPPER>_VERSION
            - Updated documentation files
            - Updated CHANGELOG.md

            ### Source

            Version tracked from: <upstream-url>

            ---

            This PR was automatically generated by the Update <Addon Name> workflow
          labels: |
            automated
            <addon_slug>
            update
          draft: false

      - name: Trigger downstream workflows
        if: steps.create-pr.outputs.pull-request-number
        run: |
          curl -X POST \
            -H "Authorization: token ${{ secrets.GITHUB_TOKEN }}" \
            -H "Accept: application/vnd.github.v3+json" \
            https://api.github.com/repos/${{ github.repository }}/dispatches \
            -d '{
              "event_type": "automated-pr-created",
              "client_payload": {
                "pull_request_number": "${{ steps.create-pr.outputs.pull-request-number }}",
                "head_sha": "${{ steps.create-pr.outputs.pull-request-head-sha }}",
                "branch": "update-<addon>-${{ needs.check-update.outputs.latest_version }}",
                "addon": "<addon_slug>"
              }
            }'
```

### Key Points

- The `repository_dispatch` trigger at the end is essential - it fires the PR validation and builder workflows, since GitHub won't trigger workflows on events from `GITHUB_TOKEN`-created PRs
- Labels must include `automated` for the auto-merge system to pick it up
- `sign-commits: true` is required (repo enforces signed commits)
- The `delete-branch: true` cleans up after merge

---

## Base Image Script Integration

After creating the addon, add it to the existing base image update script at `.github/scripts/update-base-image.sh`.

Find the line that defines the list of addons to update (it looks like):
```bash
ADDONS="arcane dockge dockhand huly portainer_ee_lts portainer_ee_sts"
```

Add the new addon slug to this list:
```bash
ADDONS="arcane dockge dockhand huly <addon_slug> portainer_ee_lts portainer_ee_sts"
```

This ensures base image updates are applied to the new addon automatically.

---

## Version Source Patterns

Different upstreams publish versions differently. Here are the patterns used in this repo:

### GitHub Releases (Latest) - Used by Arcane

```bash
RESPONSE=$(curl -s -f "https://api.github.com/repos/<owner>/<repo>/releases/latest")
LATEST_VERSION=$(echo "$RESPONSE" | jq -r '.tag_name' | sed 's/^v//')
CHANGELOG=$(echo "$RESPONSE" | jq -r '.body // "No changelog available"')
```

### GitHub Releases (Filtered by Name) - Used by Portainer

Portainer has LTS and STS tracks, filtered by release name:

```bash
RESPONSE=$(curl -s -f "https://api.github.com/repos/portainer/portainer/releases")
LATEST_VERSION=$(echo "$RESPONSE" | jq -r \
    '[.[] | select(.prerelease == false) | select(.name | test("STS"; "i"))] | .[0].tag_name')
```

### Docker Hub Tags - Used by Dockhand

```bash
# Check if a specific tag exists on Docker Hub
TAG_EXISTS=$(curl -s "https://hub.docker.com/v2/repositories/<namespace>/<image>/tags/${VERSION}" | jq -r '.name // empty')
```

### Raw File in Repository - Used by Huly

Huly fetches version from a config file in the upstream repo:

```bash
RESPONSE=$(curl -s -f "https://raw.githubusercontent.com/<owner>/<repo>/main/.template.huly.conf")
LATEST_VERSION=$(echo "$RESPONSE" | grep 'HULY_VERSION=' | cut -d'=' -f2 | tr -d '"')
```

### Changelog JSON Endpoint - Used by Dockhand

```bash
RESPONSE=$(curl -s -f "https://raw.githubusercontent.com/<owner>/<repo>/main/changelog.json")
LATEST_VERSION=$(echo "$RESPONSE" | jq -r '.[0].version')
CHANGELOG=$(echo "$RESPONSE" | jq -r '.[0] | .changes[]' | sed 's/^/- /')
```

### Choosing the Right Pattern

1. **GitHub Releases API** is the most common and reliable
2. If the project has multiple release tracks, filter by release name or tag pattern
3. If the project doesn't use GitHub Releases, check if they have a version file, changelog JSON, or Docker Hub tags
4. Always strip the `v` prefix from version tags if present (addon versions use bare numbers)
5. Always include retry logic (3 attempts, 2-second delay) for API calls
