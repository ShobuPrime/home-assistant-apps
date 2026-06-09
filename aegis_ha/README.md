# AegisHA app for Home Assistant

![Supports aarch64 Architecture][aarch64-shield]
![Supports amd64 Architecture][amd64-shield]

AegisHA is a home alarm panel for Home Assistant, inspired by [Alarmo][alarmo],
that fronts a UniFi Protect gateway (UCG Fiber and similar) and exposes a real,
keypad-driven `alarm_control_panel` entity to any Home Assistant dashboard. The
**HA** is a double entendre: **H**ome **A**larm and **H**ome **A**ssistant.

## Why AegisHA

Home Assistant's native UniFi Protect integration refuses to arm/disarm the NVR
locally when the Protect **Alarm Manager is in "Global" mode**
(`The alarm manager on this UniFi Protect NVR is set to Global mode and cannot be
controlled locally`), and the entity it does expose has no keypad. AegisHA solves
both problems:

- **AegisHA owns the alarm.** It runs its own Alarmo-faithful state machine, so
  arming/disarming works regardless of Protect's mode.
- **AegisHA is the source of truth, and drives Protect.** Arm/disarm in AegisHA
  and it drives the Protect alarm via Alarm Manager webhooks (which work even in
  Global mode), including sounding the siren/lights on a breach. **Heads-up:** in
  Protect **Global** mode the UniFi API does not expose the arm state for
  reading, so an arm done *in the UniFi Protect app* cannot be reflected back
  into AegisHA — arm from AegisHA instead. Switch the Protect Alarm Manager to
  **Local** mode if you want full two-way sync (then AegisHA also mirrors arming
  done in Protect).
- **Protect is used for what it is good at** — door/motion/leak sensors, sirens,
  alarm-hub outputs, and cameras.

## How you control it

There is no custom integration to add and **no per-user PIN list to manage**.
Identity is simply your logged-in Home Assistant user. A single optional `code`
(PIN) can be required for disarming if you want one — that is the only credential.

## Features

- **Native `alarm_control_panel.aegis_ha` entity** with a working keypad on any
  stock Lovelace dashboard — survives HA restarts
- **Optional shared PIN** with brute-force lockout (hashed at rest); leave it
  blank to arm/disarm with no code (your HA login is your identity)
- **Bidirectional, automation-native entities**: adjust exit/entry/trigger delays
  (`number`), fire panic / skip-delay / clear-lockout (`button`), read
  `changed_by` / open-sensor count / lockout (`sensor`/`binary_sensor`) — all
  readable *and* settable from automations, dashboards, and voice
- **Drives UniFi Protect** via Alarm Manager webhooks (arm/disarm/breach), with
  full two-way arm mirroring when Protect is in Local mode
- **Honest UniFi Protect handling** with non-destructive capability detection and
  a `sensor.aegis_ha_protect_link_mode` that tells you exactly what mode you are in
- **Optional ingress keypad UI** and an **optional companion Lovelace card**
- **stdlib-first Go 1.26** implementation with a deliberately tiny dependency
  surface

## Prerequisites: MQTT

AegisHA publishes its entities over **[MQTT discovery][mqtt-discovery]**. It is
an MQTT *client* — **not** a broker, and **not** something you add as an
integration. For the alarm entity and companion card to appear you need the
standard Home Assistant MQTT setup:

1. An MQTT **broker** — the official **[Mosquitto broker add-on][mosquitto]** is
   the easy choice.
2. The **[MQTT integration][mqtt-integration]** configured (Settings → Devices &
   Services). After you install Mosquitto, Home Assistant **auto-discovers** the
   broker and prompts you to set the integration up with one click — see the
   [MQTT broker setup docs][mqtt-broker-setup].

AegisHA discovers the broker's credentials automatically through the Supervisor —
you do **not** enter any MQTT settings into AegisHA. If no broker is present,
AegisHA still runs (its keypad UI works), but the `alarm_control_panel` entity and
zone entities cannot be published, and the log will say so.

### Will Home Assistant auto-discover AegisHA?

**The entities: yes — automatically.** Once the MQTT integration is configured
and AegisHA is running, its alarm panel and every companion entity appear on
their own as a device named **AegisHA** under Settings → Devices & Services →
**MQTT**. This is [MQTT discovery][mqtt-discovery] doing its job — there is no
manual YAML and **nothing to "add as an integration."** The companion card also
auto-registers as a dashboard resource.

**The app itself: no — you install it once.** Home Assistant never auto-installs
apps/add-ons (by design), so you add AegisHA from the store yourself. After that
single install, everything it exposes is auto-discovered. In short: install
Mosquitto → click *Configure* on the MQTT integration HA offers → install &
start AegisHA → the AegisHA device shows up by itself.

## Installation

1. Add this repository to your Home Assistant app store
2. Install the **AegisHA** app
3. Set up the **MQTT integration** + a broker (e.g. the Mosquitto broker app) —
   see *Prerequisites* above
4. Configure the options (see the Documentation tab). Each option has an inline
   description; nothing is required to start
5. Start the app — `alarm_control_panel.aegis_ha` appears in Home Assistant

## Configuration

Every option carries an inline description in the UI. The essentials:

- `code` — optional shared PIN (leave blank for no code)
- `unifi_host` / `unifi_api_key` — your UniFi gateway and a UniFi OS API key
- `exit_delay` / `entry_delay` / `trigger_time` — alarm timing
- `arm_modes` — which modes the panel offers (`away`, `home`, `night`, …)

See the **Documentation** tab (`DOCS.md`) for the full reference.

## Folder Access

- `/data` — app persistent data (the hashed code + lockout state live here)
- `/config` — Home Assistant config dir (only used to deliver the optional card)

## References & further reading

- **[Mosquitto broker add-on][mosquitto]** — the MQTT broker AegisHA publishes
  through
- **[MQTT integration][mqtt-integration]** & **[MQTT discovery][mqtt-discovery]**
  — how AegisHA's entities are created automatically
- **[MQTT alarm_control_panel][mqtt-acp]** — the entity type AegisHA exposes,
  including the `REMOTE_CODE` behavior it uses
- **[UniFi Protect integration][unifiprotect]** — the official integration whose
  per-sensor entities AegisHA deliberately avoids duplicating
- **[Alarmo][alarmo]** — the project whose alarm model inspired AegisHA
- **[Home Assistant apps][ha-apps]** — how apps are installed and why they are
  never auto-installed

## Support

Questions or bugs? Open an issue on the GitHub repository.

[aarch64-shield]: https://img.shields.io/badge/aarch64-yes-green.svg
[amd64-shield]: https://img.shields.io/badge/amd64-yes-green.svg
[alarmo]: https://github.com/nielsfaber/alarmo
[mosquitto]: https://github.com/home-assistant/addons/tree/master/mosquitto
[mqtt-integration]: https://www.home-assistant.io/integrations/mqtt/
[mqtt-discovery]: https://www.home-assistant.io/integrations/mqtt/#mqtt-discovery
[mqtt-broker-setup]: https://www.home-assistant.io/integrations/mqtt/#setting-up-a-broker
[mqtt-acp]: https://www.home-assistant.io/integrations/alarm_control_panel.mqtt/
[unifiprotect]: https://www.home-assistant.io/integrations/unifiprotect/
[ha-apps]: https://www.home-assistant.io/apps/

## Version

Currently running AegisHA 0.2.2
