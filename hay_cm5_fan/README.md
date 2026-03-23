# HAY CM5 Fan Controller for Home Assistant

![Supports aarch64 Architecture][aarch64-shield]

GPIO-based fan control with thermal management for Home Assistant Yellow with Raspberry Pi CM5.

## About

This app controls a fan connected to the Home Assistant Yellow's 10-pin GPIO header (connector J11) via libgpiod (`/dev/gpiochip0`). It provides hysteresis-based thermal management to keep the CM5 cool while avoiding rapid fan cycling. Fan state and all system temperatures (CPU, NVMe, etc.) are exposed as Home Assistant entities with full history and long-term statistics support.

## Features

- Hysteresis-based automatic fan control (configurable ON/OFF temperature thresholds)
- Manual override modes (always on / always off)
- Auto-discovers all hardware temperature sensors (CPU, NVMe SSD, board sensors)
- CPU temperature exposed as `sensor.hay_cpu_temperature`
- Fan state exposed as `binary_sensor.hay_cm5_cpu_fan`
- All sensors support HA history graphs, statistics cards, and long-term statistics
- Safe defaults: fan ON at startup, stays ON at shutdown
- Failsafe: fan forced ON if temperature sensor becomes unavailable
- Uses libgpiod character device interface (works on modern HAOS with read-only sysfs)

## Hardware Requirements

- Home Assistant Yellow (NabuCasa) with Raspberry Pi CM5
- Fan connected to the Yellow 10-pin GPIO header (J11):
  - Pin 4: 5V (red)
  - Pin 6: GND (black)
  - Pin 8: GPIO14 / PWM signal (blue) — gpiochip0 line 14

### Tested Fan

This app has been tested and verified compatible with the [Seeed Studio Aluminum Alloy CNC Heat Sink with Fan for Raspberry Pi CM4](https://www.electromaker.io/shop/product/aluminum-alloy-cnc-heat-sink-with-fan-for-raspberry-pi-cm4-module) (SKU: 114070161) installed on the Home Assistant Yellow. Despite being marketed for CM4, the fan is physically and electrically compatible with the CM5 on the Yellow board when wired to the 10-pin GPIO header as described above.

## Installation

1. Add this repository to your Home Assistant instance
2. Search for "HAY CM5 Fan Controller" in the app store
3. Click Install
4. Configure temperature thresholds if desired (defaults work well for most setups)
5. Start the app

## Configuration

### Option: `gpio_chip`

The GPIO character device name. Default: `gpiochip0` (pinctrl-rp1 on CM5).

### Option: `gpio_line`

The GPIO line number on the chip. Default: `14` (GPIO14 on the Yellow 10-pin header, Pin 8).

### Option: `temp_sensor_path`

Path to the CPU temperature sensor file used for fan control. Default: `/sys/class/hwmon/hwmon0/temp1_input`. All other temperature sensors are auto-discovered regardless of this setting.

### Option: `poll_interval`

How often to check CPU temperature, in seconds. Default: `5`

### Option: `temp_on`

Temperature threshold (Celsius) to turn the fan ON. Default: `55`

### Option: `temp_off`

Temperature threshold (Celsius) to turn the fan OFF. Default: `45`

### Option: `fan_mode`

- `auto`: Hysteresis-based thermal control (default)
- `on`: Fan always on
- `off`: Fan always off (use with caution)

### Option: `leave_on_at_shutdown`

Whether to leave the fan running when the app stops. Default: `true` (recommended for safety).

### Option: `log_level`

Controls log verbosity: `trace`, `debug`, `info` (default), `warning`, `error`, `fatal`

## Support

Got questions or found a bug? Please open an issue on the [GitHub repository](https://github.com/ShobuPrime/home-assistant-apps).

[aarch64-shield]: https://img.shields.io/badge/aarch64-yes-green.svg

## Version

Currently running HAY CM5 Fan Controller 1.0.0
