# Changelog

## 0.1.0

_2026-06-06_

### Initial release

- Go 1.26 Home Assistant add-on scaffold (S6 service, multi-stage Docker build, ingress on port 8099)
- Native alarm entity via MQTT discovery: a real `alarm_control_panel.aegis_ha` with a keypad on any dashboard, using the `REMOTE_CODE` sentinel so the entered PIN is forwarded to AegisHA for per-user validation
- Alarmo-inspired alarm state machine: `disarmed`, `arming` (exit delay), `pending` (entry delay), `armed_*`, `triggered`, `disarming`
- Per-user PIN store with PBKDF2-hashed PINs (stdlib `crypto/pbkdf2`), a pepper-HMAC index for O(1) lookup, roles/profiles, duress and one-time codes, and brute-force lockout
- Bidirectional automation-native entities (MQTT discovery): numbers (delays), select (arm profile), switches (bypass/siren/chime), buttons (panic/skip-delay/clear-lockout), text (code entry), plus read-only sensors and binary_sensors
- UniFi Protect integration with honest UCG Fiber "Global mode" handling: non-destructive capability detection, app-managed alarm (Protect supplies sensors + sirens/triggers/cameras) with optional local-mirror arm/disarm when Protect is in Local mode
- Optional ingress web UI (HTMX + SSE) with per-Home-Assistant-user PINs via the trusted `X-Remote-User-Id` ingress header, plus an admin user-management console
- Optional companion Lovelace card served from the add-on with best-effort auto-registration
- stdlib-first implementation: single sanctioned dependency budget, no MQTT/crypto third-party libraries
