# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Home Assistant App for Arcane Docker Manager that provides modern Docker container management through the Home Assistant interface. The app uses Home Assistant's S6-overlay init system and follows standard HA app conventions.

## Essential Commands

### Building and Testing
```bash
# Build the app locally (auto-detects architecture)
./build.sh

# Test the app locally
docker run --rm -it -p 3552:3552 -p 3553:3553 -v /var/run/docker.sock:/var/run/docker.sock local/{arch}-addon-local_arcane:{version}
```

### Version Management
```bash
# Check for Arcane updates
./update-arcane-version.sh --check-only

# Apply update with confirmation
./update-arcane-version.sh --yes

# Get JSON output for automation
./update-arcane-version.sh --check-only --json
```

## Arcane Version Scheme

Arcane uses semantic versioning: `v[MAJOR].[MINOR].[PATCH]`

The update script fetches the latest release from GitHub and updates all relevant files. Unlike Portainer, Arcane does not have separate LTS/STS tracks - it follows a single release stream.

## Architecture and Key Components

### Directory Structure
- **`/rootfs/etc/cont-init.d/`**: S6 initialization scripts that run on container start
- **`/rootfs/etc/services.d/arcane/`**: Service definition with `run` script and `finish` handler

### Critical Files
- **`config.yaml`**: App configuration (version, ports, ingress, options schema)
- **`build.yaml`**: Build configuration with base images per architecture
- **`Dockerfile`**: Downloads Arcane binary and sets up environment
- **`apparmor.txt`**: Security profile for Docker socket access

### Architecture Support
Arcane only supports:
- `amd64` (x86_64)
- `aarch64` (arm64)

armv7/armhf is NOT supported by upstream Arcane.

### Port Configuration
- **3552**: Web interface (main port, used for ingress)
- **3553**: Agent communication port

## Development Guidelines

### S6-Overlay Integration
- Use Bashio library for all configuration reading and logging
- Service scripts must be executable and use proper S6 conventions
- Exit codes: 0 for success, non-zero triggers restart with backoff

### Configuration Handling
- Read options using `bashio::config` functions
- Protection mode must be disabled for Docker socket access
- Encryption keys are auto-generated on first start and stored in `/data/arcane/.secrets`

### Environment Variables
Arcane requires these environment variables:
- `ENCRYPTION_KEY`: 32-byte encryption key (auto-generated)
- `JWT_SECRET`: JWT signing secret (auto-generated)
- `PORT`: Application port (3552)
- `DOCKER_HOST`: Docker socket location
- `DATABASE_URL`: SQLite database path
- `PUID`/`PGID`: User/Group ID for file permissions

### Testing Checklist
- Build completes successfully
- Service starts without errors
- Web UI accessible on port 3552
- Ingress access works through Home Assistant sidebar
- Docker containers visible in Arcane
- Configuration changes apply correctly
- Data persists in `/data/arcane` across restarts
- Update script correctly identifies latest version

## Important Notes

- **Never commit changes** to version numbers without testing
- **Protection mode** must be disabled for the app to function
- **Ingress** integration requires WebSocket support (ingress_stream: true)
- **AppArmor profile** is critical for security - modifications require careful testing
- **Default credentials** are `arcane`/`arcane-admin` - users must change on first login
- **Encryption keys** in `.secrets` must not be deleted or the database becomes inaccessible

## Common Issues and Troubleshooting

### Issue: Arcane Not Starting

**Symptoms:**
- App fails to start
- Logs show binary not found or permission errors

**Solution:**
1. Verify the Dockerfile downloaded the binary correctly
2. Check architecture compatibility (amd64/aarch64 only)
3. Ensure Docker socket is accessible

### Issue: WebSocket/Ingress Not Working

**Symptoms:**
- Real-time updates don't work
- Connection errors in browser console

**Solution:**
1. Verify `ingress_stream: true` in config.yaml
2. Check Home Assistant ingress configuration
3. Try direct access via port 3552

### Issue: Database Errors

**Symptoms:**
- "Database locked" errors
- Data not persisting

**Solution:**
1. Check disk space on Home Assistant
2. Verify `/data/arcane` has proper permissions
3. If corrupted, delete `arcane.db` and restart (will lose settings)

### Issue: Container Labels Not Working

**Symptoms:**
- `hide_hassio_containers` option not taking effect

**Note:**
This feature may require additional implementation in the run script to pass labels to Arcane, as Arcane may not support the same label filtering as Portainer.
