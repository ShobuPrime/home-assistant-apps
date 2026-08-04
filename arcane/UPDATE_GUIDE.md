# Update Guide for Arcane Local App

## Understanding Local App Updates

Local apps in Home Assistant don't have automatic update detection like repository apps. Updates only appear when:
1. The `version` field in `config.yaml` changes
2. You rebuild the app
3. You click "Check for updates" in the app store

## Update Methods

### Method 1: Manual Update Script (Recommended)

```bash
# SSH into Home Assistant or run from your development machine
cd /addons/arcane

# Check for updates
./update-arcane-version.sh --check-only

# Apply update if available
./update-arcane-version.sh --yes
./build.sh

# Go to Supervisor -> App Store -> Check for updates
# Install the update when it appears
```

### Method 2: Automated GitHub Workflow

This repository includes a GitHub workflow that:
- Checks for new Arcane releases daily
- Automatically creates PRs with version updates
- Updates changelog and documentation

See `.github/workflows/update-arcane.yml` for details.

### Method 3: Convert to GitHub Repository

Convert this local app to a GitHub repository for automatic updates:

1. Create a GitHub repository
2. Structure it as a Home Assistant app repository:
   ```
   arcane/
   ├── config.yaml
   ├── Dockerfile
   ├── build.yaml
   └── ... other files
   repository.json
   ```

3. Add repository.json:
   ```json
   {
     "name": "Arcane Repository",
     "url": "https://github.com/yourusername/ha-arcane",
     "maintainer": "Your Name"
   }
   ```

4. Users add your repository URL to Home Assistant
5. Updates appear automatically in the app store

## Current Limitations

- **Local apps** require manual intervention for updates
- **Version detection** only happens when config.yaml changes
- **No push notifications** for available updates
- **Manual rebuild** required after version updates

## Checking Current Version

```bash
# Check current app version
grep "version:" /addons/arcane/config.yaml

# Check latest Arcane version from GitHub
curl -s https://api.github.com/repos/getarcaneapp/arcane/releases/latest | \
  jq -r '.tag_name'
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
4. Verify architecture support (amd64/aarch64 only)

### App won't start after update
1. Check logs for specific errors
2. Verify Docker socket access
3. Ensure protection mode is disabled
4. Delete `/data/arcane/arcane.db` if database is corrupted (will reset all settings)
5. Check `.secrets` file exists in `/data/arcane/`
