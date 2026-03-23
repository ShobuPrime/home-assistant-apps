# Update Guide for Portainer EE Local App

## Understanding Local App Updates

Local apps in Home Assistant don't have automatic update detection like repository apps. Updates only appear when:
1. The `version` field in `config.yaml` changes
2. You rebuild the app
3. You click "Check for updates" in the app store

## Update Methods

### Method 1: Native Home Assistant Integration (Recommended)

1. **Copy the HA package** to your config:
   ```bash
   cp /addons/portainer_ee/homeassistant/packages/portainer_updates.yaml /config/packages/
   ```

2. **Enable packages** in `configuration.yaml`:
   ```yaml
   homeassistant:
     packages: !include_dir_named packages
   ```

3. **Restart Home Assistant**

4. **Features you get**:
   - Daily automatic update checks at 3 AM
   - Persistent notifications when updates are available with changelog
   - One-click update application from notifications
   - Update status sensor: `sensor.portainer_update_status`
   - Binary sensor: `binary_sensor.portainer_update_available`
   - Manual update check: Call service `script.check_portainer_updates`
   - Apply update: Call service `script.apply_portainer_update`
   - Watchdog monitoring for automatic restarts

See [HA_PACKAGE_GUIDE.md](HA_PACKAGE_GUIDE.md) for detailed usage instructions.

### Method 2: Manual Update
```bash
# SSH into Home Assistant
cd /addons/portainer_ee

# Check for updates
./update-portainer-version.sh --check-only

# Apply update if available
./update-portainer-version.sh --yes
./build.sh

# Go to Supervisor → App Store → Check for updates
# Install the update when it appears
```

### Method 3: GitHub Repository (Best for Auto-Updates)

Convert this local app to a GitHub repository:

1. Create a GitHub repository
2. Structure it as a Home Assistant app repository:
   ```
   portainer-ee/
   ├── config.yaml
   ├── Dockerfile
   ├── build.yaml
   └── ... other files
   repository.json
   ```

3. Add repository.json:
   ```json
   {
     "name": "Portainer EE Repository",
     "url": "https://github.com/yourusername/ha-portainer-ee",
     "maintainer": "Your Name"
   }
   ```

4. Set up GitHub Actions to:
   - Check for Portainer updates daily
   - Update version numbers automatically
   - Build and test the app
   - Create releases

5. Users add your repository URL to Home Assistant
6. Updates appear automatically in the app store

## Current Limitations

- **Local apps** require manual intervention for updates
- **Version detection** only happens when config.yaml changes
- **No push notifications** for available updates
- **Manual rebuild** required after version updates

## Checking Current Version

```bash
# Check current app version
grep "version:" /addons/portainer_ee/config.yaml

# Check latest Portainer LTS version
curl -s https://api.github.com/repos/portainer/portainer/releases | \
  jq -r '.[] | select(.tag_name | test("^2\\.[0-9]*[02468]\\.[0-9]+$")) | .tag_name' | \
  head -1
```

## Best Practices

1. **Regular Checks**: Run update checks weekly/monthly
2. **Test First**: Always test updates in a non-production environment
3. **Backup**: Create a Home Assistant backup before updating
4. **Monitor Logs**: Check app logs after updates for any issues

## Troubleshooting

### Update doesn't appear after rebuild
1. Ensure version number changed in config.yaml
2. Click "Check for updates" multiple times
3. Try reloading the Supervisor: `ha supervisor reload`

### Build fails
1. Check Docker has enough space: `df -h`
2. Review build logs for errors
3. Ensure all files have correct permissions

### App won't start after update
1. Check logs for specific errors
2. Verify Docker socket access
3. Ensure protection mode is disabled
4. Try reverting to previous version