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

# Pick a debug sink as early as possible
DEBUG_TTY="/dev/ttyAMA0"
[ -c /dev/ttyGS0 ] && DEBUG_TTY="/dev/ttyGS0"
[ -c /dev/tty1 ] && [ ! -c "$DEBUG_TTY" ] && DEBUG_TTY="/dev/tty1"

debug_echo() {
    msg="DEBUG: $1"
    echo "$msg" >> /log/init_debug.log
    echo "$msg" > /dev/kmsg
    if [ -c "$DEBUG_TTY" ]; then
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
file /controller >> /log/init_debug.log 2>&1
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
    stty -F "$SHELL_TTY" sane 2>/dev/null || true
    echo "" > "$SHELL_TTY"
    # Flush any junk input before starting the shell
    while read -t 0.1 junk; do :; done < "$SHELL_TTY"
    echo "reset" > "$SHELL_TTY"
    sleep 0.1
    debug_echo "Launching shell on $SHELL_TTY..."
    echo "seedetcher init: shell on $SHELL_TTY" > "$SHELL_TTY"
    if command -v setsid >/dev/null 2>&1; then
        exec setsid -c /bin/sh -i < "$SHELL_TTY" > "$SHELL_TTY" 2>&1 || echo "Failed to launch shell" > "$SHELL_TTY"
    else
        exec /bin/sh -i < "$SHELL_TTY" > "$SHELL_TTY" 2>&1 || echo "Failed to launch shell" > "$SHELL_TTY"
    fi
else
    debug_echo "No shell TTY found; exec'ing controller only"
    exec /controller >> /log/debug.log 2>> /log/debug.log
fi
