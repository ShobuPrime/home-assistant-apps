# Dockge Documentation

## About

Dockge manages Docker Compose stacks: a compose.yaml editor, live status and logs, and a terminal into a stack's containers.

## Configuration

### Option: `log_level`

The `log_level` option controls the level of log output by the app:
- `trace`: Show every detail
- `debug`: Shows detailed debug information
- `info`: Normal (usually) interesting events (default)
- `warning`: Exceptional occurrences that are not errors
- `error`: Runtime errors
- `fatal`: Critical errors

### Option: `hide_hassio_containers`

When enabled (default), Compose projects whose containers Home Assistant manages are left out of Dockge. Your own stacks, and Compose projects started some other way, are listed as before. Turn it off to have Dockge list every Compose project on the host.

- **It hides, it does not lock.** A hidden container can still be reached by its name or ID.
- Dockge lists Compose projects, never single containers. Home Assistant's own containers are not Compose projects, so what this option hides are the projects an app starts for itself and marks with the label `io.github.shobuprime.app-managed=true`.

## First Start

Dockge asks for an admin account on the first visit.

## Stacks

Stacks are kept in this app's data folder, so they survive restarts and updates and are part of this app's backups. Uninstalling the app deletes them, together with the folders their compose files mount by a relative path.

- **A relative path** (`./config`) is mounted from the stack's own folder.
- **An absolute path** names a folder on the host system, where Home Assistant's folders have other names: `/share` is `/mnt/data/supervisor/share` there.

Dockge deploys stacks with `docker compose`, which the app includes together with `buildx`.

## Access Methods

1. **Via Sidebar**: Click the Dockge icon in Home Assistant
2. **Direct HTTP**: `http://[your-ip]:5001`

The panel and the direct port each have their own sign-in.

## Port Information

- **5001**: Web interface (HTTP)
- **8099**: Used inside the app for the sidebar panel. Not published.

## Known Limitations

- **The console page stays off.** It would be a shell in this app's container, with full access to Docker. The terminal into a stack's containers works.
- **A panel opened in a tab of its own** does not survive the browser's reload button. Reopen it from the sidebar.

## Docker Socket Access

This app requires access to the Docker socket to manage containers. This grants the app full control over your Docker environment, and it requires protection mode to be disabled in the app configuration.

## Troubleshooting

### Cannot access Docker socket
Ensure protection mode is disabled in the app configuration.

### Stacks made with 1.5.0.1 or earlier are gone
They were kept inside the container, which Home Assistant replaces at every restart and update. Containers they started keep running and are listed as "not managed by Dockge". Create the stack again in Dockge with the same name to manage them.
