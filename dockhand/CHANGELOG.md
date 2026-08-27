# Changelog

## 1.0.45

_2026-08-27_

### Changed
- Updated Dockhand to version 1.0.45

Released: 2026-08-27

## ⚠️This is important security upgrade with a number of API hardenings. 

## What's new in v1.0.45

- 🐛 security hardening across the API
- ✨ KeePassXC as a secret provider - pull secrets from a .kdbx via keepassxc-cli, bulk or inline refs (#1460)
- ✨ compose generated from a container now captures network config and resource limits (#1464)
- ✨ compose validate flags a service with no healthcheck and a defined-but-unused named volume
- ✨ optional Webhook column on the stacks list with a git stack's webhook id and copy-URL button (#845)
- ✨ sort by Disk I/O and Net I/O - the header cycles read/write and down/up (#1111)
- ✨ option to disable the session timeout (#1302)
- 🐛 pausing or resuming a backup schedule no longer clears the retention policy (#1462)
- 🐛 emergency script to relocate stack paths after moving DATA_DIR (#651, #904)
- 🐛 git stacks can now set a custom icon (#1473)

## Docker image

```bash
docker pull fnsys/dockhand:v1.0.45
```

Also available as `fnsys/dockhand:latest`

[View on Docker Hub](https://hub.docker.com/r/fnsys/dockhand)

---


## 1.0.44

_2026-08-26_

### Changed
- Updated Dockhand to version 1.0.44

Released: 2026-08-25

## What's new in v1.0.44

- ✨ generate a compose file from any container - save as new or append to existing stack (#489)
- ✨ MQTT notification channel - publish events to a broker (#280)
- ✨ TLS backup repositories with a private CA and optional mutual TLS (#1451)
- ✨ custom stack and container icons, plus automatic container icons (#31)
- ✨ Azure Key Vault as a secret provider (bulk pull or inline azurekv:// references)
- ✨ Proton Pass inline references accept vault and item names, not just ids (pass://vault/item/field) (#1449)
- ✨ branch selector on git stack, with per-stack branch override (#649, PR#1343, @brx19)
- ✨ click the update icon in the image column to update a container (#1435)
- ✨ Pushover notifications can target specific devices (#1411)
- ✨ keep specific images from being pruned with a dockhand.prune=false label (#1391)
- 🐛 Infisical secret references now resolve to their value instead of the raw reference (#1447)
- 🐛 the version-update dialog no longer duplicates the old tag when the running image is digest-pinned (#1437)
- 🐛 a compose stack with a restarting container now shows a Stop action instead of Start (#1438)
- 🐛 optimize 1Password API calls on deploy (#1436)
- 🐛 direct-remote staging pulls the alpine helper image instead of failing when it's missing (#1442)
- 🐛 Proton Pass secrets now resolve on headless hosts with no system keyring (#1440)
- 🐛 fix switching an Infisical provider back to a static token (#1448)
- 🐛 fix backpressure in Docker streaming responses (PR#1445, @StefanSa)
- 🐛 sort IPv6 subnets and addresses numerically instead of randomly (#1453)

## Docker image

```bash
docker pull fnsys/dockhand:v1.0.44
```

Also available as `fnsys/dockhand:latest`

[View on Docker Hub](https://hub.docker.com/r/fnsys/dockhand)

---


## 1.0.43

_2026-08-21_

### Changed
- Updated Dockhand to version 1.0.43

Released: 2026-08-20

## What's new in v1.0.43

- 🐛 registry auth survives an http to https token realm redirect behind a TLS proxy (#1428)
- 🐛 security: update Go to 1.25.13 (8 HIGH stdlib CVEs) and update docker-compose to 5.5.0
- ✨ Compose validate - a preflight linter & validator for compose files
- ✨ Bitwarden Secrets Manager as a secret provider via an operator-installed bws client (PR#1416, @StefanSa)
- ✨ Proton Pass as a secret provider via an operator-installed pass-cli client
- ✨ Infisical: Universal Auth (Machine Identity) as well as a static token (#1398, @strausmann)
- ✨ detect newer version tags for pinned images and show them as a badge with release notes (#1176)
- ✨ OpenAPI spec at /api/docs with a Scalar viewer, opt-in via FEAT_API_DOCS (#814, PR#1342, @strausmann)
- ✨ Zabbix notification channel via the history.push API (PR#1419, @snesterkov)
- 🐛 concurrent git syncs sharing an SSH credential no longer fail on a missing temp key (#1413, PR#1143, @HoroTW)
- 🐛 backup notifications show a real title and message instead of "undefined" (#1414)
- 🐛 git stacks accept a compose/env path with a leading slash (#1355)
- 🐛 container update notifications now fire properly (#1424)
- 🐛 Infisical: the project ID is optional when using a service token - it's taken from the token (#1398)
- 🐛 API: adopting a stack rejects a compose path not on Dockhand's filesystem (#1375)
- 🐛 API: validating a stack's env vars no longer drops the environment id (#1423)
- 🐛 editing a secret provider no longer wipes its stored token (#1432)

## Docker image

```bash
docker pull fnsys/dockhand:v1.0.43
```

Also available as `fnsys/dockhand:latest`

[View on Docker Hub](https://hub.docker.com/r/fnsys/dockhand)

---


## 1.0.40

_2026-08-01_

### Changed
- Updated Dockhand to version 1.0.40

Released: 2026-07-31

## What's new in v1.0.40

- 🐛 git stack stop/down/remove now resolve panel environment variables (#1313, PR#1339, @strausmann)
- 🐛 the header clock falls back to the default TZ instead of UTC when the environment has none (#1340)
- 🐛 deleting a git stack no longer clears a same-named stack's variables on other environments (#1335)
- 🐛 Pangolin public URLs now default to https, honoring the ssl label (#1331)
- 🐛 disconnected environments can be deleted without timeout (#1319, PR#1320, @Solaris-star)
- ✨ Git stack source badge shows the deployed short commit hash, and git url, branch in tooltip (PR#1325, @brx19)
- 🐛 container and stack modals are centered with consistent margins, no longer shifting with the sidebar (#1308)
- 🐛 git stack webhooks now require a configured secret
- 🐛 local-path backup repos now work on a co-located socket-proxy env, and fail loudly on a remote host (#1316)
- 🐛 restoring a stack from a backup no longer drops its database-backed secret env vars (#1329)
- 🐛 restoring a stack now recreates it as a internal stack in the stacks dir, not from a tmp path
- 🐛 local-path backups are readable when Dockhand runs with a non-root PUID
- 🐛 bump docker-compose to 5.3.1-r3 (CVE-2026-56852, GHSA-hrxh-6v49-42gf, GHSA-jpjm-c3r5-q96r)
- 🐛 self-update check works on socket-proxy setups (#1203)

## Docker image

```bash
docker pull fnsys/dockhand:v1.0.40
```

Also available as `fnsys/dockhand:latest`

[View on Docker Hub](https://hub.docker.com/r/fnsys/dockhand)

---


## 1.0.39

_2026-07-27_

### Changed
- Updated Dockhand to version 1.0.39

Released: 2026-07-26

## What's new in v1.0.39

- 🐛 regression: stacks with relative bind mounts deployment fails (#1310)
- 🐛 redeploying a stack with pull now clears the dashboard pending-update badge for containers it updated (#1311)
- 🐛 stack backups no longer fail on deeply nested files whose path exceeds 100 bytes

## Docker image

```bash
docker pull fnsys/dockhand:v1.0.39
```

Also available as `fnsys/dockhand:latest`

[View on Docker Hub](https://hub.docker.com/r/fnsys/dockhand)

---


## 1.0.38

_2026-07-26_

### Changed
- Updated Dockhand to version 1.0.38

Released: 2026-07-25

## What's new in v1.0.38

- ✨ container/stacks backups (beta), gated with FEAT_BACKUPS_ENABLED
- ✨ view a Git stack's compose YAML and dependency graph read-only by clicking its name (PR#1294, @nenad)
- 🐛 deploy on a plain internal stack no longer 500s when the request has no body (#1271)
- 🐛 wider Activity event details modal (#1277)
- 🐛 container updates refresh image-provided env vars and labels, overrides are kept (#1226, #1256)
- 🐛 private repos with a token credential and no username now authenticate (#1273)
- 🐛 dashboard CPU history no longer spikes past 100% on busy/churny hosts (#1279)
- 🐛 failed updates (e.g. Docker Hub limits) show a 'check failed' instead of silently reporting no update (#1255)
- 🐛 git stacks reject uppercase names upfront instead of leaving an orphaned clone (#1284)
- 🐛 scan all images now covers untagged digest-pinned images instead of skipping them (#1286)
- 🐛 registry containers stuck on a moved tag are no longer misread as "local" and skipped from updates (#1288)
- 🐛 bump docker-compose 5.2.0-r0 -> 5.3.1-r1 (CVE-2026-42505)
- 🐛 bump Go 1.25.11 -> 1.25.12 (patches 4 stdlib CVEs)
- 🐛 volume prune now removes named unused volumes, keeping scanner caches (#1289)
- ✨ DNS_RESULT_ORDER=verbatim enables IPv6 fallback for outbound requests on IPv4-broken hosts (#1293)
- 🐛 OIDC/network errors now show the underlying cause instead of a bare "fetch failed" (#1293)
- 🐛 ntfy webhook with a trailing slash on the topic no longer fails with a 404 (#1300)
- 🐛 stack deploy and git sync notifications now actually fire on real deploys (#1295)

## Docker image

```bash
docker pull fnsys/dockhand:v1.0.38
```

Also available as `fnsys/dockhand:latest`

[View on Docker Hub](https://hub.docker.com/r/fnsys/dockhand)

---


## 1.0.37

_2026-07-12_

> _Maintenance (2026-07-25):_ **Fix the app being SIGKILLed instead of shut down cleanly.** The AppArmor profile granted `signal (send) set=(...)` but not `receive`. AppArmor mediates signal *delivery* as well as sending, so s6's SIGTERM never reached the app: the graceful shutdown was skipped and the container was killed after the grace period (exit 137). The rule is now `signal (send,receive),`. Measured on one identical image with only this rule varied: no signal rule -> 13.2s/exit 137; `signal (send) set=(...)` -> 12.7s/exit 137 with no cleanup logged; `signal (send,receive),` -> 6.9s/exit 0 with the full s6 shutdown sequence. Now enforced by `.github/scripts/validate-apparmor.sh` (rule 4) and by the smoke test, which fails on exit 137 under confinement. Rebuild/reinstall the app to pick up the corrected profile.

### Changed
- Updated Dockhand to version 1.0.37

Released: 2026-07-11

## What's new in v1.0.37

- ✨ Prometheus metrics at /metrics for env state and internals, gated by EXPORT_METRICS (#339)
- ✨ export scan results as SARIF 2.1.0, manual export and API for DefectDojo/Dependency-Track/GitHub (#415)
- ✨ bump bundled docker-compose to 5.2.0-r0 to clear known CVEs
- ✨ Vulnerabilities dashboard: aggregated CVE findings (#1038)
- ✨ update container images directly from the Stack view (#1073)
- ✨ stacks list shows an update indicator for stacks with image updates (#504)
- 🐛 copying an image to another registry keeps its tag instead of defaulting to latest (#1243)
- 🐛 `more-than-current image` re-scans the current image so a stale cached scan doesn't block (#1022)
- ✨ application logs now include the log level (INFO/WARN/ERROR) in the prefix (#1166)
- ✨ option to suppress the "What's New" popup (#1235)
- 🐛 ntfy notifications now forward the `email` query parameter so ntfy can send email (#1231)
- ✨ default scanner images bumped to grype v0.115.0 and trivy 0.71.2 (#1241)
- 🐛 the "Latest" update-check result now stays visible and doesn't revert (#1019)
- ✨ a stack with container label `dockhand.adopt=false` excluded from adoption (#998)
- 🐛 dashboard daemon info shows the real host hostname, not Dockhand's container id (#1265)
- ✨ file editor prompts to save/discard unsaved changes before closing (#1264)
- 🐛 `GET /api/containers/check-updates` now lists pending updates (POST still triggers a check) (#1266)
- ✨ sidebar menu customizable (order and visibility of items) (#1252)
- 🐛 activity and audit date filters respect the configured timezone instead of UTC (#1269)
- ✨ git deploy progress shows the changed files before the deploy starts (#1260)

## Docker image

```bash
docker pull fnsys/dockhand:v1.0.37
```

Also available as `fnsys/dockhand:latest`

[View on Docker Hub](https://hub.docker.com/r/fnsys/dockhand)

---


## 1.0.36

_2026-06-28_

### Changed
- Updated Dockhand to version 1.0.36

Released: 2026-06-27

## What's new in v1.0.36

- ✨ sidebar shows an amber update indicator when a newer Dockhand image is available (#1146)
- 🐛 Tag image modal too narrow for images identified only by SHA (#1205)
- 🐛 self-hosted ntfy: accept raw `tk_...` access tokens in `?auth=` (#1209)
- 🐛 dashboard: a single failed DB stats query no longer poisons the whole environment tile (#1210)
- 🐛 UI dates and times now honor the configured default timezone instead of the browser's timezone (#1183)
- 🐛 add missing `reset-mfa.sh` emergency script referenced by the manual (#1214)
- 🐛 registry browser: copy between registries no longer duplicates the host (#1220)
- ⚡ published image carries standard OCI annotations (source, url, title, description, vendor, licenses) (#1217)
- ✨ scanner: configurable network mode and DNS servers for vulnerability scans (#1219)
- ✨ template tiles now show a `Project` link that opens the upstream project page (#1211)
- ⚡ cron picker: hour shows just `HH` (was `HH:00`), minute granularity bumped from 15 to 5 (#1198)
- ✨ grid buttons: configurable size and optional semantic colors (#1072)
- 🐛 copy buttons (git deploy logs, API tokens) now work over plain HTTP (#1222)
- 🐛 Podman pod-infra containers no longer trigger update-check warnings (#1221)
- 🐛 terminal exec works even when the browser sends a cookie with stray `%` characters (#1224)

## Docker image

```bash
docker pull fnsys/dockhand:v1.0.36
```

Also available as `fnsys/dockhand:latest`

[View on Docker Hub](https://hub.docker.com/r/fnsys/dockhand)

---


## 1.0.35

_2026-06-20_

### Changed
- Updated Dockhand to version 1.0.35

Released: 2026-06-19

## What's new in v1.0.35

- ✨ image prune skips Dockhand's scanner images (grype, trivy), configurable (#625)
- 🐛 "No archive" download format selection not persisted (#1180)
- 🐛 regression: vulnerability scans on direct-TCP envs (#1195)
- 🐛 Pangolin labels — recognise the real `public-resources` / `private-resources` namespaces (#2)
- 🐛 healthcheck uses HTTPS probe when HTTPS_MODE=on (#1191)
- 🐛 shell detection improved for containers with non-standard PATH or shell locations (#1189)

## Docker image

```bash
docker pull fnsys/dockhand:v1.0.35
```

Also available as `fnsys/dockhand:latest`

[View on Docker Hub](https://hub.docker.com/r/fnsys/dockhand)

---


## 1.0.34

_2026-06-18_

### Changed
- Updated Dockhand to version 1.0.34

Released: 2026-06-17

## What's new in v1.0.34

> [!Important]  
> This release closes [GHSA-f6q3-chhc-4hxm](https://github.com/Finsys/dockhand/security/advisories/GHSA-f6q3-chhc-4hxm)



- ✨ raw file download — no tar wrapping (#1180)
- 🐛 update modal stuck after closing mid-pull (#1094)
- 🐛 vulnerability scans on Podman hosts (direct TCP and Hawser) (#1076)
- 🐛 crash-looping containers now appear in the logs page list (#227)
- ✨ filter containers by "Update available" (#1063)
- ✨ show hostname / IP of the selected environment in the top header (#962)
- ✨ internal auth and validation hardening and dependency bumps
- ✨ Traefik and Pangolin integration — surface proxy URLs on container and stack panels (#2)
- ✨ release-notes link next to images with updates available (#538)
- ✨ lifecycle action buttons in the container details modal (#461)
- ✨ template library — browse and deploy compose templates from configurable sources (#48)
- 🐛 file browser fails on containers with ls in /usr/sbin (#1185)

## Docker image

```bash
docker pull fnsys/dockhand:v1.0.34
```

Also available as `fnsys/dockhand:latest`

[View on Docker Hub](https://hub.docker.com/r/fnsys/dockhand)

---


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



## 1.0.32

_2026-06-07_

> _Maintenance (2026-06-10):_ hassio-addons base 20.2.0 compatibility — migrated the Traefik helper scripts from the deprecated bashio::addon.* functions to bashio::app.*.

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
