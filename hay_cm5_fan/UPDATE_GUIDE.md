# Update Guide for HAY CM5 Fan Controller

## Understanding Updates

This app has no upstream software to track — it IS the software. Version updates are made manually when new features or fixes are added to the app scripts.

## Update Methods

### Method 1: Pull Latest Changes

```bash
# SSH into Home Assistant
cd /addons/hay_cm5_fan

# Pull latest changes
git pull

# Go to Supervisor -> App Store -> Check for updates
```

### Method 2: Rebuild Locally

```bash
cd /addons/hay_cm5_fan
./build.sh
```

## Checking Current Version

```bash
grep "version:" /addons/hay_cm5_fan/config.yaml
```

## Version Bumping (For Maintainers)

When making changes to the app:

1. Update `version` in `config.yaml`
2. Add entry to `CHANGELOG.md`
3. Update version reference in `README.md`

## Best Practices

1. **Backup**: Create a Home Assistant backup before updating
2. **Monitor**: Check app logs after updates for any issues
3. **Test**: Verify fan still operates correctly after update (check `binary_sensor.cm5_cpu_fan`)
