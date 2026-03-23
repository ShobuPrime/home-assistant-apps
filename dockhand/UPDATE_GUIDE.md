# Dockhand Update Guide

This document describes how to update the Dockhand app to a new version.

## Automatic Updates (Recommended)

Use the update script to automatically update to the latest version:

```bash
# Check for updates without applying
./update-dockhand-version.sh --check-only

# Apply update with confirmation
./update-dockhand-version.sh --yes

# Get JSON output for automation
./update-dockhand-version.sh --check-only --json
```

## Manual Update Steps

If you need to update manually:

### 1. Check Latest Version

Visit the [Dockhand GitHub Releases](https://github.com/Finsys/dockhand/releases) or [Docker Hub](https://hub.docker.com/r/fnsys/dockhand/tags) to find the latest version.

### 2. Update Version Numbers

Update the following files:

**config.yaml:**
```yaml
version: "X.Y.Z"
```

**build.yaml:**
```yaml
args:
  DOCKHAND_VERSION: X.Y.Z
```

**Dockerfile:**
```dockerfile
ARG DOCKHAND_VERSION=X.Y.Z
```

### 3. Update CHANGELOG.md

Add an entry for the new version:

```markdown
## [X.Y.Z] - YYYY-MM-DD

### Changed
- Updated Dockhand to version X.Y.Z
- [List any notable changes from Dockhand release notes]
```

### 4. Test the Build

```bash
./build.sh
```

### 5. Test Functionality

Run the built image locally to verify:
- Service starts without errors
- Web UI accessible on port 3000
- Docker containers visible
- Ingress works through Home Assistant

## Version Compatibility

- Dockhand requires Docker Engine 20.10+ / Docker API 1.41+
- This app supports amd64 and aarch64 architectures only
- Home Assistant Core 2023.6.0 or later recommended

## Rollback

If an update causes issues:

1. Revert the version numbers in config.yaml, build.yaml, and Dockerfile
2. Rebuild: `./build.sh`
3. Reinstall the app in Home Assistant
