# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Home Assistant App for Arcane Docker Manager that provides modern Docker container management through the Home Assistant interface. The app uses Home Assistant's S6-overlay init system and follows standard HA app conventions.

## Essential Commands

### Building and Testing
```bash
# Build the app locally (auto-detects architecture)
./build.sh

# Test the app locally
docker run --rm -it -p 3552:3552 -p 3553:3553 -v /var/run/docker.sock:/var/run/docker.sock local/{arch}-addon-local_arcane:{version}
```

### Version Management
The bot does it every day (`.github/workflows/update-arcane.yml`). The same check by hand, from this directory:
```bash
APP_PATH=. CHECK_ONLY=true JSON_OUTPUT=true bash ../.github/scripts/update-arcane.sh
```

## Arcane Version Scheme

Arcane uses semantic versioning: `v[MAJOR].[MINOR].[PATCH]`

The update script fetches the latest release from GitHub and updates all relevant files. Arcane follows a single release stream.

## Architecture and Key Components

### Directory Structure
- **`/rootfs/etc/cont-init.d/`**: S6 initialization scripts that run on container start
- **`/rootfs/etc/services.d/arcane/`**: Service definition with `run` script and `finish` handler
- **`/rootfs/etc/services.d/arcane-ingress/`**: runs `ha-arcane-ingress` as `nobody`
- **`/rootfs/etc/services.d/arcane-filter/`**: runs `ha-docker-filter` under a user of its own when `hide_hassio_containers` is on
- **`/ha-docker-filter/`**: the filter between Arcane and the Docker socket (Go, stdlib only)
- **`/ha-arcane-ingress/`**: the ingress proxy (Go, stdlib only) and `shim.js`, the script it adds to Arcane's page
- **`/tests/ingress_test.py`**: opens the panel in a real browser behind a stand-in for Home Assistant
- **`/tests/upstream_paths.py`**: reads Arcane's default paths out of its binary and checks the image provides them
- **`/rootfs/usr/share/arcane/migrate-settings.sh`** and **`/tests/migrate-settings-test.sh`**: move a stored path an early release left behind (until 2027-06-01)

### Critical Files
- **`config.yaml`**: App configuration (version, ports, ingress, options schema)
- **`translations/en.yaml`**: Plain-English config-option names + descriptions shown in the HA Configuration tab (kept in sync with `config.yaml`'s options)
- **`build.yaml`**: Build configuration with base images per architecture
- **`Dockerfile`**: Downloads Arcane binary and sets up environment
- **`apparmor.txt`**: Security profile for Docker socket access

### Architecture Support
Arcane only supports:
- `amd64` (x86_64)
- `aarch64` (arm64)

armv7/armhf is NOT supported by upstream Arcane.

### Port Configuration
- **3552**: Web interface, straight to Arcane
- **3553**: Agent communication port
- **3554**: `ha-arcane-ingress`, the ingress port. Not published

## Ingress (the sidebar panel)

Arcane only works at the top of a site, and upstream closed the request for a
sub-path without adding one (getarcaneapp/arcane#576). `ha-arcane-ingress`
listens on 3554 and adapts requests that carry Home Assistant's
`X-Ingress-Path`. Everything else, and ports 3552 and 3553, pass untouched.

**Not SvelteKit's `base`.** The page reads `base` at run time, so the proxy
could set it. But Arcane compares the page's path with `/login` and `/logout`
(`+layout.ts`, `api-service.ts`), and under a base those never match: sign-in
loops. Arcane has to see root-relative paths.

The proxy (`main.go`):

| For | It |
|---|---|
| pages | moves `href="/..."`, `src="/..."` and `import("/...")` under the prefix, drops the manifest link, loads `shim.js` ahead of the first script |
| style sheets | moves `url(/...)` under the prefix (the fonts) |
| `Set-Cookie` | renames to `ingress.<name>`, scopes `Path` to the prefix, adds `Secure` when Home Assistant was reached over HTTPS |
| `Cookie` | passes on only `ingress.*`, mark removed |
| `Location` | moves a root-relative redirect under the prefix |

The shim (`shim.js`), in the page:

1. Removes the prefix from the page's own address (`history.replaceState`), and opens `/login` rather than `/`: Arcane's `/` answers a signed-out visitor with a full page load.
2. Puts the prefix back on what goes to the server: `fetch`, `Request`, XHR, WebSocket, the `src`/`href` of images, icons and scripts, POST forms.
3. Sends full page loads (`location.reload()`, `location.replace('/login?...')`) to the prefixed address, through the Navigation API.

Facts that cost time. Each has a test:

- **Cookies are shared across ports.** `host:3552` and `host:8123` are one cookie jar, so an unmarked `token; Path=/` from the direct port also arrives through ingress. Hence the mark and the filter: signing out of the panel would otherwise leave you signed in.
- **A `__Host-` cookie may only have `Path=/`.** Arcane names its cookie `__Host-token` when it believes the request was HTTPS. The proxy does not pass `X-Forwarded-Proto` on, so Arcane uses the plain names here, and the proxy adds `Secure` itself.
- **Re-addressing a `Request` that has a body turns the body into a stream**, which Chrome uploads only over HTTP/2: the sign-in POST failed with `ERR_ALPN_NEGOTIATION_FAILED`. The shim wraps the `Request` constructor instead.
- **Home Assistant drops the charset from `Content-Type`.** The shim goes after `<meta charset>`, which browsers look for in the first 1024 bytes, and is ASCII only.
- **"Log out" is a POST form to `/logout`** that the browser submits by itself.
- **The browser's reload button fires no `navigate` event.** In the panel that is fine: Home Assistant's page reloads and frames the app again. In a tab of its own the reload asks Home Assistant for the unprefixed address.

Not covered: OIDC and passkeys (untested), links Arcane shows for outside use, and a session running out in a browser without the Navigation API (Safari before 26.2, Firefox before 147).

### Testing it

`go test ./...` in `ha-arcane-ingress/`, and `tests/ingress_test.py`: a real
browser behind a stand-in for Home Assistant ingress. The stand-in follows
Core's and Supervisor's own source, so it drops `Accept-Encoding` and the
charset as they do. It first runs a control: the same page with no proxy must
stay blank. The smoke test runs it on every build, so an upstream change that
breaks the panel fails the bot PR instead of reaching a device.

```bash
python3 arcane/tests/ingress_test.py --ingress http://<container ip>:3554 \
    --direct http://<container ip>:3552 --password <first password> --shots /tmp/shots
```

## Development Guidelines

### S6-Overlay Integration
- Use Bashio library for all configuration reading and logging
- Service scripts must be executable and use proper S6 conventions
- Exit codes: 0 for success, non-zero triggers restart with backoff

### Configuration Handling
- Read options using `bashio::config` functions
- Protection mode must be disabled for Docker socket access
- Encryption keys are auto-generated on first start and stored in `/data/arcane/.secrets`

### Environment Variables
Arcane 3 reads only `ARCANE_`-prefixed names (`services.d/arcane/run`):
- `ARCANE_ENCRYPTION_KEY`: from `ENCRYPTION_KEY` in `/data/arcane/.secrets` (auto-generated)
- `ARCANE_PORT`: Application port (3552)
- `ARCANE_DATABASE_URL`: SQLite database path
- `ARCANE_APP_URL`, `ARCANE_LOG_LEVEL`, `ARCANE_ENVIRONMENT`
- `DOCKER_HOST`, `PUID`/`PGID`: not prefixed upstream

**It refuses to start while a legacy name is set without its `ARCANE_` twin**
(`backend/internal/config/schema/legacy.go`). The base image exports
`LOG_LEVEL`, so the run script unsets it and always sets `ARCANE_LOG_LEVEL`.
Legacy names are every `ARCANE_*` setting with the prefix removed, so check a
new base-image variable against that list.

## Hiding what Home Assistant manages

Arcane has no setting for it. Its one hide mechanism is a label that has to
sit on the hidden container (`com.getarcaneapp.arcane.hidden`), and Arcane
does not create Home Assistant's. So with `hide_hassio_containers` on, Arcane
reaches Docker through `ha-docker-filter`, which `DOCKER_HOST` points at. The
rule is in `ha-docker-filter/rule.go` and in the root `CLAUDE.md`, "Telling
Home Assistant's containers from the user's".

| Request | What the filter does |
|---|---|
| `GET /containers/json`, `/images/json`, `/system/df` | removes hidden entries and counts the totals again |
| `GET /events` | drops records about hidden containers |
| `POST /containers/prune`, `/images/prune` | sends exactly one `label!`, its own, so the prune spares them |
| `POST /containers/{id}/start` and the other verbs without a body | sends them with no body |
| everything else | nothing: exec, attach, logs, builds and compose pass as they are |

It hides, it does not enforce: by ID a hidden container still answers. Arcane
acts only on what it has listed, so Update All, auto-update, auto-heal and the
scheduled prune, which plans from `/system/df`, no longer reach them.

What decided the design:

- **Arcane takes the group of the socket it is pointed at, once, at start.**
  The filter's socket gets the real socket's group and has to exist first. The
  run script waits for it and does not start Arcane without it.
- **No fallback to the real socket.** Everything would be back with no sign.
  The health check pings Docker through the filter.
- **Lists are filtered in the answer, prunes in the request.** dockerd has no
  negative label filter for a list. For a prune it has one, but several
  `label!` values mean "has all of them", so the filter sends exactly one. A
  prune that brings its own is asked what carries that label: nothing, and the
  tool's exclusion spares nothing, so it gives way; something, and both cannot
  be kept, so the prune is refused (409) and says why.
- **The filter has a user to itself.** Its socket is the Docker socket to
  whoever may open it. The panel's proxy runs as `nobody` and must not.
- **A kept entry is never re-encoded.** Sizes are 64-bit, and a JSON round trip
  rounds them.
- **Fail closed.** A list that cannot be read becomes a 502, never an
  unfiltered list.
- **Reviewed for Docker Engine API 1.56.** A list endpoint added later would
  pass unfiltered. The filter's start line says when dockerd is newer.

Not protected: an image without an `io.hass.type` label that only a stopped
app uses, and the build cache, which has no owner.

Arcane's helper containers (vulnerability scans) do not start in this app,
with the option on or off: Arcane cannot map its socket path to a path on the
host (`failed to resolve socket path`). Checked 2026-11-20 on both; this
change does not touch it.

Tests: `go test ./...` in `ha-docker-filter/` against a stand-in dockerd, and
the smoke test, which reads the same list from the raw socket (the control),
through the filter and from Arcane's own API, then kills the filter.

### Upstream's paths
Arcane's defaults point at two directories its own image provides. This image
provides the same two:

| Path | What Arcane keeps there | Here |
|---|---|---|
| `/app/data` | projects, templates, swarm sources, git `known_hosts`, its Docker config | symlink to `/data/arcane/data`. cont-init creates the target first: `MkdirAll` through a dangling symlink fails |
| `/builds` | build workspaces | a directory owned by `PUID`, lost on restart, as in upstream's image with no volume there |

`tests/upstream_paths.py` reads every path default out of the binary, where
they are struct tags (`key:"projectsDirectory" default:"/app/data/projects"`),
and the smoke test fails when one is not provided or is new. That is how a
change to upstream's layout gets noticed. Answer it by providing the path and
adding it to `KNOWN`, not by keeping an old path alive.

Upstream reads a setting from the environment (`ARCANE_<SNAKE_CASE_KEY>`), then
the `settings` table, then the default. A new install stores no directory
rows. An environment value locks the setting in the UI, so the app sets none
for directories.

### A stored path from an early release (until 2027-06-01)
Until early 2026 the app ran Arcane from `/opt/arcane`. Arcane makes a relative
directory setting absolute against its working directory and stores it, so
those installs hold `projectsDirectory = /opt/arcane/data/projects`. Arcane has
since run as `PUID` from `/data/arcane` and cannot create that folder: `mkdir
/opt/arcane/data: permission denied` in the log every few minutes, and creating
a project returns 500. Found on a device on 2026-11-19, the same on 2.17.3 and
3.1.0.

`usr/share/arcane/migrate-settings.sh` runs from cont-init as `PUID` and
rewrites values under `/opt/arcane/data` to the same path under `/app/data`
(3.1.0.2). It changes no other value, copies files first if there are any, and
never keeps Arcane from starting. It carries `LEGACY UNTIL 2027-06-01`: after
that date the smoke test fails until the script, its call and its tests are
removed (root CLAUDE.md, "Legacy handling has an end date"). DOCS.md has the
manual fix.

No test on new data shows the failure, because a new install has no such row.
`tests/migrate-settings-test.sh` covers the script. The smoke test also stores
the old path in Arcane's own database and starts Arcane on the result.
busybox `cp -n` skips a folder that exists instead of filling it, which is why
the script copies file by file.

### After an unclean stop Arcane 3 takes two minutes to come back
Measured 2026-11-19 after a `docker rm -f`: 123 s of `Waiting for the previous
actor host registration to expire` before `/api/health` answered, and Docker
marked the container unhealthy on the way. A normal stop (SIGTERM, 4 s) leaves
nothing to wait for. Not changed here.

### Testing Checklist
- Build completes successfully
- Service starts without errors
- Web UI accessible on port 3552
- Ingress access works through Home Assistant sidebar (`tests/ingress_test.py`)
- Docker containers visible in Arcane
- Configuration changes apply correctly
- Data persists in `/data/arcane` across restarts
- With `hide_hassio_containers` on, Arcane lists none of the containers Home Assistant manages

## Important Notes

- **Never commit changes** to version numbers without testing
- **Protection mode** must be disabled for the app to function
- **Ingress** integration requires WebSocket support (ingress_stream: true)
- **AppArmor profile** is critical for security - modifications require careful testing
- **First password**: a new install gets a generated password in `/data/arcane/data/initial-admin-password`; the run script prints it to the app log. `arcane-admin` was Arcane 2
- **Ingress needs `ha-arcane-ingress`**: pointed at Arcane itself, the panel is blank (see "Ingress" above)
- **Encryption keys** in `.secrets` must not be deleted or the database becomes inaccessible

## Common Issues and Troubleshooting

### Issue: Arcane Not Starting

**Symptoms:**
- App fails to start
- Logs show binary not found or permission errors

**Solution:**
1. Verify the Dockerfile downloaded the binary correctly
2. Check architecture compatibility (amd64/aarch64 only)
3. Ensure Docker socket is accessible

### Issue: The Sidebar Panel Is Blank After an Arcane Update

**Symptoms:**
- The panel is blank or shows Home Assistant itself; port 3552 works
- The smoke test fails in the panel section

**Solution:**
1. Run `tests/ingress_test.py` with `--shots` and read its FAIL lines
2. "Panel page was not adapted": Arcane's page changed shape; fix the patterns in `main.go`
3. "no request went to Home Assistant itself" fails: the web app addresses the server in a new way; add it to `shim.js`

### Issue: Database Errors

**Symptoms:**
- "Database locked" errors
- Data not persisting

**Solution:**
1. Check disk space on Home Assistant
2. Verify `/data/arcane` has proper permissions
3. If corrupted, delete `arcane.db` and restart (will lose settings)

### Issue: Arcane Has No Docker ("Cannot connect to the Docker daemon")

**Symptoms:**
- Arcane starts late or not at all, and the log says `ha-docker-filter did not start within 30s`
- The container is reported unhealthy while Arcane's own page loads

**Solution:**
1. Read the `ha-docker-filter:` lines in the app log: it says what it serves and why it stopped
2. Turning `hide_hassio_containers` off takes the filter out of the path
