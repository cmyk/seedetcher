#!/bin/sh

#set -x  # Enable debug mode

mkdir -p /log
chmod 777 /log

echo "DEBUG MARKER: Init script has started" | tee -a /log/init_debug.log > /dev/ttyGS0

set -m  # Enable job control

#redirecting libcamera logs
export LIBCAMERA_LOG_LEVEL=ERROR
export LIBCAMERA_LOG_OUTPUT=/log/libcamera.log

mount -t devtmpfs devtmpfs /dev
mount -t proc none /proc
mkdir -p /dev/pts
mount -t sysfs none /sys
mount -t devpts none /dev/pts

# Send debug messages to ttyGS0 but avoid affecting exec shell output
debug_echo() {
    echo "DEBUG: $1" >> /log/init_debug.log
}

# Fix DRM framebuffer permissions
debug_echo "Fixing /dev/dri permissions..."
mkdir -p /dev/dri
chmod 777 /dev/dri
chmod 777 /dev/dri/* 2>/dev/null || true

timeout=10  # Max wait time in seconds

# Ensure /dev/ttyGS0 is available for shell
while [ ! -c /dev/ttyGS0 ] && [ $timeout -gt 0 ]; do
    debug_echo "Waiting for /dev/ttyGS0..."
    sleep 1
    timeout=$((timeout - 1))
done

# Reset timeout
timeout=10

# Ensure /dev/ttyGS1 is available for controller input
while [ ! -c /dev/ttyGS1 ] && [ $timeout -gt 0 ]; do
    debug_echo "Waiting for /dev/ttyGS1..."
    sleep 1
    timeout=$((timeout - 1))
done

debug_echo "USB serial devices detected!"

# setting correct permissions
chmod 666 /dev/ttyGS*

debug_echo "Setting USBDEV1 to raw mode..."
stty -F /dev/ttyGS1 raw -echo
echo "" > /dev/ttyGS1

debug_echo "Starting controller..."
/controller < /dev/ttyGS1 >> /log/debug.log 2>&1 &  # RUN IN BACKGROUND!


# Wait until the controller process is fully running
while ! pidof controller > /dev/null; do
    sleep 1
done

trap "echo 'Caught SIGINT! Exiting...'; exit 1" SIGINT

#again sending controller input to make it available on ttyACM1
#echo "ping" > /dev/ttyGS1 &


touch ~/.shrc
chmod 644 ~/.shrc

echo 'alias cat="timeout 10 cat"' >> ~/.shrc
echo 'alias cattty0="timeout 10 cat /dev/ttyGS0"' >> ~/.shrc
echo 'alias cattty1="timeout 10 cat /dev/ttyGS1"' >> ~/.shrc

export ENV=~/.shrc

debug_echo "Init finished. Starting shell..."

cat /dev/ttyGS0 > /log/serial_dump.log 2>&1 &
sleep 2
killall cat

stty -F /dev/ttyGS0 sane
echo "" > /dev/ttyGS0

# Flush any junk input before starting the shell
while read -t 0.1 junk; do :; done < /dev/ttyGS0
echo "reset" > /dev/ttyGS0
sleep 0.1

exec /bin/sh -i < /dev/ttyGS0 > /dev/ttyGS0 2>&1