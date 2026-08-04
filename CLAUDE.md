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

## Folder mappings (`map:`) and the addon → app rename

Supervisor's authoritative list is the `MappingType` enum in `supervisor/apps/const.py`. Host
source → default container target:

| `map:` name | mounted at | notes |
|---|---|---|
| `data` | `/data` | always mounted rw whether or not you list it |
| `ssl` | `/ssl` | |
| `share` | `/share` | rslave propagation |
| `media` | `/media` | rslave propagation |
| `backup` | `/backup` | |
| `homeassistant_config` | `/homeassistant` | HA's config dir |
| `config` | `/config` | **deprecated** — use `homeassistant_config` |
| `app_config` | `/config` | this app's own config dir |
| `all_app_configs` | `/app_configs` | every app's config dir |
| `local_apps` | `/local_apps` | the local apps dir |

**Deprecated as of Supervisor 2026.07** (accepted, but each logs
`uses legacy map type '<old>'; use '<new>' instead`):

| legacy | replacement | container path changes? |
|---|---|---|
| `addon_config` | `app_config` | no — both mount at `/config` |
| `all_addon_configs` | `all_app_configs` | **yes** — `/addon_configs` → `/app_configs` |
| `addons` | `local_apps` | **yes** — `/addons` → `/local_apps` |

Two rules that follow:

- **Renaming `addon_config` is free; renaming the other two is not.** `addon_config` and
  `app_config` resolve to the same source and the same `/config` target, so it is a pure config
  edit. `addons`/`all_addon_configs` change the in-container path, so any script referencing the
  old path must change with them.
- **Never list a legacy name alongside its replacement** — Supervisor logs "incompatible map
  options" and ignores the legacy one.

**An unrecognised map name is silently SKIPPED, not rejected.** Supervisor drops the mount and
installs the app anyway, so a typo costs you a volume with no error anywhere. `pr-validate.yml`
checks every `map:` name against the list above for exactly this reason.

**Container naming**: Supervisor 2026.07.4 renamed app containers from `addon_<slug>` to
`app_<slug>` (`supervisor/docker/app.py`: `return f"app_{slug}"`), with a migration that renames
existing `addon_*` containers on attach. Any script that constructs its own container name must
try `app_` first and keep `addon_` as a fallback for older Supervisors — `huly` does this in
`rootfs/etc/cont-init.d/huly.sh` and `rootfs/etc/services.d/huly-bridge/run`. Prefer reading the
real 64-hex ID out of `/proc/self/mountinfo` and treat any constructed name as a fallback.

Not renamed, and not to be "migrated": the Supervisor **REST API** is still `/addons/self/*`.
Do not extend the addon → app rename into HTTP paths.

## Maintenance notes go BELOW the version heading they amend

An app-only fix ships without a version bump, recorded as a
`> _Maintenance (YYYY-MM-DD):_ …` blockquote in that app's `CHANGELOG.md`. It must be placed
**immediately below the `## <version>` heading it amends** (after the `_date_` line), never above
it.

This is not cosmetic. Home Assistant renders release notes by slicing the changelog from the
latest version's heading down to the installed one — `async_release_notes()` in
`homeassistant/components/hassio/update.py`:

```python
regex_pattern = re.compile(
    rf"^#* {re.escape(self.latest_version)}\n"
    rf"(?:^(?!#* {re.escape(self.installed_version)}).*\n)*", re.MULTILINE)
```

Two things follow, and both have already happened here:

- A note written **above the first heading** is swallowed by whatever version the next update
  prepends. `lemonade`'s `## 11.5.1` entry ended up displaying five notes dated 07-25/07-26 —
  written while 11.5.0 was current — because the update script prepended the new section above
  them. Eight notes across seven apps had drifted this way.
- If no newer version has landed yet, the same note is instead **invisible**: the slice starts
  *at* the heading, so anything above it is never rendered. `dockge` had three notes no user
  ever saw.

Enforced by `pr-validate.yml`'s `validate-changelog` job, which rejects a `_Maintenance (` line
appearing before the first version heading. That job also rejects a `CHANGELOG.md` that is not
valid UTF-8 — the old `head -c 1000` truncation (removed in #197) cut multi-byte characters in
half and left `muninndb/CHANGELOG.md` undecodable.

## AppArmor Profile Rules

Hard-won rules from the July 2026 Huly outage on HAOS 18.1 (kernel 6.18), enforced in CI by `.github/scripts/validate-apparmor.sh` (full incident record: `huly/CHANGELOG.md` 0.7.426 maintenance notes, PRs #165/#166):

- **Profiles must be FLAT — never use nested child profiles** (`profile foo { ... }` inside the main profile with `cx ->` transitions). HAOS 18.1's kernel denies AF_UNIX socket connects from processes confined by nested child profiles regardless of the rules the child contains (verified on-device: the identical ruleset connects flat, fails nested, with both apparmor_parser 3.1.7 and 4.1.7).
- **Docker-socket apps need resolved-path rules.** The Supervisor mounts the socket at `/run/docker.sock`, and `/var/run` is a symlink to `/run`; AppArmor matches the *resolved* path, so a `/var/run/docker.sock`-only rule never matches anything. Use explicit `/run/docker.sock rw,` plus `/var/run/docker.sock rw,`.
- **Docker-socket apps need a bare `network,` rule** (all address families — AF_UNIX is needed for the socket).
- **Profile delivery lag**: the Supervisor imports `apparmor.txt` to `/data/apparmor/<repo-hash>_<slug>` (renaming the profile to the slug) and loads it into the kernel only on add-on **install/update/rebuild** — merging a profile fix does not heal a live device until the add-on is rebuilt.
- **Signal rules must grant `receive`, not just `send`** — and a profile with *no* signal rule is equally broken. AppArmor mediates signal *delivery* as well as sending, so a profile carrying only `signal (send) set=(kill,term,int,hup,cont),` (or omitting signal rules entirely) silently denies the confined process the right to receive anything: s6's SIGTERM never lands, the app's graceful shutdown is skipped, and the container is SIGKILLed after the grace period (exit 137). Use `signal (send,receive),`. Measured on one identical image (`lemonade`), varying only this rule — no signal rule → 13.2s / exit 137; `signal (send) set=(...)` → 12.7s / exit 137 with no cleanup logged; `signal (send,receive),` → 6.9s / exit 0 with the full s6 shutdown sequence. All 11 profiles now comply, and this is enforced two ways: statically by `validate-apparmor.sh` rule 4, and at runtime by `smoke-test.sh`, which hard-fails on exit 137 when the app ran confined. Because the Supervisor only reloads a profile on install/update/rebuild (see "Profile delivery lag" below), the July 2026 fix to the eight affected apps does not reach a live device until each app is rebuilt.
- **Debugging on a live device**: a `docker exec` shell runs under the top-level profile, so compare `curl --unix-socket /run/docker.sock http://localhost/_ping` from exec against the app's own process behavior; read any process's live confinement from `/proc/<pid>/attr/current`; loaded profiles are listed under `/sys/kernel/security/apparmor/policy/profiles/`. Kernel AppArmor denials do **not** appear in `dmesg` on HAOS — prove theories with throwaway test profiles (`apparmor_parser -r` + `docker run --security-opt apparmor=<name>`) instead of log-hunting.

## Never pull build stages straight from Docker Hub

Apps are built **on the user's device**, where the pull is always anonymous and
therefore subject to Docker Hub's rate limit. When it trips, the install fails
outright:

```
ERROR: failed to solve: golang:1.26.5-alpine: failed to resolve source metadata
  ... 429 Too Many Requests
```

Every `FROM` that would otherwise hit `docker.io` goes through
**`mirror.gcr.io`**, Google's pull-through cache, which needs no authentication:

| Instead of | Use |
|------------|-----|
| `golang:1.26.5-alpine` | `mirror.gcr.io/library/golang:1.26.5-alpine` |
| `debian:trixie-slim` | `mirror.gcr.io/library/debian:trixie-slim` |
| `louislam/dockge:1.5.0` | `mirror.gcr.io/louislam/dockge:1.5.0` |

Official images take the `library/` prefix; everything else keeps its
namespace. The image content is identical — only the registry changes.

`ghcr.io` is not rate-limited this way, so `$BUILD_FROM` and any other ghcr.io
reference stays as-is. A bare `docker.io` reference is the thing to avoid; it
builds fine on a CI runner and fails on a user's Pi, which is the worst place
to find out.

**This applies to scripts too, not just Dockerfiles.** `smoke-test.sh` starts
the mock Supervisor and an MQTT broker with `docker run`, and those pulls hit
the same limit — a Docker Hub timeout there failed a master Builder run while
every Dockerfile was already mirrored. Any `docker run`/`docker pull` in CI
takes the mirror as well.

## dockerd rejects Docker API calls that arrive through HA's ingress

Any app that hands the Docker socket to a web UI reachable through Home
Assistant ingress can hit this, and it presents as an application bug rather
than a transport one:

```
Failed starting container: starting container with non-empty request body was
deprecated since API v1.22 and removed in v1.24
```

- Docker removed HostConfig-in-the-body from `POST /containers/{id}/start` in
  API v1.24. Because silently ignoring a body could silently discard config a
  legacy client meant to apply, dockerd **hard-rejects** any start request that
  *might* carry one — `Content-Length > 0`, or a length that is unknown
  (`Transfer-Encoding: chunked`).
- **The check runs in the HTTP handler before any container lookup**, so the
  400 describes no container. Verified against a live dockerd: the same
  malformed request aimed at an *already running* container returns the
  identical error. Real start failures are only visible in
  `docker inspect <name> --format '{{.State.Error}}'`. This is why the bug is
  worse than it looks — it *replaces* whatever the real error was.
- Go reverse proxies emit chunked whenever the body size is unknown, which is
  what `ingress: true` + `ingress_stream: true` puts in front of the app. Any
  UI that forwards the browser's request body/encoding onward to the socket
  inherits the problem.
- Reproducer on any box with docker:
  ```bash
  curl -s -w '\n%{http_code}\n' --unix-socket /run/docker.sock \
    -X POST -H 'Content-Type: application/json' -H 'Transfer-Encoding: chunked' \
    -d '{}' http://localhost/v1.44/containers/<any-running-container>/start
  # -> 400 with the exact message, even though the container is healthy
  ```

Upstream: portainer/portainer#9239, hassio-addons/addon-portainer#127,
home-assistant/core#156363. HAOS's dockerd is not configurable (min API 1.40,
no opt-out), so **the app layer is the only place this can be fixed.**

`portainer_ee_lts` and `portainer_ee_sts` fix it with an nginx socket shim
(`rootfs/etc/nginx/docker-shim.conf`) that strips the body on the lifecycle
verbs where one is never legal and passes everything else through byte-for-byte.
The transferable parts, if another app needs the same treatment:

- **Strip only where a body is illegal** — `/containers/{id}/(start|stop|
  restart|kill|pause|unpause|wait)`. `/exec/{id}/start` takes a real JSON body
  and `…/attach` needs the upgrade tunnel; stripping either breaks the console
  with no error anywhere.
- **Run nginx from its own config with `nginx -c`, not Alpine's nginx.conf.**
  The stock config defines `$connection_upgrade` already (a second `map` for it
  is a fatal duplicate) and caps bodies at 1m, which 413s image builds.
- **Workers must run as root** — the socket is `root:docker` 0660 and the
  `nginx` user gets EACCES, which surfaces as every request 502ing.
- **Put the listen socket in a 0700 directory** — nginx chmods unix listen
  sockets to 0666 unconditionally.
- **Redirect the path the app already uses; do not move the app to a new one.**
  Portainer stores its environment address in BoltDB and ignores `--host` once
  any environment exists, so a new socket path fixes nothing for existing
  installs. `/var/run` is a symlink to `/run` in the base image, so replacing
  that symlink with a real directory containing `docker.sock -> <shim>` puts the
  shim in the path transparently, for new and existing installs alike, with the
  real socket still at `/run/docker.sock`. This is safe for two specific
  reasons, both of which must keep holding: s6-overlay's `preinit` restores
  `/var/run -> /run` before stage0 on every boot (so the swap never leaks into
  a later boot, at the cost of a `warning: /var/run is not a symlink to /run,
  fixing it` line on in-place restarts), and the one base-image writer under
  `/var/run` — `base-addon-log-level`, writing
  `/var/run/s6/container_environment/LOG_LEVEL` — is ordered ahead of
  `legacy-cont-init`, so it runs while `/var/run` is still the symlink. The
  base sets `S6_BEHAVIOUR_IF_STAGE2_FAILS=2`, so a second cont-init script or
  service writing under `/var/run/` would not degrade the app — it would stop
  the container booting.
- **Unlink the shim's listen socket before binding it.** nginx removes a unix
  listen socket only on a clean shutdown and never unlinks a pre-existing one,
  so a SIGKILL, OOM-kill or unclean container stop leaves it behind and every
  respawn dies with `bind() ... (98: Address in use)` permanently. `/run` in
  these containers is the image layer, not a tmpfs, so it even survives a
  restart. Worse, it is silent: the app keeps serving its own health port, so
  the `HEALTHCHECK` and the `watchdog:` both stay green while it has no Docker
  access at all. `rm -f` the socket path in the service's `run` script.

## Home Assistant Core is NOT running when your app's init script runs

Supervisor starts apps in stages and Core starts *between* them
(`supervisor/core.py`):

```
265:  await self.sys_apps.boot(AppStartup.SERVICES)     # <- most of our apps
274:  await self.sys_homeassistant.core.start()          # <- Core, after
```

So for any app with `startup: initialize|system|services`, `/core/api/...` in
`cont-init.d` gets a 502 — Core isn't up. Supervisor's own endpoints
(`/addons/self/info`, `/network/info`) are fine.

It fails silently, because `curl -sf | jq '.x // empty'` turns both "Core is
down" and "not configured" into the same empty string. And it never reproduces
when you restart the app by hand, so it looks intermittent.

**Retrying in init does not fix it — it deadlocks.** `App.start()` returns a
task that completes only when the app reports started (for apps with a
healthcheck, healthy *or* unhealthy), and `AppManager.boot()` awaits all of them
before Core is started. So init waits for Core, Core waits for the stage, the
stage waits for init. It breaks when `STARTUP_TIMEOUT = 120` expires
(`apps/app.py:128`): two minutes lost every boot, app possibly flagged.

Do it the other way round: **init stays fast, a long-running process fetches
the value and retries.** The app serves throughout and corrects itself when the
answer arrives. Worked example: `lemonade/ha-lemonade-bridge/origins.go`.

## Stop Timeouts (HAOS updates and backups)

Home Assistant stops an app's container for **every HAOS update** and **every
backup that includes app data**. A stop that does not complete in time is
therefore a routine-operation failure, not an edge case.

Three defaults conspire against long-running apps:

| Setting | Default | Meaning |
|---------|---------|---------|
| `timeout:` in `config.yaml` | **10s** | Supervisor SIGKILLs the container after this (`ATTR_TIMEOUT`). |
| `S6_SERVICES_GRACETIME` | **3000ms** | S6 kills a service that hasn't stopped by then. |
| `S6_KILL_FINISH_MAXTIME` | **5000ms** | S6 kills a `finish` script that hasn't returned by then. |

Rules:

- **The failure is silent.** When S6's grace expires it kills the service
  mid-cleanup and the container **still exits 0**. The only evidence is
  `s6-svwait: fatal: timed out` in the log. `smoke-test.sh` greps for this and
  fails the build; do not suppress it.
- **`timeout:` must exceed `S6_SERVICES_GRACETIME`**, or the Supervisor kills
  the container while S6 is still waiting. Raising one alone achieves nothing.
- **`timeout:` is a ceiling, not a delay** — a fast stop still returns
  immediately, so being generous costs nothing in the normal case.
- **Anything doing work in a `finish` script needs `S6_KILL_FINISH_MAXTIME`
  raised.** 5s does not fit a database backup. `muninndb`'s
  `backup_on_shutdown` runs `muninn backup` there and was being truncated; a
  half-written backup is worse than none because it looks like a restore point.

### Measured stop times

Measured on amd64, each app confined by its own AppArmor profile against the
mock Supervisor, stopped once healthy (`scratchpad/measure-stop.sh` pattern:
build, run, wait for health, `docker stop`, check exit code and grep for
`s6-svwait`). Re-measure an app after changing what it runs at shutdown.

| App | Stop | Result | `timeout:` | Notes |
|-----|------|--------|-----------|-------|
| `sonuntius` | 3.4s | clean | 10 (default) | |
| `aegis_ha` | 3.4s | clean | 10 (default) | |
| `portainer_ee_lts` | 3.0s | clean | 10 (default) | Re-measured 2026-07-31 with the nginx socket shim added. |
| `portainer_ee_sts` | 3.0s | clean | 10 (default) | Re-measured 2026-07-31 with the nginx socket shim added. |
| `dockhand` | 3.6s | clean | 10 (default) | |
| `arcane` | 3.6s | clean | 10 (default) | |
| `muninndb` | 3.7s | clean | **120** | Empty DB; shutdown backup ran in 126ms. |
| `dockge` | 5.6s | clean | 10 (default) | |
| `huly` | 9.4s | clean | **120** | Lower bound — see caveat. |
| `lemonade` | 9-13s | clean | **30** | |
| `hay_cm5_fan` | not measured | — | 10 (default) | aarch64-only. |

Every app on the 10s default has at least ~2x headroom and stops cleanly, so
those defaults are correct — **do not add `timeout:` to them speculatively.**

Two caveats on the raised ones, because both numbers are lower bounds rather
than worst cases:

- **`muninndb` 3.7s was an empty database.** The shutdown backup completed in
  126 ms writing 2.3 KB. Backup duration scales with stored data, so this
  confirms the mechanism works — it does not bound a populated instance. The
  120s stands as headroom.
- **`huly` 9.4s was measured on a stack that never reached full health** (some
  of the 14 containers were restarting under memory pressure, the same
  condition CI hits). A settled stack with real data would flush more on the
  way down. Note it already lands *at* the 10s default, so its raised value is
  doing real work rather than being precautionary.

`hay_cm5_fan` is aarch64-only and is not measured on an amd64 host (registering
an arm64 binfmt handler is a host-level change and is deliberately not done).

**It was previously recorded here as stopping promptly on the 10s default. That
was wrong.** Being a self-contained shell script does not make a stop fast: its
poll loop used a bare `sleep`, and bash defers trap handlers until the
foreground child exits, so SIGTERM sat unhandled for up to `poll_interval` — 5s
by default, up to 60s by option — and S6 killed the daemon partway through
cleanup. CI caught it intermittently on run 30163534966 (`s6-svwait: fatal:
timed out`) while the same commit passed on a later run, because whether it
fails depends on where `docker stop` lands in the poll cycle.

The lesson generalises: **a shell daemon that sleeps in its main loop cannot
handle signals promptly.** Use `sleep N & wait $!` so the trap runs
immediately. Assess against what the loop actually does, not the language it is
written in.

The smoke test reads each app's `timeout:` and judges the stop against it, so
these numbers stay honest as apps change — no value is hardcoded there.

## Notes

- Always test apps locally before pushing to the repository
- Follow Home Assistant app best practices
- Keep dependencies up to date
- Document all configuration options clearly
