# Changelog

## Version 1.18.0 (2026-04-23)


### New features

* full control over prune options ([#2372](https://github.com/getarcaneapp/arcane/pull/2372) by `kmendell`)
* add UI to create and edit custom templates ([#2351](https://github.com/getarcaneapp/arcane/pull/2351) by `mohamedhagag`)
* add raw inspect tab to container detail view ([#2368](https://github.com/getarcaneapp/arcane/pull/2368) by `GiulioSavini`)
* universal environment dashboard ([#2241](https://github.com/getarcaneapp/arcane/pull/2241) by `kmendell`)
* add dedicated healthcheck tab for containers ([#2384](https://github.com/getarcaneapp/arcane/pull/2384) by `kmendell`)
* resource updates overview page ([#2204](https://github.com/getarcaneapp/arcane/pull/2204) by `kmendell`)
* add ability to deploy Docker Swarm stacks from Git repo with GitOps updates  ([#2412](https://github.com/getarcaneapp/arcane/pull/2412) by `SplinterHead`)

### Bug fixes

* handle deferred file close errors in docker build copy helper([3cdc1dd](https://github.com/getarcaneapp/arcane/commit/3cd

---


## Version 1.16.4 (2026-03-24)

### Bug fixes

* pin and enforce trivy scanner digest([7975270](https://github.com/getarcaneapp/arcane/commit/7975270059a36e40eb6a2a7fc1d7203f90198bf4) by `kmendell` )



**Full Changelog**: https://github.com/getarcaneapp/arcane/compare/v1.16.3...v1.16.4

---


## Version 1.16.3 (2026-03-16)


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


## Version 1.16.2 (2026-03-14)


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


## Version 1.16.1 (2026-03-12)


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


## Version 1.16.0 (2026-03-07)


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


## Version 1.15.3 (2026-02-24)


### Bug fixes

* use cpuset instead of cpusnano on synology devices ([#1782](https://github.com/getarcaneapp/arcane/pull/1782) by @kmendell)
* clear image update records by image ID not just repo/tag ([#1809](https://github.com/getarcaneapp/arcane/pull/1809) by @kmendell)
* clear update records by image ID and fail closed on used-image discovery errors ([#1810](https://github.com/getarcaneapp/arcane/pull/1810) by @kmendell)
* bound environment health sync concurrency and prevent overlapping runs ([#1813](https://github.com/getarcaneapp/arcane/pull/1813) by @kmendell)
* track active updates in status maps and bound error-event logging path ([#1817](https://github.com/getarcaneapp/arcane/pull/1817) by @kmendell)
* dont force pull images on project start and respect pull policy ([#1820](https://github.com/getarcaneapp/arcane/pull/1820) by @kmendell)
* registry syncing to environments not running on initially pairing ([#1822](https://github.com/getarcaneapp/arcane/pull/1822) by @kmendell)

---


## Version 1.15.2 (2026-02-19)


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


## Version 1.15.0 (2026-02-14)

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

## Version 1.14.1 (2026-02-11)


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


## Version 1.13.2 (2026-01-20)

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


## Version 1.13.1 (2026-01-19)


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


## Version 1.12.2 (2026-01-14)

> [!IMPORTANT]
> Sorry for the double release, this release however should fix the path issues by making all projects directories absolute paths instead of relative paths.

### Backend - Bug fixes

* template editor heights being cutoff([7057deb](https://github.com/getarcaneapp/arcane/commit/7057deb42174cef218c623b1c431546c4a771396) by @kmendell)
* double label text on template buttons([6316833](https://github.com/getarcaneapp/arcane/commit/6316833c79f5b3e17c194c701ddc1446cab0b038) by @kmendell)
* use full absolute path for projects directory ([#1409](https://github.com/getarcaneapp/arcane/pull/1409) by @kmendell)
* editor cursor misalignment ([#1412](https://github.com/getarcaneapp/arcane/pull/1412) by @kmendell)



**Full Changelog**: https://github.com/getarcaneapp/arcane/compare/v1.12.1...v1.12.2

---


## Version 1.11.3 (2026-01-04)

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
