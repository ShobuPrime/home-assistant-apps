# AegisHA Documentation

## Overview

AegisHA is a Home Assistant app that implements a home alarm panel and exposes
it natively to Home Assistant. It owns an Alarmo-inspired alarm state machine and
publishes a real `alarm_control_panel` entity (plus a set of companion entities)
via MQTT discovery. It integrates a UniFi Protect gateway for sensors and sirens,
reads and mirrors Protect's arm state, and — where the Protect Alarm Manager
permits — mirrors arm/disarm to the NVR directly.

## You do NOT add AegisHA as an integration

AegisHA is **not** an integration you add under Settings → Devices & Services,
and it is **not** an MQTT broker. It is an MQTT *client* that publishes its
entities through **[MQTT discovery][mqtt-discovery]**. For its entities and
companion card to appear you need the standard Home Assistant MQTT setup:

1. An MQTT **broker** — for example the official
   **[Mosquitto broker app][mosquitto]**.
2. The **[MQTT integration][mqtt-integration]** configured (Settings → Devices &
   Services). Home Assistant [auto-discovers the broker][mqtt-broker-setup] right
   after you install Mosquitto and offers to set the integration up with one
   click.

AegisHA finds the broker automatically through the Supervisor; you never enter
broker details into AegisHA. If neither is present, AegisHA still runs and its
sidebar keypad works, but the `alarm_control_panel` entity, the companion-card
data, and any zone entities cannot be published — the app log will say
`no broker available — native alarm entity disabled`.

### Does Home Assistant auto-discover AegisHA?

**The entities — yes, automatically.** With the MQTT integration configured and
AegisHA running, its alarm panel and companion entities appear on their own as a
device named **AegisHA** under Settings → Devices & Services → **MQTT**. That is
[MQTT discovery][mqtt-discovery]: AegisHA publishes retained config to
`homeassistant/<component>/aegis_ha/<object>/config`, and Home Assistant ingests
it — no manual YAML, nothing to add. The companion card auto-registers as a
dashboard resource.

**The app itself — no.** Home Assistant never auto-installs apps (a
deliberate security boundary), so you install AegisHA from the store once. After
that, everything it exposes is auto-discovered. The end-to-end "seamless" flow is:
install [Mosquitto][mosquitto] → click *Configure* on the MQTT integration HA
offers → install & start AegisHA → the AegisHA device shows up by itself.

## How the native entity works

AegisHA publishes a retained discovery config for `alarm_control_panel.aegis_ha`.
The panel uses Home Assistant's `REMOTE_CODE` sentinel, which means **the PIN you
type on the keypad is forwarded to AegisHA** for validation against the single
shared code — rather than being checked locally by Home Assistant. If no code is
configured, you can arm/disarm with no PIN at all.

## Identity and the alarm code

There is **no per-user PIN list and no admin console**. Identity is simply your
logged-in Home Assistant user:

- When you open AegisHA from the **HA sidebar** (ingress), the Supervisor injects
  a trusted, non-spoofable `X-Remote-User-Id` — that is who performed the action
  (shown in `changed_by`).
- A single optional **`code`** can be required as an extra gate. It is the only
  credential. Leave it blank to operate with no code.

This is configured with three options:

- `code` — the optional shared PIN (numeric). Blank = no code. This is the only
  place a code is set.
- `require_code_to_arm` — require the code to arm (default **off**; arming is
  harmless).
- `require_code_to_disarm` — require the code to disarm (default **on**).

The code is hashed at rest (PBKDF2-SHA256 with a per-code salt) and protected by
a brute-force lockout (`lockout_threshold` / `lockout_duration`). The lockout
state persists across restarts; clear it from `button.aegis_ha_clear_lockout`.

## Configuration

Every option also carries a one-line description directly in the app's
Configuration tab. This is the full reference.

### `log_level`

Log verbosity: `trace`, `debug`, `info` (default), `notice`, `warning`, `error`,
`fatal`.

### UniFi Protect

#### `unifi_host`

IP or hostname of your UniFi gateway running Protect (e.g. `192.168.1.1`). Leave
blank to run AegisHA as a standalone alarm with no UniFi connection.

#### `unifi_api_key`

A UniFi OS API key (masked `password`). Create it in UniFi OS under **Settings →
Admins → your user → Create API Key**. Sent as the `X-API-KEY` header; never
logged.

#### `unifi_verify_ssl`

Verify the gateway's TLS certificate. Default `false` (gateways use a self-signed
certificate). The insecure transport is scoped to the UniFi host only and is
never shared with the Supervisor, MQTT, or Home Assistant clients.

#### `protect_mode`

- `auto` (default): detect whether the Alarm Manager is Local or Global, falling
  back to app-managed automatically.
- `local`: mirror arm/disarm to the NVR's Alarm Manager (requires Local mode).
- `app-managed`: AegisHA is the sole source of truth; it ignores Protect's arm
  state entirely (no read-sync) and uses Protect only for sensors/actuators.

`sensor.aegis_ha_protect_link_mode` always reports the detected capability
(`local` / `global` / `app-managed` / `unavailable`).

#### Arm sync — what works in which mode

- **AegisHA → Protect (always):** AegisHA is the source of truth. In **Local**
  mode it mirrors arm/disarm to the NVR directly; in **Global** mode it drives
  Protect through Alarm Manager **webhooks** (below). Either way, arm/disarm
  from AegisHA.
- **Protect → AegisHA (Local mode only):** in Local mode AegisHA polls the NVR's
  `armMode.status` and mirrors an arm/disarm done in the UniFi Protect app
  (shown with `changed_by: UniFi Protect`). The two directions can't loop — a
  mirror never re-fires a webhook.

> **Global-mode limitation (verified against UCG Fiber firmware 7.1.77).** The
> UniFi Protect *Integration* API (the one a scoped API key can reach) does
> **not** expose the global arm state for reading. Every avenue was checked
> live, while the system was armed in the Protect app:
>
> | Source | Result while armed |
> |---|---|
> | `GET /integration/v1/nvrs` → `armMode.status` | `"disabled"` (does not reflect the arm) |
> | `GET /integration/v1/alarm-manager` | `404` (no such endpoint) |
> | `GET /integration/v1/alarm-hubs` | `[]` (no hub hardware) |
> | `GET /integration/v1/events` | `404` |
> | WS `/integration/v1/subscribe/devices` + `/subscribe/events` | only motion/smartDetect — nothing on arm/disarm |
> | `GET /proxy/protect/api/{events,nvr,bootstrap}` (internal API) | `401` — rejects the API key |
>
> The arm state **does** exist server-side (the WebUI's activity log shows it),
> but only Protect's **internal** API (`/proxy/protect/api/...`) carries it, and
> that requires the WebUI's *session login* (UniFi username/password → cookie +
> CSRF), not the scoped API key. Reading it would mean storing full account
> credentials and depending on an undocumented API — a deliberate non-goal.
>
> **Decision:** in Global mode AegisHA is the **source of truth**. Arm/disarm
> from AegisHA (it drives Protect via the webhooks); an arm done in the Protect
> app will not reflect back. For true two-way sync, switch the Alarm Manager to
> **Local** mode (then `armMode.status` is live). This is a UniFi limitation, not
> an AegisHA bug — don't re-litigate it without new firmware/hardware.

#### Driving the Protect alarm with webhooks (and what the "trigger" one is for)

In **Global** mode the Integration API blocks arm-profile operations, so AegisHA
cannot write a native arm profile. To still drive Protect's alarm hardware
without leaving Global mode, use **Alarm Manager webhooks**, which the API
permits in any mode. In the Protect app, create an Alarm with a **webhook**
trigger + the action you want (siren, lights, notification), copy its **Trigger
ID**, and paste it into the matching option. AegisHA then POSTs
`/v1/alarm-manager/webhook/<id>` on the corresponding transition:

- **`unifi_webhook_trigger`** — fired the moment the alarm is **tripped**: a
  sensor is breached while armed, or you press panic. This is the one that
  *sounds the siren / flashes lights* on an actual alarm. If you only set up one
  webhook, set up this one. Leave blank if you don't want Protect to react to a
  breach.
- **`unifi_webhook_arm`** / **`unifi_webhook_disarm`** — fired when AegisHA arms /
  disarms (e.g. a confirmation chirp, a notification, or to arm/disarm Protect
  itself in lockstep). These are what let one AegisHA panel toggle Protect even
  though Protect's own webhooks are one-shot actions, which is why you create
  separate ARM and DISARM alarms in Protect.

All three are optional and independent of `protect_mode`.

#### `exit_delay_source`

Who owns the exit-delay countdown, and therefore *when* the ARM webhook fires:

- `app` (default): AegisHA runs the exit-delay countdown (`exit_delay`) and fires
  the ARM webhook **when it finishes arming**. Configure your Protect ARM alarm
  with no activation delay.
- `unifi`: AegisHA fires the ARM webhook the **moment arming begins**, so your
  Protect alarm's own activation delay governs. Set `exit_delay` to match so the
  on-screen countdown lines up.

### Alarm behavior

- `arm_modes`: which modes the panel exposes — any of `away`, `home`, `night`,
  `vacation`, `custom`. **Defaults to just `away`** (a single Arm/Disarm),
  matching UniFi Protect's Alarm Manager, which only has armed/disarmed — every
  armed mode maps to the same Protect ARM. Add `home`/`night` only if you want
  AegisHA-side perimeter modes.
- `exit_delay`: leave/exit delay in seconds before an armed state commits (0–600,
  default 60; 0 arms instantly).
- `entry_delay`: entry delay in seconds before a tripped sensor sounds the alarm
  (0–600, default 30; 0 trips instantly).
- `trigger_time`: how long the alarm stays triggered, in seconds (0–3600, default
  1800; 0 = until manually disarmed).
- `disarm_after_trigger`: return to disarmed (rather than the prior armed mode)
  when the trigger time expires.
- `ignore_blocking_sensors_after_trigger`: allow re-arming even if a sensor that
  caused the alarm is still open.

### Code / lockout

- `code`, `require_code_to_arm`, `require_code_to_disarm` — see *Identity and the
  alarm code* above.
- `lockout_threshold`: wrong-code attempts before lockout (1–20, default 5).
- `lockout_duration`: lockout length in seconds (0–3600, default 300). Persisted
  as an absolute time, so it survives a restart.

### MQTT

- `mqtt_topic_prefix`: topic + entity namespace (default `aegis_ha`). Advanced;
  leave as-is unless it collides on your broker.

### Web UI / card / zones

- `enable_web_ui`: serve the ingress keypad in the HA sidebar (default `true`).
- `enable_companion_card`: write and auto-register the AegisHA Lovelace card
  (default `true`). After install, add it to a dashboard via **Edit dashboard →
  Add card → search "AegisHA"**, or it auto-registers as a resource on
  storage-mode dashboards. The card needs the MQTT broker so the alarm entity
  exists to display.
- `expose_zone_entities`: publish a `binary_sensor` + bypass `switch` per Protect
  sensor (default **`false`**). Leave it **off** if you use the official UniFi
  Protect integration — it already gives you per-sensor entities, and AegisHA
  still uses the sensor states internally for readiness + breach detection
  regardless. Turn it **on** only if you do *not* use that integration and want
  AegisHA to publish the zone entities itself. (This is independent of the
  companion card — the card shows the alarm panel; zone entities are individual
  door/window/motion sensors.)

## Companion entities

Alongside `alarm_control_panel.aegis_ha`, AegisHA publishes (over MQTT):

- `sensor.aegis_ha_changed_by` — who last changed the alarm
- `sensor.aegis_ha_open_sensors` — count of currently-open sensors
- `binary_sensor.aegis_ha_lockout_active` — code lockout engaged
- `sensor.aegis_ha_protect_link_mode` / `binary_sensor.aegis_ha_protect_connected`
- `number.aegis_ha_exit_delay` / `_entry_delay` / `_trigger_time` — live,
  settable
- `button.aegis_ha_panic` / `_skip_delay` / `_clear_lockout`

The panel also carries attributes (`arm_mode`, `open_sensors`, `bypassed_sensors`,
`ready_to_arm`, `delay_ends`, `armed_by`, …) for automations.

## Access Methods

1. **Native entity**: `alarm_control_panel.aegis_ha` on any Lovelace dashboard.
2. **Sidebar (ingress)**: the AegisHA keypad, if `enable_web_ui`.
3. **Companion card**: optional, if `enable_companion_card`.

## Data Persistence

All state — the hashed code, lockout counters, and the committed arm mode — lives
in `/data/aegis_ha` and is included in Home Assistant backups.

## Security Considerations

- The UniFi API key is a high-value, broadly-scoped secret. It is masked in the
  UI and never logged.
- The code traverses the MQTT command topic in cleartext — rely on the
  Supervisor-managed internal broker and consider MQTT-over-TLS; never bridge
  AegisHA topics off-host.
- The keypad UI trusts the `X-Remote-User-*` headers only behind ingress and
  binds to the Supervisor IP; never expose its port directly.
- **AppArmor**: a custom profile restricts the app.

## Troubleshooting

### No `alarm_control_panel.aegis_ha` entity appears

**Cause:** AegisHA publishes over MQTT discovery, and there is no broker / the
MQTT integration is not configured. (AegisHA is a client, not a broker, and is
not added as an integration.)

**Solution:** Install the **Mosquitto broker** app and configure the **MQTT
integration**, then restart AegisHA. The app log states whether it found a
broker.

### Can't arm/disarm, or "code is denied / invalid" with a blank code

**Cause (fixed in 0.2.1):** with no `code` set, the panel still showed a PIN
field, so anything entered was checked against an empty store and rejected.
(0.2.0 had also required the logged-in user to be pre-registered.)

**Solution:** Update to 0.2.1+. With `code` blank, AegisHA no longer prompts for
or checks a code — your HA login is the identity. If you DO set a `code`, enter
it on the keypad for whichever actions you turned on
(`require_code_to_arm`/`require_code_to_disarm`).

### Arming from the UniFi Protect app doesn't show on the AegisHA panel

**Cause:** your Protect Alarm Manager is in **Global** mode, where the UniFi API
does not expose the arm state for reading (see the Global-mode limitation
above) — so AegisHA can't know you armed in the Protect app. (Also check
`protect_mode` isn't `app-managed`, and that `unifi_host`/`unifi_api_key` are
set.)

**Solution:** Arm/disarm from **AegisHA** (it drives Protect via the webhooks),
not from the Protect app. For true two-way sync, switch the Protect Alarm
Manager to **Local** mode — then AegisHA mirrors Protect's `armMode.status`
within one poll interval.

### UniFi arm/disarm from AegisHA does nothing

**Cause:** The Protect Alarm Manager is in Global mode, which blocks local
control. This is expected.

**Solution:** Use the **webhook** options to drive Protect's alarm from AegisHA
while staying in Global mode, or switch the Alarm Manager to Local mode in
Protect to mirror arm/disarm directly.

## External Resources

- [Mosquitto broker app][mosquitto] — the MQTT broker AegisHA publishes through
- [MQTT integration][mqtt-integration] / [MQTT discovery][mqtt-discovery] — how
  AegisHA's entities are created automatically
- [MQTT alarm_control_panel][mqtt-acp] — the entity type AegisHA exposes (and the
  `REMOTE_CODE` behavior it relies on)
- [UniFi Protect integration][unifiprotect] — the official integration AegisHA
  complements (and avoids duplicating)
- [Alarmo][alarmo] — the alarm model that inspired AegisHA
- [Home Assistant apps][ha-apps] — how apps are installed

[mosquitto]: https://github.com/home-assistant/addons/tree/master/mosquitto
[mqtt-integration]: https://www.home-assistant.io/integrations/mqtt/
[mqtt-discovery]: https://www.home-assistant.io/integrations/mqtt/#mqtt-discovery
[mqtt-broker-setup]: https://www.home-assistant.io/integrations/mqtt/#setting-up-a-broker
[mqtt-acp]: https://www.home-assistant.io/integrations/alarm_control_panel.mqtt/
[unifiprotect]: https://www.home-assistant.io/integrations/unifiprotect/
[alarmo]: https://github.com/nielsfaber/alarmo
[ha-apps]: https://www.home-assistant.io/apps/
