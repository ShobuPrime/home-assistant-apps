# Claude AI Assistant

This repository was created and is maintained with assistance from Claude, an AI assistant by Anthropic.

## Repository Setup

This Home Assistant apps repository was initialized using Claude Code with the following structure:

- Git repository initialization
- GitHub Actions workflows for automated building and testing
- Standard documentation (README, LICENSE, CHANGELOG)
- Proper .gitignore configuration
- Repository metadata for Home Assistant integration

## Reference

The repository structure was modeled after: https://github.com/boomam/home-assistant-addons

## Adding New Apps

Each app should be created in its own directory with the following files:

- `config.yaml` - Main configuration file defining the app metadata
- `translations/en.yaml` - Plain-English config-option names + inline descriptions shown in the Home Assistant app UI (mirrors `config.yaml`'s options; see `aegis_ha/translations/en.yaml`)
- `Dockerfile` - Container build instructions
- `README.md` - App overview and quick start
- `DOCS.md` - Detailed documentation and configuration options
- `CHANGELOG.md` - Version history
- `icon.png` - App icon (PNG, minimum 256x256)

## Maintenance

When updating apps or adding new features, you can use Claude to:

- Review and improve Dockerfiles
- Update documentation
- Debug configuration issues
- Generate changelog entries
- Write GitHub Actions workflows

## GitHub Actions Builder Configuration

The repository uses the `home-assistant/builder` action for automated app builds. Key configuration requirements:

- **Docker Hub username**: Use `--docker-hub <username>` flag to set the image repository
- **Image naming**: Use `--image <app-name>` flag to specify the image name
- **Test mode**: Use `--test` flag to build without pushing to registry
- **Target directory**: Use `--target /data/<app>` to specify which app to build

Without proper `--docker-hub` and `--image` flags, the builder will generate invalid image tags like `/:version` instead of `username/image:version`.

## Automated PR Management

The repository includes comprehensive automation for managing pull requests:

### Automatic Version Updates
- Daily checks for new Portainer releases (LTS and STS), Arcane, and Dockhand
- Automatically creates PRs with version updates and changelogs
- **IMPORTANT:** Portainer version detection is based on GitHub release **names** containing "LTS" or "STS"
  - Do NOT use version number patterns (odd/even) - Portainer does not follow a consistent mathematical pattern
  - The script filters releases by searching for "LTS" or "STS" in the release name via GitHub API
- Updates documentation with conservative regex patterns to avoid breaking section headers
  - Only updates "Currently running Portainer X.X.X" and similar specific version references
  - Does NOT update section headers like "Portainer 2.33+ Ingress Compatibility"

### Automatic Base Image Updates
- Daily checks for new `ghcr.io/hassio-addons/base` releases from `hassio-addons/app-base`
- Updates all app `build.yaml` files and Dockerfiles with inline `BUILD_FROM` defaults
- **Major version bumps** automatically get a `needs-review` label to prevent auto-merge (may contain breaking changes like architecture drops)
- The base image only supports **aarch64 and amd64** architectures (armhf/armv7/i386 dropped in v19.0.0)

### PR Validation
- Validates repository structure (required files, config format)
- Checks CHANGELOG.md updates
- Validates AppArmor profiles (`.github/scripts/validate-apparmor.sh` — compile check, flat-profile rule, docker.sock resolved-path + `network,` rules; see "AppArmor Profile Rules" below)
- Lints YAML files
- Tests build configurations
- Adds `validation-passed` label on success

### HAOS Release Watch
- Daily check for new Home Assistant OS releases (`haos-release-watch.yml`)
- Opens a per-release tracking issue (label `haos-update`) with an on-device verification checklist — CI runners don't run the HAOS kernel, so kernel/AppArmor/Docker behavior changes are only observable on the device
- Smoke tests additionally run each app confined by its own `apparmor.txt` when the CI runner supports AppArmor

### Auto-merge
- Automatically merges PRs created by github-actions[bot] that pass all validations
- Requires `automated` and `validation-passed` labels
- Blocked by `do-not-merge`, `needs-review`, or `on-hold` labels
- Uses squash merge method

### Managing Auto-merge
Use the helper script to control auto-merge behavior:
```bash
# Check PR auto-merge status
.github/scripts/manage-automerge.sh <pr-number> status

# Block auto-merge
.github/scripts/manage-automerge.sh <pr-number> block

# Unblock auto-merge
.github/scripts/manage-automerge.sh <pr-number> unblock
```

See [`.github/AUTOMATION.md`](.github/AUTOMATION.md) for complete documentation.

## Git Commit Guidelines

- **Always sign commits**: All commits must be signed with GPG/SSH signatures
- SSH agent should have the signing identity loaded
- Use `git commit -S` for GPG signing or ensure `commit.gpgsign` is configured
- For SSH signing, ensure `gpg.format` is set to `ssh` and `user.signingkey` points to your SSH key
- **Never add Claude Code attribution**: Do not include "Generated with Claude Code" or "Co-Authored-By: Claude" lines in commits

## Dockerfile Best Practices

- **Always run `apk upgrade --no-cache` before `apk add`** in Dockerfiles to resolve base image package version mismatches (e.g. libcrypto3/libssl3 conflicts with openssl)
- **Use `ARG BUILD_FROM` with no default** — the base image version is defined in `build.yaml` and passed at build time by the HA builder and `build.sh`. Do not add inline defaults as they drift out of sync.
- **Architecture support**: All apps support only `aarch64` and `amd64` (hassio-addons base v19+ dropped armhf/armv7/i386)

## AppArmor Profile Rules

Hard-won rules from the July 2026 Huly outage on HAOS 18.1 (kernel 6.18), enforced in CI by `.github/scripts/validate-apparmor.sh` (full incident record: `huly/CHANGELOG.md` 0.7.426 maintenance notes, PRs #165/#166):

- **Profiles must be FLAT — never use nested child profiles** (`profile foo { ... }` inside the main profile with `cx ->` transitions). HAOS 18.1's kernel denies AF_UNIX socket connects from processes confined by nested child profiles regardless of the rules the child contains (verified on-device: the identical ruleset connects flat, fails nested, with both apparmor_parser 3.1.7 and 4.1.7).
- **Docker-socket apps need resolved-path rules.** The Supervisor mounts the socket at `/run/docker.sock`, and `/var/run` is a symlink to `/run`; AppArmor matches the *resolved* path, so a `/var/run/docker.sock`-only rule never matches anything. Use explicit `/run/docker.sock rw,` plus `/var/run/docker.sock rw,`.
- **Docker-socket apps need a bare `network,` rule** (all address families — AF_UNIX is needed for the socket).
- **Profile delivery lag**: the Supervisor imports `apparmor.txt` to `/data/apparmor/<repo-hash>_<slug>` (renaming the profile to the slug) and loads it into the kernel only on add-on **install/update/rebuild** — merging a profile fix does not heal a live device until the add-on is rebuilt.
- **Debugging on a live device**: a `docker exec` shell runs under the top-level profile, so compare `curl --unix-socket /run/docker.sock http://localhost/_ping` from exec against the app's own process behavior; read any process's live confinement from `/proc/<pid>/attr/current`; loaded profiles are listed under `/sys/kernel/security/apparmor/policy/profiles/`. Kernel AppArmor denials do **not** appear in `dmesg` on HAOS — prove theories with throwaway test profiles (`apparmor_parser -r` + `docker run --security-opt apparmor=<name>`) instead of log-hunting.

## Notes

- Always test apps locally before pushing to the repository
- Follow Home Assistant app best practices
- Keep dependencies up to date
- Document all configuration options clearly
