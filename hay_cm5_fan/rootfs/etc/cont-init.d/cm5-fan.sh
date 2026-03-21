#!/usr/bin/with-contenv bashio
# ==============================================================================
# Home Assistant Add-on: HAY CM5 Fan Controller
# Validates libgpiod and hardware prerequisites
# ==============================================================================

# Read config
GPIO_CHIP=$(bashio::config 'gpio_chip')
GPIO_LINE=$(bashio::config 'gpio_line')
TEMP_SENSOR=$(bashio::config 'temp_sensor_path')

bashio::log.info "Initializing HAY CM5 Fan Controller..."
bashio::log.info "  GPIO chip: ${GPIO_CHIP}"
bashio::log.info "  GPIO line: ${GPIO_LINE}"
bashio::log.info "  Temperature sensor: ${TEMP_SENSOR}"

# Create data directory
mkdir -p /data/hay_cm5_fan
chmod 755 /data/hay_cm5_fan

# Validate libgpiod tools are installed
if ! command -v gpioset &> /dev/null; then
    bashio::log.error "gpioset not found! libgpiod package is missing."
    exit 1
fi

bashio::log.info "libgpiod tools found: $(gpioset --version 2>&1 | head -1)"

# Validate GPIO character device exists
# Don't hard-fail here — let the run script handle the missing device
# so the container stays up for diagnostics and CI smoke tests
if [[ ! -c "/dev/${GPIO_CHIP}" ]]; then
    bashio::log.warning "GPIO chip device /dev/${GPIO_CHIP} not found!"
    bashio::log.warning "Ensure full_access: true is set and the host has the GPIO character device."
    ls -la /dev/gpiochip* 2>/dev/null || bashio::log.warning "  No GPIO chips found"
    bashio::log.warning "Fan control will not work until GPIO device is available."
else
    bashio::log.info "GPIO chip /dev/${GPIO_CHIP} found"
fi

# Validate temperature sensor
if [[ -f "${TEMP_SENSOR}" ]]; then
    TEMP_RAW=$(cat "${TEMP_SENSOR}")
    TEMP_C=$((TEMP_RAW / 1000))
    bashio::log.info "CPU temperature sensor found: ${TEMP_C}C"
else
    bashio::log.warning "Temperature sensor not found at ${TEMP_SENSOR}"
    bashio::log.warning "Fan will remain ON as a safety measure until sensor is available"
fi

bashio::log.info "HAY CM5 Fan Controller initialization complete"
