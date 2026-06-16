# Changelog

## 1.0.33

_2026-06-16_

### Changed
- Updated Dockhand to version 1.0.33

Released: 2026-06-15

## What's new in v1.0.33

- ✨ in-place container property updates without restart — restart policy, CPU/memory limits (#1153)
- ✨ clickable stack badge in container and volume inspect modals (#1121)
- ✨ clickable stack badge in volumes list row (#1122)
- ✨ volumes list shows driver_opts type (NFS, CIFS, etc.) with sort and filter (#1123)
- ✨ Bark iOS notifications (#1095, PR#1097, @undirectlookable)
- ✨ Signal notifications via signal-cli-rest-api (#1099)
- ✨ Apprise passthrough — forward to a self-hosted caronc/apprise-api server (#1099)
- 🐛 env editor flagged Docker/Compose built-ins as MISSING (#141)
- 🐛 YAML editor indentation was inconsistent when pressing Enter (#1156)
- ✨ `dockhand.update=false`, `dockhand.hidden=true` and `localhost/*` images skip registry polling (#1083)
- 🐛 registry authentication for image pulls (#1105)
- ✨ native HTTPS listener, off by default (#1102)
- 🐛 environments stuck "Failed" after VPN/Tailscale tunnel drops until agent restart (#1160)
- 🐛 health_status events flooding container_events table (#1165)
- 🐛 git stack sync removes files deleted from the repo (hash-verified) (#966, #1162)
- ✨ upload TLS/mTLS certificate files in environment editor (#125)
- ✨ syntax highlighting for shell, Dockerfile, TOML, INI/conf and .env files in the file browser viewer (#1055)
- ✨ Animated icons now configurable (#1169)
- 🐛 stack deploys ignored the env's configured socket path (#1172)
- 🐛 environment names with characters that break path resolution (e.g. `*`) are now rejected (#1179)

## Docker image

```bash
docker pull fnsys/dockhand:v1.0.33
```

Also available as `fnsys/dockhand:latest`

[View on Docker Hub](https://hub.docker.com/r/fnsys/dockhand)

---


> _Maintenance (2026-06-10):_ hassio-addons base 20.2.0 compatibility — migrated the Traefik helper scripts from the deprecated bashio::addon.* functions to bashio::app.*.

## 1.0.32

_2026-06-07_

### Changed
- Updated Dockhand to version 1.0.32

Released: 2026-06-06

## What's new in v1.0.32

- ✨ container details tweaks: process count, label filter, copy all labels (#812)
- ✨ log improvements (#1130)
- 🐛 cleared Resources fields not persisted on container edit (#1119)
- 🐛 long container names overflowed in activity event details dialog (#1129)
- 🐛 git stack recreate and start operations ignored Dockhand-stored env vars (#1132)
- 🐛 dashboard stopped count reset to 0 after refresh for gracefully stopped containers (#1133)
- 🐛 auto-update preserves runtime `-e` env and `-l` label overrides (#1135)
- 🐛 git stack volume binds resolved to wrong host path when compose was in a subdirectory (#1139)
- 🐛 git stacks: subdir compose files now find their adjacent env files (#1136)
- ✨ env editor doesn't flag Docker/Compose built-in variables as unused (#141)
- ✨ container network mode: share another container's network namespace (#161)

## Docker image

```bash
docker pull fnsys/dockhand:v1.0.32
```

Also available as `fnsys/dockhand:latest`

[View on Docker Hub](https://hub.docker.com/r/fnsys/dockhand)

---


## 1.0.31

_2026-05-31_

### Changed
- Updated Dockhand to version 1.0.31

Released: 2026-05-30

## What's new in v1.0.31

- 🐛 502 Bad Gateway behind nginx-based reverse proxies — SvelteKit 2.51+ bloated the Link response header, pinned to 2.50.0 (#1114)

## Docker image

```bash
docker pull fnsys/dockhand:v1.0.31
```

Also available as `fnsys/dockhand:latest`

[View on Docker Hub](https://hub.docker.com/r/fnsys/dockhand)

---


## 1.0.29

_2026-05-19_
### Changed
- Updated Dockhand to version 1.0.29

Released: 2026-05-17

## What's new in v1.0.29

- ✨ optionally display internal (exposed) container ports alongside published ports (#193)
- ✨ show app version in sidebar with build info tooltip (#209)
- ✨ central label management — rename or delete labels across all environments (#661)
- ✨ find next available host port when creating or editing containers (#116)
- ✨ theme-aware scrollbar styling — scrollbars adapt to dark/light mode and color palettes (#462)
- 🐛 update buttons (single, selected, and all) now respect the "confirm dangerous actions" setting (#638, #751)
- ✨ custom URL labels - dockhand.url or dockhand.port.{port}.url to add links alongside container ports (#266)
- ✨ generate and copy token for Hawser Standard mode with run command hint (#337)
- 🐛 environment stack directory not cleaned up when environment is deleted (#1023)
- ✨ toggle to hide timestamps and container name prefix in log viewer (#124)
- 🐛 Podman containers health status not showing (#737)
- 🐛 containers with exit code 0 (init/migration) no longer cause stack "partial" status (#1026)
- 🐛 stats stream 400 on reconnect by skipping overlapping fetches (#1044)
- 🐛 env var validation false positive for values containing $ followed by text (#1048)
- 🐛 git-repos directory not cleaned up when environment is deleted (#1049)
- 🐛 webhook secret auto-generated when left empty despite hint saying otherwise (#1050)
- ✨ scan reports — combined or individual Grype/Trivy (#1056)

## Docker image

```bash
docker pull fnsys/dockhand:v1.0.29
```

Also available as `fnsys/dockhand:latest`

[View on Docker Hub](https://hub.docker.com/r/fnsys/dockhand)

---


## 1.0.24

_2026-04-04_
### Changed
- Updated Dockhand to version 1.0.24

Released: 2026-04-03

## What's new in v1.0.24

- 🐛 browsing HTTP registries fails with SSL error (#868)
- 🐛 git stack deploy options (build, re-pull, force redeploy) not persisted in edit dialog

## Docker image

```bash
docker pull fnsys/dockhand:v1.0.24
```

Also available as `fnsys/dockhand:latest`

[View on Docker Hub](https://hub.docker.com/r/fnsys/dockhand)

---


## 1.0.21

_2026-03-14_
### Changed
- Updated Dockhand to version 1.0.21

Release date: 2026-03-13

Changes:
{
  "type": "feature",
  "text": "option to truncate port list (#702)"
}
{
  "type": "feature",
  "text": "log viewer supports ANSII 256 colors (#743)"
}
{
  "type": "fix",

---


## 1.0.20

_2026-03-07_
### Changed
- Updated Dockhand to version 1.0.20

Release date: 2026-03-02

Changes:
{
  "type": "fix",
  "text": "regression on Synology DSM"
}
{
  "type": "fix",
  "text": "Fix ARM64 regression: Go collector crashing on Raspberry Pi and other ARM devices"
}
{
  "type": "fix",

---


## 1.0.18

_2026-02-19_
### Changed
- Updated Dockhand to version 1.0.18

Release date: 2026-02-16

Changes:
{
  "type": "feature",
  "text": "Dockhand self-update from the UI"
}
{
  "type": "feature",
  "text": "Show freed disk space after image removal and pruning"
}
{
  "type": "feature",

---


All notable changes to this project will be documented in this file.

## 1.0.17

_2026-02-14_
### Changed
- Updated Dockhand to version 1.0.17

### Upstream Changes (1.0.11 - 1.0.17)

#### 1.0.17
- Fix scanner failure on rootless Docker
- Increase Hawser compose operation timeout
- Fix regression in stack container updates

#### 1.0.16
- Support Docker Compose override files when deploying stacks
- Fix Hawser stack deploy failing when compose file not present on remote host
- Fix .env variables not applied on save & redeploy
- Fix single Hawser node failure cascading offline state to all environments

#### 1.0.15
- Pull before update option for container auto-update
- Usage filter on images page by usage status
- Show repository name for untagged images
- Fix memory leaks in SSE event streams
- Fix vulnerability scans hanging indefinitely
- Fix static IP not preserved during container auto-update
- Many additional bug fixes

#### 1.0.14
- Fix environment variables in .env not interpolated during remote deployment
- Fix time format 12/24 setting not respected in header clock

#### 1.0.13
- Add DISABLE_LOCAL_LOGIN env var for SSO/LDAP configurations
- GPU device configuration in container create/edit/inspect
- Scheduled image pruning per environment
- Git stack env populate button
- Fix vulnerability scanning failing with rootless Docker

#### 1.0.12
- Add SKIP_DF_COLLECTION env var for NAS devices
- Fix terminal/shell connections to TLS environments
- Skip auto-update for SHA-pinned images

#### 1.0.11
- Encryption at rest for sensitive credentials (AES-256-GCM)
- Fix registry browsing for registries with organization paths

---

## 1.0.10

_2026-01-20_
### Added
- Initial release of Dockhand Home Assistant Add-on
- Docker container management (start, stop, restart, remove)
- Real-time log streaming with ANSI color support
- Terminal access to containers
- Docker Compose stack management
- Git repository integration for stacks
- Image management and vulnerability scanning
- Multi-environment support
- Home Assistant ingress integration
- AppArmor security profile
- Persistent data storage in `/data/dockhand`

### Known Limitations
- No container label filtering (cannot hide Home Assistant system containers)
- No CSP configuration options (not needed - Dockhand doesn't set restrictive headers)
- Limited to amd64 and arm64 architectures
