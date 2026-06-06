# AegisHA Add-on for Home Assistant

![Supports aarch64 Architecture][aarch64-shield]
![Supports amd64 Architecture][amd64-shield]

AegisHA is a dynamic alarm system for Home Assistant, inspired by [Alarmo][alarmo],
that fronts a UniFi Protect gateway (UCG Fiber and similar) and exposes a real,
keypad-driven `alarm_control_panel` entity — with per-user PIN codes — to any
Home Assistant dashboard.

## Why AegisHA

Home Assistant's native UniFi Protect integration refuses to arm/disarm the NVR
locally when the Protect **Alarm Manager is in "Global" mode**
(`The alarm manager on this UniFi Protect NVR is set to Global mode and cannot be
controlled locally`), and the entity it does expose has no keypad and no PIN
support. AegisHA solves both problems:

- **AegisHA owns the alarm.** It runs its own Alarmo-faithful state machine and
  validates PINs itself, so arming/disarming works regardless of Protect's mode.
- **Protect is used for what it is good at** — door/motion/leak sensors, sirens,
  alarm-hub outputs, and cameras. When the Alarm Manager *is* in Local mode,
  AegisHA also mirrors arm/disarm to the NVR.

## Features

- **Native `alarm_control_panel.aegis_ha` entity** with a working keypad on any
  stock Lovelace dashboard — no custom card required, survives HA restarts
- **Per-Home-Assistant-user PINs**, roles/profiles, duress codes, one-time guest
  codes, and brute-force lockout (PINs are hashed at rest)
- **Bidirectional, automation-native entities**: adjust exit/entry/trigger delays
  (`number`), switch the active profile (`select`), toggle zone bypass / siren /
  chime (`switch`), fire panic / skip-delay / clear-lockout (`button`) — all
  readable *and* settable from automations, dashboards, and voice
- **Honest UniFi Protect handling** with non-destructive capability detection and
  a `sensor.aegis_ha_protect_link_mode` that tells you exactly what mode you are in
- **Optional ingress web UI** (keypad + admin console) with trusted per-user
  identity, and an **optional companion Lovelace card**
- **stdlib-first Go 1.26** implementation with a deliberately tiny dependency
  surface

## Installation

1. Add this repository to your Home Assistant add-on store
2. Install the **AegisHA** add-on
3. (Recommended) Set up the **MQTT** integration + a broker (e.g. the Mosquitto
   add-on) — this is how the native alarm entity is published
4. Configure the add-on options (see the Documentation tab), at minimum your
   keypad users
5. Start the add-on — `alarm_control_panel.aegis_ha` appears in Home Assistant

## Configuration

See the **Documentation** tab (`DOCS.md`) for every option. The essentials:

- `users` — keypad users with PINs, roles, and allowed arm modes (imported once,
  then managed in the web UI)
- `unifi_host` / `unifi_api_key` — your UniFi gateway and a UniFi OS API key
- `exit_delay` / `entry_delay` / `trigger_time` — alarm timing
- `arm_modes` — which modes the panel offers (`away`, `home`, `night`, …)

## Folder Access

- `/data` — add-on persistent data (the hashed PIN store lives here)
- `/config` — Home Assistant config dir (only used to deliver the optional card)

## Support

Questions or bugs? Open an issue on the GitHub repository.

[aarch64-shield]: https://img.shields.io/badge/aarch64-yes-green.svg
[amd64-shield]: https://img.shields.io/badge/amd64-yes-green.svg
[alarmo]: https://github.com/nielsfaber/alarmo

## Version

Currently running AegisHA 0.1.0
