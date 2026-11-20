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

### Option: `puid`

The user ID to run Dockhand as. Default is `0` (root).

With another ID, Dockhand's own start script takes ownership of its data folders for that user and gives it access to the Docker socket. Stack folders created while it ran as root stay root-owned until you change their owner once.

### Option: `pgid`

The group ID to run Dockhand as. Default is `0` (root).

## First Start

Dockhand starts with no environment. Under **Settings > Environments**, add one of type **Unix socket** with the path `/var/run/docker.sock`. That is the Docker of this Home Assistant host.

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

1. **Via Sidebar**: Click the Dockhand icon in Home Assistant
2. **Direct HTTP**: `http://[your-ip]:3000`

Dockhand forbids other sites from showing it in a frame. That stays so on its own port. Through the sidebar it allows Home Assistant, and nothing else, to frame it.

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

The app follows Dockhand's releases. A new version arrives as an update in Home Assistant once it has passed this repository's tests.

## Stacks

Dockhand deploys stacks with `docker compose`, which the app includes together with `buildx`. Stack files live in `/data/dockhand/stacks`. A relative path in a stack, such as `./config`, is mounted from that folder.

## Known Limitations

### Home Assistant's Own Containers Are Listed

Dockhand lists every container on the host, including the ones Home Assistant manages. Its own way of hiding a container is a label on that container (`dockhand.hidden=true`), which cannot be set on containers Dockhand did not create. Stopping or removing a container that Home Assistant manages disturbs Home Assistant.

### The Sidebar Panel

- Dockhand's own sign-in, if you turn it on, is separate in the panel and on the direct port.
- A panel opened in a tab of its own does not survive the browser's reload button. Reopen it from the sidebar.

## Port Information

- **3000**: Web interface (HTTP). To reach it on another port, change the host port under the app's **Network** settings.
- **8099**: Used inside the app for the sidebar panel. Not published.

The terminal and the live views use WebSockets, which also work through the sidebar.

## Licensing

Dockhand uses the Business Source License 1.1:
- **Free Edition**: All core features, unlimited containers, no restrictions
- **SMB Edition** ($499/host/year): Commercial license, premium support
- **Enterprise Edition** ($1,499/host/year): RBAC, LDAP/AD, audit logging
