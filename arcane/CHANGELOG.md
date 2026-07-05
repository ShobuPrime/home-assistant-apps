# Changelog

## 2.3.2

_2026-07-05_


### Bug fixes

* dialog now owning there own close states, cause mutiple to show at one time([d253fc5](https://github.com/getarcaneapp/arcane/commit/d253fc5d7c081ca43c1385d13da2e127d8bc9e3e) by `kmendell`)
* hide phantom projects from showing in the frontend when deleted ([#3136](https://github.com/getarcaneapp/arcane/pull/3136) by `kmendell`)
* keep modals and menus opaque in dark mode when Glass & Blur is off ([#3139](https://github.com/getarcaneapp/arcane/pull/3139) by `othyn`)
* nested compose files discovery permission issues ([#3096](https://github.com/getarcaneapp/arcane/pull/3096) by `kmendell`)
* ntfy tls regression, image update notifcation flag not be recognized ([#3143](https://github.com/getarcaneapp/arcane/pull/3143) by `kmendell`)
* notification sending rework for reliability ([#3144](https://github.com/getarcaneapp/arcane/pull/3144) by `kmendell`)
* use explicit context for notifications([6987b5e](https://github.com/getarcaneapp/arcane/commit/6987b5e046c3a00c54c88ee54219dc8f

---


## 2.3.1

_2026-07-03_


### Bug fixes

* discard env_file when loading projects to match compose CLI config-hash ([#3100](https://github.com/getarcaneapp/arcane/pull/3100) by `pkoutsovasilis`)
* set explicit gorm LRU cache TTL to avoid constantly rising heap memory ([#3102](https://github.com/getarcaneapp/arcane/pull/3102) by `kmendell`)
* only display memory usage thats non-reclaimable ([#3105](https://github.com/getarcaneapp/arcane/pull/3105) by `kmendell`)

### Dependencies

* bump prettier from 3.9.0 to 3.9.3 ([#3116](https://github.com/getarcaneapp/arcane/pull/3116) by `dependabot`[bot])
* bump `tanstack`/svelte-query from 6.1.35 to 6.1.36 ([#3115](https://github.com/getarcaneapp/arcane/pull/3115) by `dependabot`[bot])
* bump golangci/golangci-lint-action from 9.2.1 to 9.3.0 ([#3106](https://github.com/getarcaneapp/arcane/pull/3106) by `dependabot`[bot])
* bump the tanstack-table group across 1 directory with 2 updates ([#3126](https://github.com/getarcaneapp/arcane/pull/3126) by `dependabot`[bot])
* bump githu

---


## 2.3.0

_2026-07-02_


### New features

* add appearance toggles for blur and interface animations ([#3091](https://github.com/getarcaneapp/arcane/pull/3091) by `othyn`)

### Bug fixes

* edge tunnel go routine leak ([#3073](https://github.com/getarcaneapp/arcane/pull/3073) by `kmendell`)
* users with env only scopes unable to access the UI ([#3081](https://github.com/getarcaneapp/arcane/pull/3081) by `kmendell`)
* use correct disabletlsverification parameter for ntfy ([#3084](https://github.com/getarcaneapp/arcane/pull/3084) by `kmendell`)
* dont create trivy cache in server/client mode ([#3087](https://github.com/getarcaneapp/arcane/pull/3087) by `kmendell`)
* stop phantom and duplicate projects from broken syncs ([#3088](https://github.com/getarcaneapp/arcane/pull/3088) by `kmendell`)
* image polling notifications context being canceled early or not registered at all ([#3089](https://github.com/getarcaneapp/arcane/pull/3089) by `kmendell`)

### Dependencies

* bump svelte from 5.56.3 to 5.56.4 ([#3067](https:/

---


## 2.2.0

_2026-06-29_


### New features

* system mode for light/dark mode ([#2994](https://github.com/getarcaneapp/arcane/pull/2994) by `kmendell`)
* allow use of remote trivy server, and only show fixable cves ([#2999](https://github.com/getarcaneapp/arcane/pull/2999) by `kmendell`)
* preserve managed volumes on project rename ([#2919](https://github.com/getarcaneapp/arcane/pull/2919) by `NeurekaSoftware`)
* show attestations panel for images when supported ([#3036](https://github.com/getarcaneapp/arcane/pull/3036) by `kmendell`)
* add missing kill/pause container actions ([#3037](https://github.com/getarcaneapp/arcane/pull/3037) by `kmendell`)
* add image history, tagging, registry search and local comitting ([#3039](https://github.com/getarcaneapp/arcane/pull/3039) by `kmendell`)
* allow custom profile pictures ([#3023](https://github.com/getarcaneapp/arcane/pull/3023) by `OlziYT`)
* add pre-deploy hook for GitOps project syncs ([#3022](https://github.com/getarcaneapp/arcane/pull/3022) by `peitschie`)

### Bug 

---


## 2.1.0

_2026-06-19_


### New features

* add project file tree management ([#2893](https://github.com/getarcaneapp/arcane/pull/2893) by `NeurekaSoftware`)
* upgrade all environments button ([#2941](https://github.com/getarcaneapp/arcane/pull/2941) by `kmendell`)
* add support for riscv64 ([#2949](https://github.com/getarcaneapp/arcane/pull/2949) by `kmendell`)

### CLI - New features

* add registries create command ([#2874](https://github.com/getarcaneapp/arcane/pull/2874) by `manawenuz`)

### Bug fixes

* fix tables rows not flex redering to use the full table width ([#2928](https://github.com/getarcaneapp/arcane/pull/2928) by `kmendell`)
* add missing healthcheck cli command ([#2929](https://github.com/getarcaneapp/arcane/pull/2929) by `kmendell`)
* allow setting the data directroy for non docker installs ([#2931](https://github.com/getarcaneapp/arcane/pull/2931) by `kmendell`)
* fix dind path mappings for projects and swarm ([#2939](https://github.com/getarcaneapp/arcane/pull/2939) by `kmendell`)
* projects d

---


## 2.0.3

_2026-06-12_


### Bug fixes

* self updater not restarting container properly ([#2897](https://github.com/getarcaneapp/arcane/pull/2897) by `kmendell`)
* dashboard env counts not displaying in a timley manner ([#2901](https://github.com/getarcaneapp/arcane/pull/2901) by `kmendell`)
* user based api keys are capped at users permissions check ([#2918](https://github.com/getarcaneapp/arcane/pull/2918) by `kmendell`)
* serve webhooks from the manager and close edge command allowlist gaps ([#2922](https://github.com/getarcaneapp/arcane/pull/2922) by `kmendell`)
* compose updater not correctly falling back to standalone container update ([#2923](https://github.com/getarcaneapp/arcane/pull/2923) by `kmendell`)
* dont check image updates on locally built images ([#2924](https://github.com/getarcaneapp/arcane/pull/2924) by `kmendell`)

### Dependencies

* bump github.com/nicholas-fedor/shoutrrr from 0.15.1 to 0.16.0 in /backend ([#2867](https://github.com/getarcaneapp/arcane/pull/2867) by `dependabot`[bot])
* bump

---


> _Maintenance (2026-06-10):_ hassio-addons base 20.2.0 compatibility — migrated the Traefik helper scripts from the deprecated bashio::addon.* functions to bashio::app.*.

## 2.0.2

_2026-06-10_

### App build fix

* Fix the image build failing at the Alpine package step on base `20.2.0` (both aarch64 and amd64). The base pins `libcrypto3`/`libssl3` in apk `world` at an exact older revision; once the repo moved to `openssl 3.5.7-r0` (which requires `libcrypto3`/`libssl3=3.5.7-r0`), `apk add openssl` could not resolve against the held libs. Now `apk add --no-cache --upgrade openssl libcrypto3 libssl3 ...` rewrites those world entries and upgrades the whole TLS stack to the repo version in one transaction — no version pinning. (Under apk-tools 3, `apk upgrade --available` does not override an exact world pin, so the libs are listed explicitly.)

### Bug fixes

* update dockerfiles to use correct linker path for version details([725c003](https://github.com/getarcaneapp/arcane/commit/725c0034680aa366dbc8a5e02e827d1057f34ffb) by `kmendell`)
* newly synced git content does not show without a refresh ([#2870](https://github.com/getarcaneapp/arcane/pull/2870) by `kmendell`)
* incorrect height on dashboard cards on smaller screens ([#2878](https://github.com/getarcaneapp/arcane/pull/2878) by `kmendell`)
* add missing swarm-identity endpoint in edge tunnel ([#2886](https://github.com/getarcaneapp/arcane/pull/2886) by `kmendell`)
* activities stream using main context causing app to hang at certain places ([#2887](https://github.com/getarcaneapp/arcane/pull/2887) by `kmendell`)
* x-arcane.icon-light/icon-dark overwriting service-level icons ([#2888](https://github.com/getarcaneapp/arcane/pull/2888) by `kmendell`)
* bind mounts fail to update after git syncs ([#2891](https://github.com/getarcaneapp/arcane/pull/2891) by `kmendell`)
* normalize

---


## 2.0.1

_2026-06-08_


### Bug fixes

* update gomodule imports to /v2([6cb4913](https://github.com/getarcaneapp/arcane/commit/6cb491328f928532213d8efb846e96714dfd4f23) by `kmendell`)
* decrypting of notification tokens failing ([#2850](https://github.com/getarcaneapp/arcane/pull/2850) by `kmendell`)

### Dependencies

* bump updater module to new module name([50c0847](https://github.com/getarcaneapp/arcane/commit/50c084717deeb78179857e52768994d87a3690ac) by `kmendell`)



**Full Changelog**: https://github.com/getarcaneapp/arcane/compare/v2.0.0...v2.0.1

---


## 1.20.0

_2026-06-05_


### New features

* add removeOrphans option to project deploy/redeploy ([#2785](https://github.com/getarcaneapp/arcane/pull/2785) by `khanhx`)
* prune idle volume browser helper containers ([#2767](https://github.com/getarcaneapp/arcane/pull/2767) by `Zgrill2`)

### Bug fixes

* slog-go nil pointer dereference ([#2781](https://github.com/getarcaneapp/arcane/pull/2781) by `lohrbini`)
* dashboard card buttons paddings overlaps([c1a0bda](https://github.com/getarcaneapp/arcane/commit/c1a0bda6735a6c50ae989f7e4643ffb09b2edb75) by `kmendell`)
* disable schema display on text selection([058f062](https://github.com/getarcaneapp/arcane/commit/058f062c17329eb43f1968717eff73e715459b79) by `kmendell`)
* clear / check for default jwt secret([ae914bd](https://github.com/getarcaneapp/arcane/commit/ae914bdced852b4c5446a15c1dfbcbd5d6dd50e8) by `kmendell`)

### Dependencies

* bump date-fns from 4.2.1 to 4.3.0 ([#2745](https://github.com/getarcaneapp/arcane/pull/2745) by `dependabot`[bot])
* bump `sveltejs`/ki

---


## 1.19.5

_2026-05-26_


### Bug fixes

* improve environment proxy error handling ([#2649](https://github.com/getarcaneapp/arcane/pull/2649) by `kmendell`)
* align local BuildKit load/push exporter ([#2650](https://github.com/getarcaneapp/arcane/pull/2650) by `kmendell`)
* PUID and PGID being set on project subfolder on every startup ([#2656](https://github.com/getarcaneapp/arcane/pull/2656) by `kmendell`)
* system upgrade doesnt support non unix socket docker hosts ([#2651](https://github.com/getarcaneapp/arcane/pull/2651) by `kmendell`)
* resizing window discards edits in compose editors ([#2719](https://github.com/getarcaneapp/arcane/pull/2719) by `kmendell`)
* only validate project name if it has changed ([#2720](https://github.com/getarcaneapp/arcane/pull/2720) by `kmendell`)
* make Arcane reverse-proxy aware to resolve connection issues ([#2717](https://github.com/getarcaneapp/arcane/pull/2717) by `kmendell`)
* tolerate undefined typed env vars in GitOps sync ([#2721](https://github.com/getarcaneapp/arcane/pu

---


## 1.19.4

_2026-05-19_
### Bug fixes

* block unsafe compose include file reads ([#2630](https://github.com/getarcaneapp/arcane/pull/2630) by `kmendell`)
* add missing gRPC/ws tunnel commands ([#2636](https://github.com/getarcaneapp/arcane/pull/2636) by `kmendell`)
* unable to use templates due to 'not found' error ([#2634](https://github.com/getarcaneapp/arcane/pull/2634) by `kmendell`)
* retry rate limited update checks ([#2639](https://github.com/getarcaneapp/arcane/pull/2639) by `kmendell`)
* prevent projects from disappearing when projects folder is unreadable ([#2641](https://github.com/getarcaneapp/arcane/pull/2641) by `kmendell`)
* release notes not populated for manager instance ([#2643](https://github.com/getarcaneapp/arcane/pull/2643) by `kmendell`)

### Other

* publish manager and agent image tags ([#2645](https://github.com/getarcaneapp/arcane/pull/2645) by `kmendell`)
* use trivy-db mirrors from arcane-tools ([#2646](https://github.com/getarcaneapp/arcane/pull/2646) by `kmendell`)



**Full Changelog

---


## 1.16.4

_2026-03-24_
### Bug fixes

* pin and enforce trivy scanner digest([7975270](https://github.com/getarcaneapp/arcane/commit/7975270059a36e40eb6a2a7fc1d7203f90198bf4) by `kmendell` )



**Full Changelog**: https://github.com/getarcaneapp/arcane/compare/v1.16.3...v1.16.4

---


## 1.16.3

_2026-03-16_
### Bug fixes

* docker container creation on api 1.44 attach primary network then remaining networks ([#2053](https://github.com/getarcaneapp/arcane/pull/2053) by `kmendell`)
* add configurable security options for trivy scans ([#2072](https://github.com/getarcaneapp/arcane/pull/2072) by `kmendell`)
* allow configuring whether to prune trivy cache or not ([#2075](https://github.com/getarcaneapp/arcane/pull/2075) by `kmendell`)
* use configured DOCKER_HOST for trivy containers ([#2076](https://github.com/getarcaneapp/arcane/pull/2076) by `kmendell`)
* add missing arcane labels for auto updater ([#2079](https://github.com/getarcaneapp/arcane/pull/2079) by `kmendell`)
* unable to edit env when synced from git ([#2069](https://github.com/getarcaneapp/arcane/pull/2069) by `kmendell`)
* image update inspection fallback to manual vs using mobys distribution inspect ([#2080](https://github.com/getarcaneapp/arcane/pull/2080) by `kmendell`)

### Dependencies

* bump charm.land/lipgloss/v2 from 2.0.0 

---


## 1.16.2

_2026-03-14_
### Bug fixes

* forward and validate origin header in websocket tunnel ([#2003](https://github.com/getarcaneapp/arcane/pull/2003) by @kmendell)
* containers on user created networks not restarted when updated ([#2006](https://github.com/getarcaneapp/arcane/pull/2006) by @kmendell)
* avoid restoring offline environment on app init ([#2011](https://github.com/getarcaneapp/arcane/pull/2011) by @timwedde)
* incorrect volume mount in agent snippets ([#2027](https://github.com/getarcaneapp/arcane/pull/2027) by @kmendell)
* strip `TE: trailers` header to prevent false grpc requests ([#2026](https://github.com/getarcaneapp/arcane/pull/2026) by @kmendell)
* allow yaml merge syntax ([#2033](https://github.com/getarcaneapp/arcane/pull/2033) by @kmendell)
* dialogs in light mode showing too dark([8a29abc](https://github.com/getarcaneapp/arcane/commit/8a29abc4364565e286b43e98c8e49bd079f8315e) by @kmendell)
* build workspace panels using incorrect colors([e46f445](https://github.com/getarcaneapp/a

---


## 1.16.1

_2026-03-12_
### Bug fixes

* explicitly set docker api version based on daemon api version ([#1964](https://github.com/getarcaneapp/arcane/pull/1964) by @kmendell)
* dockerfile_inline builds not working from projects ([#1965](https://github.com/getarcaneapp/arcane/pull/1965) by @kmendell)
* allow rolling back migrations via ALLOW_DOWNGRADE env ([#1966](https://github.com/getarcaneapp/arcane/pull/1966) by @kmendell)
* allow remote git build contexts ([#1968](https://github.com/getarcaneapp/arcane/pull/1968) by @kmendell)
* env variables not resolving in volumes and labels  ([#1970](https://github.com/getarcaneapp/arcane/pull/1970) by @nargotik)
* last used date not being updated for environment api keys([b1f3287](https://github.com/getarcaneapp/arcane/commit/b1f3287efb985f08f4e8dc3e131591486db713b3) by @kmendell)

### Dependencies

* bump github.com/go-git/go-git/v5 from 5.16.5 to 5.17.0 in /backend ([#1917](https://github.com/getarcaneapp/arcane/pull/1917) by @dependabot[bot])
* update frontend d

---


## 1.16.0

_2026-03-07_
### New features

* add grpc support to edge agent tunnel ([#1730](https://github.com/getarcaneapp/arcane/pull/1730) by @kmendell)
* add auto-heal job to restart unhealthy containers ([#1780](https://github.com/getarcaneapp/arcane/pull/1780) by @garrett-edwards)
* editor enhancements, switch back to code mirror editor ([#1861](https://github.com/getarcaneapp/arcane/pull/1861) by @kmendell)
* updated dashboard layout with action items ([#1761](https://github.com/getarcaneapp/arcane/pull/1761) by @kmendell)
* support direct https setup via environment variables ([#1877](https://github.com/getarcaneapp/arcane/pull/1877) by @kmendell)
* selectable trivy container network ([#1896](https://github.com/getarcaneapp/arcane/pull/1896) by @kmendell)
* image build support ([#1687](https://github.com/getarcaneapp/arcane/pull/1687) by @kmendell)
* show template icons based on x-arcane labels ([#1933](https://github.com/getarcaneapp/arcane/pull/1933) by @kmendell)
* oled dark theme ([#1937](https://

---


## 1.15.3

_2026-02-24_
### Bug fixes

* use cpuset instead of cpusnano on synology devices ([#1782](https://github.com/getarcaneapp/arcane/pull/1782) by @kmendell)
* clear image update records by image ID not just repo/tag ([#1809](https://github.com/getarcaneapp/arcane/pull/1809) by @kmendell)
* clear update records by image ID and fail closed on used-image discovery errors ([#1810](https://github.com/getarcaneapp/arcane/pull/1810) by @kmendell)
* bound environment health sync concurrency and prevent overlapping runs ([#1813](https://github.com/getarcaneapp/arcane/pull/1813) by @kmendell)
* track active updates in status maps and bound error-event logging path ([#1817](https://github.com/getarcaneapp/arcane/pull/1817) by @kmendell)
* dont force pull images on project start and respect pull policy ([#1820](https://github.com/getarcaneapp/arcane/pull/1820) by @kmendell)
* registry syncing to environments not running on initially pairing ([#1822](https://github.com/getarcaneapp/arcane/pull/1822) by @kmendell)

---


## 1.15.2

_2026-02-19_
### Bug fixes

* git test connection not using default branch ([#1766](https://github.com/getarcaneapp/arcane/pull/1766) by @kmendell)
* missing settings making env settings not able to be saved ([#1775](https://github.com/getarcaneapp/arcane/pull/1775) by @kmendell)
* change notification logs to TEXT instead of VARCHAR(255) ([#1779](https://github.com/getarcaneapp/arcane/pull/1779) by @kmendell)
* allow trivy container limits to be configured ([#1778](https://github.com/getarcaneapp/arcane/pull/1778) by @kmendell)
* convert cron expressions from utc into TZ var timezone ([#1781](https://github.com/getarcaneapp/arcane/pull/1781) by @kmendell)
* image size mismatch on details page ([#1790](https://github.com/getarcaneapp/arcane/pull/1790) by @kmendell)
* use non-http context for jobs ([#1770](https://github.com/getarcaneapp/arcane/pull/1770) by @kmendell)
* silently refresh token on version mismatch instead of forcing logout ([#1791](https://github.com/getarcaneapp/arcane/pull/1791) by

---


## 1.15.0

_2026-02-14_
### New features

* sync .env files from git repositories ([#1632](https://github.com/getarcaneapp/arcane/pull/1632) by @Icehunter)
* updated table UX, additional 'all' rows option ([#1547](https://github.com/getarcaneapp/arcane/pull/1547) by @cabaucom376)
* container image vulnerability scanning ([#1657](https://github.com/getarcaneapp/arcane/pull/1657) by @kmendell)
* implement container exclusion and prune notifications ([#1635](https://github.com/getarcaneapp/arcane/pull/1635) by @spupuz)
* allow configurable LISTEN address ([#1685](https://github.com/getarcaneapp/arcane/pull/1685) by @kmendell)
* add support for Matrix notifications ([#1679](https://github.com/getarcaneapp/arcane/pull/1679) by @singularity0821)
* inline container exclusion list ([#1693](https://github.com/getarcaneapp/arcane/pull/1693) by @spupuz)
* show projects and containers used by images column ([#1715](https://github.com/getarcaneapp/arcane/pull/1715) by @kmendell)
* move port mappings to networks tab for container details ([#1723](https://github.com/getarcaneapp/arcane/pull/1723) by @kmendell)

### Bug fixes

* ssh git repos commit hash links incorrect ([#1643](https://github.com/getarcaneapp/arcane/pull/1643) by @kmendell)
* x-arcane metadata not allowing variable interpolation ([#1654](https://github.com/getarcaneapp/arcane/pull/1654) by @kmendell)
* inject agent token headers in edge tunnel proxy path ([#1680](https://github.com/getarcaneapp/arcane/pull/1680) by @dathtd119)
* abnormal cpu load climbing over time ([#1652](https://github.com/getarcaneapp/arcane/pull/1652) by @kmendell)
* adjust database connection pool settings ([#1690](https://github.com/getarcaneapp/arcane/pull/1690) by @user00265)
* scan all vulnerabilities causing lag/freezing ([#1694](https://github.com/getarcaneapp/arcane/pull/1694) by @kmendell)
* only send prune summary when resources are pruned ([#1703](https://github.com/getarcaneapp/arcane/pull/1703) by @kmendell)
* OIDC_ENABLED=false not disabling frontend switch ([#1719](https://github.com/getarcaneapp/arcane/pull/1719) by @kmendell)
* table sorting not persisting across reloads ([#1721](https://github.com/getarcaneapp/arcane/pull/1721) by @kmendell)

### Addon fixes

* fix Alpine package version conflict (libcrypto3/libssl3 vs openssl) by upgrading base packages before install
* add default value for BUILD_FROM ARG to silence Docker warning

**Full Changelog**: https://github.com/getarcaneapp/arcane/compare/v1.14.1...v1.15.0

---

## 1.14.1

_2026-02-11_
### Bug fixes

* incorrect backgrounds on lightmode ui elements([635e5d0](https://github.com/getarcaneapp/arcane/commit/635e5d0e5f98b0b7001ee2bac51dac155ac3a9dd) by @kmendell)
* align view options dropdown to right side([adac953](https://github.com/getarcaneapp/arcane/commit/adac953ec3853482f8e6ec0ad128792ff6a9e68f) by @kmendell)
* duplicated project/container logs when refreshing log viewer ([#1620](https://github.com/getarcaneapp/arcane/pull/1620) by @kmendell)
* unable to save oidc auto redirect setting([889fb65](https://github.com/getarcaneapp/arcane/commit/889fb65b79a61c3b101e5ea02bd7c089b16b4b00) by @kmendell)
* allow enabling and disabling keyboard shortcuts ([#1623](https://github.com/getarcaneapp/arcane/pull/1623) by @kmendell)
* keyboard shortcuts dont work for non qwerty layouts ([#1624](https://github.com/getarcaneapp/arcane/pull/1624) by @kmendell)
* sync timeout settings to all environments ([#1628](https://github.com/getarcaneapp/arcane/pull/1628) by @kmendell)

### Dep

---


## 1.13.2

_2026-01-20_
> [!IMPORTANT]
> Huge shoutout to @PvtSec for reporting GHSA-2jv8-39rp-cqqr, We recomend upgrading arcane to this version ASAP to fix that issue. 

### Backend - Bug fixes

* apply auth check before proxying request to environments ([#1532](https://github.com/getarcaneapp/arcane/pull/1532) by @kmendell)
* allow HTTP_PROXY and HTTPS_PROXY environment variables ([#1534](https://github.com/getarcaneapp/arcane/pull/1534) by @kmendell)
* use image pull timeout for project pull ([#1533](https://github.com/getarcaneapp/arcane/pull/1533) by @kmendell)
* update color of port badge to be more distinguishable([b0e8b54](https://github.com/getarcaneapp/arcane/commit/b0e8b54ec7c416ef089476106f23c365a74724cd) by @kmendell)

### Dependencies

* bump go version to 1.25.6([501baaf](https://github.com/getarcaneapp/arcane/commit/501baaf7708e8fc83b030650abd04919880da2e4) by @kmendell)
* bump pnpm to 10.28.1([c5ef93e](https://github.com/getarcaneapp/arcane/commit/c5ef93e54db76e44b932953af9f8303

---


## 1.13.1

_2026-01-19_
### Backend - Bug fixes

* ability to resize editor panels horizontally ([#1500](https://github.com/getarcaneapp/arcane/pull/1500) by @kmendell)
* allow oidc endpoints to be defined manually ([#1510](https://github.com/getarcaneapp/arcane/pull/1510) by @kmendell)
* remove file line from db debug logs([fbe204c](https://github.com/getarcaneapp/arcane/commit/fbe204c5ce919282a65313cfc0c889b763eebd64) by @kmendell)
* self update binary path for remote envrionments([974c675](https://github.com/getarcaneapp/arcane/commit/974c675550a0d5408f662d13fe3f8b07edb2267e) by @kmendell)
* generic webhooks do not allow ports ([#1517](https://github.com/getarcaneapp/arcane/pull/1517) by @kmendell)
* logo color not applying on refreshes([fe53985](https://github.com/getarcaneapp/arcane/commit/fe539851d621a35c1ebaa08217151e65bbaae64c) by @kmendell)

### Dependencies

* bump @sveltejs/kit from 2.49.4 to 2.49.5 in the npm_and_yarn group across 1 directory ([#1492](https://github.com/getarcaneapp/arcane/pull/1

---


## 1.12.2

_2026-01-14_
> [!IMPORTANT]
> Sorry for the double release, this release however should fix the path issues by making all projects directories absolute paths instead of relative paths.

### Backend - Bug fixes

* template editor heights being cutoff([7057deb](https://github.com/getarcaneapp/arcane/commit/7057deb42174cef218c623b1c431546c4a771396) by @kmendell)
* double label text on template buttons([6316833](https://github.com/getarcaneapp/arcane/commit/6316833c79f5b3e17c194c701ddc1446cab0b038) by @kmendell)
* use full absolute path for projects directory ([#1409](https://github.com/getarcaneapp/arcane/pull/1409) by @kmendell)
* editor cursor misalignment ([#1412](https://github.com/getarcaneapp/arcane/pull/1412) by @kmendell)



**Full Changelog**: https://github.com/getarcaneapp/arcane/compare/v1.12.1...v1.12.2

---


## 1.11.3

_2026-01-04_
### Initial Release

- Initial Home Assistant addon release
- Based on Arcane v1.11.3
- Features:
  - Container management with real-time stats
  - Docker Compose stack management
  - Resource monitoring with graphs
  - Image, volume, and network management
  - Automatic container image updates
  - Modern, mobile-friendly UI
  - Home Assistant ingress integration
  - Persistent data storage

### Arcane v1.11.3 Release Notes

For full upstream release notes, see: https://github.com/getarcaneapp/arcane/releases/tag/v1.11.3

---
