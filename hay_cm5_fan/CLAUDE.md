# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Home Assistant Add-on for controlling a GPIO-connected fan on the Home Assistant Yellow with a Raspberry Pi CM5 compute module. Unlike other addons in this repo, this addon has no upstream binary — it is a pure shell-script daemon that reads CPU temperature and controls a fan via the Linux sysfs GPIO interface.

## Essential Commands

### Building and Testing
```bash
# Build the add-on locally (must be on aarch64 or use buildx)
./build.sh

# Test locally (limited without GPIO hardware)
docker run --rm -it --privileged local/aarch64-addon-local_hay_cm5_fan:1.0.0
```

## Architecture and Key Components

### How It Works

1. **Init script** (`cont-init.d/cm5-fan.sh`): Exports sysfs GPIO, sets direction to output, turns fan ON
2. **Daemon** (`services.d/cm5-fan/run`): Polls CPU temperature, applies hysteresis logic, updates HA entities
3. **Shutdown**: SIGTERM trap leaves fan in configured state (default: ON for safety)

### GPIO Control via sysfs

The CM5 on Yellow uses sysfs GPIO number **583** (RP1 base 569 + GPIO 14). Control is via file writes:
- `/sys/class/gpio/export` — claim the GPIO pin
- `/sys/class/gpio/gpio583/direction` — set to "out"
- `/sys/class/gpio/gpio583/value` — "1" = fan on, "0" = fan off

### Temperature Reading

CPU temperature is read from `/sys/class/hwmon/hwmon0/temp1_input` (millidegrees Celsius). The path is configurable in case hwmon numbering changes.

### Home Assistant Integration

Entities are created via the Supervisor REST API (`POST /core/api/states/`):
- `sensor.hay_cpu_temperature` — CPU temperature (used for fan control decisions)
- `binary_sensor.hay_cm5_cpu_fan` — fan on/off state with attributes
- `sensor.hay_<hwmon_name>_temperature` — auto-discovered sensors (NVMe, RP1, etc.)

All temperature sensors include `state_class: measurement` and `device_class: temperature` for full history/LTS support. These are "virtual" entities — they show state but don't support service calls. Fan control is via addon config (fan_mode: auto/on/off).

### Directory Structure
- **`/rootfs/etc/cont-init.d/cm5-fan.sh`**: S6 initialization (GPIO export, fan ON)
- **`/rootfs/etc/services.d/cm5-fan/`**: Service definition with `run` (daemon) and `finish` (crash handler)

### Critical Files
- **`config.yaml`**: Add-on configuration (version, options schema, full_access)
- **`build.yaml`**: Build configuration — **aarch64 only** (CM5 hardware)
- **`Dockerfile`**: Minimal — just curl, jq, and shell scripts (no binary download)
- **`apparmor.txt`**: Security profile with GPIO and hwmon sysfs paths

### Architecture Support
- `aarch64` only — this addon is hardware-specific to the Raspberry Pi CM5

### No Upstream Version Tracking

This addon has no upstream software to track. The addon IS the software. Version bumps are manual. There is no update script or automated update workflow.

## Development Guidelines

### S6-Overlay Integration
- Use Bashio library for all configuration reading and logging
- The `run` script MUST trap SIGTERM for clean shutdown
- The daemon uses a `while true; sleep N` polling loop (not exec into a binary)

### Configuration Handling
- Read options using `bashio::config` functions
- `bashio::config.true` for boolean checks
- Threshold validation: `temp_off` must be less than `temp_on`

### Safety Invariants
- Fan MUST be ON at startup before the polling loop begins
- Fan MUST be forced ON if temperature sensor becomes unavailable
- `leave_on_at_shutdown: true` is the recommended and default setting
- The addon requires `full_access: true` for sysfs GPIO writes

### Hardware Constraints (Do NOT violate)
- GPIO14 is on/off only — no hardware PWM
- sysfs GPIO number 583 is specific to CM5 RP1 chip on Yellow
- `dtparam=cooling_fan` controls GPIO45 (CM5 dedicated header) — NOT the Yellow 10-pin header
- HAOS silently ignores custom device tree overlays
- Do NOT attempt to unbind UART driver at runtime

### Version Updates
When updating version:
1. Update `version` in config.yaml
2. Add entry to CHANGELOG.md
3. Update version reference in README.md

### Testing Checklist
- Build completes successfully on aarch64
- GPIO export succeeds on Yellow hardware
- Fan turns ON at startup
- Temperature reading works
- Hysteresis logic: fan turns ON at temp_on, OFF at temp_off, stable between
- Fan mode override works (on/off/auto)
- Shutdown handler respects leave_on_at_shutdown config
- HA entities appear (sensor.cm5_cpu_temperature, binary_sensor.cm5_cpu_fan)
- Temperature sensor failure triggers failsafe (fan ON)

## Important Notes

- **Never commit changes** to version numbers without testing on actual Yellow hardware
- **full_access: true** is required — there is no lesser privilege that allows sysfs GPIO writes
- **AppArmor profile** must include `/sys/class/gpio/**` and `/sys/class/hwmon/**` paths
- The addon creates entities via REST API, not via an integration — they don't survive HA restarts unless the addon is running

## Common Issues and Troubleshooting

### Issue: GPIO Export Fails

**Cause:** Missing sysfs support, wrong GPIO number, or insufficient privileges

**Solution:**
1. Verify `full_access: true` in config.yaml
2. Check that sysfs GPIO is available: `ls /sys/class/gpio/`
3. Confirm GPIO number (583 for CM5 GPIO14 on Yellow)

### Issue: Entities Disappear After HA Restart

**Cause:** Supervisor API entities are not persistent — they exist only while being actively updated

**Solution:** This is expected. The addon re-creates them on startup. Ensure the addon is set to `boot: auto`.
