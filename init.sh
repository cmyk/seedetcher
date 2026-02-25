#!/bin/sh

#set -x  # Enable debug mode

mkdir -p /log
chmod 777 /log

echo "DEBUG MARKER: Init script has started" | tee -a /log/init_debug.log >/dev/null

set -m  # Enable job control

# Redirect libcamera logs explicitly and suppress stdout/stderr
export LIBCAMERA_LOG_LEVEL=ERROR
export LIBCAMERA_LOG_FILE=/log/libcamera.log  # Use LIBCAMERA_LOG_FILE instead of LOG_OUTPUT
export LIBCAMERA_LOG_OUTPUT=""  # Clear any default output to terminal
export LIBCAMERA_PROVIDER_LOG=0  # Disable provider logs if supported
export LD_LIBRARY_PATH=/lib

mount -t devtmpfs devtmpfs /dev
mount -t proc none /proc
mkdir -p /dev/pts
mount -t sysfs none /sys
mount -t devpts none /dev/pts
# Disable SysRq to avoid accidental resets/noise on UART
if [ -w /proc/sys/kernel/sysrq ]; then
    echo 0 > /proc/sys/kernel/sysrq
    echo "SysRq=$(cat /proc/sys/kernel/sysrq 2>/dev/null)" >> /log/init_debug.log
fi

# Quiet-by-default debug policy:
# - always log to /log/init_debug.log
# - only mirror to /dev/kmsg when INIT_DEBUG_TO_KMSG=1
# - only mirror to a TTY when INIT_DEBUG_TO_TTY=1
INIT_DEBUG_TO_KMSG="${INIT_DEBUG_TO_KMSG:-0}"
INIT_DEBUG_TO_TTY="${INIT_DEBUG_TO_TTY:-0}"
DEBUG_TTY="${INIT_DEBUG_TTY:-/dev/null}"
if [ -f /cups-spike.env ]; then
    # shellcheck source=/dev/null
    . /cups-spike.env
    INIT_DEBUG_TO_KMSG="${INIT_DEBUG_TO_KMSG:-0}"
    INIT_DEBUG_TO_TTY="${INIT_DEBUG_TO_TTY:-0}"
    DEBUG_TTY="${INIT_DEBUG_TTY:-$DEBUG_TTY}"
fi
if [ "$INIT_DEBUG_TO_TTY" = "1" ] && [ "$DEBUG_TTY" = "/dev/null" ]; then
    if [ -c /dev/ttyGS0 ]; then
        DEBUG_TTY="/dev/ttyGS0"
    elif [ -c /dev/tty1 ]; then
        DEBUG_TTY="/dev/tty1"
    elif [ -c /dev/ttyAMA0 ]; then
        DEBUG_TTY="/dev/ttyAMA0"
    fi
fi

debug_echo() {
    msg="DEBUG: $1"
    echo "$msg" >> /log/init_debug.log
    if [ "$INIT_DEBUG_TO_KMSG" = "1" ]; then
        echo "$msg" > /dev/kmsg 2>/dev/null || true
    fi
    if [ "$INIT_DEBUG_TO_TTY" = "1" ] && [ -c "$DEBUG_TTY" ]; then
        echo "$msg" > "$DEBUG_TTY"
    fi
}

# Prefer gadget TTYs when present; otherwise use UART. Wait up to 30s for the chosen one.
if [ -c /dev/ttyGS0 ]; then
    SHELL_TTY="/dev/ttyGS0"
else
    SHELL_TTY="/dev/ttyAMA0"
fi

if [ -c /dev/ttyGS1 ]; then
    CTRL_TTY="/dev/ttyGS1"
else
    CTRL_TTY="/dev/null"
fi

wait_for() {
    tgt="$1"; shift
    timeout="$1"; shift
    while [ "$timeout" -gt 0 ] && [ ! -c "$tgt" ]; do
        sleep 1
        timeout=$((timeout - 1))
    done
    [ -c "$tgt" ]
}

wait_for "$SHELL_TTY" 30
wait_for "$CTRL_TTY" 30

debug_echo "Shell TTY: ${SHELL_TTY:-none}, Controller TTY: ${CTRL_TTY:-none}"
if [ -f /cups-spike.env ]; then
    debug_echo "CUPS spike: lazy bootstrap mode (boot init skipped)"
fi

# Fix DRM framebuffer permissions
debug_echo "Fixing /dev/dri permissions..."
mkdir -p /dev/dri
chmod 777 /dev/dri
chmod 777 /dev/dri/* 2>/dev/null || true

# Prep controller TTY if present
if [ -c "$CTRL_TTY" ] && [ "$CTRL_TTY" != "/dev/null" ]; then
    chmod 666 "$CTRL_TTY"
    debug_echo "Setting $CTRL_TTY to raw mode..."
    stty -F "$CTRL_TTY" raw -echo
    echo "" > "$CTRL_TTY"
fi

debug_echo "Checking /controller existence and permissions..."
ls -l /controller >> /log/init_debug.log 2>&1
[ -x /bin/file ] && file /controller >> /log/init_debug.log 2>&1
debug_echo "Starting controller..."
# Ensure controller’s stdout/stderr go to log; stdin from controller TTY if present
/controller < "$CTRL_TTY" >> /log/debug.log 2>> /log/debug.log &

# Wait until the controller process is fully running
while ! pidof controller > /dev/null; do
    sleep 1
done

trap "echo 'Caught SIGINT! Exiting...'; exit 1" SIGINT
debug_echo "Controller started with PID $(pidof controller)"

touch ~/.shrc
chmod 644 ~/.shrc

echo 'alias cat="timeout 10 cat"' >> ~/.shrc
echo 'alias cattty0="timeout 10 cat /dev/ttyGS0"' >> ~/.shrc
echo 'alias cattty1="timeout 10 cat /dev/ttyGS1"' >> ~/.shrc

export ENV=~/.shrc

debug_echo "Init finished. Starting shell..."

if [ -n "$SHELL_TTY" ] && [ -c "$SHELL_TTY" ]; then
    debug_echo "Launching getty on $SHELL_TTY..."
    echo "seedetcher init: shell on $SHELL_TTY" > "$SHELL_TTY"
    stty -F "$SHELL_TTY" sane cread clocal 115200 cs8 -parenb -cstopb -ixon -ixoff -echo
    exec /bin/busybox getty -L -n -l /bin/sh 115200 "$SHELL_TTY"
else
    debug_echo "No shell TTY found; exec'ing controller only"
    exec /controller >> /log/debug.log 2>> /log/debug.log
fi
