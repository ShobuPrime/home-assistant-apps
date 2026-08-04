# Dockhand Documentation

## Configuration

### Option: `log_level`

The `log_level` option controls the level of log output by the app:
- `trace`: Show every detail
- `debug`: Shows detailed debug information
- `info`: Normal (usually) interesting events (default)
- `warning`: Exceptional occurrences that are not errors
- `error`: Runtime errors
- `fatal`: Critical errors

### Option: `port`

The port Dockhand listens on internally. Default is `3000`.

Change this if you have a port conflict with another service. Note that ingress always uses port 3000 internally, so this option is primarily useful for direct access.

### Option: `puid`

The user ID to run Dockhand as. Default is `0` (root) for Docker socket access.

### Option: `pgid`

The group ID to run Dockhand as. Default is `0` (root) for Docker socket access.

## Features

### Container Management
- Start, stop, pause, restart, and remove containers
- Real-time log streaming with ANSI color support
- Terminal/shell access to containers
- Container inspection and health monitoring
- Auto-update scheduling

### Stack Management
- Docker Compose stack creation and management
- Git repository integration with auto-sync
- Visual YAML editor
- Environment variable management
- Webhook triggers for CI/CD

### Image Management
- Pull images from registries
- Vulnerability scanning (Grype/Trivy integration)
- Image cleanup and management

### Multi-Environment Support
- Manage local Docker via Unix socket
- Connect to remote Docker hosts
- Hawser agent for complex network setups

## Access Methods

1. **Via Sidebar**: Click the Dockhand icon in Home Assistant (uses ingress)
2. **Direct HTTP**: `http://[your-ip]:3000`

## Data Persistence

All data is stored in `/data/dockhand` and included in Home Assistant backups:
- `db/`: SQLite database
- `stacks/`: Git-cloned stacks and compose files

## Authentication

Dockhand authentication is **disabled by default** on first launch. You can enable it via:
1. Go to **Settings > Authentication** in the Dockhand UI
2. Create your first admin user

**Note**: Once authentication is enabled, all users have full admin access (Enterprise edition required for RBAC).

## Updating

The app tracks Dockhand releases via the upstream [changelog.json](https://github.com/Finsys/dockhand/blob/main/src/lib/data/changelog.json). GitHub Actions automatically check for new versions daily and create PRs when updates are available.

To manually check for updates:
```bash
cd dockhand
./update-dockhand-version.sh --check-only
```

To apply an update:
```bash
./update-dockhand-version.sh
```

## Known Limitations

### No Container Label Filtering

Unlike Portainer, Dockhand does **NOT** support hiding containers by labels. There is no `--hide-label` equivalent or configuration option to hide Home Assistant system containers (`io.hass.type` labels).

**Workaround**: Dockhand provides a UI toggle to show/hide stopped containers, and you can use the search/filter functionality to find specific containers by name or image.

### No CSP Configuration

Dockhand does not expose Content Security Policy configuration. However, unlike Portainer 2.33+, Dockhand does not set restrictive CSP headers by default, so ingress should work without additional configuration.

## Port Information

- **3000**: Web interface (HTTP)

WebSocket support is required for terminal and real-time features. The ingress configuration (`ingress_stream: true`) ensures this works through Home Assistant.

## Licensing

Dockhand uses the Business Source License 1.1:
- **Free Edition**: All core features, unlimited containers, no restrictions
- **SMB Edition** ($499/host/year): Commercial license, premium support
- **Enterprise Edition** ($1,499/host/year): RBAC, LDAP/AD, audit logging
