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

**The Builder builds each app for one architecture.** `builder.yml` picks amd64 when `build.yaml` has an amd64 base image, and aarch64 under QEMU only otherwise (`hay_cm5_fan`). A green Builder therefore says nothing about arm64 for every other app. To check arm64, build the changed stages on an arm64 machine with no image output: `docker buildx build --output type=cacheonly --target <stage> -`, with the Dockerfile on stdin when the stages copy nothing from the context (done for `lemonade` in PR #103).

## Automated PR Management

The repository includes comprehensive automation for managing pull requests:

### Automatic Version Updates
- Daily checks for new Portainer releases (LTS and STS), Arcane, and Dockhand
- Automatically creates PRs with version updates and changelogs
- Lemonade bumps additionally get `needs-review` when upstream's Migration page covers the crossed range, the release's Breaking Changes section names something the app's code uses, or the leading version component jumps by more than one (`.github/scripts/check-lemonade-migration.sh`, `check-lemonade-breaking.sh`, and an inline scheme check; see `lemonade/CLAUDE.md` "Version Updates"). Lemonade numbers releases `YYYY.WW.N` since 2026.39 and ships weekly; candidates are `candidate-v*` prereleases that `releases/latest` ignores
- Every `api.github.com` call in an update script goes through `gh_api` in `.github/scripts/upstream-lib.sh` (a flat file on purpose: `.gitignore` has `lib/`, so a helper under `.github/scripts/lib/` never reaches the repo and every script dies on its `source` line in CI): authenticated with `GITHUB_TOKEN` (the anonymous 60 req/h budget is per runner IP and shared), retried, and never silent — the 2026-09-20 Portainer LTS check died with no output because `set -e` fired on `LATEST_VERSION=$(get_latest_version)` before the error branch. The scripts also refuse to propose a version that sorts below the one shipping (`ver_lt`); Portainer picks the highest LTS/STS-named release, since two LTS lines are patched at once
- **Every bot PR leaves two red zero-job `pull_request` runs** (PR Validation + Builder). GitHub creates them for a `GITHUB_TOKEN`-created PR, parks them as `action_required`, and marks them `failure` the second the PR merges. They never ran and mean nothing; the `repository_dispatch` pair is the real gate. A red run *with jobs* is the one to read
- **A bump that skips releases carries their notes too** (`skipped_release_notes` in `upstream-lib.sh`, used by Lemonade and Arcane): CHANGELOG.md gets a `## <version>` section per skipped release, and Lemonade's breaking-changes check reads them. Lemonade 2026.46.2 was parked, 2026.47.1 superseded it, and 46.2's Breaking Changes would never have been checked
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
- A new release's issue closes the older ones nobody has written on, since their checklist can no longer be run. One with comments stays open and is linked from the new issue. **Run the checklist; do not just close it.** 18.5's sat unread for five days while `hay_cm5_fan` could no longer drive its GPIO (#86)
- Smoke tests additionally run each app confined by its own `apparmor.txt` when the CI runner supports AppArmor

### Auto-merge
- Automatically merges PRs created by github-actions[bot] that pass all validations
- Requires `automated` and `validation-passed` labels
- Blocked by `do-not-merge`, `needs-review`, or `on-hold` labels
- A bot PR that the pipeline parks with `needs-review` (a Lemonade gate, a base-image major bump) also requests the repo owner's review when it opens. The label alone notifies no one: Lemonade 2026.39.1 sat three days unnoticed in #38
- Lemonade's checks re-run on the open PR every day; when they all pass, the pipeline removes a `needs-review` **it** added (never one a person added), so the next dispatch auto-merges. A gate script must never exit 1 on an internal error, since 1 means "flagged": `check-lemonade-breaking.sh` crashed that way on 2026.40.0 and parked #44 with an empty hit list. It now maps any failure to exit 2, and the workflow treats "flagged with no hits" as a failed check
- A bot PR whose Builder run fails gets a comment **and a review request to the repo owner**. The comment alone reached no one: Arcane 3.0.0 failed its smoke test for four days in #91 and #92
- A newer bot PR for the same app closes the older one and deletes its branch (`close-superseded` in `pr-validate.yml`). Every release has its own branch, so #91 (Arcane 3.0.0) stayed open beside #92 (3.0.1)
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
- **Compile Go helpers in a stage of the target platform** — no `--platform=$BUILDPLATFORM` with `GOARCH=${TARGETARCH}`. The smoke-test build in `builder.yml` passes `TARGETARCH` as the Home Assistant name (`aarch64`), which Go rejects: `unsupported GOOS/GOARCH pair linux/aarch64` (hay_cm5_fan 1.4.0, PR #100). The Supervisor passes no `TARGETARCH`, so it built fine everywhere but there

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

## Re-shipping an upstream version: `X.Y.Z.N`, never `X.Y.Z-N`

A maintenance note reaches no one until they rebuild by hand. To offer a fix as an
update, give an app that mirrors an upstream version a fourth component:
`2.45.2` → `2.45.2.1` → `2.45.2.2`. Apps with no upstream (`aegis_ha`,
`sonuntius`, `hay_cm5_fan`) take a normal patch bump instead.

- **Not a dash suffix.** HA core compares with AwesomeVersion, which reads
  `2.45.2-1` as a *pre-release* of 2.45.2, so the update entity never turns on.
  (Supervisor's own `!=` check would still show it on the app page.) Verified
  in the HA container on 2026-10-22: `.1` is the only suffix ordered above
  `2.45.2` and below `2.45.3`.
- The update scripts compare upstream against `strip_revision`
  (`upstream-lib.sh`). Without it, `2.45.2` sorts below `2.45.2.1` and the
  script refuses it as a downgrade. The next upstream release writes a plain
  `X.Y.Z`, dropping the `.N`.
- HA's release-notes slice stops at the first heading that *starts with* the
  installed version, so a user going straight from 2.45.2 to 2.45.3 never sees
  the `## 2.45.2.1` notes. Repeat anything they must know in the next entry.

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

**Images an app pulls at runtime count too.** `huly` pulls 14 images with
`docker-compose pull` on every version change, all anonymous. They failed every
CI smoke test from 2026-09-28 on the rate limit, blocking all base-image PRs.
Its compose template now references `mirror.gcr.io/...` throughout.

`ghcr.io` is not rate-limited this way, so `$BUILD_FROM` and any other ghcr.io
reference stays as-is. A bare `docker.io` reference is the thing to avoid; it
builds fine on a CI runner and fails on a user's Pi, which is the worst place
to find out.

**This applies to scripts too, not just Dockerfiles.** `smoke-test.sh` starts
the mock Supervisor and an MQTT broker with `docker run`, and those pulls hit
the same limit — a Docker Hub timeout there failed a master Builder run while
every Dockerfile was already mirrored. Any `docker run`/`docker pull` in CI
takes the mirror as well.

## A distro's package server at build time: small, cached, with a fallback

`apt-get update && apt-get install` in a build stage downloads from the
distro's own servers, on the device, and no registry cache covers it. On
2026-11-19 Ubuntu's archive was failing for everyone. Lemonade's update sat at
"installing 0%" for 19 minutes, 18 of them in that step (the Supervisor shows
no percentage for a build).

`lemonade/Dockerfile`'s `glibc-provider` stage is the pattern:

- **No `ARG` above the package step.** Every `RUN` after an `ARG` runs again
  when its value changes, so a version ARG at the top of the stage downloads
  the packages again on every bump.
- **Fetch only the lists you use.** Ubuntu's `main` is 4 MB; all four
  components are 27 MB.
- **Give apt a mirror list** (`mirror+file:`). It moves to the next server when
  one does not answer. A server that answers slowly is not a failure to apt,
  so only the smaller download helps there.
- **Keep tools out of that stage.** curl and CA certificates added 26 packages
  to it to download one archive; the app's base image already has both.

Together: 4.5 MB from Ubuntu instead of 34 MB. Measured on amd64 with servers
made unreachable (`--add-host <name>:192.0.2.1`): normal 7s, main server silent
62s, two of three silent 97s, all three a clear failure after 54s.

`muninndb`'s `source` stage has the same kind of step against Debian's server
and has not been changed.

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

## An app built for the site root can still run under ingress

Arcane has no sub-path setting and compares the page's path with `/login`, so
neither a rewritten base path nor a plain proxy works. `arcane/ha-arcane-ingress`
does it with a proxy plus a script in the page; `arcane/CLAUDE.md` ("Ingress")
has the design.

`dockhand/ha-ingress-adapter` is the same proxy and script with nothing of one
app in them: the app's address comes from `UPSTREAM_URL`, and
`INGRESS_OPEN_AT` names a page to open in place of `/`. Use it for the next
app that needs this.

What transfers to another app:

- **Mimic Home Assistant exactly when testing.** It drops `Accept-Encoding` from
  the request and the charset from `Content-Type`, keeps `Host`, and adds
  `X-Ingress-Path`. `.github/scripts/ingress_harness.py` has a stand-in written
  from Core's and Supervisor's source and a browser driver, and counts requests
  that leave the prefix. Each app's `tests/ingress_test.py` brings its own
  checks.
- **Always run the control.** The same page without the fix must fail, or the
  test proves nothing.
- **A page can be drawn and still be broken.** Dockhand's shell rendered while
  every data request went to Home Assistant. Count the requests that leave the
  prefix; do not judge by what is on screen.
- **An app that forbids framing shows nothing in a panel.** The adapter turns
  `X-Frame-Options` into `SAMEORIGIN` and `frame-ancestors` into `'self'`, for
  ingress requests only. Home Assistant serves a panel from its own origin, so
  that allows Home Assistant and no other site, and the app's own port keeps
  its protection.
- **Entry imports written relative to the page** (`import("./_app/...")`) have
  to name the prefix too, because the page's address has lost it by then.
- **Cookies are per host, not per port.** A cookie the app sets on its direct
  port also arrives through ingress on the same host name. Scope and mark the
  ingress ones.
- **A `__Host-` cookie cannot be scoped to a path**, and a plain-HTTP test never
  shows one. Check what the app does over HTTPS before rewriting `Path`.

## Telling Home Assistant's containers from the user's

The Docker manager apps leave out what Home Assistant manages
(`hide_hassio_containers`). Three marks decide it, and none is a list that
goes stale:

- label `supervisor_managed`, empty value: the Supervisor sets it on every
  container it starts (since 2022.07.1) and finds its own events by it
- `io.hass.type=supervisor`: the Supervisor's own container, which HAOS starts
- `io.github.shobuprime.app-managed=true`: this repository's convention. An
  app that starts containers of its own (a compose stack, a helper) labels
  them, because the Supervisor does not know them and stopping one from a
  manager breaks the app

**A manager app never names another app.** It knows the marks. An app opts in
by labelling what it starts, so a new app needs no change in any manager.

**Not by `io.hass.type` in general.** It is an image label, and three things
follow. Some images carry none, so a list of its values misses their
containers: the list shipping before this, even with `app` added, missed one
of a device's 24. Other images carry one although anybody can run them, so a
list would also hide somebody's own container started from such an image. And
the values get added to: `app` arrived in 2026.04, and the list written before
it hid fewer than half. No name rule either: `app_db` would be somebody's
database.

The first two marks matched all 24 Home Assistant containers on a device and
none of its 20 others (2026-11-20).

Portainer's `--hide-label` matches name and value exactly and filters only its
container list: a hidden container is still reachable by ID, and counters,
images and networks are untouched. The Supervisor writes `supervisor_managed`
with an empty value and reads it back by name only, so nothing on its side
would notice the value changing. The Portainer apps therefore take the value
from their own container at start, which the Supervisor started too, and warn
when the label is missing.

The smoke test starts every app with that label, as the Supervisor does, and
reads Portainer's list through its API, with stand-in containers and a control
on the raw socket.

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
| `arcane` | 4.4s | clean | 10 (default) | Re-measured 2026-11-19 with the ingress proxy added. |
| `muninndb` | 3.7s | clean | **120** | Empty DB; shutdown backup ran in 126ms. |
| `dockge` | 5.6s | clean | 10 (default) | |
| `huly` | 24-25s | clean | **120** | Also backup downtime (`backup: cold`). |
| `lemonade` | 9-13s | clean | **30** | |
| `hay_cm5_fan` | not measured | — | 10 (default) | aarch64-only. |

Every app on the 10s default has at least ~2x headroom and stops cleanly, so
those defaults are correct — **do not add `timeout:` to them speculatively.**

Caveat on the raised ones:

- **`muninndb` 3.7s was an empty database.** The shutdown backup completed in
  126 ms writing 2.3 KB. Backup duration scales with stored data, so this
  confirms the mechanism works — it does not bound a populated instance. The
  120s stands as headroom.

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

## A green merge does not keep an app installable

Apps are built on the device at install and update time, so every URL a
Dockerfile fetches has to still answer then. MuninnDB 0.12.1 merged on
2026-11-17 with a green build; the same day `github.com/microsoft/onnxruntime`
began returning 404, and from then on 0.12.1 could not be built on any
architecture. The device shows only `An unknown error occurred while trying to
build the image`; the failing step is in the Supervisor log
(`sudo docker logs hassio_supervisor`).

Nothing in CI sees this after the merge. When an update fails to build on the
device, rebuild the same version locally before suspecting the device or the
architecture. The fix here was upstream's next release (0.13.0 pins hashes and
falls back to a mirror). Any PR that touches an app's directory rebuilds that
app, so a docs-only change to a broken app fails its Builder too.

## An upstream major version needs an upgrade test, not just a smoke test

The smoke test boots an empty data directory, so it proves a new install and
nothing about an existing one. Before shipping an upstream major, boot the
shipping image on an empty `/data`, create real state through the app's API
(a changed password, a stored secret), stop it, and boot the new image on the
same directory. Arcane 2.17.3 → 3.1.0 was checked this way: database migrated,
old password accepted, and a registry token encrypted by 2.17.3 arrived intact
at a capture server.

Data made by the previous version is still not a long-lived install. Arcane
3.1.0 passed this test, and on a device first installed in early 2026 it could
not create projects: the settings table still held a path an old release had
stored (`arcane` 3.1.0.2). When a real install is at hand, read its settings
read-only and compare them with what the test created.

The smoke test did catch Arcane 3, but only because Arcane refuses to start on
its renamed settings. A major that starts and silently ignores an old setting,
such as an encryption key, would pass and auto-merge.

## Legacy handling has an end date

Code that exists only for installs an old release made (a settings migration,
a path rewrite, a compatibility branch) follows three rules:

- **Migrate, do not alias.** Move the install to what upstream uses today. A
  symlink or fallback that keeps the old path working carries upstream's past
  layout for good, and the next upstream change stacks on top of it.
- **Touch only what a release wrote.** Rewrite a value this app or upstream
  stored as a default. A value a person chose stays, even an odd one. Copy
  files before changing where they are looked for, never over an existing
  file, and let the app start whatever happens.
- **Give it a date.** Put `LEGACY UNTIL YYYY-MM-DD` in a comment on the code
  and its tests. `.github/scripts/check-legacy-markers.sh` runs in the smoke
  test: after the date that app's build fails until the code is gone, or the
  date is moved with a reason. Six months is the usual span. Put the manual
  fix in DOCS.md for whoever updates later than that.

The other half is noticing upstream's changes when they happen. Where an app
depends on upstream's layout, read that layout from the artifact itself and
check it in the smoke test, so a bot PR stops instead of merging.
`arcane/tests/upstream_paths.py` reads Arcane's default paths out of its binary
and fails on one the image does not provide.

First use: `arcane` 3.1.0.2 (2026-11-19). An install from early 2026 held a
projects directory under `/opt/arcane`, stored when the app still ran Arcane
from there, and could not create projects.

## Notes

- Always test apps locally before pushing to the repository
- Follow Home Assistant app best practices
- Keep dependencies up to date
- Document all configuration options clearly
