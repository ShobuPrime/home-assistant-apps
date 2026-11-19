# shellcheck shell=bash
# ==============================================================================
# Home Assistant App: HAY CM5 Fan Controller
# Fan line control. Sourced by the daemon (and by the tests).
#
# Needs GPIO_CHIP, GPIO_LINE, POLL_INTERVAL and bashio's log functions.
# Runs under bashio's `set -euo pipefail`, so every command that may fail is
# guarded.
#
# Two ways to drive the line:
#
#   gpio    `gpioset` holds the line as an output. How the app always worked.
#
#   serial  HAOS 18.5 (kernel 6.18.59) turned on strict pinmux for the RP1: a
#           pin muxed to a peripheral can no longer be requested as a GPIO.
#           gpioset gets EINVAL and gpioinfo shows consumer="kernel". On the
#           Yellow, GPIO14 is UART0 TX. A serial break drives TX low, so
#           `fan-uart-hold` holds a break on the port for "fan off". When it
#           exits, or the port closes for any reason, TX idles high: fan on.
#
# A helper process holds the state in both. In serial mode one exists only
# while the fan is OFF, so losing it by any route leaves the fan ON.
# ==============================================================================

FAN_CONTROL=""       # gpio | serial | none, chosen by fan_control_init
FAN_STATE=""         # 1 = on, 0 = off, "" = not set yet
FAN_HOLDER_PID=""    # gpioset, or fan-uart-hold while the fan is off
FAN_RETRY_IN=0       # poll cycles until `none` looks for a way again

FAN_UART_LINE=14     # UART0 TX: the only line the serial method fits
FAN_UART_DEVICE="${FAN_UART_DEVICE:-/dev/ttyAMA0}"
FAN_UART_HOLD="${FAN_UART_HOLD:-/usr/bin/fan-uart-hold}"

fan_holder_alive() {
    [ -n "${FAN_HOLDER_PID}" ] && kill -0 "${FAN_HOLDER_PID}" 2>/dev/null
}

fan_kill_holder() {
    if fan_holder_alive; then
        kill "${FAN_HOLDER_PID}" 2>/dev/null || true
        wait "${FAN_HOLDER_PID}" 2>/dev/null || true
    fi
    FAN_HOLDER_PID=""
}

# Start a helper in the background. Succeeds if it is still running a moment
# later, which is how both helpers signal that they got the line.
fan_start_holder() {
    "$@" &
    FAN_HOLDER_PID=$!
    sleep 0.3
    fan_holder_alive
}

# True when a pinmux function owns the line and gpioset would be refused.
# No pipeline: under pipefail an early-exiting grep can fail a match.
fan_line_owned_by_kernel() {
    local info line
    info="$(gpioinfo -c "${GPIO_CHIP}" 2>/dev/null || true)"
    line="$(grep -E "line +${GPIO_LINE}:" <<< "${info}" || true)"
    [[ "${line}" == *'consumer="kernel"'* ]]
}

# Stop trying for a while. The fan is left as the line idles; on the Yellow's
# serial pin that is ON.
fan_control_give_up() {
    FAN_HOLDER_PID=""
    FAN_CONTROL="none"
    FAN_STATE=1
    FAN_RETRY_IN=$(( 300 / POLL_INTERVAL ))
    if [ "${FAN_RETRY_IN}" -lt 1 ]; then FAN_RETRY_IN=1; fi
    bashio::log.error "Cannot control the fan: $1"
    bashio::log.error "Leaving the line alone and trying again in 5 minutes."
}

# Pick how to drive the line. Safe to call again.
fan_control_init() {
    # A helper from an earlier run of this daemon would hold the fan off with
    # nothing watching the temperature.
    pkill -x "${FAN_UART_HOLD##*/}" 2>/dev/null || true

    if ! fan_line_owned_by_kernel; then
        FAN_CONTROL="gpio"
        return 0
    fi

    if [ "${GPIO_LINE}" = "${FAN_UART_LINE}" ] && [ -c "${FAN_UART_DEVICE}" ] && [ -x "${FAN_UART_HOLD}" ]; then
        FAN_CONTROL="serial"
        bashio::log.info "The kernel holds GPIO${GPIO_LINE} as a serial pin, so it cannot be claimed as a GPIO (HAOS 18.5 and later)."
        bashio::log.info "Driving the fan through ${FAN_UART_DEVICE} instead: a serial break pulls the line low."
        return 0
    fi

    fan_control_give_up "the kernel holds line ${GPIO_LINE} on ${GPIO_CHIP}, and it is not a pin this app can drive through ${FAN_UART_DEVICE}"
}

fan_set_gpio() {
    local want=$1 attempt=0
    fan_kill_holder
    # A previous container's gpioset can hold the line for a moment.
    while [ "${attempt}" -lt 3 ]; do
        if fan_start_holder gpioset -c "${GPIO_CHIP}" -C "hay_cm5_fan" "${GPIO_LINE}=${want}"; then
            FAN_STATE=${want}
            if [ "${want}" = "1" ]; then
                bashio::log.info "Fan turned ON (gpioset PID: ${FAN_HOLDER_PID})"
            else
                bashio::log.info "Fan turned OFF (gpioset PID: ${FAN_HOLDER_PID})"
            fi
            return 0
        fi
        attempt=$((attempt + 1))
        if [ "${attempt}" -lt 3 ]; then
            bashio::log.warning "GPIO line busy, retry ${attempt}/3..."
            sleep 2
        fi
    done
    fan_control_give_up "gpioset could not hold line ${GPIO_LINE} on ${GPIO_CHIP}"
}

fan_set_serial() {
    local want=$1
    fan_kill_holder
    if [ "${want}" = "1" ]; then
        FAN_STATE=1
        bashio::log.info "Fan turned ON (serial break released)"
        return 0
    fi
    if fan_start_holder "${FAN_UART_HOLD}" "${FAN_UART_DEVICE}"; then
        FAN_STATE=0
        bashio::log.info "Fan turned OFF (serial break held by PID ${FAN_HOLDER_PID})"
        return 0
    fi
    fan_control_give_up "could not hold a serial break on ${FAN_UART_DEVICE}"
}

set_fan() {
    local want=$1
    if [ "${want}" = "${FAN_STATE}" ]; then
        return 0
    fi
    case "${FAN_CONTROL}" in
        gpio)   fan_set_gpio "${want}" ;;
        serial) fan_set_serial "${want}" ;;
        *)      return 0 ;;
    esac
}

# Once per poll cycle: notice a lost helper, and look for a way again after
# giving up.
fan_control_watchdog() {
    case "${FAN_CONTROL}" in
        gpio)
            if [ -n "${FAN_HOLDER_PID}" ] && ! fan_holder_alive; then
                bashio::log.warning "gpioset process died unexpectedly, restarting with fan ON..."
                FAN_STATE=""
                set_fan 1
            fi
            ;;
        serial)
            if [ "${FAN_STATE}" = "0" ] && ! fan_holder_alive; then
                bashio::log.warning "The serial break was released unexpectedly; the fan is running again."
                FAN_HOLDER_PID=""
                FAN_STATE=1
            fi
            ;;
        *)
            FAN_RETRY_IN=$((FAN_RETRY_IN - 1))
            if [ "${FAN_RETRY_IN}" -le 0 ]; then
                FAN_STATE=""
                fan_control_init
                set_fan 1
            fi
            ;;
    esac
}

# $1 = true to leave the fan on, false to leave it off.
fan_control_shutdown() {
    local leave_on=$1
    fan_kill_holder
    case "${FAN_CONTROL}" in
        gpio)
            if [ "${leave_on}" = "true" ]; then
                bashio::log.info "Leaving fan ON for safety"
                gpioset -c "${GPIO_CHIP}" -C "hay_cm5_fan" "${GPIO_LINE}=1" &
            else
                bashio::log.info "Turning fan OFF"
                gpioset -c "${GPIO_CHIP}" -C "hay_cm5_fan" "${GPIO_LINE}=0" &
            fi
            sleep 0.2
            ;;
        serial)
            if [ "${leave_on}" = "true" ]; then
                bashio::log.info "Leaving fan ON for safety"
            else
                bashio::log.info "The fan stays ON: through the serial port it can only be held off while this app runs"
            fi
            ;;
        *)
            ;;
    esac
}
