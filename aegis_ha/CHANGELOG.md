# Changelog

## 0.2.0

_2026-06-09_

### Simplified configuration and identity (breaking)

- **Identity is now your Home Assistant login.** Removed the per-user PIN list
  and the admin user-management console. Replaced the whole user/role model with
  a single optional shared **`code`** (PIN) — the only place a code is set.
- **Fixed "ARM Home → denied code".** The keypad no longer requires the
  logged-in user to be pre-registered; with no code required (the default),
  any authenticated Home Assistant user can arm/disarm. A code is only enforced
  when set and the matching `require_code_to_arm` / `require_code_to_disarm`
  toggle is on.
- **Two-way arm sync with UniFi Protect.** AegisHA now reads the NVR's
  `armMode.status` (readable even in Global mode) and mirrors it: arm/disarm from
  the UniFi Protect app is reflected on the AegisHA panel (`changed_by: UniFi
  Protect`). Echo-suppressed so it can't loop with the arm/disarm webhooks.
  `protect_mode: app-managed` opts out.
- **Plain-English options with inline descriptions.** Added `translations/en.yaml`
  so every option shows a friendly name + description in the Configuration tab.
- **Removed confusing/unused options:** `users`, `admin_usernames`,
  `default_role`, `sensors`, `sensor_groups`, `unifi_site`, `pin_min_length`,
  `pin_max_length`, `mqtt_code_format`, and the three `*_requires_code` options
  (replaced by `code` + `require_code_to_arm` + `require_code_to_disarm`).
- **Clarified MQTT setup.** Documented that AegisHA is an MQTT *client* — not a
  broker, and not added as an integration — and needs the Mosquitto broker app +
  the MQTT integration for its entities and card to appear. Startup log is
  explicit when no broker is found.
- Wording: "Add-on" → "App" throughout, matching Home Assistant's terminology.

## 0.1.0

_2026-06-06_

### Initial release

- Go 1.26 Home Assistant add-on scaffold (S6 service, multi-stage Docker build, ingress on port 8099)
- Native alarm entity via MQTT discovery: a real `alarm_control_panel.aegis_ha` with a keypad on any dashboard, using the `REMOTE_CODE` sentinel so the entered PIN is forwarded to AegisHA for per-user validation
- Alarmo-inspired alarm state machine: `disarmed`, `arming` (exit delay), `pending` (entry delay), `armed_*`, `triggered`, `disarming`
- Per-user PIN store with PBKDF2-hashed PINs (stdlib `crypto/pbkdf2`), a pepper-HMAC index for O(1) lookup, roles/profiles, duress and one-time codes, and brute-force lockout
- Full Alarmo-style sensor model: per-sensor `modes`, `always_on`, `immediate`, `use_exit_delay`, `auto_bypass`, `allow_open` (arm-on-close), `trigger_unavailable`, manual bypass (per-zone switch entities), and sensor groups (event-count-within-timeout debounce) — configurable via the `sensors` / `sensor_groups` add-on options
- Bidirectional automation-native entities (MQTT discovery): numbers (delays), switches (per-zone bypass), buttons (panic/skip-delay/clear-lockout), plus sensors and binary_sensors (open/bypassed, link mode, per-zone)
- UniFi Protect integration with honest UCG Fiber "Global mode" handling, verified against real hardware: non-destructive capability detection via `GET /v1/arm-profiles`, app-managed alarm (Protect supplies sensors + sirens/triggers) with optional local-mirror arm/disarm when Protect is in Local mode, and a device-event WebSocket for low-latency breach detection
- Global-mode actuation via Protect Alarm Manager webhooks (`unifi_webhook_arm`/`_disarm`/`_trigger`): AegisHA fires `/v1/alarm-manager/webhook/<id>` on the matching transition to drive Protect's native alarm (siren/lights) while staying in Global mode (where arm profiles are API-blocked)
- `expose_zone_entities` option (default off) to avoid duplicating the official UniFi Protect integration's sensor entities; the engine still consumes the sensors internally
- Optional ingress web UI (HTMX + WebSocket) with per-Home-Assistant-user PINs via the trusted `X-Remote-User-Id` ingress header, plus an admin user-management console
- Optional companion Lovelace card, auto-registered as a Lovelace resource over the Supervisor Core-WebSocket (storage mode; manual snippet logged for YAML mode)
- HA bus events (`aegis_ha_command_success`/`_failed_to_arm`/`_triggered`/`_duress`) for automations
- stdlib-first implementation: one native-Go dependency (`golang.org/x/net/websocket`); hand-rolled MQTT 3.1.1 client and stdlib `crypto/pbkdf2` otherwise
