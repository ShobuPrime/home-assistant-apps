# Changelog

## 1.5.0.2

_2026-11-20_

- **Dockge can manage stacks.** The app had no Docker client, and Dockge does everything with `docker compose`, so it could list, deploy and start nothing. The app now has `docker`, `docker compose` and `buildx`.
- **Stacks are kept.** They were in `/opt/stacks` inside the container, which Home Assistant replaces at every restart and update, so they were lost each time. They now live in this app's data folder and are part of its backups. The `stacks_dir` option is gone.
- **A stack's relative folders (`./config`) are mounted from the stack's folder.** Docker looks for them on the host, so the app gives Dockge the host's name for its data folder.
- **The sidebar panel works.** Dockge's page asked for its files and its connection from the site root, which under the sidebar is Home Assistant itself. A small adapter now serves the panel. Dockge's own port is unchanged.
- **`hide_hassio_containers` does what it says.** It had no effect. Compose projects whose containers Home Assistant manages are now left out of Dockge's list. An app marks the containers it starts for itself with `io.github.shobuprime.app-managed=true`.

Same Dockge as 1.5.0.

## 1.5.0.1

_2026-10-22_

- The image requires Alpine's fixed CA bundle (`ca-certificates` 20261020-r0), which drops the emSign roots behind the compromised eMudhra intermediates (CVE-2026-111943).
- Node trusts the system CA bundle (`--use-openssl-ca`) instead of its compiled-in roots, and moves from 22.14.0 to 22.23.4, which also builds on aarch64 again.

These fixes first shipped as maintenance notes under 1.5.0, which needed a manual rebuild. This version number makes Home Assistant offer them as an update.

## 1.5.0

> _Maintenance (2026-10-22):_ Node 22.14.0 → 22.23.4. 22.14.0 has no arm64 musl build, so the app could not build on aarch64. 22.23.4 also drops the compromised eMudhra roots from Node's own store (CVE-2026-111943).

> _Maintenance (2026-10-22):_ The image now requires `ca-certificates-bundle>=20261020-r0` and `ca-certificates>=20261020-r0`, Alpine's bundle without the root behind the compromised eMudhra intermediates (CVE-2026-111943). This also forces a stale build cache to reinstall packages on the next rebuild. Rebuild the app to apply.

> _Maintenance (2026-10-22):_ Node now trusts the system CA bundle (`NODE_OPTIONS=--use-openssl-ca`) instead of its compiled-in one, which still includes the root behind the compromised eMudhra intermediates (CVE-2026-111943). Rebuild the app to apply; the rebuild also pulls Alpine's fixed `ca-certificates` 20261020. Also unpins `musl` so the build resolves again: the base image pins `musl=1.2.6-r2`, Alpine now ships `musl-dev` r3, and `g++` failed to install against the pin.

> _Maintenance (2026-07-19):_ AppArmor profile flattened — the `node` child profile (and its `cx ->` transition) is folded into the main profile. HAOS 18.1's kernel (6.18) denies AF_UNIX socket connects from processes confined by nested child profiles regardless of the rules they contain (verified on-device with the Huly add-on; parser 3.1.7 and 4.1.7 both affected), which would deny Dockge all Docker API access. Flat matches the unaffected Portainer/Arcane/Dockhand profiles.

> _Maintenance (2026-07-18):_ AppArmor — allow the Docker socket at its resolved path (`/{,var/}run/docker.sock rw,`). The child profile only allowed `/var/run/docker.sock`, but the Supervisor mounts the socket at `/run/docker.sock` (`/var/run` is a symlink) and AppArmor matches resolved paths; HAOS 18.1+ enforces this and would deny all Docker API access (the same failure that broke the Huly add-on).

> _Maintenance (2026-06-10):_ hassio-addons base 20.2.0 compatibility — migrated the Traefik helper scripts from the deprecated bashio::addon.* functions to bashio::app.*.

### 🆕 New Features
- Docker client updated to 28.0.4
- Docker Compose updated to 2.34.0
- Console improvements

### ⬆️ Improvements
- Removed unnecessary scrollbar
- Fixed default compose version issue

### 🐛 Bug Fixes
- Preserved YAML comments when reordering items
- Various minor bug fixes

### ⚠️ Breaking Change
**Breaking change: Console feature now disabled by default for security reasons. Can be enabled via `DOCKGE_ENABLE_CONSOLE=true` environment variable.**

### 🌐 Translations
- Added Irish language
- Multiple translation updates

### 🔒 Security
- Addressed GHSA-7vx4-hf96-mqq6 security advisory

---

For full release notes, see: https://github.com/louislam/dockge/releases/tag/1.5.0
