# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Home Assistant App for Dockge, a self-hosted Docker Compose stack manager. The app provides a web interface for managing Docker Compose stacks through Home Assistant.

### Development History
- Initially created with full custom build (Dockerfile, S6 scripts, etc.)
- Simplified to use official `louislam/dockge` Docker image directly
- Removed all user configuration options for simplicity
- Data automatically persists in `/data/stacks`

## Lessons Learned from Home Assistant App Development

### Critical Base Image Selection
- **ALWAYS** use `ghcr.io/hassio-addons/base:18.0.3` or latest
- **NEVER** use the old `ghcr.io/home-assistant/*-base` images (they are outdated)
- The hassio-addons base images include Bashio and S6-overlay pre-configured

### Docker Socket Access Pattern
- Must set `docker_api: true` in config.yaml
- Protection mode MUST be disabled by users
- Always check socket existence in init scripts before starting services
- AppArmor profile must include `/var/run/docker.sock rw`

### S6-Overlay Best Practices
- All scripts must use `#!/usr/bin/with-contenv bashio` shebang
- Init scripts in `/rootfs/etc/cont-init.d/` run before services
- Service scripts in `/rootfs/etc/services.d/{service}/run`
- Always make scripts executable with `chmod +x`
- Use Bashio functions for all logging and config reading

### Essential config.yaml Settings for Privileged Apps
```yaml
hassio_api: true
hassio_role: manager
docker_api: true
apparmor: true
privileged:
  - SYS_RAWIO
  - NET_ADMIN
  - SYS_ADMIN
  - SYS_PTRACE
  - SYS_MODULE
  - DAC_READ_SEARCH
```

### Using Official Docker Images vs Building Custom
- When the upstream project provides an official multi-arch Docker image, use it directly with the `image:` field in config.yaml
- This is much simpler than building a custom image and ensures you get official updates
- Example: `image: "louislam/dockge"` in config.yaml
- Only build custom images when you need to add HA-specific integrations or the upstream doesn't provide suitable images
- **Trade-off**: You lose ability to customize environment variables or startup behavior
- **Example**: Dockge 1.5.0 console feature requires `DOCKGE_ENABLE_CONSOLE=true` which we can't set dynamically

### WebSocket Support for Ingress
- Dockge uses WebSockets for real-time updates
- Must set `ingress_stream: true` in config.yaml for WebSocket support
- Without this, ingress will fail with "websocket error" messages

### Data Persistence in HAOS
- ALWAYS store persistent data under `/data/` in Home Assistant apps
- Don't provide user options for paths if they must be under `/data/` anyway
- Use environment variables in config.yaml to pass hardcoded paths to the container
- If you must allow custom paths, validate they start with `/data/`
- **Important Path Mapping Pattern**: When mapping volumes, the host path is `/data` + container path
  - Example: Container path `/opt/stacks` → Host path `/data/opt/stacks`
  - Example: Container path `/app/data` → Host path `/data/app/data`

### Automatic Update Scripts
- Create update scripts similar to `update-dockge-version.sh` for easy version management
- Script should support `--check-only`, `--json`, and `--yes` flags
- Automatically update config.yaml version and fetch changelogs
- Keep backups of modified files with timestamps

### Common Pitfalls to Avoid
1. Forgetting to make scripts executable (when building custom)
2. Using relative paths instead of absolute paths
3. Not checking for Docker socket before starting
4. Using wrong base images (see above)
5. Not using Bashio for config reading (when building custom)
6. Building custom images when official ones would work

## Essential Commands

### Testing
Since this app uses the official Dockge Docker image, no building is required. To test locally:
```bash
# Test with docker directly
docker run --rm -it -p 5001:5001 -v /var/run/docker.sock:/var/run/docker.sock -v $(pwd)/data:/app/data louislam/dockge:1
```

## Architecture and Key Components

### Simplified Structure (using official image)
This app now uses the official `louislam/dockge:1` Docker image directly, which greatly simplifies the app:

### Critical Files
- **`config.yaml`**: App configuration with image reference, ports, ingress settings
- **`apparmor.txt`**: Security profile for Docker socket access
- **No build files needed**: Uses official Docker image directly

### Port Configuration
- **5001**: Web interface

## Development Guidelines

### Configuration Handling
- Uses environment variables passed through config.yaml
- Protection mode must be disabled for Docker socket access
- Stacks directory is hardcoded to `/data/stacks` for data persistence

### Version Updates
When updating Dockge version:
1. Run `./update-dockge-version.sh` to check for updates
2. The script automatically updates config.yaml and creates changelog
3. Test the new version before committing

### Console Feature Limitation
- Dockge 1.5.0+ disables console by default for security
- Cannot dynamically set `DOCKGE_ENABLE_CONSOLE` with official image
- Would require custom Dockerfile to enable (trade-off: lose automatic updates)

### Testing Checklist
- App installs successfully
- Service starts without errors
- Web UI accessible on port 5001
- Docker socket is accessible
- Can create and manage Docker Compose stacks
- Stacks persist in configured directory
- Ingress integration works properly

## Important Notes

- **Official Image**: This app uses the official `louislam/dockge` image
- **Protection mode** must be disabled for Docker socket access
- **Ingress** integration uses port 5001
- **AppArmor profile** is critical for security
- **Docker socket** access is required at `/var/run/docker.sock`
- **Stack files** stored in configurable directory (default `/data/stacks`)
- **No custom build process** - significantly simpler than building from source