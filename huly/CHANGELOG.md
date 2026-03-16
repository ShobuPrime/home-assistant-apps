# Changelog

## Version 0.7.382 (2026-03-07)

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

## Version 0.7.375 (2026-03-01)

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
