# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Home Assistant App for Dockhand, a Docker management web UI with
Compose stacks. The app uses Home Assistant's S6-overlay init system and
follows standard HA app conventions.

## Essential Commands

### Building and Testing
```bash
# Build the app locally (auto-detects architecture)
./build.sh

# Full smoke test with a mock Supervisor (from repo root)
.github/scripts/smoke-test.sh dockhand local/amd64-addon-local_dockhand:<version>

# The adapter's unit tests
(cd ha-ingress-adapter && go test ./...)
```

### Version Management
The bot does it every day (`.github/workflows/update-dockhand.yml`). The same
check by hand, from this directory:
```bash
APP_PATH=. CHECK_ONLY=true JSON_OUTPUT=true bash ../.github/scripts/update-dockhand.sh
```

## Dockhand Version Scheme

Semantic versioning, `[MAJOR].[MINOR].[PATCH]`. The update script reads the
latest version from upstream's `src/lib/data/changelog.json`. The image is
`fnsys/dockhand:v<version>`, pulled through `mirror.gcr.io`.

## Architecture and Key Components

### Directory Structure
- **`/rootfs/etc/cont-init.d/`**: creates `/data/dockhand` and links it as the app's data folder
- **`/rootfs/etc/services.d/dockhand/`**: sets the environment and runs upstream's start script
- **`/rootfs/etc/services.d/dockhand-ingress/`**: runs `ha-ingress-adapter` as `nobody`
- **`/rootfs/etc/services.d/dockhand-filter/`**: runs `ha-docker-filter` under a user of its own when `hide_hassio_containers` is on
- **`/ha-ingress-adapter/`**: the proxy behind the sidebar panel (Go, stdlib only) and `shim.js`, the script it adds to the page
- **`/ha-docker-filter/`**: the Docker socket filter. A copy: other apps carry the same one, and CI fails a PR that changes one copy and not the others
- **`/tests/ingress_test.py`**: opens the panel in a real browser behind a stand-in for Home Assistant

### Critical Files
- **`config.yaml`**: version, ports, ingress, options schema
- **`translations/en.yaml`**: option names and descriptions shown in the Configuration tab
- **`build.yaml`**: base images per architecture
- **`Dockerfile`**: takes the application and its start script out of upstream's image
- **`apparmor.txt`**: security profile

### Architecture Support
`amd64` and `aarch64`, as upstream.

### Port Configuration
- **3000**: Dockhand's web interface. Fixed: the published port, the health check and the watchdog name it. There was a `port` option until 1.0.58.1, and any other value broke all three
- **8099**: `ha-ingress-adapter`, the ingress port. Not published

## Started the way upstream starts it

The Dockerfile copies `/app` and `/usr/local/bin/docker-entrypoint.sh` from
upstream's image, and the service script ends in `exec` of that script. It
picks the command (`node ... /app/server.js`), takes ownership of the data
folders for `PUID`, adds that user to the Docker socket's group and drops to
it. None of that is reproduced here, so it follows upstream.

- **`/app` is a link to `/opt/dockhand`**, because the script names `/app`.
  The build fails if the script stops naming `/app/server.js`: read the new
  script before changing the service.
- **`server.js` is the entry point, not `build/index.js`.** It wraps the bundle
  and serves the container shell's WebSocket. Until 1.0.58.1 the app ran the
  bundle alone and the shell could not work.
- **`docker` and `docker-compose` are not copied from upstream's image.** Its
  binaries are linked against glibc. Alpine's `docker-cli`,
  `docker-cli-compose` and `docker-cli-buildx` are used. Dockhand calls
  `docker compose` in the plugin form, which works on arm64 with these
  packages (checked on a device, 2026-11-20).
- **The system CA store is patched in** (`Dockerfile`), in the bundle and in
  `server.js`. The build fails on a use it did not reach.
- **Dockhand finds its data folder on the host by itself**, from its own
  container's mounts. A relative path in a stack is a host path to the Docker
  daemon, so stacks depend on it. The smoke test deploys a stack with one and
  checks where it was mounted from.

## The sidebar panel

Two things kept the panel empty, and each alone is enough:

1. Dockhand sends `X-Frame-Options: DENY` and `frame-ancestors 'none'` (since
   1.0.57). A panel is a frame.
2. Its web interface calls `/api/...` from the site root. Under ingress that is
   Home Assistant, not Dockhand. Only the page shell loads.

`ha-ingress-adapter` listens on the ingress port and adapts requests that
carry Home Assistant's `X-Ingress-Path`. Root `CLAUDE.md`, "An app built for
the site root can still run under ingress", has what it does. For Dockhand:

- **Framing is relaxed for those requests only**, to `SAMEORIGIN` and
  `frame-ancestors 'self'`. Home Assistant serves the panel from its own
  origin, so that allows Home Assistant and nothing else. Dockhand's own port
  keeps refusing. `DISABLE_FRAME_PROTECTION` is not set: it would lift the
  protection everywhere.
- **The page's entry imports are relative** (`import("./_app/...")`). The
  adapter rewrites them to name the prefix, because the page's address has
  lost it by the time they run.
- **A new install has no environment.** The user adds the local socket once
  (DOCS.md, "First Start").

Not covered: Dockhand's own sign-in inside the panel (untested), and a panel
opened in a tab of its own, which the browser's reload button takes to an
address that belongs to Home Assistant.

### Testing it

`go test ./...` in `ha-ingress-adapter/`, and `tests/ingress_test.py`. The
stand-in for Home Assistant and the browser driver are shared, in
`.github/scripts/ingress_harness.py`. The test first runs a control: the same
page with nothing adapting it must stay empty.

```bash
python3 dockhand/tests/ingress_test.py --ingress http://<container ip>:8099 \
    --direct http://<container ip>:3000 --container <a running container> --shots /tmp/shots
```

## Hiding what Home Assistant manages

Dockhand's only hide mechanism is a label that has to sit on the hidden
container (`dockhand.hidden=true`), and it does not create Home Assistant's.
So with `hide_hassio_containers` on, Dockhand reaches Docker through
`ha-docker-filter`. Root `CLAUDE.md`, "Telling Home Assistant's containers
from the user's", has the rule and says where the filter's design is written
down. What is particular to Dockhand:

- **The filter sits at `/var/run/docker.sock`.** Dockhand stores a socket path
  per environment (default `/var/run/docker.sock`), and its start script, its
  `docker compose` children and its look at its own container use that path.
  Environment variables do not move them. So cont-init makes `/var/run`, a
  link to `/run` in the base image, a directory holding
  `docker.sock -> /run/docker-filter/docker.sock`. This is the `/var/run`
  swap root `CLAUDE.md` describes for a socket shim, under the same
  conditions; s6-overlay restores the link at every boot.
- **The link carries the socket's group** (`chown -h`). Upstream's start script
  reads the group with `stat` and does not follow the link. Without it, a
  `puid` other than 0 is given group 0 and no Docker.
- **An environment on `/run/docker.sock` goes round the filter.** That path is
  the user's choice, so cont-init names such environments in the log and
  changes nothing.
- **Dockhand's image prune always brings a `label!`** (`dockhand.prune=false`).
  The filter asks Docker what carries it: nothing, and it gives way to the
  exclusion that spares Home Assistant's images; something, and the prune is
  refused with a 409 that says why. A dangling-only prune has no `label!`.
- **Its volume prune is its own**: it deletes, one by one, every volume no
  listed container uses. Docker refuses one that a hidden container uses, so
  that ends in an error, not a loss.
- **Dockhand waits for the filter to answer**, not for its socket: a hard stop
  leaves the socket behind in `/run`, which is the container's layer here.

Tests: the smoke test checks the link, the users, Dockhand's own list against
stand-ins with the raw socket as control, events and an image prune shaped
like Dockhand's. Checked by hand on 2026-11-20 and not in the smoke test: the
option off lists everything; `puid: 1000` reaches Docker through the filter;
an in-place restart sets the link again and names an environment on
`/run/docker.sock`.

## Development Guidelines

### Configuration Handling
- Read options using `bashio::config` functions
- Protection mode must be disabled for Docker socket access
- Data is stored in `/data/dockhand`

### Environment Variables
Set by the service script: `PORT`, `HOST`, `DATA_DIR`, `PUID`, `PGID`, `HOME`,
`NODE_ENV`, `LOG_LEVEL`. Upstream's script reads `PUID`, `PGID` and `DATA_DIR`.

### Traefik
`generate-traefik-config.sh` routes to the ingress port by default. Here that
is the adapter, which is not published, so cont-init passes `TRAEFIK_PORT`:
the host port of 3000.

### Testing Checklist
- Build completes
- The smoke test passes: entry point, a stack deployed and removed through
  Dockhand's API, framing headers, the panel in a browser
- Data persists in `/data/dockhand` across restarts

## Important Notes

- **Never commit changes** to version numbers without testing
- **Protection mode** must be disabled for the app to function
- **Authentication is off by default** in Dockhand, and its port is published.
  It logs a warning about that at every start
- **Home Assistant's containers are hidden by default** (`hide_hassio_containers`,
  see "Hiding what Home Assistant manages")

## Common Issues and Troubleshooting

### Issue: The Sidebar Panel Is Empty After a Dockhand Update

**Symptoms:**
- The panel is blank or shows no data; port 3000 works
- The smoke test fails in the panel section

**Solution:**
1. Run `tests/ingress_test.py` with `--shots` and read its FAIL lines
2. "no request went to Home Assistant itself" fails: the web interface addresses the server in a new way; add it to `shim.js`
3. The page shows nothing at all: its shell changed shape; fix the patterns in `main.go`

### Issue: The Build Fails After a Dockhand Update

**Symptoms:**
- "Upstream no longer starts /app/server.js"
- "Unpatched Node root store use"

**Solution:**
1. Read upstream's `docker-entrypoint.sh` in the new image and adjust `services.d/dockhand/run`
2. Extend the `sed` in the Dockerfile to the new use of the root store

### Issue: Database Errors

**Symptoms:**
- "Database locked" errors
- Data not persisting

**Solution:**
1. Check disk space on Home Assistant
2. Verify `/data/dockhand` has proper permissions
3. If corrupted, delete the `db/` directory and restart (will lose settings)
