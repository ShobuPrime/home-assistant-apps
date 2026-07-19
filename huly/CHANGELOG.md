# Changelog

## 0.7.426

_2026-07-05_

Updated to Huly version 0.7.426

> _Maintenance (2026-07-19):_ **Fix the stack still failing to start on HAOS 18.1 — AppArmor profile flattened.** The 2026-07-18 socket-path fix below was necessary but not sufficient: HAOS 18.1's kernel (6.18) denies AF_UNIX socket connects from processes confined by **nested child profiles** regardless of the rules they contain. Verified empirically on-device — the identical ruleset connects fine as a flat profile and is denied as a child profile, with both AppArmor parser 3.1.7 (HAOS) and 4.1.7. The `docker` child profile (and its `cx ->` transitions) is therefore folded into the main profile, matching the flat Portainer/Arcane/Dockhand profiles, which were unaffected. Rebuild/reinstall the add-on to pick up the corrected profile.

> _Maintenance (2026-07-18):_ **Fix the entire stack failing to start on HAOS 18.1+** (`permission denied while trying to connect to the docker API at unix:///var/run/docker.sock`, crash-looping every second with the watchdog restarting the add-on every 3 minutes). The AppArmor child profile that confines `docker-compose` only allowed the socket at `/var/run/docker.sock` — but the Supervisor mounts it at `/run/docker.sock` (`/var/run` is a symlink to `/run`), and AppArmor matches the **resolved** path, so the rule never applied. HAOS 18.1 (kernel 6.18) began enforcing this. The rule is now `/{,var/}run/docker.sock rw,`, matching the repo's Portainer/Arcane/Dockhand profiles, which were unaffected. Rebuild/reinstall the add-on to pick up the corrected profile.

---


> _Maintenance (2026-06-10):_ hassio-addons base 20.2.0 compatibility — migrated the Traefik helper scripts from the deprecated bashio::addon.* functions to bashio::app.*.

## 0.7.423

_2026-05-10_

### App build fix (2026-06-10)

* Fix the image build failing at the Alpine package step on base `20.2.0`. The base pins `libcrypto3`/`libssl3` in apk `world` at an exact older revision; once the repo moved to `openssl 3.5.7-r0` (which requires `libcrypto3`/`libssl3=3.5.7-r0`), `apk add openssl` could not resolve against the held libs. Now `apk add --no-cache --upgrade openssl libcrypto3 libssl3 ...` rewrites those world entries and upgrades the whole TLS stack in one transaction — no version pinning.

Update to v0.7.423 (#299)

Signed-off-by: Artem Savchenko <armisav@gmail.com>
Add front url for print (#298)

Signed-off-by: Artem Savchenko <armisav@gmail.com>
Disable auto-translate and mailboxes (#294)

* Disable auto-translate and mailboxes

Signed-off-by: Artem Savchenko <armisav@gmail.com>

* Clean up

Signed-off-by: Artem Savchenko <armisav@gmail.com>

---------

Signed-off-by: Artem Savchenko <armisav@gmail.com>
Describe how to skip workspace initial content (#288)

---


## 0.7.382

_2026-03-07_
refactor: simplify - docker compose autogenerates container names (#275)

also, this makes it easier to identify which stack a container belongs to in docker ps -a when you run multiple docker compose stacks on the same host
Add troubleshooting section (#274)

* Add troubleshooting section

Signed-off-by: Artem Savchenko <armisav@gmail.com>

* Clean up

Signed-off-by: Artem Savchenko <armisav@gmail.com>

---------

Signed-off-by: Artem Savchenko <armisav@gmail.com>
Update to version 382 (#276)

* Update to version 382

---

## 0.7.375

_2026-03-01_
### Initial Release
- Initial Home Assistant addon for Huly self-hosted
- Full Huly platform stack with all 14 services
- Ingress integration for Home Assistant sidebar access
- WebSocket support for real-time updates
- Automatic secret generation on first run
- Persistent data storage included in Home Assistant backups
- Configurable instance title, language, and display preferences
- Docker Compose orchestration of the complete Huly stack

---

For full release notes, see: https://github.com/hcengineering/huly-selfhost
