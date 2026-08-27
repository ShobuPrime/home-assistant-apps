# Changelog

## 2.45.0

_2026-08-27_

## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

### New and improved features

- Added advanced Kubernetes node drain options with agent failover
- Added native Portainer APIs for writing Kubernetes secrets, configmaps, deployments and persistent volume claims, replacing direct kube-apiserver proxy calls
- Added a generic Kubernetes manifest dry-run API
- Edge Compute settings can now be configured during initial setup and via cli flags
- Authentication events now record the real client IP from X-Forwarded-For when the request comes through a trusted proxy
- The namespace YAML tab now shows every resource quota in the namespace
- Added GetCharts() support to the async Edge agent, so chart data syncs on agent startup
- Edge agent connectivity checks now report progress while probing, and wait longer before failing
- Creating a git source now skips the source type selection step
- The source form now explains git polling versus webhook triggers, and exposes the polling interval setting that was missing from the UI
- Removed the redundant single-tab handle from the workflow details view
- Reworded the registry creation tooltip to make the default behaviour obvious
- Clarified that Portainer supports Podman through its Docker-compatible API only

### Security improvements

- Fixed a critical Docker proxy authorization bypass. Unrecognised API version prefixes like /v1.47.0/ or /v01.47/ skipped access control entirely, letting non-admin users reach the Docker API directly
- Closed a remaining gap in the CVE-2026-44849 (GHSA-5fxq-qcf3-244w) fix and broadened bind-mount restrictions for non-admin users, now including Compose and Swarm stack deployments
- Single-namespace Kubernetes endpoints now check the caller’s namespace authorization instead of running as admin
- Read-only and Helpdesk users can no longer view Kubernetes secret data
- Standard users can no longer manage registry access
- Kubernetes authorization denials now return HTTP 403 instead of 500
- Fixed a Kubernetes shell authorization flaw. Caller-supplied query parameters could override the server's pod target, letting a standard user run commands in any pod on Agent-managed Kubernetes environments
- Updated the Go toolchain to 1.26.6, fixing CVE-2026-39821 (Critical, 9.6), an IDNA validation bypass of hostname-based access controls, along with CVE-2026-42505, CVE-2026-39822, CVE-2026-56862, CVE-2026-56860, CVE-2026-56859, CVE-2026-56858, CVE-2026-56853, CVE-2026-46600 and CVE-2026-33818
- Updated oras.land/oras-go/v2 to 2.6.2, fixing CVE-2026-50163
- Updated github.com/go-git/go-git/v5 to 5.19.2, fixing CVE-2026-71556 and CVE-2026-71557
- Updated go.opentelemetry.io/otel to 1.44.0, fixing CVE-2026-41178
- Updated github.com/klauspost/compress to 1.18.7, fixing GHSA-259r-337f-4rfw
- Upgraded libcurl to 8.21.0-r0 in the kubectl-shell image to address the following CVEs: CVE-2026-11856, CVE-2026-10536, CVE-2026-11564, CVE-2026-12064, CVE-2026-11586, CVE-2026-11352, CVE-2026-9547, CVE-2026-9546, CVE-2026-9545, CVE-2026-9080, CVE-2026-9079, CVE-2026-8932, CVE-2026-8927, CVE-2026-8926, CVE-2026-8925, CVE-2026-8924, CVE-2026-8458, and CVE-2026-8286.
- Upgraded c-ares to 1.34.8-r0 in the kubectl-shell image to address CVE-2026-33630

### Bug fixes

- Removing a stack now uninstalls the underlying Helm release, which used to be left running
- Fixed Edge stack workloads staying up after deletion on Kubernetes. The entry file and namespace are now sent on removal
- Fixed collisions during Edge stack removal and reassignment. The old stack is now removed before the new one deploys
- Fixed `Lstat /data/edge_stacks/<id>/v1: no such file or directory` on Kubernetes async agents by persisting the artifact commit hash before creating the Edge stack
- Fixed Azure Blob backup settings not saving, by resolving redacted credentials on write
- Fixed manual team-membership sync diverging from login-time sync by sharing a single implementation
- Fixed server-managed API key metadata fields being wiped on policy update
- Fixed spaces in setup policy names causing Kubernetes label errors
- Fixed the Policies card on the Kubernetes dashboard not counting all applied policy types
- Policy operations now return a meaningful error when the policy isn’t found, instead of a bare 404
- Fixed the Edit button being disabled for a Helm chart stack deployed from GitHub
- Fixed the browser title bar staying stuck on “Loading” after a page refresh
- Fixed multiple error toasts appearing for a metrics time range over 31 days, and stopped the request over-retrying
- Fixed alert modal validation desyncing from the input
- Fixed the “Open in OneUptime” navigation using an incorrect URL
- Fixed the Uninstall button being enabled in the add-on list but disabled in the detail view while an install was running
- The Restart button is no longer shown for failed add-on installs

## Deprecated and removed features

### Deprecated features

None.

### Removed features

None

## Community contributions since 2.39.0 LTS

A huge thank you to all our community contributors.
This LTS release rolls up work from the 2.40 to 2.44 STS releases, including fixes and improvements contributed by these community members:

- Nguyen Quang Minh ([`minhng99`](https://github.com/minhng99)) - corrected the OAuth scope placeholder format
- Zach ([`zacxihu`](https://github.com/zacxihu)) - relative env_file: paths now resolve against the compose file's directory, fixing Git sub-directory stacks
- Rian Moraes ([`rshmdev`](https://github.com/rshmdev)) - env var descriptions now shown as tooltips in application templates
- Siddam Vinay ([`siddamvinay2001`](https://github.com/siddamvinay2001)) - truncated published-port lists with a show-more badge, and made early async Swarm deploy failures surface instead of reporting success
- Immanuel Tikhonov ([`immanuwell`](https://github.com/immanuwell)) - fixed a registry cache type mismatch that meant the cache never hit
- [`ferreiraborgesaxel-design`](https://github.com/ferreiraborgesaxel-design) - fixed reversed markdown link syntax in the contributing docs
- Ion Jaureguialzo Sarasola ([`ijaureguialzo`](https://github.com/ijaureguialzo)) - fixed a Kubernetes crash on malformed web-editor manifests
- Du Feilong ([`dfldylan`](https://github.com/dfldylan)) and Jerry ([`jerry-yuan`](https://github.com/jerry-yuan)) - added Docker --security-opt support and Swarm cluster self-heal after manager reap
- SRIKANTH K ([`srikanth-karthi`](https://github.com/srikanth-karthi)) - Portainer now tolerates malformed OAuth Content-Type headers from resource endpoints

---


## 2.39.6

_2026-08-21_

## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

### Security improvements

- Implemented an SSRF protection mechanism with a configurable allow-list in settings (off / audit / enforce modes)
- Changed a default setting to enforce server-side EdgeID on first connection
- Fixed path traversal in the swarm compose deployer, where configs/secrets file paths escaped the project root
- Upgraded the Go toolchain from 1.25.11 to 1.25.12 to address the following CVEs:
    - CVE-2026-42505
    - CVE-2026-39822
- Upgraded `github.com/go-git/go-git/v5` to 5.19.2 to address the following CVEs:
    - CVE-2026-71556
    - CVE-2026-71557
- Upgraded `oras.land/oras-go/v2` to 2.6.2 to address CVE-2026-50163
- Upgraded `go.opentelemetry.io/otel` to 1.44.0 to address CVE-2026-41178
- Upgraded `github.com/klauspost/compress` to 1.18.7 to address GHSA-259r-337f-4rfw
- Upgraded `golang.org/x/net` to 0.56.0 and `golang.org/x/text` to 0.39.0 in the Portainer updater to address the following CVEs:
    - CVE-2026-46600
    - CVE-2026-56852
- Upgraded `github.com/containerd/containerd` (v1) to 1.7.33 to address the following CVEs:
    - CVE-2026-53488
    - CVE-2026-47262
    - Upgraded `google.golang.org/grpc` to 1.82.1 to address GHSA-hrxh-6v49-42gf

### Bug fixes

- Fixed a user's direct environment access being incorrectly removed when a team they belonged to was deleted
- Fixed multiple "Cannot read properties of undefined (reading 'message')" error toasts appearing on Kubernetes application pages when an API call failed without a response (e.g. while pods are restarting after a redeploy)
- Fixed an "Invalid Swarm ID" / `503` error when creating a stack from a Swarm worker node
- Fixed Kubernetes Ingress service ports always showing `0`
- Fixed `kubectl port-forward` failing with "error upgrading connection" against Agent 2.35+ on older Kubernetes clusters
- Fixed "This node is not a swarm manager" errors when starting/stopping a Swarm stack from within the swarm itself
- Fixed Docker image builds failing with `unauthorized` against private registries referenced in a Dockerfile's `FROM` line (both the UI's "Build a new image" flow and the `/docker/build` API proxy)
- Fixed Swarm stack deployments failing to re-pull private Docker Hub images on a forced re-pull, even with valid registry credentials configured
- Fixed request-handler panics being logged as unexpected crashes when a client disconnected mid-request (e.g. a long-poll on a Kubernetes Jobs watch)

## Deprecated and removed features

### Deprecated features

None.

### Removed features

None

---


## 2.39.5

_2026-07-14_

> _Maintenance (2026-07-31):_ **Fix "Failed starting container: starting container with non-empty request body was deprecated since API v1.22 and removed in v1.24".** Every attempt to start a container from the Portainer UI failed with this. dockerd rejects any `POST /containers/{id}/start` whose body length is unknown, and Portainer's Docker-API proxy forwards whatever transfer encoding it received — chunked, coming through Home Assistant's streaming ingress. The check runs in dockerd's HTTP handler *before* it looks at the container, so the message describes none: sending the same malformed request at an already-running container returns the identical 400. That made it a mask as well as a bug — it replaced whatever the real per-container start error was (in the production case that prompted this, an IPv6 address collision) and sent the investigation the wrong way. Portainer EE is a closed binary and HAOS's dockerd is not configurable, so the app now proxies Portainer through nginx (`rootfs/etc/nginx/docker-shim.conf`), which drops the request body on the lifecycle verbs where one is never legal (`start`, `stop`, `restart`, `kill`, `pause`, `unpause`, `wait`) and passes everything else through untouched — hijacked exec/attach consoles, log and event streams, and multi-megabyte image builds included. **No migration is needed.** Portainer's environment address lives in its database and `--host` only seeds it on first init, so rather than move Portainer to a new socket (which would mean re-creating your environment by hand and orphaning its stacks), `/var/run/docker.sock` — the address every install already has — now resolves to the shim. Rebuild/reinstall the app to pick this up. Guarded by the smoke test, which reproduces the 400 on the raw socket and then requires the same request to succeed through the shim.

> _Maintenance (2026-07-25):_ **Fix the app being SIGKILLed instead of shut down cleanly.** The AppArmor profile granted `signal (send) set=(...)` but not `receive`. AppArmor mediates signal *delivery* as well as sending, so s6's SIGTERM never reached the app: the graceful shutdown was skipped and the container was killed after the grace period (exit 137). The rule is now `signal (send,receive),`. Measured on one identical image with only this rule varied: no signal rule -> 13.2s/exit 137; `signal (send) set=(...)` -> 12.7s/exit 137 with no cleanup logged; `signal (send,receive),` -> 6.9s/exit 0 with the full s6 shutdown sequence. Now enforced by `.github/scripts/validate-apparmor.sh` (rule 4) and by the smoke test, which fails on exit 137 under confinement. Rebuild/reinstall the app to pick up the corrected profile.

## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

- Fixed a 2.39.4 regression where a relative `env_file:` in a Git stack whose compose file lives in a repository sub-directory was resolved against the project root instead of the compose file's own directory, deploying stacks with an empty environment or failing outright
- Improved Edge tunnel reliability over high-latency links (satellite/VSAT): the server no longer tears down a half-established tunnel on timeout, keep-alive and unlimited background retries were added on the agent, and the ping timeout was raised from 3s to 8s
- Fixed standard users not seeing all of their te

---


## 2.39.4

_2026-06-25_

## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot
- kubectl port-forward fails with Portainer kubeconfig in some configurations

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

- Added an API endpoint to refresh Team/Group membership for a user
- Fixed an issue where users with no environment access are able to enumerate Kubernetes resources
- Fixed ecr token pre-validation error with warning log
- Fixed the way a standard user could not redeploy team stack or delete registry image
- Fixed the restore endpoint allowing admin takeover for uninitialised Portainer instances
- Fixed link on timed out page
- Replaced docker binary with libstack
- Fixed the volume label drop

---



## 2.39.3

_2026-06-04_

> _Maintenance (2026-06-10):_ hassio-addons base 20.2.0 compatibility — migrated the Traefik helper scripts from the deprecated bashio::addon.* functions to bashio::app.*.

## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Support for only CentOS 9, Podman 5 rootful
- Auto onboarding a Podman environment defaults to "Standard" and not "Podman"
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)

## Changes

- Fixed a panic in Chisel
- Bumped in-toto-golang to 0.11.0 to address GHSA-pmwq-pjrm-6p5r
- Fixed a team access escalation via AuthorizedResourceControlUpdate logic flaw
- Fixed a full-read server-side request forgery (SSRF) vulnerability in the GitLab Registry Proxy endpoint that could be exploited via the X-Gitlab-Domain header
- Bumped github.com/go-git/go-git/v5 to 5.18.0 to address the following CVEs:
  - CVE-2026-34165
  - GHSA-3xc5-wrhm-f963
  - CVE-2026-33762
- Bumped golang.org/x/net to >= 0.53.0 to address the following CVEs:
  - CVE-2026-27141

---


## 2.39.2

_2026-05-09_
## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

- Fixed an issue where the kubectl-shell-image flag only takes effect on the first Portainer run 
- Fixed an issue where deleting a kube edge stack results in a downed environment
- Fixed an issue where Edge stack deployment retries stopped working
- Fixed an issue with saving Git credentials 
- Fixed a Docker API proxy authorisation bypass that allowed regular users to circumvent deny-plugin restrictions
- Changed a default setting to enforce server-side EdgeID on first connection
- Fixed a bind mount restriction bypass via HostConfig.Mounts during container creation
- Fixed a bi

---


## 2.33.8

_2026-05-07_
## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

- Fixed a Docker API proxy authorisation bypass that allowed regular users to circumvent deny-plugin restrictions
- Changed a default setting to enforce server-side EdgeID on first connection
- Fixed a path traversal vulnerability in custom template handling
- Fixed unauthorized access to custom template file contents via a direct API endpoint
- Removed the option to pass a JWT token as a query string parameter
- Removed the possibility to clone Git repositories that contain symlinks
- Fixed a bind mount restriction bypass via HostConfig.Mounts during container creation 
-

---


## 2.39.1

_2026-03-20_
## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

- Fixed an issue where a Git-based Docker stack from GitLab failed validation for non-admin users
- Re-enabled image registries for FIPS
- Fixed an issue where groups were missing after an upgrade
- Fixed an issue where not all containers for a service were shown in v2.39.0 Alpine
- Fixed an issue where users could not add new environments to an existing group when the group already contained a large number of environments
- Fixed an issue where the Edit this application button was disabled for non-admin users
- Fixed an issue where custom template file content was accessib

---


## 2.39.0

_2026-02-26_
## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Auto onboarding a Podman environment defaults to "Standard" and not "Podman"
- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

- Fixed an issue preventing environment group changes for Kubernetes standard agents from the environment details view
- Addressed security vulnerability disclosure
- Updated form behavior to only show errors after the input has been touched/visited or submitted
- Improved HTTP response code handling via the Portainer API
- Added default alphabetical sorting to the namespace dropdown list
- Fixed a UI issue where the dropdown form elements were overlapping with the footer
- Updated styling of sh

---


## 2.33.7

_2026-02-11_
## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

- Fixed an issue where clicking the Update stack button would do nothing
- Fixed an issue that would cause the Containers page to not load
- Fixed an error when updating Edge Stacks
- Fixed a panic in Edge Group creation
- Fixed a deadlock in the auto onboarding
- Fixed a problem that prevented the loading of the Containers page
- Fixed a problem in Edge Stacks and GitOps when the entry file name was not at the repository root
- Upgraded compose to v2.40.3 to fix a nil pointer error
- Resolved the following CVEs:
	- CVE-2025-61726
	- CVE-2025-68121

## Deprecated and 

---


## 2.33.6

_2025-12-18_
## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

- Fixed an issue where a standard stack could not pull private images from a private registry during a GitOps update (polling/webhook) when "Re-pull image" was enabled and a relative path was configured
- Fixed an issue where starting a Stack failed when a private image referenced by the Stack had been removed from the environment
- Fixed an issue where empty Docker snapshot could cause issues
- Fixed an issue where Duplicate/Edit Container adds persistent MAC address causing Network issues
- Fixed an issue where Docker Compose configs were not injected into containers for st

---


## 2.33.5

_2025-11-28_
## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes
- Added support for Docker v29

# Breaking change
- Removed the optional raw snapshot response from some endpoint requests 

## Deprecated and removed features

**Deprecated features**
- None

**Removed features**
- None

---


## 2.33.3

_2025-11-01_
# Release 2.33.3 LTS

## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

- Improved stability by attempting to compact using a read-only database
- Fixed an issue where WebSocket upgrade failed with Portainer generated `kubeconfig`
- Fixed an issue where a memory leak occured during Kubernetes stack auto redeployment
- Fixed missing dependency versions displayed in the popup
- Fixed an issue where adding a team access to a namespace threw a panic error
- Fixed typos in Content-Security-Policy
- Resolved CVE-2025-62725

## Deprecated and removed features

**Deprecated features**

- None

**Removed features**

- N

---


## 2.34.0
### Add-on Changes

**IMPORTANT FIX:** Added `CSP=false` environment variable to fix Home Assistant ingress/iframe compatibility. Portainer 2.33.0+ introduced Content-Security-Policy headers that block iframe embedding, preventing access through Home Assistant's ingress. This fix disables those restrictive headers to restore functionality.

If you're experiencing issues accessing Portainer through Home Assistant after updating to 2.34.0, you'll need to rebuild and restart the add-on for this fix to take effect.

## Portainer 2.34.0 Release Notes

## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## New in this release

- Increased Content-Security-Policy restrictions
- Added enforcement of a minimum polling interval value for GitOps
- Fixed environment type detection for the image status indicator
- Fixed an access control bug in Custom Templates
- Fixed inaccurate display of healthy containers count in environment listing
- Implemented higher priority for interactive database transactions over background processes like edge agent polling
- Fixed a data race in the job scheduler
- Removed the password from the response of the registry update request
- Fixed a problem that pr

---

For full release notes, see: https://github.com/portainer/portainer/releases/tag/2.34.0
