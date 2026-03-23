# Arcane Docker Manager Documentation

## Overview

Arcane is a modern, open-source Docker management platform that provides an intuitive web interface for managing your Docker environment. It's designed as a lightweight, beautiful alternative to Portainer with a focus on simplicity, speed, and a clean user interface.

## Configuration

### Option: `log_level`

The `log_level` option controls the level of log output by the app:
- `trace`: Show every detail
- `debug`: Shows detailed debug information
- `info`: Normal (usually) interesting events (default)
- `warning`: Exceptional occurrences that are not errors
- `error`: Runtime errors
- `fatal`: Critical errors

### Option: `app_url`

The base URL for accessing Arcane. This is used for generating links and webhook URLs.

- Leave empty for default (`http://localhost:3552`)
- Set to your domain if using a reverse proxy (e.g., `https://arcane.example.com`)

### Option: `puid` / `pgid`

User ID and Group ID for file permissions when Arcane creates files.

- Default: `1000` for both
- Change if you need specific ownership for mounted volumes

### Option: `hide_hassio_containers`

When enabled (default), hides Home Assistant system containers from the Arcane interface.
- `true`: Hide supervisor, core, audio, dns, multicast, cli, observer, and app containers
- `false`: Show all containers

## Access Methods

1. **Via Sidebar**: Click the Arcane icon in Home Assistant (uses ingress)
2. **Direct HTTP**: `http://[your-ip]:3552`

## Port Information

- **3552**: Main web interface
- **3553**: Agent communication port (for multi-host setups)

## Data Persistence

All data is stored in `/data/arcane` and included in Home Assistant backups:

- `arcane.db` - SQLite database with all settings and data
- `.secrets` - Auto-generated encryption keys (do not delete!)
- `projects/` - Docker Compose project files (if using Compose management)

## Features

### Container Management
- View, start, stop, restart, and remove containers
- Real-time logs with search and filtering
- Execute commands inside containers
- Inspect container details and configuration

### Resource Monitoring
- Real-time CPU and memory usage graphs
- Network I/O statistics
- Container health status

### Docker Compose
- Deploy and manage Compose stacks
- Built-in template library
- Edit compose files directly in the UI

### Image Management
- View all images with size information
- Pull new images
- Remove unused images
- Automatic image update detection

### Volume & Network Management
- Create, inspect, and remove volumes
- Identify unused volumes for cleanup
- Network creation and management

### System Cleanup
- Built-in Docker prune functionality
- Clean up unused images, volumes, and networks
- Reclaim disk space safely

## First Time Setup

1. Access Arcane through the sidebar or direct URL
2. Default login credentials:
   - Username: `arcane`
   - Password: `arcane-admin`
3. **Change the password immediately** after first login
4. Your Docker environment is automatically detected

## Multi-Host Management

Arcane supports managing multiple Docker hosts using agents:

1. Install the Arcane agent on remote hosts:
   ```bash
   docker run -d \
     --name arcane-agent \
     -p 3553:3553 \
     -v /var/run/docker.sock:/var/run/docker.sock \
     ghcr.io/getarcaneapp/arcane-headless:latest
   ```

2. Add the remote environment in Arcane UI
3. Use the Bootstrap Token for secure pairing

## Security Considerations

- **Protection Mode**: Must be disabled for Docker socket access
- **Default Credentials**: Change immediately after first login
- **Encryption Keys**: Auto-generated and stored in `/data/arcane/.secrets`
- **AppArmor**: Custom profile restricts app permissions appropriately

### Optional: Socket Proxy

For enhanced security, you can use a Docker Socket Proxy:

1. Set up Tecnativa Docker Socket Proxy on your host
2. Configure Arcane to use the proxy instead of direct socket access
3. Limit API access to only required endpoints

## Troubleshooting

### Arcane Won't Start

1. Check logs in Home Assistant Supervisor
2. Verify Docker socket is accessible
3. Ensure protection mode is disabled
4. Check if port 3552 is available

### Can't See Containers

1. Verify Docker socket permissions
2. Check if containers have labels that hide them
3. Restart the app

### Database Issues

If the database becomes corrupted:

1. Stop the app
2. Delete `/data/arcane/arcane.db`
3. Restart the app (you'll need to reconfigure)

### WebSocket Connection Issues

If real-time updates aren't working:

1. Check if ingress is properly configured
2. Verify no firewall is blocking WebSocket connections
3. Try accessing directly via port 3552

## Updating

The app automatically tracks Arcane releases. Updates appear in the Home Assistant UI when available.

## Known Limitations

- Only supports amd64 and aarch64 architectures
- armv7/armhf not currently supported by Arcane upstream
- Agent mode requires separate installation on remote hosts

## External Resources

- [Arcane Documentation](https://getarcane.app/docs)
- [Arcane GitHub](https://github.com/getarcaneapp/arcane)
- [Docker Documentation](https://docs.docker.com)
