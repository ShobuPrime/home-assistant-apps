# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Home Assistant App for controlling a GPIO-connected fan on the Home Assistant Yellow with a Raspberry Pi CM5 compute module. Unlike other apps in this repo, this app has no upstream binary — it is a shell-script daemon that reads CPU temperature and controls a fan via the libgpiod character device interface (`/dev/gpiochip0`), or, where the kernel owns the pin, through the serial port on that pin with a small Go helper (`fan-uart-hold/`).

## Essential Commands

### Building and Testing
```bash
# Build the app locally (must be on aarch64 or use buildx)
./build.sh

# Test locally (limited without GPIO hardware)
docker run --rm -it --privileged local/aarch64-addon-local_hay_cm5_fan:1.0.0
```

## Architecture and Key Components

### How It Works

1. **Init script** (`cont-init.d/cm5-fan.sh`): Validates libgpiod tools and `/dev/gpiochip0` exist
2. **Daemon** (`services.d/cm5-fan/run`): Manages background `gpioset` process for fan control, polls CPU temperature, applies hysteresis logic, updates HA entities
3. **Shutdown**: SIGTERM trap leaves fan in configured state (default: ON for safety)

### GPIO Control via libgpiod

The app uses `gpioset` from the libgpiod tools to control GPIO14 on `gpiochip0` (pinctrl-rp1).

**Why libgpiod instead of sysfs?** Modern HAOS mounts `/sys/class/gpio/` read-only. The sysfs GPIO interface cannot be used even with `full_access: true`. The libgpiod character device interface (`/dev/gpiochip0`) works correctly.

**How `gpioset` works:**
- `gpioset gpiochip0 14=1` sets GPIO14 HIGH and **blocks** (holds the line)
- The daemon runs `gpioset` as a background process
- To toggle state: kill the existing `gpioset`, start a new one with the desired value
- When `gpioset` exits, the line is released — GPIO14/UART TX idles HIGH (fan ON = safe failsafe)

### Serial break: how the fan is driven on HAOS 18.5 and later

HAOS 18.5 moved the Raspberry Pi kernel from raspberrypi/linux `60ea684a8`
(6.18.39) to `9b488bc7a` (6.18.59), which includes `11c58b576` "pinctrl: rp1:
enable strict pinmux mode". A pin muxed to a peripheral can no longer be
requested as a GPIO. GPIO14 is UART0 TX, so `gpioset` gets
`unable to request lines ... Invalid argument` and `gpioinfo` shows
`consumer="kernel"`. 1.3.1 looped on that every ~11 s for six days, logging
"Fan turned ON" each time, before anyone looked (issue #86).

`rootfs/usr/share/hay_cm5_fan/fan-control.sh` picks the method at start:

| `FAN_CONTROL` | When | Fan off | Fan on |
|---|---|---|---|
| `gpio` | the line is free | `gpioset LINE=0` holds it | `gpioset LINE=1` holds it |
| `serial` | `gpioinfo` says `consumer="kernel"` and the line is 14 | `fan-uart-hold /dev/ttyAMA0` holds a break (TX low) | no helper: TX idles high |
| `none` | neither works | — | whatever the pin idles at; retried every 5 min |

- A helper exists in serial mode only while the fan is OFF, so every way of
  losing it leaves the fan ON: SIGTERM, SIGKILL (the kernel clears the break
  when the last holder closes the port), and the daemon dying (the helper
  watches its parent). Nothing else has `/dev/ttyAMA0` open; the console is
  `ttyAMA2` and the radio is `ttyAMA1`.
- `leave_on_at_shutdown: false` cannot be honoured in serial mode. The fan is on
  whenever the app is stopped.
- `/dev/ttyAMA0` must be in `devices:`. The AppArmor profile's blanket `file,`
  already covers it.
- The daemon runs under `set -euo pipefail`. `wait` on a killed helper returns
  143, so it is `|| true`. The old code's kill-and-wait path had never run on
  the device: at 60 °C and up the fan never switches off.

**Verified on the Yellow, 2026-11-19**, with the line level read from the RP1
registers (`/dev/gpiomem0`, GPIO14 = bit 14 of SYNC_IN at 0x10008): break
asserted -> low, cleared -> high, port closed with the break set -> high. With
the line held low for 90 s the CPU went from 68.0 to 73.8 °C and fell back
after release, so the fan itself follows.

**Testing without touching the installed app** (it leaves nothing behind):
`docker create` a container from an image already on the device with
`--device /dev/ttyAMA0` (and `--device /dev/gpiomem0:/dev/gpiomem0:r` for the
readback, from an image with python3), `docker cp -` the files in as a tar on
stdin, `docker start -ai` with the script on stdin, `docker rm -f`. The SSH
app's own container can open the tty but not gpiomem.
`tests/fan-control-test.sh` runs the same logic with no hardware.

### Temperature Reading

CPU temperature is read from `/sys/class/hwmon/hwmon0/temp1_input` (millidegrees Celsius). The path is configurable in case hwmon numbering changes.

### Home Assistant Integration

Entities are created via the Supervisor REST API (`POST /core/api/states/`):
- `sensor.hay_cpu_temperature` — CPU temperature (used for fan control decisions)
- `binary_sensor.hay_cm5_cpu_fan` — fan on/off state with attributes
- `sensor.hay_<hwmon_name>_temperature` — auto-discovered sensors (NVMe, RP1, etc.)

All temperature sensors include `state_class: measurement` and `device_class: temperature` for full history/LTS support. These are "virtual" entities — they show state but don't support service calls. Fan control is via app config (fan_mode: auto/on/off).

### Directory Structure
- **`/rootfs/etc/cont-init.d/cm5-fan.sh`**: S6 initialization (validate libgpiod, check `/dev/gpiochip0`)
- **`/rootfs/etc/services.d/cm5-fan/`**: Service definition with `run` (daemon) and `finish` (crash handler)
- **`/rootfs/usr/share/hay_cm5_fan/fan-control.sh`**: how the fan line is driven (`set_fan`, method selection, watchdog); sourced by `run`. Under `usr/share`, not `usr/lib`: the repo's `.gitignore` ignores every `lib/` directory, so a file there is never committed and the image is built without it
- **`/fan-uart-hold/`**: Go helper that holds a serial break; stdlib only
- **`/tests/fan-control-test.sh`**: tests `fan-control.sh` with fake GPIO tools and the real helper on a pty

### Critical Files
- **`config.yaml`**: App configuration (version, options schema, full_access)
- **`translations/en.yaml`**: Plain-English config-option names + descriptions shown in the HA Configuration tab (kept in sync with `config.yaml`'s options)
- **`build.yaml`**: Build configuration — **aarch64 only** (CM5 hardware)
- **`Dockerfile`**: Minimal — curl, jq, libgpiod, and shell scripts (no binary download)
- **`apparmor.txt`**: Security profile with `/dev/gpiochip*` and hwmon sysfs paths

### Architecture Support
- `aarch64` only — this app is hardware-specific to the Raspberry Pi CM5

### No Upstream Version Tracking

This app has no upstream software to track. The app IS the software. Version bumps are manual. There is no update script or automated update workflow.

## Development Guidelines

### S6-Overlay Integration
- Use Bashio library for all configuration reading and logging
- The `run` script MUST trap SIGTERM for clean shutdown
- The daemon uses a `while true; sleep N` polling loop (not exec into a binary)

### Configuration Handling
- Read options using `bashio::config` functions
- `bashio::config.true` for boolean checks
- Threshold validation: `temp_off` must be less than `temp_on` (in auto mode)

### Safety Invariants
- Fan MUST be ON at startup before the polling loop begins
- Fan MUST be forced ON if temperature sensor becomes unavailable
- Fan MUST be forced ON if the `gpioset` background process dies unexpectedly
- In serial mode the fan MUST come on if `fan-uart-hold` is lost by any route
- `leave_on_at_shutdown: true` is the recommended and default setting
- Device access is the `devices:` list in `config.yaml`, not `full_access`

### Hardware Constraints (Do NOT violate)
- GPIO14 is on/off only — no hardware PWM
- GPIO chip is `gpiochip0` (pinctrl-rp1) on CM5, GPIO line is 14
- Do NOT use the legacy sysfs GPIO interface — it is read-only on modern HAOS
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
- `/dev/gpiochip0` is accessible in the container
- `gpioset` successfully holds GPIO line 14
- Fan turns ON at startup
- Temperature reading works
- Hysteresis logic: fan turns ON at temp_on, OFF at temp_off, stable between
- Fan mode override works (on/off/auto)
- Shutdown handler respects leave_on_at_shutdown config
- HA entities appear (sensor.hay_cpu_temperature, binary_sensor.hay_cm5_cpu_fan)
- Temperature sensor failure triggers failsafe (fan ON)
- gpioset process death triggers failsafe (fan ON)

## Important Notes

- **Never commit changes** to version numbers without testing on actual Yellow hardware
- **Auto-update is on for this app on the device**, so a merge is a deployment
- **AppArmor profile** must include `/dev/gpiochip*` and `/sys/class/hwmon/**` paths
- The app creates entities via REST API, not via an integration — they don't survive HA restarts unless the app is running

## Common Issues and Troubleshooting

### Issue: GPIO Chip Not Found

**Cause:** `/dev/gpiochip0` not available in the container

**Solution:**
1. Verify `full_access: true` in config.yaml
2. Check that the device exists on the host: `ls -la /dev/gpiochip*`
3. The pinctrl-rp1 chip should be `gpiochip0` on CM5

### Issue: gpioset Process Dies

**Cause:** Line requested by another process, or permissions issue

**Solution:** The daemon auto-detects this and restarts gpioset with fan ON. Check logs for repeated warnings.

### Issue: Entities Disappear After HA Restart

**Cause:** Supervisor API entities are not persistent — they exist only while being actively updated

**Solution:** This is expected. The app re-creates them on startup. Ensure the app is set to `boot: auto`.
