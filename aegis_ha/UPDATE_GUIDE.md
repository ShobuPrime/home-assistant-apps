# Update Guide for the AegisHA Add-on

## How AegisHA is versioned

AegisHA has no upstream binary to track — the add-on **is** the software. There is
no automated update workflow. New versions ship by bumping the `version` field in
`config.yaml` and adding a `CHANGELOG.md` entry, then rebuilding.

The only automated maintenance is the shared **base image** updater
(`.github/scripts/update-base-image.sh`), which keeps `aegis_ha/build.yaml` on the
current `ghcr.io/hassio-addons/base` release alongside the other add-ons.

## Releasing a new version

1. Make and test your changes (`go vet ./...`, `go test ./...`, `./build.sh`, and
   the smoke test).
2. Bump `version:` in `config.yaml`.
3. Add a `## X.Y.Z` entry to `CHANGELOG.md` (bare version header, date on the next
   line).
4. Commit (signed) on a feature branch and open a PR — the repo's PR validation
   and Builder workflows run automatically.

## Updating an installed instance

```bash
# SSH into Home Assistant
cd /addons/aegis_ha

# Pull the latest changes
git pull

# Rebuild
./build.sh

# Supervisor -> Add-on Store -> Check for updates
```

## Checking the current version

```bash
grep "version:" /addons/aegis_ha/config.yaml
```

## Best Practices

1. Always run `go test ./...` and the smoke test before releasing.
2. Back up Home Assistant before updating an alarm add-on.
3. Verify `alarm_control_panel.aegis_ha` arms/disarms after an update before relying
   on it.
