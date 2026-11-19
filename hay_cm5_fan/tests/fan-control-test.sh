#!/usr/bin/env bash
# Tests rootfs/usr/share/hay_cm5_fan/fan-control.sh without hardware.
#
#   hay_cm5_fan/tests/fan-control-test.sh
#
# gpioinfo and gpioset are fakes steered through files. fan-uart-hold is the
# real helper, built for this machine and pointed at a pseudo-terminal, which
# accepts a break and ignores it. Needs bash, go, python3 and procps (pgrep -c).
# Runs under the same strict mode bashio gives the daemon.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP="$(dirname "${HERE}")"
WORK="$(mktemp -d)"
PASS=0
FAIL=0

cleanup() {
    pkill -P $$ 2>/dev/null || true
    rm -rf "${WORK}"
}
trap cleanup EXIT

# --- fakes -------------------------------------------------------------------
mkdir -p "${WORK}/bin"
cat > "${WORK}/bin/gpioinfo" <<EOF
#!/bin/sh
case "\$(cat "${WORK}/line" 2>/dev/null)" in
    free)   printf '\tline  14:\t"GPIO14"        \tinput\n\tline  17:\t"GPIO17"        \tinput\n' ;;
    kernel) printf '\tline  14:\t"GPIO14"        \tinput consumer="kernel"\n\tline  17:\t"GPIO17"        \tinput consumer="kernel"\n' ;;
    *)      echo "gpioinfo: unable to open chip" >&2; exit 1 ;;
esac
EOF
cat > "${WORK}/bin/gpioset" <<EOF
#!/bin/sh
echo "\$*" >> "${WORK}/gpioset.calls"
if [ "\$(cat "${WORK}/gpioset" 2>/dev/null)" = ok ]; then
    exec sleep 600
fi
echo "gpioset: unable to request lines on chip '/dev/gpiochip0': Invalid argument" >&2
exit 1
EOF
printf '#!/bin/sh\nexit 1\n' > "${WORK}/bin/broken-hold"
chmod +x "${WORK}/bin/"*
(cd "${APP}/fan-uart-hold" && go build -o "${WORK}/bin/fan-uart-hold" .)

# A pseudo-terminal that stays open for the whole run.
python3 -c 'import os, pty, time; m, s = pty.openpty(); print(os.ttyname(s), flush=True); time.sleep(600)' > "${WORK}/pty" &
for _ in $(seq 1 50); do [ -s "${WORK}/pty" ] && break; sleep 0.1; done
PTY="$(cat "${WORK}/pty")"

export PATH="${WORK}/bin:${PATH}"
LOG="${WORK}/log"
bashio::log.info()    { echo "INFO  $*" >> "${LOG}"; }
bashio::log.warning() { echo "WARN  $*" >> "${LOG}"; }
bashio::log.error()   { echo "ERROR $*" >> "${LOG}"; }

# --- helpers -----------------------------------------------------------------
check() {
    if "${@:2}"; then
        PASS=$((PASS + 1)); echo "  ok   $1"
    else
        FAIL=$((FAIL + 1)); echo "  FAIL $1"
    fi
}
is()        { [ "$1" = "$2" ]; }
alive()     { fan_holder_alive; }
not_alive() { ! fan_holder_alive; }
holder_is() { [ "$(cat "/proc/${FAN_HOLDER_PID}/comm" 2>/dev/null)" = "$1" ]; }
logged()    { grep -q "$1" "${LOG}"; }
count_is()  { [ "$(grep -c "$1" "${LOG}" || true)" = "$2" ]; }

# Fresh state for each case. $1 = line state, $2 = gpioset behaviour.
begin() {
    echo "$3"
    fan_kill_holder 2>/dev/null || true
    pkill -x fan-uart-hold 2>/dev/null || true
    : > "${LOG}"; : > "${WORK}/gpioset.calls"
    echo "$1" > "${WORK}/line"; echo "$2" > "${WORK}/gpioset"
    GPIO_CHIP="gpiochip0"; GPIO_LINE=14; POLL_INTERVAL=5
    FAN_UART_DEVICE="${PTY}"; FAN_UART_HOLD="${WORK}/bin/fan-uart-hold"
    # shellcheck source=/dev/null
    . "${APP}/rootfs/usr/share/hay_cm5_fan/fan-control.sh"
}

# --- cases -------------------------------------------------------------------
begin free ok "HAOS up to 18.4: the line is free, gpioset holds it"
fan_control_init; set_fan 1
check "uses gpio"                        is "${FAN_CONTROL}" gpio
check "fan on, gpioset holding"          is "${FAN_STATE}" 1
check "gpioset is the holder"            alive
set_fan 0
check "fan off"                          is "${FAN_STATE}" 0
check "gpioset asked for line=0"         grep -q '14=0' "${WORK}/gpioset.calls"
kill "${FAN_HOLDER_PID}"; sleep 0.1
fan_control_watchdog
check "lost gpioset: back on"            is "${FAN_STATE}" 1
check "lost gpioset: warned"             logged "died unexpectedly"
fan_control_shutdown true
check "shutdown leaves it on"            logged "Leaving fan ON"

begin kernel fail "HAOS 18.5: the kernel owns GPIO14, so the serial port is used"
fan_control_init; set_fan 1
check "uses serial"                      is "${FAN_CONTROL}" serial
check "fan on with no helper"            not_alive
check "gpioset never tried"              is "$(wc -l < "${WORK}/gpioset.calls")" 0
set_fan 0
check "fan off"                          is "${FAN_STATE}" 0
check "fan-uart-hold is the holder"      holder_is fan-uart-hold
set_fan 0
check "repeat off keeps one helper"      is "$(pgrep -cx fan-uart-hold)" 1
set_fan 1
check "fan on again, helper gone"        is "$(pgrep -cx fan-uart-hold || true)" 0
set_fan 0; kill -9 "${FAN_HOLDER_PID}"; sleep 0.1
fan_control_watchdog
check "helper killed: reported on"       is "${FAN_STATE}" 1
check "helper killed: warned once"       count_is "released unexpectedly" 1
set_fan 0
check "can be switched off again"        holder_is fan-uart-hold
fan_control_shutdown false
check "shutdown releases the break"      is "$(pgrep -cx fan-uart-hold || true)" 0
check "shutdown says the fan stays on"   logged "stays ON"

begin kernel fail "A helper left over from an earlier run is removed at start"
"${FAN_UART_HOLD}" "${PTY}" & sleep 0.3
check "stale helper is running"          is "$(pgrep -cx fan-uart-hold)" 1
fan_control_init
sleep 0.2
check "init removed it"                  is "$(pgrep -cx fan-uart-hold || true)" 0

begin kernel fail "The helper cannot hold the port"
FAN_UART_HOLD="${WORK}/bin/broken-hold"
fan_control_init; set_fan 1; set_fan 0
check "gives up"                         is "${FAN_CONTROL}" none
check "reports the fan on"               is "${FAN_STATE}" 1
check "one error, not a loop"            count_is "Cannot control the fan" 1
for _ in $(seq 1 59); do fan_control_watchdog; done
check "quiet while waiting"              count_is "Cannot control the fan" 1
FAN_UART_HOLD="${WORK}/bin/fan-uart-hold"
fan_control_watchdog
check "tries again after 5 minutes"      is "${FAN_CONTROL}" serial

begin kernel fail "The kernel owns a line that is not the serial pin"
GPIO_LINE=17
fan_control_init; set_fan 1; set_fan 0
check "gives up"                         is "${FAN_CONTROL}" none
check "no helper started"                is "$(pgrep -cx fan-uart-hold || true)" 0

begin missing fail "No GPIO chip at all (CI)"
fan_control_init; set_fan 1
check "tried gpio three times"           is "$(wc -l < "${WORK}/gpioset.calls")" 3
check "then gives up"                    is "${FAN_CONTROL}" none
: > "${WORK}/gpioset.calls"
for _ in $(seq 1 30); do fan_control_watchdog; set_fan 1; set_fan 0; done
check "no retry storm"                   is "$(wc -l < "${WORK}/gpioset.calls")" 0

echo
echo "${PASS} passed, ${FAIL} failed"
[ "${FAIL}" -eq 0 ]
