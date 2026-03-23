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
- Lints YAML files
- Tests build configurations
- Adds `validation-passed` label on success

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

## Notes

- Always test apps locally before pushing to the repository
- Follow Home Assistant app best practices
- Keep dependencies up to date
- Document all configuration options clearly
