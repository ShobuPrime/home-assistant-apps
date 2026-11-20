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

When enabled (default), the containers Home Assistant manages are left out of Portainer's container list. Your own containers and stacks are not affected.

- **It hides, it does not lock.** This is Portainer's own "Hidden containers" list. A hidden container can still be reached by its ID, and the counters, images and networks are not filtered.
- **Turning it off does not show them again.** Portainer keeps its stored list. Remove the entries under Settings → Hidden containers.
- **While it is on, the app sets that list at every start.** Entries added by hand under Settings → Hidden containers are replaced at the next restart.

A container counts as managed by Home Assistant when the Supervisor started it, or when it carries the label `io.github.shobuprime.app-managed=true`, which an app puts on containers it starts for itself.

## Access Methods

1. **Via Sidebar**: Click the Docker icon in Home Assistant (uses ingress)
2. **Direct HTTP**: `http://[your-ip]:9000`
3. **Direct HTTPS**: `https://[your-ip]:9443`

## Port Information

- **8000**: Edge agent tunnel service (for remote agent connections)
- **9000**: HTTP web interface
- **9443**: HTTPS web interface

## Docker Socket Shim

Portainer does not talk to Docker directly in this app. It talks to a small
nginx proxy inside the container, which forwards to the real socket:

```
portainer  ->  /var/run/docker.sock (symlink)
           ->  /run/docker-shim/docker.sock   (nginx)
           ->  /run/docker.sock               (the socket the Supervisor mounts)
```

**Why.** Docker removed support for a request body on
`POST /containers/{id}/start` in API v1.24, and dockerd now rejects any start
request whose body length is unknown. Portainer's Docker-API proxy forwards the
transfer encoding it received from your browser, and requests arriving through
Home Assistant's streaming ingress have an unknown length — so every attempt to
start a container failed with:

> Failed starting container: starting container with non-empty request body was
> deprecated since API v1.22 and removed in v1.24

That check runs before dockerd looks at the container, so the message described
no container in particular — and it replaced whatever the *real* start failure
was. The shim strips the body on the lifecycle verbs where one is never legal
(`start`, `stop`, `restart`, `kill`, `pause`, `unpause`, `wait`) and passes
everything else through byte-for-byte, so Portainer once again shows Docker's
actual per-container errors.

**Nothing to migrate.** Portainer stores its environment address in its own
database, so the shim was put where that address already points rather than
somewhere new — upgrading an existing install needs no UI changes and keeps
your stacks attached to their environment.

**If you ever need the raw socket** (to compare behaviour, or because you added
a second environment by hand), it is still at `/run/docker.sock` inside the
container and is untouched:

```bash
# from inside the app container — the request the shim exists to fix
curl -s -w '\n%{http_code}\n' --unix-socket /run/docker.sock \
  -X POST -H 'Content-Type: application/json' -H 'Transfer-Encoding: chunked' \
  -d '{}' http://localhost/containers/<a-running-container>/start
# 400 on the raw socket, 304 through /run/docker-shim/docker.sock
```

If nginx fails to start, the app logs an error and falls back to the raw
socket: Portainer still works, with the container-start bug back.

## Data Persistence

All data is stored in `/data/portainer` and included in Home Assistant backups.

## Updating

The app automatically tracks STS releases. Updates appear in the Home Assistant UI when available.

To manually check for updates:
```bash
/root/addons/portainer-ee/update-portainer-version.sh
```

