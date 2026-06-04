# Changelog

## 2.39.3

_2026-06-04_

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
