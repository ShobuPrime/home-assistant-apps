# Changelog

## Version 1.0.27 (2026-05-03)

### Changed
- Updated Dockhand to version 1.0.27

Released: 2026-04-26

## What's new in v1.0.27

- ✨ network graph visualization on networks page (#894, @Penlane)
- ✨ customizable compose template for new stacks in settings (#632, @oratory)
- ✨ Microsoft Teams notifications via Power Automate Workflows (#355, @slokhorst)
- ✨ container label controls: dockhand.update, dockhand.hidden, dockhand.notify (#6, #53, #94, #215)
- ✨ configurable label filter matching mode (any/all) for environment dashboard (#607)
- ✨ log search filter mode to hide non-matching lines (#916)
- ✨ inline terminal on logs page with resizable split layout (#900)
- 🐛 disable Telegram link preview in notifications (#910, @deenle)
- 🐛 cron editor rejects 6-field expressions with seconds (#839, @GiulioSavini)
- 🐛 mirror Dockhand's ExtraHosts into scanner and self-update containers (#836, @YewFence)
- 🐛 duplicate volume binds during container recreate (#765, @itsDNNS)
- 🐛 log timestamp formatting not applied on main logs page (#882)
- 🐛 uploaded files now inherit container user ownership (#732, @ivanjx)
- 🐛 extraneous backslash in Telegram notification environment name (#955)
- 🐛 collapse ports into ranges only if 3 or more consecutive ports
- 🐛 git operations auto-merge system CAs with custom cert (#967)

## Docker image

```bash
docker pull fnsys/dockhand:v1.0.27
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
