# Changelog

## 2.44.0

_2026-07-31_

> _Maintenance (2026-07-31):_ **Fix "Failed starting container: starting container with non-empty request body was deprecated since API v1.22 and removed in v1.24".** Every attempt to start a container from the Portainer UI failed with this. dockerd rejects any `POST /containers/{id}/start` whose body length is unknown, and Portainer's Docker-API proxy forwards whatever transfer encoding it received — chunked, coming through Home Assistant's streaming ingress. The check runs in dockerd's HTTP handler *before* it looks at the container, so the message describes none: sending the same malformed request at an already-running container returns the identical 400. That made it a mask as well as a bug — it replaced whatever the real per-container start error was (in the production case that prompted this, an IPv6 address collision) and sent the investigation the wrong way. Portainer EE is a closed binary and HAOS's dockerd is not configurable, so the app now proxies Portainer through nginx (`rootfs/etc/nginx/docker-shim.conf`), which drops the request body on the lifecycle verbs where one is never legal (`start`, `stop`, `restart`, `kill`, `pause`, `unpause`, `wait`) and passes everything else through untouched — hijacked exec/attach consoles, log and event streams, and multi-megabyte image builds included. **No migration is needed.** Portainer's environment address lives in its database and `--host` only seeds it on first init, so rather than move Portainer to a new socket (which would mean re-creating your environment by hand and orphaning its stacks), `/var/run/docker.sock` — the address every install already has — now resolves to the shim. Rebuild/reinstall the app to pick this up. Guarded by the smoke test, which reproduces the 400 on the raw socket and then requires the same request to succeed through the shim.

## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

### New and improved features

- Added a basic workflow details screen
- Added GPU visibility in the Environment Details view
- Made the Portainer setup token easier to spot in the installation logs
- Tracked the Source, Workflow, and Artifact status persistently
- Upgraded bbolt to v1.5.0 for performance and robustness improvements
- Moved the build pipeline to BuildKit v0.31.2 (previously v0.27.0); image build provenance attestations moved to the SLSA v1.0 format (previously v0.2) — any tooling that parses attestations needed to be verified against the new format

##

---


## 2.43.0

_2026-06-25_

> _Maintenance (2026-07-25):_ **Fix the app being SIGKILLed instead of shut down cleanly.** The AppArmor profile granted `signal (send) set=(...)` but not `receive`. AppArmor mediates signal *delivery* as well as sending, so s6's SIGTERM never reached the app: the graceful shutdown was skipped and the container was killed after the grace period (exit 137). The rule is now `signal (send,receive),`. Measured on one identical image with only this rule varied: no signal rule -> 13.2s/exit 137; `signal (send) set=(...)` -> 12.7s/exit 137 with no cleanup logged; `signal (send,receive),` -> 6.9s/exit 0 with the full s6 shutdown sequence. Now enforced by `.github/scripts/validate-apparmor.sh` (rule 4) and by the smoke test, which fails on exit 137 under confinement. Rebuild/reinstall the app to pick up the corrected profile.

## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

### New and improved features

- GitOps Sources: new Source Creation wizard, Source Detail screen and Source editing, with reuse of existing sources when adding Docker repository stacks and Kubernetes Helm-from-git installs
- Display cached container images per node on Kubernetes
- In-product installation flow for KubeSolo-based single-node edge deployments
- Kubernetes application list and pod logs now default to expanded
- Environment Group Detail View updated with a new sortable-list-based group list UI

### Security improvements

- Added a one-time setup token, prin

---



## 2.42.0

_2026-05-21_
> _Maintenance (2026-06-10):_ hassio-addons base 20.2.0 compatibility — migrated the Traefik helper scripts from the deprecated bashio::addon.* functions to bashio::app.*.

## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot.

### Known issues with Podman support

- Support for only CentOS 9, Podman 5 rootful.

## Changes

### Breaking changes

Changes to the CSRF protection implementation may cause failures when upgrading:

- Removal of legacy CSRF fallback (scheduled). The legacy-csrf feature flag, introduced in 2.41 as a temporary migration aid, has been removed as scheduled. Users still relying on this flag must resolve any CSRF configuration issues before upgrading (see the 2.41 breaking changes for details). This change also resolves CVE-2025-47909.

### New and improved features

- Added theme selector to the user menu, allowing switching between light, dark, and high-contrast themes without navigating to settings.
- Added GitOps sources list view and source detail view for managing Git sources used in deployments.
- Added a connectivity test before adding edge

---


## 2.41.1

_2026-05-12_
## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

- Added Age as a sort option on the Home environments list and made it the default sort order, with "Oldest" (ascending by environment ID) and "Newest" (descending) toggles
- Fixed the Talos Cluster Details page rendering blank by reverting the Omni cluster phase fields to int32 so they match the frontend OmniClusterPhase / OmniClusterUpgradePhase enum contract

## Deprecated and removed features

### Deprecated features

None.

### Removed features

None

---


## 2.41.0

_2026-04-30_
## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Support for only CentOS 9, Podman 5 rootful

## Changes

### Breaking changes

Changes to the CSRF protection implementation may cause failures when upgrading:

- Portainer fails to start with a fatal log entry like `failed to build server | error="invalid url for trusted origin... trusted_origin: \"portainer.example.com\""`. The new implementation requires each entry in the trusted origins list to be a full URL including scheme (e.g. `https://portainer.example.com/`); bare hostnames are no longer accepted.
- Browser requests return `403 Forbidden` on state-changing actions, with `CSRF check failed` entries in the server logs. This means the browser's origin is not in the trusted origins list and needs to be added.

The previous CSRF implementation can be re-enabled by starting Portainer with the `legacy-csrf`

---


## 2.40.0

_2026-03-26_
## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

### New and improved features

- Added an information panel showing current and planned GitOps deployment details when a Git URL or config path is changed
- Docker Compose GitOps stacks can now have their Git URL, config path, and entry point edited after creation
- Cleaned up Git authentication token handling — GitHub tokens can now be entered directly in the Token field rather than the Basic auth field
- Added a -remove-orphans / prune option when deploying Docker Compose stacks
- Added support for -security-opt when creating Docker containers
- Upgraded Helm Go SDK to

---


## 2.38.1

_2026-02-14_
## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

- Fixed an issue around changing an environment group for Kubernetes standard agent within the environment details view
- Fixed an issue where local environments using Docker would have their protocol removed
- Improved the namespace dropdown list to be sorted alphabetically by default
- Resolved the following CVEs:
  - CVE-2025-61726
  - CVE-2025-61728
  - CVE-2025-61730

---

## 2.38.0

_2026-02-11_
## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

- Fixed an issue where starting Stack was failed when the private image referenced by the stack was removed from the environment
- Fixed an issue where deploying a Stack in Kubernetes caused a memory leak
- Fixed a UI issue when updating edge stacks
- Changed the Docker security settings to safer default values
- Fixed a panic in Edge Group creation
- Fixed quote handling in TLS CLI flags
- Fixed error in GitOps while updating Stacks
- Fixed a problem that would cause for the Containers page to not load
- Bumped up the max Docker API version in the proxy
- Fixed a proble

---


## 2.37.0

_2025-12-12_
## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

- Fixed an issue where a standard stack could not pull private images from a private registry during a GitOps update (polling/webhook) when "Re-pull image" was enabled and a relative path was configured
- Fixed an issue where the Update the Stack button was disabled when editing a standard stack deployed via the Web Editor
- Fixed Service view display for Docker Swarm
- Fixed a regression in the stack updates view
- Fixed the disabled Save button for GitHub Credentials Authentication
- Fixed the undesired regeneration of the webhook IDs
- Fixed the disabled Update stack but

---


## 2.36.0

_2025-11-28_
## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## Changes

- Fixed local development build scripts for community contributors with Apple M series chips
- Improved ECR session management in the Agent
- Added support for Docker v29
- Improved the consistency for GitOps across different scenarios
- Fixed the External label for Kubernetes environments
- Fixed namespace selection in the registry access page
- Improve the registry credential handling in compose files
- Fixed CVEs in the password reset helper
- Fixed the Prune services toggle for Swarm
- Added a --data-path flag to the password reset helper
- Fixed oversized custom ic

---


## 2.35.0

_2025-10-27_
## Known issues

- On Async Edge environments, an invalid update schedule date can be displayed when browsing a snapshot

### Known issues with Podman support

- Podman environments aren't supported by auto-onboarding script
- It's not possible to add Podman environments via socket, when running a Portainer server on Docker (and vice versa)
- Support for only CentOS 9, Podman 5 rootful

## New in this release

- Fixed a bug where the Edit Ingress page wasn't displaying updated information immediately after making an update
- Fixed an issue where GitOps webhook URLs could be reused
- Fixed a data race issue caused by the Kubernetes client
- Fixed an issue that caused a memory leak when redeploying a Kubernetes stack
- Fixed an issue where the environment status filter did not properly handle the "Failed" state when used with Edge Stacks
- Added support for IPV6 network configuration for IPvlan Docker networks
- Added a new command flag --compact-db to allow database co

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
