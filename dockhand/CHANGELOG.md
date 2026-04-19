# Changelog

## Version 1.0.25 (2026-04-19)

### Changed
- Updated Dockhand to version 1.0.25

Released: 2026-04-18

## What's new in v1.0.25

- ✨ API token authentication — Bearer tokens for CI/CD pipelines and scripts
- ✨ Telegram topic support — send notifications to supergroup topics (#855)
- 🐛 allow removing healthcheck, ports, and honor startAfterUpdate=false during container edit (#892)
- 🐛 validate stack names and prevent broken DB entries on invalid input (#876)
- 🐛 use per-environment timezone for schedule execution log timestamps (#882)
- 🐛 "Pull image before update" and "Start after update" settings ignored (#909)
- 🐛 image prune timeout on hawser-standard when pruning many images (#905)
- 🐛 bump Docker Compose to 5.1.3
- 🐛 mask secret environment variables in container inspect modal (#924)
- 🐛 viewer role can toggle, delete, and run schedules (#923)
- 🐛 settings show defaults instead of saved values after login until page refresh (#921)
- 🐛 settings toggle notifications show wrong state (#931)
- 🐛 stack memory tooltip shows inflated total on multi-container stacks (#936)

## Docker image

```bash
docker pull fnsys/dockhand:v1.0.25
```

Also available as `fnsys/dockhand:latest`

[View on Docker Hub](https://hub.docker.com/r/fnsys/dockhand)

---


## Version 1.0.24 (2026-04-04)

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


## [1.0.21] - 2026-03-14

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


## [1.0.20] - 2026-03-07

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


## [1.0.18] - 2026-02-19

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

## [1.0.17] - 2026-02-14

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

## [1.0.10] - 2026-01-20

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
