# Changelog

## 0.7.426

_2026-07-05_

Updated to Huly version 0.7.426

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
