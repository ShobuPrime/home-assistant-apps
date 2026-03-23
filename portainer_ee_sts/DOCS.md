# Portainer EE Documentation

## Configuration

### Option: `log_level`

The `log_level` option controls the level of log output by the app:
- `trace`: Show every detail
- `debug`: Shows detailed debug information
- `info`: Normal (usually) interesting events (default)
- `warning`: Exceptional occurrences that are not errors
- `error`: Runtime errors
- `fatal`: Critical errors

### Option: `ssl`

Enables/Disables SSL (HTTPS) on port 9443 using custom SSL certificates.
- `true`: Use SSL with certificates from `/ssl/fullchain.pem` and `/ssl/privkey.pem`
- `false`: Use self-signed certificates (default)

Note: Both HTTP (port 9000) and HTTPS (port 9443) are always available.

### Option: `agent_secret`

Sets a secret for Portainer agents when managing remote Docker environments.

### Option: `hide_hassio_containers`

When enabled (default), hides Home Assistant system containers from the Portainer interface.
- `true`: Hide all containers with `io.hass.type` labels (supervisor, core, audio, dns, multicast, cli, observer, addon)
- `false`: Show all containers

**Important**: Due to how Portainer caches settings, changes to this option may require manual intervention:
1. If changing from `true` to `false`: Go to Portainer Settings → Hidden containers to unhide them
2. If changing from `false` to `true`: The containers will be hidden automatically on next restart

## Access Methods

1. **Via Sidebar**: Click the Docker icon in Home Assistant (uses ingress)
2. **Direct HTTP**: `http://[your-ip]:9000`
3. **Direct HTTPS**: `https://[your-ip]:9443`

## Port Information

- **8000**: Edge agent tunnel service (for remote agent connections)
- **9000**: HTTP web interface
- **9443**: HTTPS web interface

## Data Persistence

All data is stored in `/data/portainer` and included in Home Assistant backups.

## Updating

The app automatically tracks STS releases. Updates appear in the Home Assistant UI when available.

To manually check for updates:
```bash
/root/addons/portainer-ee/update-portainer-version.sh
```

