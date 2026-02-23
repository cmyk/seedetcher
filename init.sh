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

# Optional CUPS spike bootstrap.
if [ -f /cups-spike.env ]; then
    mkdir -p /nix /etc/cups /var/run/cups /var/spool/cups /var/log/cups
    tries=10
    while [ "$tries" -gt 0 ] && [ ! -b /dev/mmcblk0p2 ]; do
        sleep 1
        tries=$((tries - 1))
    done
    if [ -b /dev/mmcblk0p2 ]; then
        mount -t ext4 /dev/mmcblk0p2 /nix >> /log/cups.log 2>&1 || debug_echo "CUPS spike: failed to mount /dev/mmcblk0p2 on /nix"
    else
        debug_echo "CUPS spike: /dev/mmcblk0p2 not found after wait"
    fi
fi
if [ -f /cups-spike.env ] && [ -x /bin/cupsd ]; then
    # Minimal identity DB for CUPS on initramfs-only userspace.
    if [ ! -f /etc/group ]; then
        cat > /etc/group <<'EOF'
root:x:0:
sys:x:3:
lp:x:7:
lpadmin:x:19:
EOF
    fi
    if [ ! -f /etc/passwd ]; then
        cat > /etc/passwd <<'EOF'
root:x:0:0:root:/root:/bin/sh
lp:x:7:7:CUPS:/var/spool/cups:/bin/false
EOF
    fi
    # Ensure expected groups exist.
    grep -q '^lpadmin:' /etc/group || echo 'lpadmin:x:19:' >> /etc/group
    grep -q '^lp:' /etc/group || echo 'lp:x:7:' >> /etc/group
    grep -q '^sys:' /etc/group || echo 'sys:x:3:' >> /etc/group
    grep -q '^root:' /etc/group || echo 'root:x:0:' >> /etc/group
    grep -q '^lp:' /etc/passwd || echo 'lp:x:7:7:CUPS:/var/spool/cups:/bin/false' >> /etc/passwd

    mkdir -p /var/cache/cups /var/spool/cups/tmp /run/cups /var/run/cups /var/log/cups
    chmod 1777 /var/spool/cups/tmp

    # Resolve CUPS roots from actual cupsd binary target.
    CUPS_BIN_ROOT="$(readlink /bin/cupsd 2>/dev/null | sed 's#/bin/cupsd$##')"
    [ -z "$CUPS_BIN_ROOT" ] && CUPS_BIN_ROOT="/nix/store"
    CUPS_DATA_ROOT="$CUPS_BIN_ROOT"
    if [ ! -d "${CUPS_DATA_ROOT}/share/cups" ] && [ -d "${CUPS_BIN_ROOT}-lib/share/cups" ]; then
        CUPS_DATA_ROOT="${CUPS_BIN_ROOT}-lib"
    fi

    # Copy CUPS serverbin to writable storage and enforce ownership/perms expected by cupsd.
    mkdir -p /var/cups-serverbin/lib
    rm -rf /var/cups-serverbin/lib/cups
    if [ -d "${CUPS_BIN_ROOT}/lib/cups" ]; then
        cp -a "${CUPS_BIN_ROOT}/lib/cups" /var/cups-serverbin/lib/
    fi
    if [ -d /var/cups-serverbin/lib/cups ]; then
        chown -R root:root /var/cups-serverbin/lib/cups || true
        find /var/cups-serverbin/lib/cups -type d -exec chmod 755 {} \; 2>/dev/null || true
        find /var/cups-serverbin/lib/cups -type f -exec chmod 555 {} \; 2>/dev/null || true
    fi
    if [ -d /var/cups-serverbin/lib/cups/backend ]; then
        chmod 700 /var/cups-serverbin/lib/cups/backend/* 2>/dev/null || true
    fi

    # Force a minimal, valid cups-files.conf for this spike.
    cat > /etc/cups/cups-files.conf <<EOF
SystemGroup root lpadmin
FileDevice Yes
RequestRoot /var/spool/cups
ServerRoot /etc/cups
CacheDir /var/cache/cups
DataDir ${CUPS_DATA_ROOT}/share/cups
ServerBin /var/cups-serverbin/lib/cups
EOF

    if [ ! -f /etc/cups/cupsd.conf ]; then
        cat > /etc/cups/cupsd.conf <<'EOF'
LogLevel warn
Listen 0.0.0.0:631
Browsing Off
DefaultAuthType None
WebInterface No
<Location />
  Order allow,deny
  Allow all
</Location>
EOF
    fi
    # Start CUPS and provision a raw queue. Prefer usb:// backend discovery, then fallback.
    if /bin/cupsd >> /log/cups.log 2>&1; then
        if [ -x /bin/lpadmin ]; then
            tries=10
            while [ "$tries" -gt 0 ]; do
                if [ -S /var/run/cups/cups.sock ]; then
                    QUEUE_URI=""
                    USB_BACKEND="/var/cups-serverbin/lib/cups/backend/usb"
                    if [ -x "$USB_BACKEND" ]; then
                        QUEUE_URI="$("$USB_BACKEND" 2>/dev/null | awk '$1=="direct" && $2 ~ /^usb:\/\// {print $2; exit}')"
                    fi
                    if [ -z "$QUEUE_URI" ] && [ -c /dev/usb/lp0 ]; then
                        QUEUE_URI="file:///dev/usb/lp0"
                    fi
                    /bin/lpadmin -h /var/run/cups/cups.sock -x test >/dev/null 2>&1 || true
                    if [ -n "$QUEUE_URI" ]; then
                        /bin/lpadmin -h /var/run/cups/cups.sock -p test -E -v "$QUEUE_URI" -m raw >> /log/cups.log 2>&1 || true
                        debug_echo "CUPS spike: queue test configured uri=$QUEUE_URI"
                    else
                        debug_echo "CUPS spike: no printer URI discovered (usb backend + /dev/usb/lp0 fallback failed)"
                    fi
                    break
                fi
                sleep 1
                tries=$((tries - 1))
            done
        fi
        debug_echo "CUPS spike: scheduler started"
    else
        debug_echo "CUPS spike: failed to start cupsd"
    fi
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
