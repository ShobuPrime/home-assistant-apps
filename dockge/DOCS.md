# Dockge Documentation

## About

Dockge is a fancy, easy-to-use and reactive self-hosted docker compose.yaml stack-oriented manager. It provides a clean web interface for managing Docker Compose stacks with features like syntax highlighting, web terminal, and real-time updates.

## Configuration

### Option: `log_level`

The `log_level` option controls the level of log output by the app:
- `trace`: Show every detail
- `debug`: Shows detailed debug information
- `info`: Normal (usually) interesting events (default)
- `warning`: Exceptional occurrences that are not errors
- `error`: Runtime errors
- `fatal`: Critical errors

### Option: `stacks_dir`

The directory where Docker Compose stacks will be stored.
- Default: `/opt/stacks`
- All stacks are automatically included in Home Assistant backups

### Option: `hide_hassio_containers`

When enabled (default), hides Home Assistant system containers from the Dockge interface.
- `true`: Hide all containers with `io.hass.type` labels (supervisor, core, audio, dns, multicast, cli, observer, addon)
- `false`: Show all containers

## Access Methods

1. **Via Sidebar**: Click the Dockge icon in Home Assistant (uses ingress)
2. **Direct HTTP**: `http://[your-ip]:5001`

## Port Information

- **5001**: HTTP web interface (default and only port)

## Features

- **Interactive Editor**: Edit compose.yaml files with syntax highlighting
- **Web Terminal**: Access container terminal directly from the browser (requires console feature to be enabled)
- **Real-time Updates**: See container status changes instantly
- **Docker Compose Management**: Start, stop, restart stacks easily
- **Convert Docker Run**: Convert `docker run` commands to compose.yaml

## Data Persistence

- **Dockge data**: Stored in `/data/dockge` (application data)
- **Docker stacks**: Stored in `/opt/stacks` (compose files and volumes)

All data is automatically included in Home Assistant backups.

## Important Notes

### Console Feature

As of Dockge 1.5.0, the console/terminal feature is disabled by default for security. Since this app uses the official Docker image, the console is not available unless you build a custom image with `DOCKGE_ENABLE_CONSOLE=true`.

### Docker Socket Access

This app requires access to the Docker socket to manage containers. This grants the app full control over your Docker environment.

### Protection Mode

For security reasons, this app requires protection mode to be disabled in the app configuration.

## Updating

The app automatically tracks official Dockge releases. Updates appear in the Home Assistant UI when available.

To manually check for updates, run:
```bash
/config/addons/dockge/update-dockge-version.sh
```

## Troubleshooting

### Cannot access Docker socket
Ensure protection mode is disabled in the app configuration.

### Stacks not persisting after restart
Check that `/opt/stacks` is correctly mapped and has write permissions.

### Ingress not working
Ensure the ingress port is set to 5001 (the default Dockge port).
