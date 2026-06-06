# AegisHA Documentation

## Overview

AegisHA is a Home Assistant add-on that implements a dynamic alarm system and
exposes it natively to Home Assistant. It owns an Alarmo-inspired alarm state
machine, validates per-user PIN codes itself, and publishes a real
`alarm_control_panel` entity (plus a set of companion entities) via MQTT
discovery. It integrates a UniFi Protect gateway for sensors and sirens, and —
where the Protect Alarm Manager permits — mirrors arm/disarm to the NVR.

## How the native entity works

AegisHA auto-detects the Supervisor-managed MQTT broker (the add-on requests the
`mqtt` service) and publishes a retained MQTT discovery config for
`alarm_control_panel.aegis_ha`. The panel is configured with Home Assistant's
`REMOTE_CODE` sentinel, which means **the PIN you type on the keypad is
forwarded to AegisHA** for validation against the per-user PIN store — rather than
being checked locally by Home Assistant against a single static code. This is
what makes true per-user PINs possible on a stock dashboard card.

> Requires the Home Assistant **MQTT integration** to be configured (not just a
> broker running). Without it, discovery messages are published but no entity is
> created.

## Configuration

### Option: `log_level`

Controls add-on log verbosity: `trace`, `debug`, `info` (default), `notice`,
`warning`, `error`, `fatal`.

### UniFi Protect

#### Option: `unifi_host`

Hostname or IP of your UniFi gateway (e.g. the UCG Fiber). Local, HTTPS.

#### Option: `unifi_api_key`

A UniFi OS API key (stored as a masked `password`). Create it in the UniFi OS UI
under **Settings → Control Plane → Integrations / Admins → API Keys**. The key is
sent as the `X-API-KEY` header. It is never logged.

#### Option: `unifi_verify_ssl`

Verify the gateway's TLS certificate. Default `false`, because UniFi gateways use
a self-signed certificate. The insecure transport is scoped to the UniFi host
only and is never shared with the Supervisor, MQTT, or Home Assistant clients.

#### Option: `unifi_site`

UniFi site id. Default `default`.

#### Option: `protect_mode`

- `auto` (default): detect whether the Alarm Manager is Local or Global and fall
  back to app-managed automatically
- `local`: mirror arm/disarm to the NVR's Alarm Manager (requires Local mode)
- `app-managed`: AegisHA owns arm state entirely; Protect provides sensors and
  actuators only (works even in Global mode)

`sensor.aegis_ha_protect_link_mode` always reports the detected capability
(`local` / `global` / `app-managed` / `unavailable`).

### Alarm behavior

- `arm_modes`: which modes the panel exposes — any of `away`, `home`, `night`,
  `vacation`, `custom` (default `away`, `home`, `night`)
- `exit_delay`: leave/exit delay in seconds before an armed state commits (0–600,
  default 60)
- `entry_delay`: entry delay in seconds before a tripped sensor triggers the
  alarm (0–600, default 30)
- `trigger_time`: how long the alarm stays triggered, in seconds (0–3600, default
  1800; 0 = indefinite)
- `arming_requires_code` / `disarm_requires_code` / `trigger_requires_code`:
  whether a PIN is required for each action (defaults: arm no, disarm yes,
  trigger no)
- `disarm_after_trigger`: disarm automatically when the trigger time expires
- `ignore_blocking_sensors_after_trigger`: re-arm even if sensors are still open

### MQTT

- `mqtt_topic_prefix`: topic + entity namespace (default `aegis_ha`)
- `mqtt_code_format`: `number` (numeric keypad, default) or `text` (alphanumeric)

### PIN / lockout policy

- `lockout_threshold`: failed attempts before lockout (1–20, default 5)
- `lockout_duration`: lockout length in seconds (0–3600, default 300). Persisted
  as an absolute time, so it survives an add-on restart
- `pin_min_length` / `pin_max_length`: PIN length bounds (default 4–8)
- `default_role`: role assigned to new users (`admin`, `user`, `guest`)

### Web UI / card / admin

- `enable_web_ui`: serve the ingress keypad/admin UI (default `true`)
- `enable_companion_card`: write and auto-register the AegisHA Lovelace card
  (default `false`)
- `admin_usernames`: Home Assistant usernames treated as AegisHA admins (least-
  privilege alternative to elevating the add-on's Supervisor role)
- `users`: bootstrap keypad users — imported **once** into the hashed store on
  first boot, after which you should clear the plaintext PINs from options and
  manage users in the web UI. Each entry:
  - `name` (required)
  - `ha_user_id` (optional — the Home Assistant user UUID to bind this PIN to)
  - `pin` (required, masked)
  - `role` (`admin` / `user` / `guest`)
  - `allowed_arm_modes` (subset of `arm_modes`)

## Per-user PINs

AegisHA validates every PIN itself; PINs are hashed at rest (PBKDF2-SHA256 with a
per-PIN salt) and indexed by a server-side pepper-HMAC so a keypad entry resolves
to a single user in constant work. Two entry paths are supported:

- **MQTT keypad** (any dashboard): carries no identity, so AegisHA resolves the
  acting user by matching the PIN. PINs must therefore be globally unique.
- **Ingress web keypad**: the Supervisor injects a trusted `X-Remote-User-Id`
  header, so AegisHA binds the PIN to the logged-in Home Assistant user.

Duress PINs silently disarm and fire a duress event. One-time/guest codes expire
after use. After `lockout_threshold` failures, further attempts are rejected for
`lockout_duration`; an admin can clear a lockout from the web UI or the
`button.aegis_ha_clear_lockout` entity.

## Access Methods

1. **Native entity**: `alarm_control_panel.aegis_ha` on any Lovelace dashboard
2. **Sidebar (ingress)**: the AegisHA panel (keypad + admin), if `enable_web_ui`
3. **Companion card**: optional, if `enable_companion_card`

## Data Persistence

All state — the hashed PIN store and the alarm configuration — lives in
`/data/aegis_ha` and is included in Home Assistant backups.

## Security Considerations

- The UniFi API key is a high-value, broadly-scoped secret. It is masked in the
  UI and never logged.
- PINs traverse the MQTT command topic in cleartext — rely on the Supervisor-
  managed internal broker and consider MQTT-over-TLS; never bridge AegisHA topics
  off-host.
- The web UI trusts the `X-Remote-User-*` headers only behind ingress and binds
  to the Supervisor IP; never expose its port directly.
- **AppArmor**: a custom profile restricts the add-on.

## Troubleshooting

### No `alarm_control_panel.aegis_ha` entity appears

**Cause:** The MQTT integration is not configured in Home Assistant, or no broker
is available.

**Solution:** Install/configure the MQTT integration and a broker (e.g. the
Mosquitto add-on), then restart AegisHA. The add-on log states whether it found a
broker.

### UniFi arm/disarm does nothing

**Cause:** The Protect Alarm Manager is in Global mode, which blocks local
control. This is expected and not a AegisHA bug.

**Solution:** AegisHA runs app-managed in this case (it still owns the alarm). To
mirror arm/disarm to the NVR, switch the Alarm Manager to Local mode in Protect.

## External Resources

- [Alarmo](https://github.com/nielsfaber/alarmo)
- [Home Assistant MQTT alarm_control_panel](https://www.home-assistant.io/integrations/alarm_control_panel.mqtt/)
