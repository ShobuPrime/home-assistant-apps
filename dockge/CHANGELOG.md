# Changelog

> _Maintenance (2026-07-19):_ AppArmor profile flattened — the `node` child profile (and its `cx ->` transition) is folded into the main profile. HAOS 18.1's kernel (6.18) denies AF_UNIX socket connects from processes confined by nested child profiles regardless of the rules they contain (verified on-device with the Huly add-on; parser 3.1.7 and 4.1.7 both affected), which would deny Dockge all Docker API access. Flat matches the unaffected Portainer/Arcane/Dockhand profiles.

> _Maintenance (2026-07-18):_ AppArmor — allow the Docker socket at its resolved path (`/{,var/}run/docker.sock rw,`). The child profile only allowed `/var/run/docker.sock`, but the Supervisor mounts the socket at `/run/docker.sock` (`/var/run` is a symlink) and AppArmor matches resolved paths; HAOS 18.1+ enforces this and would deny all Docker API access (the same failure that broke the Huly add-on).

> _Maintenance (2026-06-10):_ hassio-addons base 20.2.0 compatibility — migrated the Traefik helper scripts from the deprecated bashio::addon.* functions to bashio::app.*.

## 1.5.0
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
