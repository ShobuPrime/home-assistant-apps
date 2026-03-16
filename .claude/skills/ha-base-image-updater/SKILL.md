---
name: ha-base-image-updater
description: Ensure the hassio-addons base image is correctly and consistently updated across all Home Assistant add-ons in this repository. Use this skill whenever the user mentions base image updates, hassio-addons/base, BUILD_FROM, build.yaml base image versions, or when adding a new addon that needs to be registered in the base image update pipeline. Also use when investigating base image version mismatches between addons, or when a new base image release needs to be applied. Covers both automated pipeline registration and manual verification/fixes.
---

# Home Assistant Base Image Updater

This skill ensures the `ghcr.io/hassio-addons/base` image is properly and consistently managed across all add-ons in the `ShobuPrime/home-assistant-apps` repository.

## Why This Matters

Every addon in this repo is built on top of the hassio-addons base image. When a new base image is released, ALL addons need to be updated in lockstep. The base image version is defined in:

1. **`build.yaml`** - `build_from:` section (per-architecture) - this is the **source of truth** that the HA builder uses to determine what base image to build from
2. The `build.sh` local build script reads from `build.yaml` and passes it as `--build-arg BUILD_FROM=...`

The Dockerfile's `ARG BUILD_FROM` intentionally has **no default value** — the version always comes from `build.yaml` at build time. Do NOT add inline defaults to `ARG BUILD_FROM` in Dockerfiles, as they can drift out of sync with `build.yaml` and cause confusion about which version is actually being used.

If an addon isn't registered in the update pipeline, it will silently fall behind on security patches and compatibility fixes.

## When This Skill Triggers

- User adds a new addon and needs to register it for base image updates
- User notices base image version mismatches between addons
- User wants to manually update the base image across all addons
- User wants to verify the base image update pipeline is correctly configured
- A new `ghcr.io/hassio-addons/base` version is released and needs review
- User asks about BUILD_FROM, build.yaml, or base image versions

## The Two Places to Update

For each addon, the base image version appears in:

### 1. `<addon>/build.yaml` (source of truth)
```yaml
build_from:
  aarch64: ghcr.io/hassio-addons/base:<VERSION>
  amd64: ghcr.io/hassio-addons/base:<VERSION>
```
Both architectures should always use the same base image version. This is what the HA builder and `build.sh` use.

The Dockerfile should have `ARG BUILD_FROM` with **no default** — the value comes from `build.yaml` at build time.

### 2. Pipeline Registration
The addon slug must appear in `.github/scripts/update-base-image.sh` in the `ADDON_DIRS` variable, and the addon should also be listed in the workflow PR body template in `.github/workflows/update-base-image.yml`.

## Verification Procedure

When asked to verify or fix the base image setup, follow these steps:

### Step 1: Audit Current State

Check the base image version across all addons:

```bash
# For each addon directory in the repo
for addon in $(ls -d */build.yaml 2>/dev/null | xargs -I{} dirname {}); do
    echo "=== $addon ==="
    # build.yaml version
    grep "ghcr.io/hassio-addons/base:" "$addon/build.yaml" 2>/dev/null || echo "  (no base image in build.yaml)"
    # Dockerfile inline default
    grep "ARG BUILD_FROM" "$addon/Dockerfile" 2>/dev/null || echo "  (no BUILD_FROM in Dockerfile)"
done
```

### Step 2: Check for Mismatches

All addons should be on the same base image version. Flag any that differ.

Common causes of mismatches:
- A new addon was created with a different base image version than existing addons
- The base image update script missed an addon (not in `ADDON_DIRS`)
- A manual update only touched some addons

### Step 3: Check Pipeline Registration

Verify the addon is in the update script:

```bash
grep "ADDON_DIRS=" .github/scripts/update-base-image.sh
```

The addon slug must be in that space-separated list. Also check the workflow PR body template in `.github/workflows/update-base-image.yml` to ensure it lists all addons in the "Affected Addons" section.

### Step 4: Check Dockerfile ARG BUILD_FROM

Dockerfiles should have `ARG BUILD_FROM` with **no default value**. The version comes from `build.yaml` at build time via the HA builder or `build.sh`.

```dockerfile
# Correct (no default — version comes from build.yaml):
ARG BUILD_FROM

# Avoid (inline default can drift out of sync with build.yaml):
ARG BUILD_FROM=ghcr.io/hassio-addons/base:20.0.1
```

## Registering a New Addon

When a new addon is created and needs to be added to the base image update pipeline:

### 1. Verify the addon's build.yaml uses the current base image

Read an existing addon's `build.yaml` to find the current version, then ensure the new addon matches:

```yaml
build_from:
  aarch64: ghcr.io/hassio-addons/base:<CURRENT_VERSION>
  amd64: ghcr.io/hassio-addons/base:<CURRENT_VERSION>
```

### 2. Verify the Dockerfile has ARG BUILD_FROM (no default)

```dockerfile
ARG BUILD_FROM
```

The version is supplied by `build.yaml` at build time — do not hardcode a default.

### 3. Add to the update script

Edit `.github/scripts/update-base-image.sh` and add the addon slug to `ADDON_DIRS`:

```bash
ADDON_DIRS="arcane dockge dockhand <new_addon> portainer_ee_lts portainer_ee_sts"
```

Keep the list alphabetically sorted for readability.

### 4. Update the workflow PR body

Edit `.github/workflows/update-base-image.yml` and add the addon to the "Affected Addons" list in the PR body template:

```yaml
body: |
  ...
  ### Affected Addons

  All addons using the hassio-addons base image:
  - `arcane`
  - `dockge`
  - `dockhand`
  - `<new_addon>`
  - `portainer_ee_lts`
  - `portainer_ee_sts`
```

Keep the list alphabetically sorted.

## Manual Base Image Update

To manually update all addons to a new base image version:

### 1. Find the latest version

```bash
curl -s "https://api.github.com/repos/hassio-addons/app-base/releases/latest" | jq -r '.tag_name'
```

### 2. Run the update script

```bash
# Run from the repository root
REPO_ROOT=. CHECK_ONLY=false bash .github/scripts/update-base-image.sh
```

### 3. Verify the update

After running, verify all `build.yaml` files were updated:

```bash
grep -r "ghcr.io/hassio-addons/base:" */build.yaml
```

All versions should match. If any don't, the addon wasn't in `ADDON_DIRS`.

## Major Version Bumps

Major version bumps (e.g., 19.x -> 20.x) get special handling because they may contain breaking changes:

- The workflow automatically adds a `needs-review` label to the PR, which blocks auto-merge
- Historical example: v19.0.0 dropped support for armhf/armv7/i386 architectures
- When reviewing a major bump:
  1. Read the release notes at `https://github.com/hassio-addons/app-base/releases`
  2. Check if any architectures were added or dropped
  3. Check if any base packages were removed or replaced
  4. Test build at least one addon with the new base image before approving
  5. Run `apk upgrade --no-cache` still works (base image may have changed package repos)

## Architecture Constraints

The hassio-addons base image v19+ only supports:
- **aarch64** (ARM 64-bit)
- **amd64** (x86 64-bit)

Architectures **dropped** in v19.0.0:
- armhf
- armv7
- i386

All addons in this repo should only list `aarch64` and `amd64` in their `config.yaml` `arch:` field and `build.yaml` `build_from:` section.

## Previously Fixed Inconsistencies

These were fixed on 2026-03-14:

- `huly` is now registered in both `.github/scripts/update-base-image.sh` and the workflow PR body
- All 6 addons (arcane, dockge, dockhand, huly, portainer_ee_lts, portainer_ee_sts) are tracked in the base image update pipeline

## Quick Reference

| File | What to update | Pattern |
|------|---------------|---------|
| `<addon>/build.yaml` | `build_from:` per-arch entries | `ghcr.io/hassio-addons/base:<VERSION>` |
| `.github/scripts/update-base-image.sh` | `ADDON_DIRS` variable | Space-separated addon slugs |
| `.github/workflows/update-base-image.yml` | PR body "Affected Addons" | Markdown bullet list |
