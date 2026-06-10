# Changelog

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
