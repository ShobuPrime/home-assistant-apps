# HAY CM5 Fan Controller Documentation

## Overview

This addon provides GPIO-based fan control for the Home Assistant Yellow with a Raspberry Pi CM5 compute module. It uses the Linux sysfs GPIO interface to control a fan connected to the Yellow's 10-pin GPIO header, with hysteresis-based thermal management to prevent rapid cycling.

In addition to fan control, the addon automatically discovers and exposes **all hardware temperature sensors** (CPU, NVMe SSD, board sensors, etc.) as Home Assistant entities with full long-term statistics support for history graphs and dashboards.

## Hardware Wiring

### Yellow 10-Pin GPIO Header (Connector J11)

```
Pin 1 [3.3V]     Pin 2  [5V]       (Pin 1 = square pad on PCB)
Pin 3 [GPIO2]    Pin 4  [5V]
Pin 5 [GPIO3]    Pin 6  [GND]
Pin 7 [GPIO4]    Pin 8  [GPIO14]   <-- Fan PWM/enable signal
Pin 9 [GND]      Pin 10 [GPIO15]
```

Connect your fan:
- **Pin 4** (5V) -> Red wire (power)
- **Pin 6** (GND) -> Black wire (ground)
- **Pin 8** (GPIO14) -> Blue wire (PWM/enable signal)

### Tested Fan

This addon has been tested and verified compatible with the [Seeed Studio Aluminum Alloy CNC Heat Sink with Fan for Raspberry Pi CM4](https://www.electromaker.io/shop/product/aluminum-alloy-cnc-heat-sink-with-fan-for-raspberry-pi-cm4-module) (SKU: 114070161) installed on the Home Assistant Yellow. Despite being marketed for CM4, this fan is physically and electrically compatible with the CM5 on the Yellow board when connected to the 10-pin GPIO header.

### GPIO Details

- GPIO14 is shared with UART0 (ttyAMA0), but sysfs export overrides the UART pinmux
- sysfs GPIO number: **583** (RP1 base 569 + GPIO 14)
- Control is on/off only (no hardware PWM on GPIO14)
- UART TX idles HIGH, so the fan runs by default as a failsafe even without this addon

## Configuration

### Option: `gpio_number`

The sysfs GPIO number for the fan control pin. Default: `583`

This corresponds to GPIO14 on the CM5's RP1 chip (base offset 569 + GPIO number 14). Only change this if you've wired your fan to a different GPIO pin.

### Option: `temp_sensor_path`

Path to the CPU temperature sensor used for fan control decisions. Default: `/sys/class/hwmon/hwmon0/temp1_input`

The file contains temperature in millidegrees Celsius (e.g., `37500` = 37.5C). Only change this if your hwmon numbering differs. Note: all other temperature sensors are auto-discovered regardless of this setting.

### Option: `poll_interval`

How often to check CPU temperature, in seconds (1-60). Default: `5`

Lower values provide faster response but slightly more CPU overhead. 5 seconds is a good balance — temperature changes slowly.

### Option: `temp_on`

Temperature threshold in Celsius to turn the fan ON (30-90). Default: `55`

When CPU temperature reaches or exceeds this value, the fan turns on.

### Option: `temp_off`

Temperature threshold in Celsius to turn the fan OFF (25-85). Default: `45`

When CPU temperature drops to or below this value, the fan turns off. Must be lower than `temp_on` to create hysteresis.

### Option: `fan_mode`

Controls fan behavior:
- `auto` (default): Hysteresis-based thermal control using temp_on/temp_off thresholds
- `on`: Fan runs continuously regardless of temperature
- `off`: Fan is off regardless of temperature (use with caution — CPU may overheat)

### Option: `leave_on_at_shutdown`

Whether to leave the fan running when the addon stops or restarts. Default: `true`

Recommended to leave this enabled. If set to `false`, the fan will turn off when the addon stops, which could allow the CPU to overheat if the addon isn't restarted promptly.

### Option: `log_level`

- `trace`: Show every detail
- `debug`: Shows detailed debug information
- `info`: Normal events (default)
- `warning`: Exceptional occurrences that are not errors
- `error`: Runtime errors
- `fatal`: Critical errors

## Home Assistant Entities

All entities are created with `state_class: measurement` and proper `device_class` attributes, which means:

- **History graphs**: Full historical data is automatically recorded and available in the History panel
- **Long-term statistics (LTS)**: HA computes hourly min/mean/max statistics, retained indefinitely
- **Statistics cards**: Use the Statistics Graph card to visualize trends over days, weeks, or months
- **Energy/Analytics**: Temperature data integrates with HA's analytics system

### Primary Entities

#### `sensor.hay_cpu_temperature`

- **Type**: Sensor
- **Device class**: temperature
- **State class**: measurement (enables LTS)
- **Unit**: C
- **State**: Current CPU temperature (e.g., `37.5`)
- **Updates**: Every poll cycle when value changes

#### `binary_sensor.hay_cm5_cpu_fan`

- **Type**: Binary sensor
- **Device class**: running
- **State**: `on` when fan is running, `off` when stopped
- **History**: Binary sensors are recorded in HA history by default — you can see exactly when the fan was on/off over time
- **Attributes**:
  - `temperature`: Current CPU temperature
  - `gpio`: GPIO number in use
  - `mode`: Current fan mode (auto/on/off)
  - `temp_on_threshold`: ON temperature threshold
  - `temp_off_threshold`: OFF temperature threshold

### Auto-Discovered Temperature Sensors

The addon scans all `/sys/class/hwmon/` devices at startup and creates additional entities for every temperature sensor found. Common sensors on the Yellow with CM5:

- `sensor.hay_nvme_temperature` — NVMe SSD temperature (if present)
- `sensor.hay_rp1_temperature` — RP1 I/O controller temperature
- Other sensors vary by hardware configuration

Each discovered sensor has:
- **Device class**: temperature
- **State class**: measurement (enables LTS)
- **Unit**: C
- **Source attribute**: Shows the hwmon path for identification

Check the addon logs at startup to see which sensors were discovered and their entity IDs.

## Dashboard Examples

### Temperature History Graph (recommended)

Shows temperature trends over time with automatic min/mean/max statistics:

```yaml
type: history-graph
title: HAY System Temperatures
hours_to_show: 24
entities:
  - entity: sensor.hay_cpu_temperature
    name: CPU
  - entity: sensor.hay_nvme_temperature
    name: NVMe SSD
```

### Statistics Graph Card (long-term trends)

Shows hourly/daily/weekly/monthly statistics — great for spotting thermal trends:

```yaml
type: statistics-graph
title: Temperature Trends (7 days)
period: day
stat_types:
  - mean
  - min
  - max
entities:
  - sensor.hay_cpu_temperature
  - sensor.hay_nvme_temperature
```

### Mini Graph Card (requires HACS mini-graph-card)

Compact sparkline graph for a dashboard overview:

```yaml
type: custom:mini-graph-card
entities:
  - entity: sensor.hay_cpu_temperature
    name: CPU
  - entity: sensor.hay_nvme_temperature
    name: NVMe
name: System Temperatures
hours_to_show: 12
points_per_hour: 12
line_width: 2
show:
  labels: true
  points: false
```

### Entities Card with Fan Status

```yaml
type: entities
title: HAY CM5 Cooling
entities:
  - entity: binary_sensor.hay_cm5_cpu_fan
    name: CPU Fan
    secondary_info: last-changed
  - entity: sensor.hay_cpu_temperature
    name: CPU Temperature
  - entity: sensor.hay_nvme_temperature
    name: NVMe Temperature
```

### Gauge Card

```yaml
type: gauge
entity: sensor.hay_cpu_temperature
name: CPU Temperature
min: 20
max: 90
severity:
  green: 20
  yellow: 55
  red: 75
```

### Conditional Fan Alert Card

```yaml
type: conditional
conditions:
  - condition: numeric_state
    entity: sensor.hay_cpu_temperature
    above: 70
card:
  type: alert
  title: High CPU Temperature
  content: >
    CPU is at {{ states('sensor.hay_cpu_temperature') }}C.
    Fan is {{ states('binary_sensor.hay_cm5_cpu_fan') }}.
```

## Example Automations

### High Temperature Alert

```yaml
automation:
  - alias: "HAY CM5 High Temperature Alert"
    trigger:
      - platform: numeric_state
        entity_id: sensor.hay_cpu_temperature
        above: 75
        for:
          minutes: 5
    action:
      - service: notify.mobile_app
        data:
          title: "CM5 Temperature Warning"
          message: >
            CPU temperature is {{ states('sensor.hay_cpu_temperature') }}C
            with fan {{ states('binary_sensor.hay_cm5_cpu_fan') }}.
            NVMe: {{ states('sensor.hay_nvme_temperature') }}C
```

### NVMe Overheat Warning

```yaml
automation:
  - alias: "HAY NVMe Temperature Warning"
    trigger:
      - platform: numeric_state
        entity_id: sensor.hay_nvme_temperature
        above: 70
        for:
          minutes: 2
    action:
      - service: notify.mobile_app
        data:
          title: "NVMe Temperature Warning"
          message: "NVMe SSD temperature is {{ states('sensor.hay_nvme_temperature') }}C"
```

### Log Fan Runtime

Track how often the fan runs by recording state changes:

```yaml
automation:
  - alias: "HAY Fan State Changed"
    trigger:
      - platform: state
        entity_id: binary_sensor.hay_cm5_cpu_fan
    action:
      - service: logbook.log
        data:
          name: "CM5 Fan"
          message: "Fan turned {{ states('binary_sensor.hay_cm5_cpu_fan') }} at CPU {{ states('sensor.hay_cpu_temperature') }}C"
```

## How Hysteresis Works

In `auto` mode, the fan uses hysteresis to prevent rapid on/off cycling:

1. Fan turns **ON** when temperature reaches `temp_on` (default 55C)
2. Fan stays **ON** as temperature drops
3. Fan turns **OFF** only when temperature drops to `temp_off` (default 45C)
4. Fan stays **OFF** as temperature rises
5. Cycle repeats from step 1

The 10C gap (default) between thresholds prevents the fan from toggling every few seconds when temperature hovers near a single threshold.

## Thermal Performance Reference

Based on testing with the CM5 on Home Assistant Yellow:
- **Fan ON**: ~37C steady state
- **Fan OFF**: Temperature climbs ~3C/min
- **No fan at all**: 80-82C under normal load

## Safety Features

- Fan starts ON at addon startup (before thermal monitoring begins)
- Fan forced ON if temperature sensor file becomes unavailable
- Configurable option to leave fan ON at addon shutdown
- UART TX pin idles HIGH, so the fan runs even without this addon as a hardware failsafe
- Configuration validation prevents `temp_off >= temp_on` (would disable hysteresis)

## Security Considerations

- **Full access**: This addon requires `full_access: true` to write to sysfs GPIO. This gives the addon container elevated privileges.
- **AppArmor**: Custom profile restricts access to only GPIO and hwmon sysfs paths

## Troubleshooting

### Fan Not Starting

**Symptoms:**
- Addon starts but fan doesn't spin
- Logs show GPIO export errors

**Solutions:**
1. Verify wiring (Pin 4=5V, Pin 6=GND, Pin 8=GPIO14)
2. Check that `gpio_number` is correct (583 for GPIO14 on CM5)
3. Ensure HAOS supports sysfs GPIO (HAOS 17.1+ recommended)
4. Check addon logs for specific error messages

### Temperature Sensor Not Found

**Symptoms:**
- Log shows "Temperature sensor not found"
- Fan stays ON permanently (failsafe)

**Solutions:**
1. Check the hwmon path: `cat /sys/class/hwmon/hwmon0/temp1_input`
2. Try other hwmon indices: `ls /sys/class/hwmon/`
3. Update `temp_sensor_path` in addon config if needed

### NVMe or Other Sensors Not Appearing

**Symptoms:**
- Only CPU temperature entity created

**Solutions:**
1. Check addon startup logs — all discovered sensors are listed
2. Verify hwmon devices exist: `ls /sys/class/hwmon/*/name`
3. Some devices may not expose temperature sensors via hwmon

### Fan Cycling Rapidly

**Symptoms:**
- Fan turns on and off every few seconds

**Solutions:**
1. Increase the gap between `temp_on` and `temp_off` (e.g., 60/40 instead of 55/45)
2. Increase `poll_interval` to reduce check frequency

### Entities Disappear After HA Restart

**Cause:** Supervisor API entities exist only while being actively updated by the addon.

**Solution:** This is expected. The addon re-creates them on startup. Ensure the addon is set to `boot: auto`. Historical data in HA's recorder is preserved even when entities temporarily disappear.

## Important Notes

- This addon is specific to the **Raspberry Pi CM5** on **Home Assistant Yellow**
- GPIO14's dedicated fan header (GPIO45 / `dtparam=cooling_fan`) is NOT used — it's not routed to the Yellow's 10-pin header
- Hardware PWM is not available on GPIO14 — control is on/off only
- Do NOT use device tree overlays — HAOS bootloader silently ignores them
- All temperature entities use `state_class: measurement` for automatic long-term statistics

## External Resources

- [Home Assistant Yellow Hardware](https://yellow.home-assistant.io/)
- [Raspberry Pi CM5 Datasheet](https://www.raspberrypi.com/documentation/computers/compute-module.html)
- [Linux sysfs GPIO Interface](https://www.kernel.org/doc/Documentation/gpio/sysfs.txt)
- [HA Long-Term Statistics](https://www.home-assistant.io/docs/configuration/state_object/#state_class)
