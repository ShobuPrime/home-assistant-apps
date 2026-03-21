# Changelog

## Version 1.0.0 (2026-03-21)

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
