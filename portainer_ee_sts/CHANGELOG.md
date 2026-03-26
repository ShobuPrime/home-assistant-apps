# Changelog

## Version 2.40.0 (2026-03-26)

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


## Version 2.38.1 (2026-02-14)

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

## Version 2.38.0 (2026-02-11)

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


## Version 2.37.0 (2025-12-12)

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


## Version 2.36.0 (2025-11-28)

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


## Version 2.35.0 (2025-10-27)

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


## Version 2.34.0

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
