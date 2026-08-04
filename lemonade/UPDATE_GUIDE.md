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

The add-on adapts upstream's web app in two ways, both applied at **build
time** in `Dockerfile` and both dependent on the shape of what upstream ships.
An upstream redesign can invalidate them.

**None of this needs an AI to diagnose or repair.** Each symptom maps to one
cause with a mechanical fix.

### Why the adaptation exists

Upstream's web app is an Electron renderer repurposed for the browser. It
derives its API base from `window.location.origin`, **discarding the path** —
correct at an origin root, wrong under
`/api/hassio_ingress/<token>/web-app/`. Home Assistant's Supervisor sends no
`X-Ingress-Path` header, so the prefix is knowable only in the browser and no
server-side setting can substitute.

This is a known upstream limitation, not a misconfiguration here:

- [lemonade-sdk/lemonade#631](https://github.com/lemonade-sdk/lemonade/issues/631)
  — "Web interface unusable through reverse proxy", closed **not planned**
- [lemonade-sdk/lemonade#1584](https://github.com/lemonade-sdk/lemonade/pull/1584)
  — closed unmerged, stating sub-path proxying is *"not handled — derivation
  uses `window.location.origin` only, dropping the path"*, and that the fix is
  to extend it to `origin + window.location.pathname`

That PR describes exactly the patch applied here. **If upstream ever ships it,
delete this adaptation** rather than repairing it.

### The build tells you first

Both patches assert their anchors and **fail the build** rather than shipping
silently:

```
PATCH FAILED: bundle <script> tag not found in web-app/index.html — upstream layout changed
PATCH FAILED: expected exactly 1 'window.location?.origin' in renderer.bundle.js, found N
```

A failing build is the designed outcome — it means upstream moved and the
anchor needs re-deriving. CI will not auto-repair this; a human must look.

### Symptom → cause → fix

| Symptom | Cause | Fix |
|---|---|---|
| Panel renders, stuck on "connecting", log view empty | Base-URL patch missing or ineffective | A |
| Chat returns `403 {"error": "Origin not allowed"}` | Origin allowlist wrong or empty | B |
| Companion app on Android jumps to the Play Store | Shim not served | C |
| Works on `<ha-ip>:13305`, only ingress broken | Base URL only — Origin is fine | A |

**A. Base URL.** Confirm the patch is in the running image:

```bash
docker exec addon_<hash>_lemonade sh -c \
  'grep -c __haLemonadeBase /opt/lemonade/resources/web-app/renderer.bundle.js'
```

`1` means applied. `0` means the image predates the patch — rebuild. If the
build failed instead, upstream changed how the base URL is derived: find the
new expression and update the `sed` in `Dockerfile`. The browser console on the
panel logs the base URL the app chose, which tells you what it is using now.

**B. Origin.** The allowlist is derived at startup and printed:

```bash
docker logs addon_<hash>_lemonade 2>&1 | grep -A10 "Allowed browser origins"
```

The origin your browser sends must appear **exactly** — scheme required, port
significant, no path, no wildcards. Note `http` and `ws` normalise to port 80,
`https`/`wss` to 443, so `https://host` and `https://host:443` are equivalent.

There is **no same-origin exemption** — `lemond` never inspects `Host` — which
is why ingress needs an entry even though the UI and API share an origin. If
your origin is missing (a VPN hostname, a second domain), add it to the
`allowed_origins` option. Verify the boundary still holds afterwards:

```bash
# must stay 403
curl -s -o /dev/null -w '%{http_code}\n' -X POST \
  -H 'Origin: https://evil.example.com' -H 'Content-Type: application/json' \
  -d '{"model":"none","messages":[]}' http://<ha-ip>:13305/api/v1/chat/completions
```

Avoid `*`. With no `api_key` set it lets any site you visit drive model
management and host info, not just chat.

**C. Android / shim.** Confirm it is served and ordered before the bundle:

```bash
docker exec addon_<hash>_lemonade sh -c \
  'curl -s http://127.0.0.1:13305/web-app/ | grep -c "ha-addon-shim.js\"></script><script defer"'
```

`1` is correct. The shim must load **before** the deferred bundle; if that
ordering breaks, the base URL is read before the shim publishes it.

### The fallback that always works

Direct access at `http://<ha-ip>:13305/web-app/` does not depend on the
base-URL adaptation at all — there is no path prefix to reconstruct. If ingress
breaks, use the port directly while the anchor is fixed.

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
