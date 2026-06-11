# File Templates Reference

This file contains exact templates for every file in a Home Assistant app. Replace all `<placeholders>` with app-specific values.

## Table of Contents

1. [AppArmor Profile](#apparmor-profile)
2. [build.sh](#buildsh)
3. [translations/en.yaml](#translationsenyaml)
4. [README.md](#readmemd)
5. [DOCS.md](#docsmd)
6. [CHANGELOG.md](#changelogmd)
7. [UPDATE_GUIDE.md](#update_guidemd)
8. [CLAUDE.md](#claudemd)

---

## AppArmor Profile

The apparmor.txt profile name MUST match the app slug. This is the standard template that works for Docker-management apps. For apps that don't need Docker socket access, remove the Docker-specific sections.

```
#include <tunables/global>

profile <app_slug> flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>

  # Capabilities
  capability,
  file,
  signal (send) set=(kill,term,int,hup,cont),
  network,
  mount,

  # S6-Overlay
  /init ix,
  /bin/** ix,
  /usr/bin/** ix,
  /run/{s6,s6-rc*,service}/** ix,
  /package/** ix,
  /command/** ix,
  /etc/services.d/** rwix,
  /etc/cont-init.d/** rwix,
  /etc/cont-finish.d/** rwix,
  /run/{,**} rwk,
  /dev/tty rw,

  # Bashio
  /usr/lib/bashio/** ix,
  /tmp/** rwk,

  # <App Name> binary and data
  /opt/<app>/** ix,
  /data/** rw,

  # Access to options.json and other app data
  /data/** rw,

  # SSL certificates
  /ssl/** r,

  # Docker socket access - only include if docker_api: true
  /var/run/docker.sock rw,
  /run/docker.sock rw,
  /{,var/}run/docker.sock rw,

  # Docker runtime access - only include if docker_api: true
  /sys/fs/cgroup/** r,
  /proc/sys/net/ipv4/ip_forward r,

  # Shared volumes
  /share/** rw,
  /media/** rw,

  # DNS resolution
  /etc/hosts r,
  /etc/resolv.conf r,
  /etc/nsswitch.conf r,
  /etc/passwd r,
  /etc/group r,

  # Device access
  /dev/null rw,
  /dev/urandom r,
  /dev/random r,
  /dev/net/tun rw,

  # Proc filesystem
  /proc/*/mountinfo r,
  /proc/*/stat r,
  /proc/*/status r,

  # <App Name> specific paths
  owner /data/<app>/** rwk,

  # Allow process tracing for container management - only if docker_api: true
  ptrace (read,trace) peer=docker-default,
  ptrace (read,trace) peer=unconfined,
}
```

### Customization Notes

- **No Docker access needed**: Remove the Docker socket, Docker runtime, and ptrace sections
- **Additional data directories**: Add `owner /data/<other-dir>/** rwk` entries
- **Network services**: The `network` capability covers TCP/UDP listening and connections
- **If the app writes to `/tmp`**: Already covered by `/tmp/** rwk`
- **If the app needs to execute child processes**: Already covered by capability rules

---

## build.sh

This is the local build script. The pattern is identical across all apps with only the version ARG name, description, and test ports changing.

```bash
#!/bin/bash
# Build script for <App Name> app

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}=== <App Name> App Builder ===${NC}"

# Check if we're in the right directory
if [ ! -f "config.yaml" ] || [ ! -f "build.yaml" ] || [ ! -f "Dockerfile" ]; then
    echo -e "${RED}Error: This script must be run from the app directory!${NC}"
    exit 1
fi

# Get app slug and version
APP_SLUG=$(grep "^slug:" config.yaml | cut -d'"' -f2 | tr -d ' ')
APP_VERSION=$(grep "^version:" config.yaml | cut -d'"' -f2)

echo -e "Building ${GREEN}${APP_SLUG}${NC} version ${YELLOW}${APP_VERSION}${NC}"

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64)
        BUILD_ARCH="amd64"
        ;;
    aarch64)
        BUILD_ARCH="aarch64"
        ;;
    *)
        echo -e "${RED}Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

echo -e "Architecture: ${YELLOW}${BUILD_ARCH}${NC}"

# Get base image from build.yaml
BASE_IMAGE=$(grep -A10 "^build_from:" build.yaml | grep "${BUILD_ARCH}:" | head -1 | awk '{print $2}')
if [ -z "$BASE_IMAGE" ]; then
    echo -e "${RED}Error: No base image found for architecture ${BUILD_ARCH}${NC}"
    exit 1
fi

echo -e "Base image: ${YELLOW}${BASE_IMAGE}${NC}"

# Get app version from build.yaml
<APP_UPPER>_VERSION=$(grep "<APP_UPPER>_VERSION:" build.yaml | cut -d':' -f2 | tr -d ' ')
echo -e "<App Name> version: ${YELLOW}${<APP_UPPER>_VERSION}${NC}"

# Build image name - use local naming convention to avoid Docker Hub pulls
# For local apps, use local/ prefix
IMAGE_NAME="local/${BUILD_ARCH}-addon-local_${APP_SLUG}"
IMAGE_TAG="${APP_VERSION}"
FULL_IMAGE="${IMAGE_NAME}:${IMAGE_TAG}"

echo ""
echo -e "${BLUE}Building Docker image...${NC}"
echo "Image: ${FULL_IMAGE}"
echo ""

# Build the Docker image
docker build \
    --build-arg BUILD_FROM="${BASE_IMAGE}" \
    --build-arg BUILD_ARCH="${BUILD_ARCH}" \
    --build-arg BUILD_DATE="$(date -u +"%Y-%m-%dT%H:%M:%SZ")" \
    --build-arg BUILD_DESCRIPTION="<App Name> for Home Assistant" \
    --build-arg BUILD_NAME="${APP_SLUG}" \
    --build-arg BUILD_REF="$(git rev-parse --short HEAD 2>/dev/null || echo 'local')" \
    --build-arg BUILD_REPOSITORY="local" \
    --build-arg BUILD_VERSION="${APP_VERSION}" \
    --build-arg <APP_UPPER>_VERSION="${<APP_UPPER>_VERSION}" \
    -t "${FULL_IMAGE}" \
    .

if [ $? -eq 0 ]; then
    echo ""
    echo -e "${GREEN}Build successful!${NC}"
    echo -e "Image: ${GREEN}${FULL_IMAGE}${NC}"
    echo ""
    echo "To test locally:"
    echo -e "  ${YELLOW}docker run --rm -it <port-flags> ${FULL_IMAGE}${NC}"
else
    echo ""
    echo -e "${RED}Build failed!${NC}"
    exit 1
fi
```

Replace `<APP_UPPER>` with the uppercase version ARG name (e.g., `PORTAINER`, `ARCANE`).
Replace `<port-flags>` with `-p <port>:<port>` for each exposed port.

---

## translations/en.yaml

Lives at `<app>/translations/en.yaml`. Home Assistant uses it to render the option **label** and **helper text** in the app's Configuration tab, so the raw `config.yaml` keys never appear in the UI. **Required** for every app, and must stay in sync with `config.yaml`'s `options`/`schema`.

A single top-level `configuration:` map keyed by the **exact** option keys from `config.yaml`. Each entry has a Title Case `name:` and a plain-English `description:` (use a folded `>-` scalar for multi-line text so it wraps in the UI). Include an entry for **every** option, including `log_level`.

```yaml
configuration:
  log_level:
    name: Log level
    description: >-
      How much detail the app writes to its log. Leave on "info" unless you
      are troubleshooting, in which case use "debug".
  <option_key>:
    name: <Title Case Label>
    description: >-
      <What this option does, plus guidance/defaults. Keep it plain-English and
      jargon-free — this is what the user reads in the Configuration tab.>
  <boolean_option>:
    name: <Title Case Label>
    description: >-
      <When ON ...; when OFF .... State the recommended default.>
```

See `aegis_ha/translations/en.yaml` for a complete, well-written exemplar covering hosts, API keys, booleans, numeric delays, and advanced options.

---

## README.md

```markdown
# <App Name> App for Home Assistant

![Supports aarch64 Architecture][aarch64-shield]
![Supports amd64 Architecture][amd64-shield]

<One-line description of what the app does.>

## About

<2-3 sentence description of the upstream project and how this app integrates it with Home Assistant.>

## Features

- <Key feature 1>
- <Key feature 2>
- Ingress support for seamless sidebar integration
- Persistent data storage included in backups

## Installation

1. Add this repository to your Home Assistant instance
2. Search for "<App Name>" in the app store
3. Click Install
4. Configure the app options (if needed)
5. Start the app
6. Click "OPEN WEB UI" or access via the sidebar

## Configuration

### Option: `log_level`

The `log_level` option controls the level of log output by the app:

- `trace`: Show every detail
- `debug`: Shows detailed debug information
- `info`: Normal (usually) interesting events (default)
- `warning`: Exceptional occurrences that are not errors
- `error`: Runtime errors
- `fatal`: Critical errors

<Document each app-specific option here>

## Folder Access

This app has access to the following Home Assistant directories:

- `/ssl` - SSL certificates (read-only)
- `/data` - App persistent data (read/write)
- `/media` - Home Assistant media folder (read/write)
- `/share` - Home Assistant share folder (read/write)

## First Time Setup

<App-specific first-time setup instructions>

## Support

Got questions or found a bug? Please open an issue on the GitHub repository.

[aarch64-shield]: https://img.shields.io/badge/aarch64-yes-green.svg
[amd64-shield]: https://img.shields.io/badge/amd64-yes-green.svg

## Version

Currently running <App Name> X.Y.Z
```

---

## DOCS.md

```markdown
# <App Name> Documentation

## Overview

<Brief overview of the app and its purpose.>

## Configuration

### Option: `log_level`

The `log_level` option controls the level of log output by the app:
- `trace`: Show every detail
- `debug`: Shows detailed debug information
- `info`: Normal (usually) interesting events (default)
- `warning`: Exceptional occurrences that are not errors
- `error`: Runtime errors
- `fatal`: Critical errors

<Document each app-specific option with details>

## Access Methods

1. **Via Sidebar**: Click the <icon> in Home Assistant (uses ingress)
2. **Direct HTTP**: `http://[your-ip]:<port>`

## Port Information

- **<port>**: <Description>

## Data Persistence

All data is stored in `/data/<app>` and included in Home Assistant backups.

<Document what files are stored and their purpose>

## Features

<Detailed feature descriptions>

## First Time Setup

<Step-by-step first-time configuration>

## Security Considerations

- **Protection Mode**: <Whether it must be disabled and why>
- **AppArmor**: Custom profile restricts app permissions appropriately

## Troubleshooting

### <Common Issue Title>

**Symptoms:**
- <What the user sees>

**Solution:**
1. <Step to fix>

## Updating

The app automatically tracks releases. Updates appear in the Home Assistant UI when available.

## External Resources

- [<Project> Documentation](<docs-url>)
- [<Project> GitHub](<source-url>)
```

---

## CHANGELOG.md

> **Format note:** Home Assistant Core's `update.<app>` entity extracts
> release notes with the regex `^#* {version}\n` — meaning the version
> header must be the entire line content. Using `## Version X.Y.Z (date)`
> or `## [X.Y.Z]` will break extraction and the UI will dump the entire
> changelog every time. Put the date on a separate line below.

```markdown
# Changelog

## X.Y.Z

_YYYY-MM-DD_

### Initial release

- Initial Home Assistant app for <App Name>
- <Key feature 1>
- <Key feature 2>
- Ingress support for sidebar integration
- Automatic version update checks
```

---

## UPDATE_GUIDE.md

```markdown
# Update Guide for <App Name> App

## Understanding Local App Updates

Local apps in Home Assistant don't have automatic update detection like repository apps. Updates only appear when:
1. The `version` field in `config.yaml` changes
2. You rebuild the app
3. You click "Check for updates" in the app store

## Update Methods

### Method 1: Automatic (GitHub Actions)

This app has automated update detection via GitHub Actions. When a new version is available:
1. A PR is automatically created with the version bump
2. The PR is validated and auto-merged if all checks pass
3. Pull the latest changes and rebuild

### Method 2: Manual Update

```bash
# SSH into Home Assistant
cd /addons/<app_slug>

# Pull latest changes
git pull

# Rebuild
./build.sh

# Go to Supervisor -> App Store -> Check for updates
```

## Checking Current Version

```bash
grep "version:" /addons/<app_slug>/config.yaml
```

## Best Practices

1. **Regular Checks**: Pull latest changes regularly
2. **Test First**: Always test updates in a non-production environment
3. **Backup**: Create a Home Assistant backup before updating
4. **Monitor Logs**: Check app logs after updates for any issues

## Troubleshooting

### Update doesn't appear after rebuild
1. Ensure version number changed in config.yaml
2. Click "Check for updates" multiple times
3. Try reloading the Supervisor: `ha supervisor reload`
```

---

## CLAUDE.md

```markdown
# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Home Assistant App for <App Name> that provides <description> through the Home Assistant interface. The app uses Home Assistant's S6-overlay init system and follows standard HA app conventions.

## Essential Commands

### Building and Testing
```bash
# Build the app locally (auto-detects architecture)
./build.sh

# Test the app locally
docker run --rm -it <port-flags> <volume-flags> local/{arch}-addon-local_<app_slug>:{version}
```

### Version Management
```bash
# Check for updates (from repo root)
.github/scripts/update-<app>.sh  # with CHECK_ONLY=true
```

## <App Name> Version Scheme

<How versioning works for this upstream project. Single release stream? Multiple tracks?>

## Architecture and Key Components

### Directory Structure
- **`/rootfs/etc/cont-init.d/`**: S6 initialization scripts that run on container start
- **`/rootfs/etc/services.d/<name>/`**: Service definition with `run` script and `finish` handler

### Critical Files
- **`config.yaml`**: App configuration (version, ports, ingress, options schema)
- **`build.yaml`**: Build configuration with base images per architecture
- **`Dockerfile`**: <How the app is built>
- **`apparmor.txt`**: Security profile

### Architecture Support
- `amd64` (x86_64)
- `aarch64` (arm64)

### Port Configuration
- **<port>**: <Description>

## Development Guidelines

### S6-Overlay Integration
- Use Bashio library for all configuration reading and logging
- Service scripts must be executable and use proper S6 conventions
- Exit codes: 0 for success, non-zero triggers restart with backoff

### Configuration Handling
- Read options using `bashio::config` functions
- <App-specific configuration notes>

### <Any Critical Fixes or Workarounds>

<Document any critical workarounds like the CSP fix in Portainer>

### Version Updates
When updating version:
1. Update `ARG <APP>_VERSION` in Dockerfile
2. Update `version` in config.yaml
3. Update version in build.yaml args
4. Test on at least one architecture before committing

### Testing Checklist
- Build completes successfully
- Service starts without errors
- Web UI accessible on configured port(s)
- Ingress access works through Home Assistant sidebar
- Configuration changes apply correctly
- Data persists across restarts
- Update script correctly identifies latest version

## Important Notes

- **Never commit changes** to version numbers without testing
- **Ingress** integration requires WebSocket support (ingress_stream: true)
- **AppArmor profile** is critical for security - modifications require careful testing

## Common Issues and Troubleshooting

### Issue: <Title>

**Symptoms:**
- <What happens>

**Cause:** <Why>

**Solution:**
1. <Fix steps>
```
