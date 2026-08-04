# Changelog

## 1.3.0

_2026-08-02_

### Changes

**Entity IDs are now short natively — `sensor.hay_cm5_processor_temperature`, not `sensor.hay_cm5_fan_controller_cm5_processor_temperature`.** 1.2.0 renamed the device and entities, which gave new installs the short form, but existing installs kept the IDs Home Assistant had already pinned. This release makes the change reach them too, from the app itself — no manual renaming.

The mechanism matters, because the obvious approaches do not work. Verified against a live Home Assistant, each of these leaves an existing entity_id unchanged:

- renaming the device and entities (what 1.2.0 did — new installs only),
- deleting the discovery config and republishing it (HA restores the entity_id from its `deleted_entities` record),
- changing `unique_id` on the *same* discovery topic (HA migrates the unique_id in place and keeps the entity).

Only a **new discovery topic carrying a new unique_id** registers a genuinely new entity, which is what lets HA derive the ID from the current names. So the discovery namespace moved from `hay_cm5_fan` to `hay_cm5` — it is both the MQTT node_id (`homeassistant/<domain>/hay_cm5/<object>/config`) and the `unique_id` prefix — and on start the app publishes an empty retained payload to each pre-1.3.0 topic to retire the old entities first. The device is unchanged, so the entities stay grouped under "HAY CM5".

> **Migration.** This happens automatically on update; there is nothing to click. Two consequences:
>
> - **History restarts for these ten sensors.** The new entities are genuinely new registry entries, so their recorder history begins now. The old entities are removed rather than orphaned, and their history is retained under the old IDs until you purge it. If you would rather keep continuity, rename the entities in the UI *before* updating — a UI rename is sticky and this release will not override it.
> - **Anything referencing the old IDs needs updating** — dashboards, automations, template sensors. Home Assistant does not rewrite those. The new ID is the old one with `_fan_controller_cm5` / `_fan_controller` removed, except `..._i_o_controller_temperature`, which becomes `sensor.hay_cm5_io_controller_temperature`.

Verified on a live HA Yellow: a seeded pre-1.3.0 retained config was retired on startup, all ten configs republished under the new namespace with the new unique_ids, the device identity preserved, and the daemon ran clean with no restarts.

---

## 1.2.0

_2026-08-01_

### Fixes (entities frozen or duplicated after a reboot)

**The MQTT probe was one-shot, so a boot-order race decided entity naming for the whole run.** Diagnosed live: this app started **582 ms before** `core-mosquitto`, its single startup check found no broker, and it spent the rest of the day publishing over the Supervisor REST API to a *second* set of entity IDs. The MQTT entities the dashboard watches showed only broker-retained values from before the reboot — CPU clock frozen at 1900 MHz for six hours while the CPU measurably ran 2.4 GHz. `services: mqtt:want` does **not** prevent this; it expresses a preference, not an ordering guarantee.

The probe is now retried at startup and, more importantly, re-run every 60 s for as long as the app is in REST fallback. When the broker appears the app republishes its discovery configs, forces every entity to repopulate, and logs the switch — no restart needed. The startup retry is deliberately capped at 15 s rather than minutes: it runs before the poll loop, so waiting longer would defer thermal control after a boot.

**REST fallback no longer forks your history.** It previously posted to `sensor.hay_cm5_<name>` while MQTT discovery created `sensor.hay_cm5_fan_controller_cm5_<name>`, so a fallback boot silently split every metric across two entities. The REST path now targets exactly the same entity IDs MQTT produces.

> **Migration:** if a previous fallback boot recorded history under the old `sensor.hay_cm5_*` IDs, those entities are now orphaned — nothing writes to them any more. They will linger in the recorder until purged. Delete them under **Settings → Devices & Services → Entities** (filter for `hay_cm5_` and remove the ones *without* `fan_controller` in the ID), or leave them; they are harmless, just stale.

**Failed publishes are no longer silent.** The REST branch was `curl -s … >/dev/null 2>&1`, discarding the HTTP status. It now checks for 200/201, logs failures (rate-limited to one line a minute so an outage cannot flood the log), and — critically — leaves the change-gate cache untouched so the next cycle retries. A value that never reached Home Assistant is no longer recorded as reported.

**Availability now self-heals after a broker restart.** Found while testing this release on the device: restarting mosquitto drops its retained messages, so the `online` published at startup vanished and every entity went `unavailable` in Home Assistant even though fresh states kept arriving. Availability is now re-asserted every poll — a few retained bytes — so it recovers within one cycle instead of never.

**Entities repopulate after a Home Assistant restart.** On-change publishing meant a Core restart left entities empty until some value happened to move. All states are now republished unconditionally every 5 minutes, in both transports.

### Changes

**Entity IDs are now `hay_cm5_<thing>` instead of `hay_cm5_fan_controller_cm5_<thing>`.** The MQTT device is named "HAY CM5" and the entity names no longer repeat "CM5", so Home Assistant composes e.g. `sensor.hay_cm5_processor_temperature` and `binary_sensor.hay_cm5_fan`. Confirmed by publishing a probe discovery config against a live Home Assistant rather than assuming: the payload's `object_id` has been set since the first MQTT release and HA ignores it, because these entities carry a device and `has_entity_name` — the device and entity names are what actually compose the ID.

> **Migration.** Unique IDs are unchanged, so Home Assistant keeps the entity IDs it already assigned — an existing install will *not* rename itself, and nothing breaks if you do nothing.
>
> To adopt the shorter IDs:
> 1. **Restart Home Assistant Core first.** Earlier versions left up to seven REST-created entities (`sensor.hay_cm5_processor_temperature` and friends) sitting on exactly the IDs the new scheme wants. They have no unique ID, exist only in memory, and nothing recreates them once this version is running — but until Core restarts they squat on those IDs and new entities land as `..._2`.
> 2. Then either **rename each entity** in Settings → Devices & Services → Entities (this preserves history, because the recorder follows a UI rename), **or** delete the "HAY CM5" device under the MQTT integration and let it re-register within one keepalive period (simpler, but history restarts).
>
> This also clears the *"This entity does not have a unique ID, therefore its settings cannot be managed from the UI"* warning: that warning was those REST-created entities. Everything this version registers comes from MQTT discovery and therefore has a unique ID.

**CPU clock speed is now a rolling average.** A single instantaneous `scaling_cur_freq` sample aliases badly under the `ondemand` governor: five readings a second apart measured 2000/1800/1800/1900/1900 while the entity sat at a flat 1900. The published value is the mean of the last 6 samples, with `instantaneous_mhz`, `window_min_mhz` and `window_max_mhz` carried as attributes so the stepping is still visible.

**Dropped `full_access`.** The app now declares only the two device nodes it actually uses — `/dev/gpiochip0` and `/dev/vcio` (which `vcgencmd` opens for the firmware throttle flags). Everything else it reads — CPU clock, utilisation, memory, hwmon temperatures — comes from `/proc` and `/sys`, which device permissions never gated. Verified on a live Yellow: with only these two devices, `vcgencmd`, `gpiodetect`, `/proc/stat`, `/proc/meminfo` and all four hwmon sensors still work, and the container sees **16** `/dev` entries instead of **224**. This also un-breaks the `devices:` list, which Supervisor had been ignoring outright ("has full device access; its selective device access configuration is redundant and ignored") for as long as `full_access` was set.

---

## 1.1.1

_2026-06-13_

> _Maintenance (2026-07-25):_ **Fix the daemon being killed partway through shutdown, leaving a stale 'available' entity in Home Assistant.** The poll loop used a bare `sleep`, and bash defers trap handlers until the foreground child exits — so SIGTERM went unhandled for up to `poll_interval` (5s by default, up to 60s by option) while S6's 3s grace expired. `cleanup()` was then killed before it could publish the retained MQTT `offline` status, which is the exact condition #148 was written to fix. The sleep is now `sleep N & wait $!`, so the trap runs the instant the signal arrives. S6_SERVICES_GRACETIME and `timeout:` are also raised, because cleanup()'s MQTT publish has no timeout and the broker may already be stopping. This was intermittent — it depends where `docker stop` lands in the poll cycle — which is why CI failed on one run and passed on another with identical code.

> _Maintenance (2026-07-25):_ **Fix the app being SIGKILLed instead of shut down cleanly.** The AppArmor profile granted `signal (send) set=(...)` but not `receive`. AppArmor mediates signal *delivery* as well as sending, so s6's SIGTERM never reached the app: the graceful shutdown was skipped and the container was killed after the grace period (exit 137). The fan daemon therefore never ran its shutdown path and was killed outright. The rule is now `signal (send,receive),`. Measured on one identical image with only this rule varied: no signal rule -> 13.2s/exit 137; `signal (send) set=(...)` -> 12.7s/exit 137 with no cleanup logged; `signal (send,receive),` -> 6.9s/exit 0 with the full s6 shutdown sequence. Now enforced by `.github/scripts/validate-apparmor.sh` (rule 4) and by the smoke test, which fails on exit 137 under confinement. Rebuild/reinstall the app to pick up the corrected profile.

### Fixes (entities stuck "Unknown" — CM5 Fan, CM5 Undervoltage, CM5 CPU Throttled)

State topics were published **only on change** and **not retained**. Entities whose value rarely changes (the fan and the throttle/undervoltage binary sensors) published their state once at startup — before Home Assistant finished processing discovery and subscribed — and never again, so HA never received a value and showed them `Unknown`. Frequently-updated sensors (temperature, clock speed, utilization) were unaffected because they republish every cycle.

- **Retain state + attribute publishes**, so HA gets the last known value the instant it subscribes (including after an HA restart).
- **Add an availability topic** to every entity (`availability_topic: hay_cm5_fan/status`). Combined with the retained `online`/`offline` status, entities now show **Unavailable** when the add-on is stopped instead of a stale retained value.
- **Publish `status: online` (retained) at startup**, clearing the stale retained `offline` a previous shutdown left behind (which is why `status` read `offline` while the add-on was running).

## 1.1.0

_2026-06-13_

### Added

- **`temperature_unit` option (`celsius` (default) / `fahrenheit`).** Controls the unit for the `temp_on`/`temp_off` thresholds and the add-on's own logs/status table. Thresholds are converted to Celsius internally for fan control (hwmon is always Celsius), and the schema ranges were widened to accept either scale (a soft warning flags out-of-range values for the chosen unit).

### Notes

- The Home Assistant **entities are unchanged** — temperature sensors are still published in native °C. HA already converts the *displayed* unit per your HA unit settings (the "Home Information" page, `/config/general`), so if your dashboard reads °F that's HA's conversion, not the add-on. Set `temperature_unit: fahrenheit` only to enter thresholds and read the add-on log in °F. Defaults preserve existing behavior (Celsius).

## 1.0.3

_2026-06-13_

### Fixes (daemon crash-loop under bashio strict mode — all sensors stuck "Unknown")

The base image now runs service scripts through a bashio that enables `set -o nounset`/`errexit`/`pipefail`. Under this, the daemon aborted on its first **state** publish and S6 respawned it in a tight loop — it published the (retained) MQTT discovery configs, so the entities appeared in HA, but it crashed before the main loop ever sent a value, leaving every sensor reading `Unknown` (and gauge cards reporting "Entity is non-numeric").

- **`mqtt_pub` referenced `$3` unconditionally** (`[ "${3}" = "retain" ]`). State/attribute publishes call it with only 2 args, so under `set -u` the bare `$3` was an unbound variable and aborted the whole daemon. Now uses `${3:-}`.
- **Made publishing non-fatal** (`mosquitto_pub ... || true`). Under `set -e` a transient broker error would otherwise kill the thermal-control daemon; fan safety must not depend on telemetry succeeding.

## 1.0.2

_2026-06-13_

### Fixes (MQTT discovery — sensors silently rejected on HA 2026.6.x)

Home Assistant tightened MQTT discovery validation: an invalid `unit_of_measurement` ↔ `device_class` pairing is now a hard error (the entity is skipped), where older versions only warned. Two bugs in the discovery configs caused five sensors — `cpu_clock_speed`, `cpu_utilization`, `processor_temperature`, `nvme_temperature`, `io_controller_temperature` — to vanish.

- **Wrong `device_class` on non-temperature sensors.** `device_class: "temperature"` was applied as the default to every sensor. Fixed: `cpu_clock_speed` now uses `frequency` (valid with `MHz`), and `cpu_utilization` drops `device_class` entirely (no device class is valid for a generic `%`; `state_class: measurement` is retained).
- **Double-escaped degree sign.** The default unit was the source literal `\u00b0C`, but bash does not expand `\u` escapes in string literals — so jq serialized those 7 characters verbatim and the retained config carried `"\\u00b0C"`, which HA decodes to the literal 7-char string `\u00b0C` rather than the degree sign. This made even the real temperature sensors fail validation. The default unit is now built with `printf` octal escapes so the real UTF-8 bytes (`C2 B0 43`, the degree-Celsius symbol) reach the wire.

Bumping the version triggers HA to re-publish (and overwrite) the retained discovery configs, so the corrected entities appear after updating.

## 1.0.1

_2026-06-10_

### Fixes (hassio-addons base 20.2.0)

- **Add `hassio_role: manager`.** With only `hassio_api: true` and no role, base 20.2.0's stricter Supervisor returns `Unable to access the API, forbidden` — so `bashio::config` couldn't read the fan options and the banner showed no name/version. A role restores Supervisor API access (config + entity publishing).
- **Migrate `bashio::addon.version` → `bashio::app.version`** in the service runner (base 20.2.0 deprecated the `bashio::addon.*` functions).

## 1.0.0

_2026-03-21_
### Initial release

- GPIO-based fan control via sysfs for Home Assistant Yellow with CM5
- Hysteresis-based thermal management (configurable ON/OFF thresholds)
- Three fan modes: auto (thermal), always on, always off
- Auto-discovery of all hardware temperature sensors (CPU, NVMe, board sensors)
- CPU temperature exposed to Home Assistant (`sensor.hay_cpu_temperature`)
- Fan state exposed to Home Assistant (`binary_sensor.hay_cm5_cpu_fan`)
- All additional hwmon sensors exposed as `sensor.hay_<name>_temperature`
- All sensors include `state_class: measurement` for long-term statistics, history graphs, and statistics cards
- Configurable GPIO number, poll interval, and temperature thresholds
- Safe defaults: fan ON at startup, optionally stays ON at shutdown
- Failsafe: fan forced ON if temperature sensor becomes unavailable
- Tested with Seeed Studio Aluminum Alloy CNC Heat Sink with Fan for CM4 (SKU: 114070161)
