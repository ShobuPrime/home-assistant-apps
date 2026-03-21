#!/usr/bin/with-contenv bashio
# ==============================================================================
# Home Assistant Add-on: CM5 Fan Controller
# Initializes GPIO for fan control on Home Assistant Yellow with CM5
# ==============================================================================

# Read GPIO number from config
GPIO_NUM=$(bashio::config 'gpio_number')
GPIO_PATH="/sys/class/gpio/gpio${GPIO_NUM}"
TEMP_SENSOR=$(bashio::config 'temp_sensor_path')

bashio::log.info "Initializing CM5 Fan Controller..."
bashio::log.info "  GPIO number: ${GPIO_NUM}"
bashio::log.info "  GPIO sysfs path: ${GPIO_PATH}"
bashio::log.info "  Temperature sensor: ${TEMP_SENSOR}"

# Create data directory
mkdir -p /data/hay_cm5_fan
chmod 755 /data/hay_cm5_fan

# Validate sysfs GPIO is available
if [[ ! -d /sys/class/gpio ]]; then
    bashio::log.error "sysfs GPIO interface not available!"
    bashio::log.error "Ensure the addon has full_access: true and the host supports sysfs GPIO."
    exit 1
fi

# Export GPIO pin (ignore error if already exported)
if [[ ! -d "${GPIO_PATH}" ]]; then
    bashio::log.info "Exporting GPIO ${GPIO_NUM}..."
    echo "${GPIO_NUM}" > /sys/class/gpio/export 2>/dev/null || {
        bashio::log.error "Failed to export GPIO ${GPIO_NUM}!"
        bashio::log.error "Check that GPIO number is correct (CM5 on Yellow: RP1 base 569 + GPIO 14 = 583)"
        exit 1
    }
    # Brief delay for sysfs to create the GPIO directory
    sleep 0.3
fi

if [[ ! -d "${GPIO_PATH}" ]]; then
    bashio::log.error "GPIO ${GPIO_NUM} directory not created after export!"
    exit 1
fi

# Set direction to output
bashio::log.info "Setting GPIO ${GPIO_NUM} direction to output..."
echo "out" > "${GPIO_PATH}/direction" || {
    bashio::log.error "Failed to set GPIO direction!"
    exit 1
}

# Turn fan ON as initial safe state
bashio::log.info "Turning fan ON (initial safe state)..."
echo "1" > "${GPIO_PATH}/value" || {
    bashio::log.error "Failed to set GPIO value!"
    exit 1
}

# Verify fan state
FAN_VALUE=$(cat "${GPIO_PATH}/value" 2>/dev/null)
if [[ "${FAN_VALUE}" = "1" ]]; then
    bashio::log.info "Fan is ON (GPIO ${GPIO_NUM} = 1)"
else
    bashio::log.warning "Unexpected fan state: ${FAN_VALUE}"
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

bashio::log.info "CM5 Fan Controller initialization complete"
