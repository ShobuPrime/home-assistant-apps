# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Home Assistant App for Dockhand Docker Manager that provides modern Docker container management through the Home Assistant interface. The app uses Home Assistant's S6-overlay init system and follows standard HA app conventions.

## Essential Commands

### Building and Testing
```bash
# Build the app locally (auto-detects architecture)
./build.sh

# Test the app locally
docker run --rm -it -p 3000:3000 -v /var/run/docker.sock:/var/run/docker.sock local/{arch}-addon-local_dockhand:{version}
```

### Version Management
```bash
# Check for Dockhand updates
./update-dockhand-version.sh --check-only

# Apply update with confirmation
./update-dockhand-version.sh --yes

# Get JSON output for automation
./update-dockhand-version.sh --check-only --json
```

## Dockhand Version Scheme

Dockhand uses semantic versioning: `[MAJOR].[MINOR].[PATCH]`

**Version Source**: Dockhand publishes version information in their changelog.json file:
- https://raw.githubusercontent.com/Finsys/dockhand/main/src/lib/data/changelog.json

**Note**: While Dockhand uses semantic versioning internally, the Docker Hub image (`fnsys/dockhand`) only has a `latest` tag - no version-specific tags exist. The app version tracks the Dockhand version from changelog.json.

The update script fetches the latest version from changelog.json and updates all relevant files. The GitHub Actions workflow runs daily to check for updates.

## Architecture and Key Components

### Directory Structure
- **`/rootfs/etc/cont-init.d/`**: S6 initialization scripts that run on container start
- **`/rootfs/etc/services.d/dockhand/`**: Service definition with `run` script and `finish` handler

### Critical Files
- **`config.yaml`**: App configuration (version, ports, ingress, options schema)
- **`build.yaml`**: Build configuration with base images per architecture
- **`Dockerfile`**: Multi-stage build that extracts Dockhand from official Docker image
- **`apparmor.txt`**: Security profile for Docker socket access

### Architecture Support
Dockhand only supports:
- `amd64` (x86_64)
- `aarch64` (arm64)

armv7/armhf is NOT supported by upstream Dockhand.

### Port Configuration
- **3000**: Web interface (HTTP, used for ingress)

## Development Guidelines

### S6-Overlay Integration
- Use Bashio library for all configuration reading and logging
- Service scripts must be executable and use proper S6 conventions
- Exit codes: 0 for success, non-zero triggers restart with backoff

### Configuration Handling
- Read options using `bashio::config` functions
- Protection mode must be disabled for Docker socket access
- Data is stored in `/data/dockhand` with symlink from `/opt/dockhand/data`

### Environment Variables
Dockhand uses these environment variables:
- `PORT`: Application port (3000)
- `DATA_DIR`: Persistent data location (/data/dockhand)
- `PUID`/`PGID`: User/Group ID for file permissions
- `NODE_ENV`: Runtime environment (production)
- `LOG_LEVEL`: Logging verbosity

### Testing Checklist
- Build completes successfully (multi-stage Docker build)
- Service starts without errors
- Web UI accessible on port 3000
- Ingress access works through Home Assistant sidebar
- Docker containers visible in Dockhand
- Configuration changes apply correctly
- Data persists in `/data/dockhand` across restarts
- Update script correctly identifies latest version

## Important Notes

- **Never commit changes** to version numbers without testing
- **Protection mode** must be disabled for the app to function
- **Ingress** integration requires WebSocket support (ingress_stream: true)
- **AppArmor profile** is critical for security - modifications require careful testing
- **No CSP configuration** - Unlike Portainer 2.33+, Dockhand does not set restrictive CSP headers, so no CSP workaround is needed
- **No label filtering** - Dockhand does NOT support hiding containers by labels like Portainer's `--hide-label` flag

## Key Differences from Portainer App

### CSP (Content Security Policy)
- **Portainer**: Requires `CSP=false` environment variable for versions 2.33.0+ to work with ingress
- **Dockhand**: No CSP configuration needed - does not set restrictive headers by default

### Container Label Filtering
- **Portainer**: Supports `--hide-label=io.hass.type=supervisor` and similar flags to hide HA containers
- **Dockhand**: **NOT SUPPORTED** - No mechanism to hide containers by labels. Only UI-level filtering by container name/image/stack

### Architecture
- **Portainer**: Supports amd64, aarch64, armhf, armv7, i386
- **Dockhand**: Only amd64 and aarch64

### Build Method
- **Portainer**: Downloads release tarball directly from GitHub
- **Dockhand**: Multi-stage Docker build extracting from official `fnsys/dockhand` image

## Common Issues and Troubleshooting

### Issue: Dockhand Not Starting

**Symptoms:**
- App fails to start
- Logs show application not found or permission errors

**Solution:**
1. Verify the Dockerfile multi-stage build completed correctly
2. Check architecture compatibility (amd64/aarch64 only)
3. Ensure Docker socket is accessible

### Issue: WebSocket/Ingress Not Working

**Symptoms:**
- Real-time updates don't work
- Terminal access fails
- Connection errors in browser console

**Solution:**
1. Verify `ingress_stream: true` in config.yaml
2. Check Home Assistant ingress configuration
3. Try direct access via port 3000

### Issue: Home Assistant Containers Visible

**Symptoms:**
- All HA system containers visible in Dockhand
- Want to hide supervisor, core, audio, dns containers

**Note:**
This is a **known limitation** of Dockhand. Unlike Portainer, there is no CLI flag or configuration to hide containers by labels. The only workaround is to use the search/filter functionality in the UI to find specific containers.

### Issue: Database Errors

**Symptoms:**
- "Database locked" errors
- Data not persisting

**Solution:**
1. Check disk space on Home Assistant
2. Verify `/data/dockhand` has proper permissions
3. If corrupted, delete the `db/` directory and restart (will lose settings)
