# Changelog

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
