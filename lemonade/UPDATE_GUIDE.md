# Update Guide for Lemonade App

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
cd /addons/lemonade

# Pull latest changes
git pull

# Rebuild
./build.sh

# Go to Supervisor -> App Store -> Check for updates
```

## Checking Current Version

```bash
grep "version:" /addons/lemonade/config.yaml
```

## What Survives an Update

Everything under `/data/lemonade` — downloaded models, the llama.cpp backend,
`config.json` and your registered models — is untouched by an app update. Only
the `lemond` binary and the bundled glibc closure are replaced.

If Lemonade ships a new pinned llama.cpp version, the matching backend is
downloaded on the next model load; the old one stays cached.

## Upstream-Specific Notes

Lemonade releases often. The update workflow tracks the latest GitHub release
and verifies the `lemonade-embeddable-<version>-ubuntu-{arm64,x64}.tar.gz`
assets exist before proposing a bump — a release that ships without them (or
under a new name) is skipped rather than merged as a broken build.

## If the Web UI breaks after an upstream bump

One small script adapts upstream's web app to ingress:
`rootfs/opt/lemonade/resources/web-app/ha-addon-shim.js`. The `Dockerfile` adds
it to `index.html`. It does nothing on direct access. An upstream redesign can
still invalidate it, as 2026.45.3's did to the bundle patches it replaced.

Each symptom below maps to one cause with a mechanical fix.

### Why the adaptation exists

Upstream's web app assumes it is served from an origin root, so it discards the
path, which is wrong under `/api/hassio_ingress/<token>/web-app/`. The
Supervisor sends no `X-Ingress-Path` header, so the prefix is knowable only in
the browser and no server-side setting can substitute.

The shim does three things under ingress:

| Upstream behaviour (`src/app/src/api.ts`, `core/web_ui.cpp`) | Shim |
|---|---|
| The API client reads `lemonade_base_url` from localStorage before its own default | Sets it to origin + prefix |
| lemond's `window.api.getSettings()` returns `apiUrl: location.origin`, and the client lets that replace the stored value | Wraps `getSettings` to return origin + prefix |
| `_buildWebSocketUrl` assigns `url.pathname`, dropping the prefix | Wraps `WebSocket` to put it back |

It is a deferred script placed first, so it runs after lemond's inline
`window.api` mock and before the app's bundles. It edits nothing in the bundles,
so upstream's chunk layout does not matter.

Resetting the server URL in the app's Connect view clears the base until the
next page load. Reload the panel.

This is a known upstream limitation, not a misconfiguration here:

- [lemonade-sdk/lemonade#631](https://github.com/lemonade-sdk/lemonade/issues/631)
  — "Web interface unusable through reverse proxy", closed **not planned**
- [lemonade-sdk/lemonade#1584](https://github.com/lemonade-sdk/lemonade/pull/1584)
  — closed unmerged, stating sub-path proxying is *"not handled — derivation
  uses `window.location.origin` only, dropping the path"*, and that the fix is
  to extend it to `origin + window.location.pathname`

**If upstream ever ships sub-path support, delete the shim** rather than
repairing it.

### The build tells you first

The `Dockerfile` **fails the build** rather than shipping a shim that cannot
work:

```
PATCH FAILED: expected the shim once, ahead of the deferred bundles in web-app/index.html
PATCH FAILED: 'lemonade_base_url' not found in the web app; the shim no longer sets its API base
```

Neither proves the panel works. These check that the mechanism is still present,
and only a browser under ingress shows the rest. CI will not auto-repair this; a
human must look.

### Symptom → cause → fix

| Symptom | Cause | Fix |
|---|---|---|
| Panel renders, stuck on "connecting" | Shim missing or ineffective | A |
| Log view or dictation fails, everything else works | WebSocket path | A |
| Chat returns `403 {"error": "Origin not allowed"}` | Origin allowlist wrong or empty | B |
| Works on `<ha-ip>:13305`, only ingress broken | Base URL only — Origin is fine | A |

**A. Shim.** Confirm it is served and is the first deferred script:

```bash
docker exec app_<hash>_lemonade sh -c \
  'curl -s http://127.0.0.1:13305/web-app/ | grep -o "<script defer=\"defer\" src=\"[^\"]*\"" | head -1'
```

It must name `ha-addon-shim.js`. If it does and the panel still fails, open the
browser's network tab: any request outside `/api/hassio_ingress/<token>/` means
upstream changed how it builds URLs. Compare against the table above.

**B. Origin.** The allowlist is derived at startup and printed:

```bash
docker logs addon_<hash>_lemonade 2>&1 | grep -A10 "Allowed browser origins"
```

The origin your browser sends must appear **exactly** — scheme required, port
significant, no path, no wildcards. Note `http` and `ws` normalise to port 80,
`https`/`wss` to 443, so `https://host` and `https://host:443` are equivalent.

The bridge has **no same-origin exemption** — it matches the `Origin` header
exactly against its list — which is why ingress needs an entry even though the
page and the API share an origin there. (Lemonade 11.9.0 gained a zero-config
same-origin rule of its own, but `lemond` sits behind the bridge with a
wildcard, so it never decides.) If your origin is missing (a VPN hostname, a
second domain), add it to the `allowed_origins` option. Verify the boundary
still holds afterwards:

```bash
# must stay 403
curl -s -o /dev/null -w '%{http_code}\n' -X POST \
  -H 'Origin: https://evil.example.com' -H 'Content-Type: application/json' \
  -d '{"model":"none","messages":[]}' http://<ha-ip>:13305/api/v1/chat/completions
```

Avoid `*`. With no `api_key` set it lets any site you visit drive model
management and host info, not just chat.

### The fallback that always works

Direct access at `http://<ha-ip>:13305/` does not depend on the shim at all:
there is no path prefix to reconstruct. If ingress breaks, use the port directly
while it is fixed.

## Best Practices

1. **Regular Checks**: Pull latest changes regularly
2. **Test First**: Always test updates in a non-production environment
3. **Backup**: Create a Home Assistant backup before updating
4. **Monitor Logs**: Check app logs after updates for any issues — in
   particular for `error while loading shared libraries`, which would mean the
   new upstream build needs a library the bundled glibc closure lacks

## Troubleshooting

### Update doesn't appear after rebuild
1. Ensure version number changed in config.yaml
2. Click "Check for updates" multiple times
3. Try reloading the Supervisor: `ha supervisor reload`

### App fails to start after an update
Check the log for `error while loading shared libraries`. That means the new
upstream binary needs a library the `glibc-provider` stage doesn't copy. Roll
back by reverting the version in `config.yaml`, `build.yaml` and `Dockerfile`,
then open an issue.
