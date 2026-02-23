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
    # shellcheck source=/dev/null
    . /cups-spike.env

    # Optional drop-in payload from boot partition for drivers/filters not in image closure.
    # Expected archive layout:
    #   lib/cups/... and optionally share/cups/model or share/ppd
    BRLASER_DROPIN_ROOT=""
    mkdir -p /mnt/boot /var/cups-extra
    if [ -b /dev/mmcblk0p1 ]; then
        mount -t vfat /dev/mmcblk0p1 /mnt/boot >/dev/null 2>&1 || true
        for CAND in /mnt/boot/brlaser-root.tar.gz /mnt/boot/brlaser-root.tgz /mnt/boot/brlaser-root.tar; do
            if [ -f "$CAND" ]; then
                rm -rf /var/cups-extra/brlaser-root
                mkdir -p /var/cups-extra/brlaser-root
                if tar -xf "$CAND" -C /var/cups-extra/brlaser-root >/dev/null 2>&1; then
                    BRLASER_DROPIN_ROOT=/var/cups-extra/brlaser-root
                    # Support archives that include an extra top-level directory.
                    if [ -d /var/cups-extra/brlaser-root/brlaser-root ]; then
                        BRLASER_DROPIN_ROOT=/var/cups-extra/brlaser-root/brlaser-root
                    fi
                    debug_echo "CUPS spike: loaded brlaser drop-in from $(basename "$CAND")"
                else
                    debug_echo "CUPS spike: failed to extract brlaser drop-in $(basename "$CAND")"
                fi
                break
            fi
        done
        umount /mnt/boot >/dev/null 2>&1 || true
    fi

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

    # Copy CUPS serverbin to writable storage.
    mkdir -p /var/cups-serverbin/lib
    rm -rf /var/cups-serverbin/lib/cups
    if [ -d "${CUPS_BIN_ROOT}/lib/cups" ]; then
        cp -a "${CUPS_BIN_ROOT}/lib/cups" /var/cups-serverbin/lib/
    fi
    # Overlay optional filters/drivers from brlaser/cups-filters.
    for EXTRA_ROOT in "${BRLASER_ROOT:-}" "$BRLASER_DROPIN_ROOT" "${CUPS_FILTERS_ROOT:-}"; do
        [ -n "$EXTRA_ROOT" ] || continue
        if [ -d "$EXTRA_ROOT/lib/cups" ]; then
            cp -a "$EXTRA_ROOT/lib/cups/." /var/cups-serverbin/lib/cups/ 2>/dev/null || true
        fi
    done
    if [ -d /var/cups-serverbin/lib/cups ]; then
        chown -R root:root /var/cups-serverbin/lib/cups || true
        find /var/cups-serverbin/lib/cups -type d -exec chmod 755 {} \; 2>/dev/null || true
        find /var/cups-serverbin/lib/cups -type f -exec chmod 555 {} \; 2>/dev/null || true
    fi
    if [ -d /var/cups-serverbin/lib/cups/backend ]; then
        chmod 700 /var/cups-serverbin/lib/cups/backend/* 2>/dev/null || true
    fi

    # Build writable CUPS data dir and merge optional models/PPDs.
    CUPS_RUNTIME_DATA=/var/cups-data
    rm -rf "$CUPS_RUNTIME_DATA"
    mkdir -p "$CUPS_RUNTIME_DATA"
    if [ -d "${CUPS_DATA_ROOT}/share/cups" ]; then
        cp -a "${CUPS_DATA_ROOT}/share/cups/." "$CUPS_RUNTIME_DATA/" 2>/dev/null || true
    fi
    for EXTRA_ROOT in "${BRLASER_ROOT:-}" "$BRLASER_DROPIN_ROOT" "${CUPS_FILTERS_ROOT:-}"; do
        [ -n "$EXTRA_ROOT" ] || continue
        if [ -d "$EXTRA_ROOT/share/cups/drv" ]; then
            mkdir -p "$CUPS_RUNTIME_DATA/drv"
            cp -a "$EXTRA_ROOT/share/cups/drv/." "$CUPS_RUNTIME_DATA/drv/" 2>/dev/null || true
        fi
        if [ -d "$EXTRA_ROOT/share/cups/model" ]; then
            mkdir -p "$CUPS_RUNTIME_DATA/model"
            cp -a "$EXTRA_ROOT/share/cups/model/." "$CUPS_RUNTIME_DATA/model/" 2>/dev/null || true
        fi
        if [ -d "$EXTRA_ROOT/share/ppd" ]; then
            mkdir -p "$CUPS_RUNTIME_DATA/model"
            cp -a "$EXTRA_ROOT/share/ppd/." "$CUPS_RUNTIME_DATA/model/" 2>/dev/null || true
        fi
    done

    # Prebuilt drop-ins can carry Nix store paths from a different build host.
    # If so, make compatibility symlinks so execv does not fail with ENOENT.
    repair_elf_runtime() {
        ELF_BIN="$1"
        [ -x "$ELF_BIN" ] || return 0
        [ -x /bin/readelf ] || return 0

        # Repair missing program interpreter (musl loader).
        INTERP_PATH="$(/bin/readelf -l "$ELF_BIN" 2>/dev/null | sed -n 's@.*Requesting program interpreter: \(.*\)]@\1@p' | head -n 1)"
        if [ -n "$INTERP_PATH" ] && [ ! -e "$INTERP_PATH" ] && [ -e /lib/ld-musl-armhf.so.1 ]; then
            mkdir -p "$(dirname "$INTERP_PATH")"
            ln -snf /lib/ld-musl-armhf.so.1 "$INTERP_PATH" 2>/dev/null || true
            debug_echo "CUPS spike: repaired interp for $(basename "$ELF_BIN") -> $INTERP_PATH"
        fi

        # Repair missing RUNPATH directories (cups/libstdc++/musl).
        RUNPATHS="$(/bin/readelf -d "$ELF_BIN" 2>/dev/null | sed -n 's@.*Library runpath: \[\(.*\)\]@\1@p' | head -n 1)"
        [ -n "$RUNPATHS" ] || return 0

        GCC_LIB_PATH="$(find /nix/store -path '*gcc*lib*/armv6l-unknown-linux-musleabihf/lib' 2>/dev/null | head -n 1)"
        CUPS_LIB_PATH=""
        if [ -d "${CUPS_BIN_ROOT}/lib" ]; then
            CUPS_LIB_PATH="${CUPS_BIN_ROOT}/lib"
        elif [ -d "${CUPS_BIN_ROOT}-lib/lib" ]; then
            CUPS_LIB_PATH="${CUPS_BIN_ROOT}-lib/lib"
        fi

        OLDIFS="$IFS"
        IFS=':'
        for RP in $RUNPATHS; do
            [ -n "$RP" ] || continue
            [ -e "$RP" ] && continue
            TARGET=""
            case "$RP" in
                *musl*"/lib")
                    TARGET="/lib"
                    ;;
                *cups*"/lib")
                    TARGET="$CUPS_LIB_PATH"
                    ;;
                *gcc*"/armv6l-unknown-linux-musleabihf/lib")
                    TARGET="$GCC_LIB_PATH"
                    ;;
            esac
            if [ -n "$TARGET" ] && [ -d "$TARGET" ]; then
                mkdir -p "$(dirname "$RP")"
                rm -rf "$RP" 2>/dev/null || true
                ln -snf "$TARGET" "$RP" 2>/dev/null || true
                debug_echo "CUPS spike: repaired runpath for $(basename "$ELF_BIN") -> $RP"
            fi
        done
        IFS="$OLDIFS"
    }

    # Apply runtime-path repair to all drop-in filters.
    if [ -d /var/cups-serverbin/lib/cups/filter ]; then
        for F in /var/cups-serverbin/lib/cups/filter/*; do
            [ -f "$F" ] || continue
            repair_elf_runtime "$F"
        done
    fi
    # brlaser drop-in ships .drv; expose concrete models by generating PPDs at boot.
    BRLASER_DRV="$CUPS_RUNTIME_DATA/drv/brlaser.drv"
    if [ -x /bin/ppdc ]; then
        debug_echo "CUPS spike: ppdc available"
    else
        debug_echo "CUPS spike: ppdc missing"
    fi
    if [ -f "$BRLASER_DRV" ]; then
        debug_echo "CUPS spike: brlaser drv found at $BRLASER_DRV"
    else
        debug_echo "CUPS spike: brlaser drv missing at $BRLASER_DRV"
    fi
    if [ -f "$BRLASER_DRV" ] && [ -x /bin/ppdc ]; then
        mkdir -p "$CUPS_RUNTIME_DATA/model"
        # Keep this bounded; if generation fails/hangs, continue with raw queue path.
        /bin/timeout 10 /bin/ppdc -d "$CUPS_RUNTIME_DATA/model" "$BRLASER_DRV" >/dev/null 2>&1 || \
            debug_echo "CUPS spike: ppdc model generation timed out/failed"
        MODEL_COUNT="$(find "$CUPS_RUNTIME_DATA/model" -type f 2>/dev/null | wc -l | tr -d ' ')"
        debug_echo "CUPS spike: model file count after ppdc=$MODEL_COUNT"
        FIRST_MODELS="$(find "$CUPS_RUNTIME_DATA/model" -type f 2>/dev/null | head -n 3 | tr '\n' ';')"
        [ -n "$FIRST_MODELS" ] && debug_echo "CUPS spike: model sample=$FIRST_MODELS"
    fi

    # Force a minimal, valid cups-files.conf for this spike.
    cat > /etc/cups/cups-files.conf <<EOF
SystemGroup root lpadmin
FileDevice Yes
RequestRoot /var/spool/cups
ServerRoot /etc/cups
CacheDir /var/cache/cups
DataDir ${CUPS_RUNTIME_DATA}
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

    # One-command UART-friendly spike test runner.
    cat > /bin/cups-spike-selftest <<'EOF'
#!/bin/sh
SOCK=/var/run/cups/cups.sock
echo "[1] Queues"
lpstat -h "$SOCK" -p -v
echo

echo "[2] Raw queue smoke test"
printf '\033ESE raw selftest\r\n\f\033%%-12345X' > /tmp/cups-raw-test.pcl
lp -h "$SOCK" -d test -o raw /tmp/cups-raw-test.pcl || true
echo

echo "[3] HBP queue smoke test (if present)"
if lpstat -h "$SOCK" -p test-hbp >/dev/null 2>&1; then
  cat > /tmp/cups-hbp-test.ps <<'PS'
%!PS
/Helvetica findfont 24 scalefont setfont
72 720 moveto
(SE HBP SELFTEST) show
showpage
PS
  if command -v gs >/dev/null 2>&1; then
    gs -q -dSAFER -dBATCH -dNOPAUSE -sDEVICE=cups -sOutputFile=/tmp/cups-hbp-test.ras /tmp/cups-hbp-test.ps
    lp -h "$SOCK" -d test-hbp -o document-format=application/vnd.cups-raster /tmp/cups-hbp-test.ras || true
  else
    echo "gs not found; skipping raster generation"
  fi
else
  echo "test-hbp queue missing; skipping"
fi
echo

echo "[4] Recent jobs"
lpstat -h "$SOCK" -W not-completed -W completed
echo

echo "[5] Last 60 CUPS log lines"
tail -n 60 /var/log/cups/error_log
EOF
    chmod 755 /bin/cups-spike-selftest
    brlaser_filter_usable() {
        FILTER_BIN="/var/cups-serverbin/lib/cups/filter/rastertobrlaser"
        [ -x "$FILTER_BIN" ] || return 1
        # If exec fails (ENOENT/ABI mismatch), timeout/return code will indicate unusable.
        /bin/timeout 2 "$FILTER_BIN" >/dev/null 2>&1
        RC="$?"
        [ "$RC" -ne 126 ] && [ "$RC" -ne 127 ]
    }

    provision_spike_queue() {
        # Return 0 when queue is configured, 1 otherwise.
        [ -x /bin/lpadmin ] || return 1
        [ -S /var/run/cups/cups.sock ] || return 1

        QUEUE_URI=""
        USB_BACKEND="/var/cups-serverbin/lib/cups/backend/usb"
        if [ -x "$USB_BACKEND" ]; then
            QUEUE_URI="$("$USB_BACKEND" 2>/dev/null | awk '$1=="direct" && $2 ~ /^usb:\/\// {print $2; exit}')"
        fi
        if [ -z "$QUEUE_URI" ] && [ -c /dev/usb/lp0 ]; then
            QUEUE_URI="file:///dev/usb/lp0"
        fi
        [ -n "$QUEUE_URI" ] || return 1

        /bin/lpadmin -h /var/run/cups/cups.sock -x test >/dev/null 2>&1 || true
        /bin/lpadmin -h /var/run/cups/cups.sock -p test -E -v "$QUEUE_URI" -m raw >> /log/cups.log 2>&1 || return 1
        debug_echo "CUPS spike: queue test configured uri=$QUEUE_URI (raw)"

        # Optional non-raw queue via generated PPD first, then model lookup.
        if ! brlaser_filter_usable; then
            debug_echo "CUPS spike: rastertobrlaser not executable; skipping test-hbp queue"
            return 0
        fi

        PPD=""
        if [ -d "$CUPS_RUNTIME_DATA/model" ]; then
            PPD="$(find "$CUPS_RUNTIME_DATA/model" -type f \( -iname '*HL-L5000D*.ppd' -o -iname '*HL-L5000D*.ppd.gz' \) | head -n 1)"
            if [ -z "$PPD" ]; then
                PPD="$(find "$CUPS_RUNTIME_DATA/model" -type f \( -iname '*Brother*HL-*.ppd' -o -iname '*Brother*HL-*.ppd.gz' \) | head -n 1)"
            fi
        fi
        MODEL=""
        if [ -z "$PPD" ] && [ -x /bin/lpinfo ]; then
            MODEL="$(/bin/lpinfo -h /var/run/cups/cups.sock -m 2>/dev/null | awk 'toupper($0) ~ /HL-L5000D/ {print $1; exit}')"
            if [ -z "$MODEL" ]; then
                MODEL="$(/bin/lpinfo -h /var/run/cups/cups.sock -m 2>/dev/null | awk '/Brother/ && /HL-/ {print $1; exit}')"
            fi
        fi
        if [ -n "$PPD" ]; then
            /bin/lpadmin -h /var/run/cups/cups.sock -x test-hbp >/dev/null 2>&1 || true
            /bin/lpadmin -h /var/run/cups/cups.sock -p test-hbp -E -v "$QUEUE_URI" -P "$PPD" >> /log/cups.log 2>&1 || true
            debug_echo "CUPS spike: queue test-hbp configured ppd=$PPD"
        elif [ -n "$MODEL" ]; then
            /bin/lpadmin -h /var/run/cups/cups.sock -x test-hbp >/dev/null 2>&1 || true
            /bin/lpadmin -h /var/run/cups/cups.sock -p test-hbp -E -v "$QUEUE_URI" -m "$MODEL" >> /log/cups.log 2>&1 || true
            debug_echo "CUPS spike: queue test-hbp configured model=$MODEL"
        else
            debug_echo "CUPS spike: no brlaser model found for non-raw queue"
        fi
        return 0
    }

    # Start CUPS and provision a raw queue. Prefer usb:// backend discovery, then fallback.
    if /bin/cupsd >> /log/cups.log 2>&1; then
        # Try immediate provisioning first.
        if ! provision_spike_queue; then
            debug_echo "CUPS spike: no printer URI discovered at boot; starting retry loop"
            # Printer can enumerate after boot; retry for up to 3 minutes in background.
            (
                retries=90
                while [ "$retries" -gt 0 ]; do
                    sleep 2
                    if provision_spike_queue; then
                        exit 0
                    fi
                    retries=$((retries - 1))
                done
                debug_echo "CUPS spike: queue provisioning timed out (no URI discovered)"
            ) &
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
