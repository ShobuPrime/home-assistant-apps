# Changelog

## 0.2.4

_2026-06-10_

### Companion card lifecycle

- **Reconcile the card on every start.** AegisHA now treats the card
  declaratively: when `enable_companion_card` is on it deploys + registers (and
  updates the resource URL if the version changed); when it's off it **removes**
  the deployed `aegis_ha-card.js` and **unregisters** the Lovelace resource. So
  disabling the option no longer leaves a stale file and a dangling resource
  pointing at it.
- This runs on install and every boot. Changing the option restarts the add-on,
  so the card is cleaned up (or re-deployed) on that restart automatically. Both
  the file removal and the resource unregister are idempotent — safe no-ops when
  there's nothing to clean.

## 0.2.3

_2026-06-10_

### Fix: companion card 404

- **The Lovelace card now actually loads.** It was written to `/config/www`
  inside the add-on's own container, but the `homeassistant_config:rw` map mounts
  Home Assistant's config at **`/homeassistant`** — so `/local/aegis_ha/aegis_ha-card.js`
  (which serves HA's `/config/www`) had no file and 404'd. The card is now written
  to `/homeassistant/www/aegis_ha/`, the correct location for the `/local/` URL.
- Deploy now verifies the file exists at the HA www path before logging "deployed"
  and registering the resource, so a bad mapping can't silently advertise a 404
  URL. (Combined with the 0.2.2 cache-buster fix, an upgrade now both writes the
  card to the right place and re-points the resource so the browser re-fetches it.)
- Recorded the UniFi Global-mode arm-state limitation (the full evidence + the
  decision to keep AegisHA as the source of truth) in DOCS so it isn't
  re-investigated.

## 0.2.2

_2026-06-09_

### UniFi arm-sync reality, card naming, and the card cache-buster

- **Global-mode arm read-sync honestly scoped.** Verified live against UCG Fiber
  firmware 7.1.77 that the UniFi Integration API does not expose the global arm
  state — `GET /v1/nvrs` returns `armMode.status: "disabled"` even while armed in
  the Protect app, and there is no alarm-manager status endpoint. AegisHA now
  only polls the arm state in **Local** mode (where it is meaningful), logs the
  limitation once in Global mode, and stops wasting API calls polling it in
  Global (also easing the rate limit). In Global mode AegisHA is the source of
  truth and drives Protect via webhooks; arm from AegisHA, not the Protect app.
- **Fix: companion card / entity showed "AegisHA AegisHA".** Entity names are now
  the role only ("Alarm Manager", "Last Changed By", "Panic", …) so Home
  Assistant composes them with the device name as "AegisHA Alarm Manager", etc.
- **Fix: card showed all arm modes (Arm Home/Away/…).** HA's MQTT alarm panel
  always reports every arm mode in `supported_features`; the card now renders the
  configured modes from a new `arm_modes` panel attribute instead, and labels a
  single mode simply "Arm". The card also shows a title and the trigger cause.
- **Fix: card updates didn't reach the browser.** The Lovelace resource
  registration kept the original `?v=` cache-buster forever; it now updates the
  resource URL on a version change so browsers re-fetch the new card.

## 0.2.1

_2026-06-09_

### Fixes & UX

- **Fix: blank code made the alarm un-disarmable.** With no `code` set, the
  panel still advertised a PIN field, so anything entered was checked against an
  empty store and rejected ("denied code"). Now, when no code is configured,
  AegisHA never prompts for one and never denies on code grounds — your Home
  Assistant login is the identity. The panel only advertises a PIN field when a
  code is actually set.
- **Fix: native UniFi disarm now clears a triggered alarm.** The Protect→AegisHA
  arm read-sync previously left a `triggered` state alone; disarming in the UniFi
  Protect app now mirrors through and clears the AegisHA alarm from any state.
- **Fix: UniFi HTTP 429 rate-limiting.** Event-driven polls are now coalesced
  (max one per few seconds) and capability detection runs far less often, so
  AegisHA stays within UniFi's ~10 req/s limit. A transient 429 no longer flaps
  the detected mode (which had been silently disabling arm-sync).
- **Surface the trigger cause.** The breaching sensor is logged on every trigger
  and shown on the keypad ("Triggered by …"), along with open/bypassed sensors.
- **Polished web keypad.** Redesigned the ingress UI: clearer state card with a
  live countdown, trigger cause, and open-sensor list; the PIN pad is hidden when
  no code is required.
- **Simpler arming.** The default `arm_modes` is now a single `away` (Arm/Disarm),
  matching UniFi Protect's Alarm Manager, which only has armed/disarmed. Add
  `home`/`night` back if you want AegisHA-side perimeter modes. (Existing installs
  keep their saved value — set `arm_modes` to `[away]` for the new single-button
  panel.)

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
