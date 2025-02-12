#!/bin/sh

set -x  # Enable debug mode

mkdir -p /log
chmod 777 /log

echo "DEBUG MARKER: Init script has started" | tee -a /log/init_debug.log > /dev/ttyGS0

set -m  # Enable job control

mount -t devtmpfs devtmpfs /dev
mount -t proc none /proc
mkdir -p /dev/pts
mount -t sysfs none /sys
mount -t devpts none /dev/pts

# Send debug messages to ttyGS0 but avoid affecting exec shell output
debug_echo() {
    echo "DEBUG: $1" | tee -a /log/init_debug.log > /dev/ttyGS0
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

stty -F /dev/ttyGS0 sane
stty -F /dev/ttyGS1 sane

# setting correct permissions
chmod 666 /dev/ttyGS*

# Pre-fill FIFO before starting the controller
debug_echo "Pre-filling FIFO to unblock controller..."
echo "" > /dev/ttyGS1 &

debug_echo "Starting controller..."
/controller < /dev/ttyGS1 > /dev/ttyGS1 2>&1 &  # RUN IN BACKGROUND!

# Wait until the controller process is fully running
while ! pidof controller > /dev/null; do
    sleep 1
done

trap "echo 'Caught SIGINT! Exiting...'; exit 1" SIGINT

#again sending controller input to make it available on ttyACM1
echo "ping" > /dev/ttyGS1 &


touch ~/.shrc
chmod 644 ~/.shrc
touch ~/.profile
chmod 644 ~/.profile

echo 'alias cat="timeout 10 cat"' >> ~/.shrc
echo 'alias cattty0="timeout 10 cat /dev/ttyGS0"' >> ~/.shrc
echo 'alias cattty1="timeout 10 cat /dev/ttyGS1"' >> ~/.shrc
echo 'export ENV=~/.shrc' >> ~/.profile


debug_echo "Init finished. Starting shell..."
exec /bin/sh -i < /dev/ttyGS0 > /dev/ttyGS0 2>&1


