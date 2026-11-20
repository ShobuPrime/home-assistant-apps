# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Home Assistant App for Dockge, a Docker Compose stack manager. It
copies Dockge from upstream's image onto the repository's Alpine base and runs
it from its TypeScript source with `tsx`, as upstream does.

## Essential Commands

```bash
# Build (there is no build.sh)
docker build --build-arg BUILD_FROM=ghcr.io/hassio-addons/base:21.0.11 \
    --build-arg BUILD_ARCH=amd64 --build-arg BUILD_VERSION=<version> \
    -t local/amd64-addon-local_dockge:<version> dockge

# Full smoke test with a mock Supervisor (from repo root)
.github/scripts/smoke-test.sh dockge local/amd64-addon-local_dockge:<version>
```

## Version Scheme and Updates

Upstream's last release is 1.5.0 (2025-03-30). No workflow watches for a new
one. A bump changes the `FROM` tag and `ARG DOCKGE_VERSION` in the Dockerfile,
`args` in `build.yaml` and `version` in `config.yaml` together, and needs the
smoke test, which checks the parts of Dockge this app depends on.

## Architecture and Key Components

### Directory Structure
- **`/rootfs/etc/cont-init.d/dockge.sh`**: data folders, the link from the host's name for `/data`, Traefik
- **`/rootfs/etc/services.d/dockge/`**: points Dockge at its stacks and at the filter, runs it
- **`/rootfs/etc/services.d/dockge-filter/`**: runs `ha-docker-filter` under a user of its own when `hide_hassio_containers` is on
- **`/rootfs/etc/services.d/dockge-ingress/`**: runs `ha-ingress-adapter` as `nobody`
- **`/rootfs/usr/local/bin/docker`**: wrapper that gives Compose the host's name for a stack's folder
- **`/ha-docker-filter/`, `/ha-ingress-adapter/`**: copies of helpers other apps carry too; CI fails a PR that changes one copy and not the others
- **`/tests/ingress_test.py`**: opens the panel in a real browser behind a stand-in for Home Assistant

### Port Configuration
- **5001**: Dockge's web interface
- **8099**: `ha-ingress-adapter`, the ingress port. Not published

## Dockge does everything with the docker CLI

Dockge never opens the Docker socket itself. Listing, deploying, logs and the
terminal are all `docker compose ...` processes, plain children or in a
pseudo-terminal, and both inherit Dockge's environment. Until 1.5.0.2 the
image had no `docker` at all, and the app could do nothing. Alpine's
`docker-cli`, `docker-cli-compose` and `docker-cli-buildx` work on aarch64
(checked on a device for another app, 2026-11-20).

## Stacks and the host's name for /data

Compose turns a stack's relative folders (`./config`) into absolute paths and
hands them to the Docker daemon, which looks for them on the host. Upstream
mounts the stacks folder at the same path on both sides ("Left Stacks Path ===
Right Stacks Path"), which an app cannot ask for. So:

- cont-init reads this container's mounts through the socket and finds the
  host's path for `/data` (on HAOS `/mnt/data/supervisor/addons/data/<slug>`).
  It makes that path a link to `/data` inside the container and writes it to
  `/run/dockge/host-data`.
- Dockge's stacks folder is `<that path>/stacks`.
- That is not enough by itself. Dockge starts Compose with the stack's folder
  as working directory, the kernel reports the folder by its real name
  (`/data/stacks/x`), and Go's `os.Getwd` takes `PWD` only when `PWD` names
  the same folder. Node does not set `PWD`. So `/usr/local/bin/docker`, which
  is ahead of `/usr/bin` on `PATH`, sets `PWD` to the host's name and runs
  the real `docker`.

Without the wrapper, a relative folder becomes `/data/stacks/x/config` on the
host, which Docker creates there or fails on. The smoke test's stack uses
`create_host_path: false`, so a regression fails instead of leaving a folder
on the machine running the test.

`stacks_dir` was removed in 1.5.0.2: its default, `/opt/stacks`, was in the
container's layer and lost at every restart, and no other folder gets the
host-path treatment.

## Hiding what Home Assistant manages

Dockge lists Compose projects only (`docker compose ls`), never single
containers. With `hide_hassio_containers` on, `DOCKER_HOST` points the CLI at
`ha-docker-filter`, which leaves containers carrying Home Assistant's marks out
of the container list, so a project made of them is not listed. Root
`CLAUDE.md`, "Telling Home Assistant's containers from the user's", has the
rule and where the filter's design is written down. Dockge has no prune, so
the filter's prune handling does not come into play here.

Dockge waits for the filter to answer before it starts: a hard stop leaves
the filter's socket behind in `/run`, the container's own layer.

## The sidebar panel

Dockge's page loads `/assets/...`, draws its logo with `<object
data="/icon.svg">` and opens socket.io at `/socket.io`, all from the site
root. Under ingress that is Home Assistant. `ha-ingress-adapter` (root
`CLAUDE.md`, "An app built for the site root can still run under ingress")
serves the ingress port and keeps all of it under the ingress path. Dockge
sends no framing headers.

The browser test reaches Dockge's root component as
`document.querySelector('#app')._vnode.component.proxy`: a production Vue
build has no `__vue_app__._instance`. It signs in through Dockge's own socket
events and deploys a stack with `emitAgent('', 'deployStack', ...)`, as the
editor does.

## Important Notes

- **Protection mode** must be disabled for Docker socket access
- **The console page stays off** (`DOCKGE_ENABLE_CONSOLE` unset): it would be
  a root shell in this container, next to the real socket
- **Stack names** must match `^[a-z0-9_-]+$` (Dockge's rule)
