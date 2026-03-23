# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Home Assistant App for Portainer Enterprise Edition (EE) that provides Docker container management through the Home Assistant interface. The app uses Home Assistant's S6-overlay init system and follows standard HA app conventions.

## Essential Commands

### Building and Testing
```bash
# Build the app locally (auto-detects architecture)
./build.sh

# Test the app locally
docker run --rm -it -p 8000:8000 -p 9000:9000 -p 9443:9443 -v /var/run/docker.sock:/var/run/docker.sock local/{arch}-addon-local_portainer_ee:{version}
```

### Version Management
```bash
# Check for Portainer updates
./update-portainer-version.sh --check-only

# Apply update with confirmation
./update-portainer-version.sh --yes

# Get JSON output for automation
./update-portainer-version.sh --check-only --json
```

## Portainer Version Scheme

Portainer uses a semantic versioning scheme with two release tracks:

- **LTS (Long Term Support)**: Stable releases with extended support and maintenance
- **STS (Short Term Support)**: Feature releases with the latest updates

The update script filters for LTS releases by checking the GitHub release name for "LTS" designation. This ensures only stable, long-term supported versions are installed, regardless of version number patterns.

## Architecture and Key Components

### Directory Structure
- **`/rootfs/etc/cont-init.d/`**: S6 initialization scripts that run on container start
- **`/rootfs/etc/services.d/portainer/`**: Service definition with `run` script and `finish` handler
- **`/homeassistant/packages/`**: Home Assistant integration package for automatic update checking

### Critical Files
- **`config.yaml`**: App configuration (version, ports, ingress, options schema)
- **`build.yaml`**: Build configuration with base images per architecture
- **`Dockerfile`**: Downloads Portainer binary, verifies checksum, sets up environment
- **`apparmor.txt`**: Security profile for Docker socket access

### Architecture Mapping
The app supports multiple architectures with naming differences from Portainer:
- `aarch64` → `arm64` (Portainer binary)
- `armhf`/`armv7` → `arm` (Portainer binary)
- Other architectures use the same name

### Port Configuration
- **9000**: HTTP web interface
- **9443**: HTTPS web interface (always available)
- **8000**: Edge agent tunnel service

## Development Guidelines

### S6-Overlay Integration
- Use Bashio library for all configuration reading and logging
- Service scripts must be executable and use proper S6 conventions
- Exit codes: 0 for success, non-zero triggers restart with backoff

### Configuration Handling
- Read options using `bashio::config` functions
- Protection mode must be disabled for Docker socket access
- SSL option controls custom certificate usage only (HTTPS always available)
- Hidden container settings are cached by Portainer in `/data/portainer/.hide_hassio_containers`

### Critical: Content Security Policy (CSP) Fix

**Portainer 2.33.0+ Breaking Change**: Portainer introduced Content-Security-Policy headers with `frame-ancestors 'none'` that block iframe embedding, breaking Home Assistant ingress integration.

**Solution**: The app sets `export CSP=false` in `/rootfs/etc/services.d/portainer/run` to disable restrictive CSP headers and restore ingress functionality.

**Location**: `/addons/portainer_ee/rootfs/etc/services.d/portainer/run:9`

```bash
export CSP=false
```

**NEVER remove this environment variable** for Portainer versions 2.33.0 or later, as it will break Home Assistant ingress access. Users experiencing "refused to display in a frame" errors after updating need to rebuild the app with this fix.

### Version Updates
When updating Portainer version:
1. Update `ARG PORTAINER_VERSION` in Dockerfile
2. Update `version` in config.yaml
3. Update checksums for all architectures in Dockerfile
4. Test on at least one architecture before committing

### Home Assistant Integration Package

The app includes a Home Assistant integration package for automatic update detection and management:

**Setup Instructions:**
1. Copy package to Home Assistant config:
   ```bash
   cp /addons/portainer_ee/homeassistant/packages/portainer_updates.yaml /config/packages/
   ```

2. Add to `configuration.yaml`:
   ```yaml
   homeassistant:
     packages: !include_dir_named packages
   ```

3. Restart Home Assistant to activate

**Features Provided:**
- `sensor.portainer_update_status` - Update availability sensor
- `binary_sensor.portainer_update_available` - Binary update indicator
- Daily automatic update checks (3 AM)
- Automatic notifications when updates are available
- One-click update application via `script.apply_portainer_update`
- Manual update check via `script.check_portainer_updates`

**Important**: The integration relies on the update script being accessible at `/addons/portainer_ee/update-portainer-version.sh`. This typically requires the SSH app or Terminal app to be installed.

### Testing Checklist
- Build completes successfully
- Service starts without errors
- Web UI accessible on configured ports
- **Ingress/iframe access works (check for CSP errors in browser console)**
- Docker containers visible in Portainer
- SSL works with valid certificates when enabled
- Configuration changes apply correctly
- Data persists in `/data/portainer` across restarts
- Update script correctly identifies LTS versions only

## Important Notes

- **Never commit changes** to version numbers without testing
- **Protection mode** must be disabled for the app to function
- **Ingress** integration requires specific port configuration (9443 for HTTPS)
- **AppArmor profile** is critical for security - modifications require careful testing
- **Hidden containers** feature requires manual cache clearing in Portainer UI if changed
- **CSP environment variable** (`CSP=false`) is required for Portainer 2.33.0+ to work with Home Assistant ingress

## Common Issues and Troubleshooting

### Issue: Portainer Not Accessible Through Home Assistant After Update

**Symptoms:**
- Browser shows "refused to display in a frame" error
- Console shows CSP violation errors
- Direct access to ports 9000/9443 works, but ingress doesn't

**Cause:** Portainer 2.33.0+ introduced restrictive Content-Security-Policy headers

**Solution:**
1. Verify `CSP=false` is set in `/rootfs/etc/services.d/portainer/run:9`
2. Rebuild the app: `./build.sh`
3. Update the app in Home Assistant Supervisor
4. Restart the app

### Issue: Update Script Selects Wrong Version (STS Instead of LTS)

**Symptoms:**
- Script updates to an STS version instead of LTS
- Expected LTS version but got STS

**Cause:** This should not happen - the script explicitly filters for LTS versions by checking release names

**Verification:**
```bash
# Check what the script considers latest LTS
./update-portainer-version.sh --check-only

# Manually verify LTS releases
curl -s https://api.github.com/repos/portainer/portainer/releases | \
  jq -r '.[] | select(.prerelease == false) | select(.name | test("LTS"; "i")) | .tag_name' | head -5
```

**Note:** If you're on an STS version, the update script will correctly update you to the latest LTS version based on the GitHub release name designation.

### Issue: Home Assistant Not Detecting App Updates Automatically

**Symptoms:**
- Manual updates work, but HA doesn't show update notifications
- Update sensors not showing in Home Assistant

**Cause:** Home Assistant integration package not installed or configured

**Solution:**
1. Ensure `/config/packages/portainer_updates.yaml` exists
2. Verify `configuration.yaml` includes:
   ```yaml
   homeassistant:
     packages: !include_dir_named packages
   ```
3. Restart Home Assistant
4. Check that sensors exist: `sensor.portainer_update_status` and `binary_sensor.portainer_update_available`
5. Manually trigger check: Call `script.check_portainer_updates` service

### Issue: Build Fails with Checksum Mismatch

**Symptoms:**
- Build fails during Portainer download verification
- Error: "SHA256 checksum mismatch"

**Cause:** Portainer binary was updated on GitHub but local Dockerfile still has old checksum

**Solution:**
1. Re-run update script to get latest checksums: `./update-portainer-version.sh --yes`
2. The script automatically updates Dockerfile with correct checksums
3. Rebuild: `./build.sh`