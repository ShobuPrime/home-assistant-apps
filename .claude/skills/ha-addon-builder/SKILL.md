---
name: ha-addon-builder
description: Build new Home Assistant add-ons for the ShobuPrime/home-assistant-apps repository. Use this skill whenever the user wants to create a new HA addon, scaffold an addon, add a new service to their Home Assistant setup, or integrate a new Docker-based application as a Home Assistant add-on. Also use when the user asks about addon structure, CI/CD automation for addons, or how their existing addons work. This skill covers the full lifecycle - scaffolding all required files, setting up automated version updates, and integrating with the repository's CI/CD pipeline.
---

# Home Assistant Add-on Builder

This skill guides you through creating production-ready Home Assistant add-ons for the `ShobuPrime/home-assistant-apps` repository. It covers the full lifecycle: scaffolding, S6-overlay integration, CI/CD automation, and documentation.

## Before You Start

Read `references/templates.md` for exact file templates and `references/ci-automation.md` for GitHub Actions and update script patterns. For multi-service or Docker Compose-based addons, also read `references/multi-service.md`.

## When to Use This Skill

- User wants to add a new service/application as a Home Assistant add-on
- User asks to scaffold or create a new addon
- User wants to integrate an existing Docker image or binary into HA
- User asks about the addon structure or conventions in this repo

## Phase 1: Gather Requirements

Before writing any files, determine:

1. **What software?** Name, source repo, how it's distributed (binary download, Docker image, npm package, etc.)
2. **Version source**: Where to find the latest version (GitHub releases, Docker Hub tags, changelog endpoint)
3. **Architecture**: What CPU architectures does the upstream software support? This repo only supports `aarch64` and `amd64`.
4. **Ports**: What ports does the service expose? Which is the main web UI port (for ingress)?
5. **Data persistence**: What directories need persistent storage? (mapped to `/data/<addon-name>/`)
6. **Configuration options**: What should be user-configurable? (credentials, URLs, feature toggles)
7. **Docker API needed?** Does it need access to the Docker socket?
8. **Single-service or multi-service?** Does it need multiple processes (e.g., app + database + reverse proxy)?
9. **Health check**: What endpoint or port can be used for the watchdog?

If the user doesn't know all of these, research the upstream project to fill in the gaps. Check GitHub releases, Docker Hub, and documentation.

## Phase 2: Choose the Pattern

### Single-Binary / Single-Process Addons
Best for: Applications distributed as a single binary or that run as a single process.
Examples in this repo: `portainer_ee_sts`, `portainer_ee_lts`, `arcane`

Pattern:
- Download binary in Dockerfile from GitHub releases
- Single S6 service definition
- Direct `exec` into the binary

### Official Docker Image (Multi-Stage Extract)
Best for: Applications that publish an official multi-arch Docker image and don't need deep HA integration.
Examples in this repo: `dockge`, `dockhand`

Pattern:
- Use the upstream Docker image as a build stage: `FROM upstream/image:version AS source`
- Copy the application files into the hassio-addons base: `COPY --from=source /app /opt/<addon>`
- Add S6 scripts for service management
- Simpler than building from source, and ensures you get the exact upstream build

```dockerfile
ARG BUILD_FROM
FROM louislam/dockge:1.5.0 AS dockge-source

FROM $BUILD_FROM

COPY --from=dockge-source /app /opt/dockge
```

Trade-off: You can't customize the upstream build, but you get exact parity with what the project ships. Updates are version-bumps to the `FROM` tag.

### Multi-Service / Compose-Based Addons
Best for: Applications that require multiple cooperating processes (app server + database + cache + reverse proxy, etc.)

Pattern:
- Install Docker Compose or individual services in Dockerfile
- Multiple S6 service definitions (one per process), OR use a process manager
- Init scripts handle inter-service dependencies
- See `references/multi-service.md` for detailed patterns

### Hardware-Specific / Shell-Script Addons
Best for: Custom daemon scripts that interact with host hardware (GPIO, sensors, firmware). No upstream binary to download.
Example in this repo: `hay_cm5_fan`

Pattern:
- No binary download — the addon IS shell scripts in rootfs
- May support only `aarch64` (hardware-specific)
- `build.yaml` may only list one architecture
- No `ARG <ADDON>_VERSION=` in Dockerfile (no upstream version to track)
- No update script or workflow needed
- Requires `full_access: true` for `/dev/` and `/sys/` access
- Smoke test needs a dedicated case since hardware isn't available on CI runners
- Init script should **warn** (not `exit 1`) on missing hardware so the container stays up for diagnostics
- Packages like `libgpiod` (GPIO), `raspberrypi-utils-vcgencmd` (firmware), `mosquitto-clients` (MQTT) available in Alpine

Key gotchas discovered building `hay_cm5_fan`:
- **`local` only inside functions**: bashio uses `set -e`; `local` in a `while` loop body (outside a function) crashes the script
- **Pipe SIGPIPE**: Piping commands through `grep -q` under bashio's `pipefail` causes SIGPIPE. Write to a temp file first, then grep
- **MQTT discovery**: REST API entities (`POST /api/states/`) don't get `unique_id`. Add `services: ["mqtt:want"]` to config.yaml and use MQTT discovery with `mosquitto_pub` for proper entity registration with unique_id and device grouping
- **`vcgencmd`**: Available inside addon containers via `raspberrypi-utils-vcgencmd` when `/dev/vcio` is accessible (`full_access: true`, Protection Mode off)

## Phase 3: Create the Addon Directory

The addon slug should be lowercase, using underscores for word separation. Create all files in `<repo-root>/<addon-slug>/`.

### Required Files (create in this order)

Read `references/templates.md` for the exact content templates. Customize each template for the specific addon.

1. **`config.yaml`** - Add-on metadata, ports, options schema
2. **`build.yaml`** - Base image and build args per architecture
3. **`Dockerfile`** - Container build with binary download or service installation
4. **`apparmor.txt`** - Security profile (start from the standard template, add addon-specific paths)
5. **`rootfs/etc/cont-init.d/<name>.sh`** - S6 initialization script
6. **`rootfs/etc/services.d/<name>/run`** - S6 service runner
7. **`rootfs/etc/services.d/<name>/finish`** - S6 finish handler
8. **`build.sh`** - Local build script
9. **`icon.png`** - PNG icon, minimum 256x256 (required by HA and CI validation). Source from upstream project logo/favicon.
10. **`CHANGELOG.md`** - Initial version entry
12. **`README.md`** - User-facing overview and installation guide
13. **`DOCS.md`** - Detailed configuration documentation
14. **`CLAUDE.md`** - AI assistant guidance for future maintenance
15. **`UPDATE_GUIDE.md`** - How to update the addon

### Critical Conventions (Non-Negotiable)

These patterns exist because they solve real problems encountered in this repo:

- **`ARG BUILD_FROM` with no default** in the Dockerfile — the base image version comes from `build.yaml` at build time. Do not add inline defaults as they drift out of sync with `build.yaml`.
- **`apk upgrade --no-cache` before `apk add`** to resolve base image package version conflicts (libcrypto3/libssl3 vs openssl)
- **Architecture: only `aarch64` and `amd64`** - the hassio-addons base image v19+ dropped armhf/armv7/i386
- **`bashio::require.unprotected`** as the first line in cont-init.d scripts when Docker API access is needed
- **`exec`** the main process in the run script so it gets PID 1 and receives signals properly
- **`#!/usr/bin/with-contenv bashio`** shebang for all S6 scripts
- **`chmod a+x`** on all scripts in the Dockerfile
- **Version must appear in three places**: `config.yaml` version field, `build.yaml` args, and `Dockerfile` `ARG <ADDON>_VERSION=` default — all must match. The update scripts maintain all three automatically.
- **Signed commits** - never add Claude Code attribution lines

### config.yaml Conventions

```yaml
name: "Human-Readable Name"
description: "One-line description"
version: "X.Y.Z"
slug: "addon_slug"
init: false
ingress: true
ingress_port: <main-web-port>
ingress_stream: true
panel_icon: mdi:<icon-name>
panel_title: <Short Title>
arch:
  - aarch64
  - amd64
startup: services
boot: auto
watchdog: tcp://[HOST]:[PORT:<main-port>]/<health-endpoint>
ports:
  <port>/tcp: <port>
ports_description:
  <port>/tcp: "Description"
host_network: false
apparmor: true
hassio_api: true
docker_api: true         # Only if Docker socket access needed
hassio_role: admin        # Only if Docker socket access needed
map:
  - ssl:ro
  - data:rw
  - media:rw
  - share:rw
options:
  log_level: info
  # addon-specific defaults...
schema:
  log_level: list(trace|debug|info|warning|error|fatal)?
  # addon-specific schema...
```

### build.yaml Conventions

```yaml
build_from:
  aarch64: ghcr.io/hassio-addons/base:20.0.1
  amd64: ghcr.io/hassio-addons/base:20.0.1
args:
  <ADDON_NAME>_VERSION: X.Y.Z
```

Always check what the current base image version is by looking at existing addons' build.yaml files - use the same version.

### Dockerfile Structure

```dockerfile
ARG BUILD_FROM
FROM $BUILD_FROM

ARG <ADDON>_VERSION=X.Y.Z

# Always upgrade first to resolve package conflicts
RUN apk upgrade --no-cache \
    && apk add --no-cache \
        ca-certificates \
        curl \
        jq \
        # ... addon-specific packages

# Download and install the application
RUN mkdir -p /opt/<addon> \
    && ARCH="$(uname -m)" \
    && if [ "${ARCH}" = "aarch64" ]; then \
        <ADDON>_ARCH="arm64"; \
    elif [ "${ARCH}" = "x86_64" ]; then \
        <ADDON>_ARCH="amd64"; \
    else \
        echo "Unsupported architecture: ${ARCH}"; \
        exit 1; \
    fi \
    && echo "Downloading <Addon> v${<ADDON>_VERSION} for ${<ADDON>_ARCH}..." \
    && curl -L -f -S -o /tmp/<addon>.tar.gz \
        "<download-url>" \
    && # Extract/install as appropriate \
    && rm /tmp/<addon>.tar.gz

COPY rootfs /

RUN chmod a+x /etc/cont-init.d/*.sh \
    && chmod a+x /etc/services.d/*/run \
    && chmod a+x /etc/services.d/*/finish

# Build arguments
ARG BUILD_ARCH
ARG BUILD_DATE
ARG BUILD_DESCRIPTION
ARG BUILD_NAME
ARG BUILD_REF
ARG BUILD_REPOSITORY
ARG BUILD_VERSION

# Labels
LABEL \
    io.hass.name="${BUILD_NAME}" \
    io.hass.description="${BUILD_DESCRIPTION}" \
    io.hass.arch="${BUILD_ARCH}" \
    io.hass.type="addon" \
    io.hass.version=${BUILD_VERSION} \
    maintainer="<Addon> for Home Assistant" \
    <addon>.version="${<ADDON>_VERSION}" \
    org.opencontainers.image.title="${BUILD_NAME}" \
    org.opencontainers.image.description="${BUILD_DESCRIPTION}" \
    org.opencontainers.image.vendor="Home Assistant Add-ons" \
    org.opencontainers.image.authors="<Addon> for Home Assistant" \
    org.opencontainers.image.licenses="<license>" \
    org.opencontainers.image.url="<project-url>" \
    org.opencontainers.image.source="<source-url>" \
    org.opencontainers.image.documentation="<docs-url>" \
    org.opencontainers.image.created=${BUILD_DATE} \
    org.opencontainers.image.revision=${BUILD_REF} \
    org.opencontainers.image.version=${BUILD_VERSION}
```

If the upstream provides checksums, verify them (see Portainer's Dockerfile for the SHA256 verification pattern).

### S6 Scripts

**cont-init.d/<name>.sh** - Initialization (runs once on startup):
```bash
#!/usr/bin/with-contenv bashio
# ==============================================================================
# Home Assistant Add-on: <Addon Name>
# Runs initialization for <Addon Name>
# ==============================================================================
bashio::require.unprotected  # Only if docker_api: true

# Create data directories
bashio::log.info "Creating data directories..."
mkdir -p /data/<addon>
chmod 755 /data/<addon>

# Validate installation
if [[ ! -f /opt/<addon>/<binary> ]]; then
    bashio::log.error "<Binary> not found!"
    exit 1
fi

if [[ ! -x /opt/<addon>/<binary> ]]; then
    bashio::log.warning "<Binary> not executable, fixing..."
    chmod +x /opt/<addon>/<binary>
fi

# Check Docker socket (only if docker_api: true)
if [[ -S /var/run/docker.sock ]]; then
    bashio::log.info "Docker socket found"
elif [[ -S /run/docker.sock ]]; then
    bashio::log.info "Docker socket found at /run/docker.sock"
else
    bashio::log.error "Docker socket not found!"
fi

# Any one-time setup (secrets generation, DB init, etc.)

bashio::log.info "<Addon Name> initialization complete"
```

**services.d/<name>/run** - Service runner (supervised, restarts on exit):
```bash
#!/usr/bin/with-contenv bashio
# ==============================================================================
# Home Assistant Add-on: <Addon Name>
# Runs <Addon Name>
# ==============================================================================

bashio::log.info 'Starting <Addon Name>...'

# Read configuration and set environment variables
# Use bashio::config for string values
# Use bashio::config.true for boolean checks
# Use bashio::config.has_value for optional values

# Log level mapping (standard pattern)
if bashio::config.has_value 'log_level'; then
    case "$(bashio::config 'log_level')" in
        trace|debug)
            export LOG_LEVEL="debug"
            ;;
        warning|error|fatal)
            export LOG_LEVEL="error"
            ;;
        *)
            export LOG_LEVEL="info"
            ;;
    esac
fi

# Log configuration at startup
bashio::log.info "Starting with configuration:"
bashio::log.info "  KEY: ${VALUE}"

# Execute the main process (MUST use exec)
exec /opt/<addon>/<binary> [options...]
```

**services.d/<name>/finish** - Exit handler:
```bash
#!/usr/bin/with-contenv bashio
# ==============================================================================
# Home Assistant Add-on: <Addon Name>
# Take down the S6 supervision tree when <Addon Name> fails
# ==============================================================================
if [[ "${1}" -ne 0 ]] && [[ "${1}" -ne 256 ]]; then
    bashio::log.warning "<Addon Name> crashed with exit code ${1}. Respawning..."
fi
```

### AppArmor Profile

Start with the standard template from `references/templates.md`. The profile name should match the addon slug. Add addon-specific paths for:
- Binary location (`/opt/<addon>/** ix`)
- Data directories (`owner /data/<addon>/** rwk`)
- Any additional filesystem paths the addon needs
- Docker socket access (if `docker_api: true`)

**Critical: Character device access (GPIO, vcio, etc.)**: If the addon uses `ioctl()` on character devices (e.g., `/dev/gpiochip*`, `/dev/vcio`), do NOT add specific path rules like `/dev/gpiochip* rw,`. Specific path rules override the blanket `file,` rule and strip ioctl permission. Use only the blanket `file,` rule:

```
profile addon_slug flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>
  capability,
  file,
  signal (send) set=(kill,term,int,hup,cont),
  network,
}
```

If even this doesn't work (HAOS kernel AppArmor version may block ioctl regardless), Protection Mode must be disabled by the user in the HA UI.

### build.sh

The build script follows an identical pattern across all addons. Read `references/templates.md` for the template. Customize:
- The version ARG name (e.g., `PORTAINER_VERSION`, `ARCANE_VERSION`)
- The build description string
- The test `docker run` command with correct ports

## Phase 4: CI/CD Integration

Read `references/ci-automation.md` for full details. This phase creates:

1. **Update script** (`.github/scripts/update-<addon>.sh`) - Checks upstream for new versions and updates all addon files
2. **Update workflow** (`.github/workflows/update-<addon>.yml`) - Runs the update script daily and creates PRs
3. **Modifications to existing files**:
   - Add the addon to `.github/scripts/update-base-image.sh` addon list
   - The builder workflow (`builder.yml`) auto-discovers addons, so no changes needed there

### Update Script Pattern

The update script must:
- Support `CHECK_ONLY=true` mode (for CI check job)
- Support `JSON_OUTPUT=true` mode (for workflow output parsing)
- Fetch the latest version from the upstream source (GitHub API, Docker Hub, etc.)
- Compare with current version in `config.yaml`
- Update all files where the version appears: `config.yaml`, `build.yaml`, `Dockerfile`, `README.md`, `DOCS.md`, `CHANGELOG.md`
- Use conservative regex for documentation updates (only update specific version references, never section headers)
- Include retry logic for API calls (3 attempts, 2-second delay)

### Update Workflow Pattern

```yaml
name: Update <Addon Name>
on:
  schedule:
    - cron: '<MM> <HH> * * *'  # Pick a unique time slot (check existing workflows)
  workflow_dispatch:
```

Use `peter-evans/create-pull-request@v6` with `sign-commits: true` and labels `automated, <addon-slug>, update`. Trigger `repository_dispatch` after PR creation so the validation workflow runs.

## Phase 5: Verification Checklist

Before considering the addon complete, verify:

- [ ] `config.yaml` has all required fields and valid schema types
- [ ] Version is consistent across `config.yaml`, `build.yaml` args, and `Dockerfile` `ARG <ADDON>_VERSION=`
- [ ] `build.yaml` uses the same base image version as other addons
- [ ] Dockerfile starts with `apk upgrade --no-cache` before `apk add`
- [ ] Dockerfile has `ARG BUILD_FROM` with no default (version comes from `build.yaml`)
- [ ] Architecture detection covers aarch64 and amd64 only
- [ ] All S6 scripts use `#!/usr/bin/with-contenv bashio` shebang
- [ ] Init script calls `bashio::require.unprotected` (if Docker API needed)
- [ ] Run script uses `exec` for the main process
- [ ] Finish script handles exit codes correctly
- [ ] AppArmor profile covers all required paths
- [ ] `build.sh` extracts correct version ARG name from build.yaml
- [ ] Update script handles the upstream's version format correctly
- [ ] Update workflow has a unique cron schedule (don't overlap with existing ones)
- [ ] Base image update script includes the new addon
- [ ] `icon.png` exists (PNG, minimum 256x256, sourced from upstream project branding)
- [ ] CHANGELOG.md has an initial entry
- [ ] README.md has correct architecture shields (only aarch64 and amd64)
- [ ] CLAUDE.md accurately describes the addon's architecture and critical details
- [ ] Local build succeeds: `cd <addon> && ./build.sh`

## Existing Cron Schedule Reference

Before choosing a cron slot, check the actual workflow files for the current schedule:

```bash
grep -r "cron:" .github/workflows/update-*.yml .github/workflows/update-base-image.yml | sort
```

As of last update, the occupied slots were:
- 1:00 AM UTC - Base image updates
- 2:00 AM UTC - Portainer LTS + STS updates
- 3:00 AM UTC - Arcane + Dockhand updates
- 3:30 AM UTC - Huly updates
- 4:00 AM UTC - MuninnDB updates

Pick an unoccupied slot (e.g., 4:00, 4:30, 5:00 AM UTC). Always verify with the grep above since new addons may have claimed slots since this list was written.

## Icon File

Each addon **must** have an `icon.png` in its root directory (CI validation will fail without it). Requirements:
- **Format**: PNG, minimum 256x256 pixels
- **Source**: Download from the upstream project's website or GitHub repo (logo, favicon, og:image)
- **Conversion** (if needed): `magick input.{jpg,svg,webp} -resize <size>x<size> -background none -flatten PNG32:icon.png`

The icon appears in the Home Assistant addon store and sidebar.
