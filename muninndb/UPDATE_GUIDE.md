# Update Guide for MuninnDB App

## Understanding Local App Updates

Local apps in Home Assistant don't have automatic update detection like repository apps. Updates only appear when:
1. The `version` field in `config.yaml` changes
2. You rebuild the app
3. You click "Check for updates" in the app store

## Update Methods

### Method 1: Automatic (GitHub Actions)

This app has automated update detection via GitHub Actions. When a new version is available:
1. A PR is automatically created with the version bump
2. The PR is validated and auto-merged if all checks pass
3. Pull the latest changes and rebuild

### Method 2: Manual Update

```bash
# SSH into Home Assistant
cd /addons/muninndb

# Pull latest changes
git pull

# Rebuild
./build.sh

# Go to Supervisor -> App Store -> Check for updates
```

## Checking Current Version

```bash
grep "version:" /addons/muninndb/config.yaml
```

## Best Practices

1. **Regular Checks**: Pull latest changes regularly
2. **Test First**: Always test updates in a non-production environment
3. **Backup**: Create a Home Assistant backup before updating
4. **Monitor Logs**: Check app logs after updates for any issues

## Troubleshooting

### Update doesn't appear after rebuild
1. Ensure version number changed in config.yaml
2. Click "Check for updates" multiple times
3. Try reloading the Supervisor: `ha supervisor reload`
