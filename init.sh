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
    #
    # Load policy:
    # - CUPS_SPIKE_LOAD_DROPIN=1  -> always try loading boot drop-in
    # - CUPS_SPIKE_LOAD_DROPIN=0  -> never load boot drop-in
    # - unset/auto (default)      -> load only when BRLASER_ROOT is not provided
    BRLASER_DROPIN_ROOT=""
    mkdir -p /mnt/boot /var/cups-extra
    BRLASER_RUNTIME_ROOT=/var/cups-extra/brlaser-runtime
    mkdir -p "$BRLASER_RUNTIME_ROOT/filter" "$BRLASER_RUNTIME_ROOT/lib"
    SHOULD_LOAD_DROPIN=0
    case "${CUPS_SPIKE_LOAD_DROPIN:-auto}" in
        1|yes|true|on)
            SHOULD_LOAD_DROPIN=1
            ;;
        0|no|false|off)
            SHOULD_LOAD_DROPIN=0
            ;;
        *)
            [ -z "${BRLASER_ROOT:-}" ] && SHOULD_LOAD_DROPIN=1
            ;;
    esac
    if [ "$SHOULD_LOAD_DROPIN" -eq 1 ] && [ -b /dev/mmcblk0p1 ]; then
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
    elif [ "$SHOULD_LOAD_DROPIN" -eq 0 ]; then
        debug_echo "CUPS spike: drop-in loading disabled"
    else
        debug_echo "CUPS spike: no boot partition for drop-in loading"
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
    # Precedence: in-image BRLASER_ROOT (known ABI) should override drop-in fallback.
    for EXTRA_ROOT in "$BRLASER_DROPIN_ROOT" "${BRLASER_ROOT:-}" "${CUPS_FILTERS_ROOT:-}"; do
        [ -n "$EXTRA_ROOT" ] || continue
        if [ -d "$EXTRA_ROOT/lib/cups" ]; then
            cp -a "$EXTRA_ROOT/lib/cups/." /var/cups-serverbin/lib/cups/ 2>/dev/null || true
        fi
        # Future-proof runtime: stash shared libs from artifact roots for wrapper-based loading.
        if [ -d "$EXTRA_ROOT/lib" ]; then
            find "$EXTRA_ROOT/lib" -maxdepth 2 -type f \( -name '*.so' -o -name '*.so.*' \) -exec cp -a {} "$BRLASER_RUNTIME_ROOT/lib/" \; 2>/dev/null || true
        fi
    done
    if [ -d /var/cups-serverbin/lib/cups ]; then
        # Avoid recursive permission walks at boot; keep this minimal for speed.
        chown root:root /var/cups-serverbin/lib/cups 2>/dev/null || true
        chown root:root /var/cups-serverbin/lib/cups/backend 2>/dev/null || true
        chown root:root /var/cups-serverbin/lib/cups/filter 2>/dev/null || true
        chown root:root /var/cups-serverbin/lib/cups/driver 2>/dev/null || true
        chown root:root /var/cups-serverbin/lib/cups/daemon 2>/dev/null || true
        chmod 755 /var/cups-serverbin/lib/cups 2>/dev/null || true
        chmod 755 /var/cups-serverbin/lib/cups/backend 2>/dev/null || true
        chmod 755 /var/cups-serverbin/lib/cups/filter 2>/dev/null || true
        chmod 755 /var/cups-serverbin/lib/cups/driver 2>/dev/null || true
        chmod 755 /var/cups-serverbin/lib/cups/daemon 2>/dev/null || true
        chown root:root /var/cups-serverbin/lib/cups/backend/* 2>/dev/null || true
        chown root:root /var/cups-serverbin/lib/cups/filter/* 2>/dev/null || true
        chown root:root /var/cups-serverbin/lib/cups/driver/* 2>/dev/null || true
        chown root:root /var/cups-serverbin/lib/cups/daemon/* 2>/dev/null || true
        chmod 555 /var/cups-serverbin/lib/cups/backend/* 2>/dev/null || true
        chmod 555 /var/cups-serverbin/lib/cups/filter/* 2>/dev/null || true
        chmod 555 /var/cups-serverbin/lib/cups/driver/* 2>/dev/null || true
        chmod 555 /var/cups-serverbin/lib/cups/daemon/* 2>/dev/null || true
    fi
    if [ -d /var/cups-serverbin/lib/cups/backend ]; then
        chmod 700 /var/cups-serverbin/lib/cups/backend/* 2>/dev/null || true
    fi

    # Build writable CUPS data dir and merge optional models/PPDs.
    # Default to minimal copy for boot-time speed.
    # Set CUPS_SPIKE_DATA_COPY=full to copy the entire share/cups tree.
    CUPS_RUNTIME_DATA=/var/cups-data
    rm -rf "$CUPS_RUNTIME_DATA"
    mkdir -p "$CUPS_RUNTIME_DATA"
    if [ -d "${CUPS_DATA_ROOT}/share/cups" ]; then
        if [ "${CUPS_SPIKE_DATA_COPY:-minimal}" = "full" ]; then
            cp -a "${CUPS_DATA_ROOT}/share/cups/." "$CUPS_RUNTIME_DATA/" 2>/dev/null || true
        else
            # Minimal set for fast boot + ppdc fallback support.
            # `brlaser.drv` includes ppdc helper defs (for example font.defs).
            for D in mime usb drv ppdc; do
                if [ -d "${CUPS_DATA_ROOT}/share/cups/$D" ]; then
                    mkdir -p "$CUPS_RUNTIME_DATA/$D"
                    cp -a "${CUPS_DATA_ROOT}/share/cups/$D/." "$CUPS_RUNTIME_DATA/$D/" 2>/dev/null || true
                fi
            done
        fi
    fi
    for EXTRA_ROOT in "$BRLASER_DROPIN_ROOT" "${BRLASER_ROOT:-}" "${CUPS_FILTERS_ROOT:-}"; do
        [ -n "$EXTRA_ROOT" ] || continue
        if [ -d "$EXTRA_ROOT/share/cups/drv" ]; then
            mkdir -p "$CUPS_RUNTIME_DATA/drv"
            cp -a "$EXTRA_ROOT/share/cups/drv/." "$CUPS_RUNTIME_DATA/drv/" 2>/dev/null || true
        fi
        if [ -d "$EXTRA_ROOT/share/cups/model" ]; then
            mkdir -p "$CUPS_RUNTIME_DATA/model"
            cp -a "$EXTRA_ROOT/share/cups/model/." "$CUPS_RUNTIME_DATA/model/" 2>/dev/null || true
        fi
        if [ -d "$EXTRA_ROOT/share/cups/ppdc" ]; then
            mkdir -p "$CUPS_RUNTIME_DATA/ppdc"
            cp -a "$EXTRA_ROOT/share/cups/ppdc/." "$CUPS_RUNTIME_DATA/ppdc/" 2>/dev/null || true
        fi
        if [ -d "$EXTRA_ROOT/share/ppd" ]; then
            mkdir -p "$CUPS_RUNTIME_DATA/model"
            cp -a "$EXTRA_ROOT/share/ppd/." "$CUPS_RUNTIME_DATA/model/" 2>/dev/null || true
        fi
    done
    # Ensure runtime data dirs are writable for on-demand PPD generation.
    mkdir -p "$CUPS_RUNTIME_DATA/model" "$CUPS_RUNTIME_DATA/drv" "$CUPS_RUNTIME_DATA/usb" "$CUPS_RUNTIME_DATA/mime" "$CUPS_RUNTIME_DATA/ppdc"
    chown root:root "$CUPS_RUNTIME_DATA" "$CUPS_RUNTIME_DATA/model" "$CUPS_RUNTIME_DATA/drv" "$CUPS_RUNTIME_DATA/usb" "$CUPS_RUNTIME_DATA/mime" "$CUPS_RUNTIME_DATA/ppdc" 2>/dev/null || true
    chmod 755 "$CUPS_RUNTIME_DATA" "$CUPS_RUNTIME_DATA/model" "$CUPS_RUNTIME_DATA/drv" "$CUPS_RUNTIME_DATA/usb" "$CUPS_RUNTIME_DATA/mime" "$CUPS_RUNTIME_DATA/ppdc" 2>/dev/null || true

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

    install_brlaser_wrapper() {
        FILTER_BIN="/var/cups-serverbin/lib/cups/filter/rastertobrlaser"
        RUNTIME_FILTER="$BRLASER_RUNTIME_ROOT/filter/rastertobrlaser.real"
        [ -x "$FILTER_BIN" ] || return 0

        # Preserve the original filter binary under runtime root.
        cp -a "$FILTER_BIN" "$RUNTIME_FILTER" 2>/dev/null || return 0

        cat > "$FILTER_BIN" <<'EOF'
#!/bin/sh
RUNTIME_ROOT="/var/cups-extra/brlaser-runtime"
REAL_FILTER="$RUNTIME_ROOT/filter/rastertobrlaser.real"
[ -x "$REAL_FILTER" ] || exit 127
LD_PATHS="$RUNTIME_ROOT/lib:/lib:/usr/lib"
if [ -n "${BRLASER_LD_LIBRARY_PATH:-}" ]; then
  LD_PATHS="$BRLASER_LD_LIBRARY_PATH:$LD_PATHS"
fi
if [ -n "${LD_LIBRARY_PATH:-}" ]; then
  LD_PATHS="$LD_LIBRARY_PATH:$LD_PATHS"
fi
export LD_LIBRARY_PATH="$LD_PATHS"
exec "$REAL_FILTER" "$@"
EOF
        chmod 555 "$FILTER_BIN" 2>/dev/null || true
        debug_echo "CUPS spike: installed brlaser wrapper at $FILTER_BIN"
    }

    # Repair only needed CUPS executables by default to reduce boot latency.
    if [ "${CUPS_SPIKE_REPAIR_ALL_FILTERS:-0}" = "1" ] && [ -d /var/cups-serverbin/lib/cups/filter ]; then
        for F in /var/cups-serverbin/lib/cups/filter/*; do
            [ -f "$F" ] || continue
            repair_elf_runtime "$F"
        done
        if [ -d /var/cups-serverbin/lib/cups/driver ]; then
            for F in /var/cups-serverbin/lib/cups/driver/*; do
                [ -f "$F" ] || continue
                repair_elf_runtime "$F"
            done
        fi
    else
        repair_elf_runtime /var/cups-serverbin/lib/cups/filter/rastertobrlaser
        # Needed for `lpadmin -m drv:///...` model resolution.
        repair_elf_runtime /var/cups-serverbin/lib/cups/driver/drv
        repair_elf_runtime /var/cups-serverbin/lib/cups/driver/cups-driverd
    fi
    install_brlaser_wrapper
    run_ppdc_brlaser() {
        OUT_DIR="$1"
        DRV_FILE="$2"
        TIMEOUT_SECS="${3:-10}"
        [ -n "$OUT_DIR" ] && [ -n "$DRV_FILE" ] || return 1

        set -- /bin/ppdc -d "$OUT_DIR"
        for INC in "$CUPS_RUNTIME_DATA/ppdc" "$CUPS_RUNTIME_DATA/drv" "$CUPS_DATA_ROOT/share/cups/ppdc"; do
            [ -d "$INC" ] || continue
            set -- "$@" -I "$INC"
        done
        set -- "$@" "$DRV_FILE"
        /bin/timeout "$TIMEOUT_SECS" "$@"
    }

    # brlaser drop-in ships .drv. PPD generation via ppdc is optional and disabled
    # by default to keep boot latency low. Enable only for debugging:
    #   CUPS_SPIKE_ENABLE_PPDC=1
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
    if [ "${CUPS_SPIKE_ENABLE_PPDC:-0}" = "1" ] && [ -f "$BRLASER_DRV" ] && [ -x /bin/ppdc ]; then
        mkdir -p "$CUPS_RUNTIME_DATA/model"
        # Keep this bounded; if generation fails/hangs, continue with raw queue path.
        run_ppdc_brlaser "$CUPS_RUNTIME_DATA/model" "$BRLASER_DRV" 10 >/dev/null 2>&1 || \
            debug_echo "CUPS spike: ppdc model generation timed out/failed"
        MODEL_COUNT="$(find "$CUPS_RUNTIME_DATA/model" -type f 2>/dev/null | wc -l | tr -d ' ')"
        debug_echo "CUPS spike: model file count after ppdc=$MODEL_COUNT"
        FIRST_MODELS="$(find "$CUPS_RUNTIME_DATA/model" -type f 2>/dev/null | head -n 3 | tr '\n' ';')"
        [ -n "$FIRST_MODELS" ] && debug_echo "CUPS spike: model sample=$FIRST_MODELS"
    elif [ "${CUPS_SPIKE_ENABLE_PPDC:-0}" != "1" ]; then
        debug_echo "CUPS spike: ppdc disabled (set CUPS_SPIKE_ENABLE_PPDC=1 to enable)"
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

    # Helper: print a PDF through test-hbp by pre-converting to CUPS raster.
cat > /bin/print-hbp-pdf <<'EOF'
#!/bin/sh
set -eu
SOCK="${CUPS_SERVER_SOCK:-/var/run/cups/cups.sock}"
QUEUE="${HBP_QUEUE:-test-hbp}"
PDF="${1:-}"
DPI="${2:-600}"
if [ -z "$PDF" ] || [ ! -f "$PDF" ]; then
  echo "usage: print-hbp-pdf /path/to/file.pdf [dpi]" >&2
  exit 2
fi
case "$DPI" in
  ''|*[!0-9]*)
    echo "invalid dpi: '$DPI'" >&2
    exit 2
    ;;
esac
if [ "$DPI" -lt 300 ] || [ "$DPI" -gt 1200 ]; then
  echo "dpi out of range: $DPI (expected 300..1200)" >&2
  exit 2
fi

ensure_runtime_tools() {
  command -v lpadmin >/dev/null 2>&1 && command -v lpstat >/dev/null 2>&1 && return 0

  # Recovery path after SD detach: restore /nix from RAM-backed stage, if present.
  if [ -d /run/hbp-ram-runtime/nix/store ]; then
    mount --bind /run/hbp-ram-runtime/nix /nix >/dev/null 2>&1 || true
  fi

  command -v lpadmin >/dev/null 2>&1 && command -v lpstat >/dev/null 2>&1
}

if ! ensure_runtime_tools; then
  echo "print-hbp-pdf: CUPS tools unavailable (likely /nix not mounted). Run 'cups-spike-ram-feasibility stage core' before SD removal." >&2
  exit 4
fi

ensure_printer_connected() {
  ls /dev/usb/lp* >/dev/null 2>&1
}

if ! ensure_printer_connected; then
  echo "print-hbp-pdf: printer not connected (/dev/usb/lp* missing); refusing to queue job." >&2
  exit 5
fi

ensure_hbp_queue() {
  lpstat -h "$SOCK" -p "$QUEUE" >/dev/null 2>&1 && return 0

  URI=""
  if lpstat -h "$SOCK" -p test >/dev/null 2>&1; then
    URI="$(lpstat -h "$SOCK" -v 2>/dev/null | awk '$1=="device" && $3=="test:" {print $4; exit}')"
  fi
  if [ -z "$URI" ] && [ -x /var/cups-serverbin/lib/cups/backend/usb ]; then
    URI="$(/var/cups-serverbin/lib/cups/backend/usb 2>/dev/null | awk '$1=="direct" && $2 ~ /^usb:\/\// {print $2; exit}')"
  fi
  [ -n "$URI" ] || return 1

  model_from_uri() {
    # usb://Brother/HL-L5000D%20series?serial=... -> HL-L5000D series
    printf '%s' "$1" | sed -n 's@^usb://Brother/\([^?]*\).*$@\1@p' | sed 's/%20/ /g'
  }
  MODEL_NAME="$(model_from_uri "$URI")"
  find_brlaser_ppd() {
    model="$1"
    model_dir="/var/cups-data/model"
    [ -d "$model_dir" ] || return 0

    if [ -n "$model" ]; then
      # Fast path: direct filename match.
      ppd="$(find "$model_dir" -type f \( -iname "*${model}*.ppd" -o -iname "*${model}*.ppd.gz" \) | head -n 1)"
      [ -n "$ppd" ] && { echo "$ppd"; return 0; }

      # ppdc-generated brlaser files are often named like brl5000d.ppd, so also match content.
      ppd="$(grep -RilsF "ModelName: \"Brother $model\"" "$model_dir" 2>/dev/null | head -n 1)"
      [ -n "$ppd" ] && { echo "$ppd"; return 0; }

      ppd="$(grep -RilsF "MDL:$model;" "$model_dir" 2>/dev/null | head -n 1)"
      [ -n "$ppd" ] && { echo "$ppd"; return 0; }
    fi

    find "$model_dir" -type f \
      \( -iname 'brl*.ppd' -o -iname 'brl*.ppd.gz' -o -iname '*Brother*HL-*.ppd' -o -iname '*Brother*HL-*.ppd.gz' \) \
      | head -n 1
  }
  run_ppdc_brlaser() {
    out_dir="$1"
    drv_file="$2"
    [ -n "$out_dir" ] && [ -n "$drv_file" ] || return 1
    set -- /bin/ppdc -d "$out_dir"
    for inc in /var/cups-data/ppdc /var/cups-data/drv; do
      [ -d "$inc" ] || continue
      set -- "$@" -I "$inc"
    done
    CUPS_BIN_ROOT="$(readlink /bin/cupsd 2>/dev/null | sed 's#/bin/cupsd$##')"
    if [ -n "$CUPS_BIN_ROOT" ] && [ -d "$CUPS_BIN_ROOT/share/cups/ppdc" ]; then
      set -- "$@" -I "$CUPS_BIN_ROOT/share/cups/ppdc"
    elif [ -n "$CUPS_BIN_ROOT" ] && [ -d "${CUPS_BIN_ROOT}-lib/share/cups/ppdc" ]; then
      set -- "$@" -I "${CUPS_BIN_ROOT}-lib/share/cups/ppdc"
    fi
    set -- "$@" "$drv_file"
    /bin/timeout 60 "$@" >/tmp/ppdc-hbp.out 2>/tmp/ppdc-hbp.err
  }

  # Prefer PPD path to avoid cups-driverd/dvr:/// dependency.
  PPD=""
  if [ -d /var/cups-data/model ]; then
    PPD="$(find_brlaser_ppd "$MODEL_NAME")"
  fi
  if [ -z "$PPD" ] && [ -x /bin/ppdc ] && [ -f /var/cups-data/drv/brlaser.drv ]; then
    mkdir -p /var/cups-data/model
    run_ppdc_brlaser /var/cups-data/model /var/cups-data/drv/brlaser.drv || true
    PPD="$(find_brlaser_ppd "$MODEL_NAME")"
  fi

  echo "print-hbp-pdf: queue='$QUEUE' model='$MODEL_NAME' uri='$URI' ppd='${PPD:-<none>}' dpi='$DPI'" >&2
  lpadmin -h "$SOCK" -x "$QUEUE" >/dev/null 2>&1 || true
  if [ -n "$PPD" ]; then
    if ! lpadmin -h "$SOCK" -p "$QUEUE" -E -v "$URI" -P "$PPD" >/tmp/lpadmin-hbp.out 2>/tmp/lpadmin-hbp.err; then
      echo "lpadmin failed while creating '$QUEUE' (see /tmp/lpadmin-hbp.err)" >&2
      return 1
    fi
  else
    echo "no usable PPD for '$QUEUE' (ppdc may have failed; see /tmp/ppdc-hbp.err)" >&2
    return 1
  fi
  if ! lpstat -h "$SOCK" -p "$QUEUE" >/dev/null 2>&1; then
    echo "queue '$QUEUE' still missing after lpadmin (see /tmp/lpadmin-hbp.err)" >&2
    return 1
  fi
}

if ! ensure_hbp_queue; then
  echo "queue '$QUEUE' not found on $SOCK and auto-create failed" >&2
  exit 3
fi

# SeedEtcher expects one immediate physical print, not backlog replay.
# Clear any stale pending jobs on this queue before submitting the new one.
if command -v cancel >/dev/null 2>&1; then
  cancel -h "$SOCK" -a "$QUEUE" >/dev/null 2>&1 || true
fi

RAS="/tmp/print-hbp.ras"
gs -q -dSAFER -dBATCH -dNOPAUSE -sDEVICE=cups -sOutputFile="$RAS" -r"$DPI" -dDEVICEWIDTHPOINTS=595 -dDEVICEHEIGHTPOINTS=842 -dFIXEDMEDIA -dPDFFitPage "$PDF"
lp -h "$SOCK" -d "$QUEUE" -o document-format=application/vnd.cups-raster "$RAS"
EOF
    chmod 755 /bin/print-hbp-pdf

    # RAM feasibility harness for optional SD-removal HBP mode.
    cat > /bin/cups-spike-ram-feasibility <<'EOF'
#!/bin/sh
set -eu

RAM_ROOT="${HBP_RAM_ROOT:-/run/hbp-ram-runtime}"
MIN_AVAIL_MB="${HBP_RAM_MIN_AVAIL_MB:-100}"
SOCK="${CUPS_SERVER_SOCK:-/var/run/cups/cups.sock}"
TMP_RAW="/tmp/hbp-ram-roots.raw.$$"
TMP_LIST="/tmp/hbp-ram-roots.list.$$"
TMP_DONE_FILES="/tmp/hbp-ram-done-files.$$"
trap 'rm -f "$TMP_RAW" "$TMP_LIST" "$TMP_DONE_FILES"' EXIT INT TERM

usage() {
  cat <<USAGE
usage:
  cups-spike-ram-feasibility estimate [core|full]
  cups-spike-ram-feasibility stage [core|full]
  cups-spike-ram-feasibility check
  cups-spike-ram-feasibility status
  cups-spike-ram-feasibility detach-sd
  cups-spike-ram-feasibility unstage

notes:
  - core: curated runtime roots for HBP print path.
  - full: all paths from /cups-spike-store-paths (requires latest image build).
  - env:
      HBP_RAM_ROOT=/run/hbp-ram-runtime
      HBP_RAM_SIZE=<tmpfs size, e.g. 320m>   (stage only)
      HBP_RAM_MIN_AVAIL_MB=100               (check gate)
USAGE
}

store_root_of() {
  p="${1:-}"
  [ -n "$p" ] || return 1
  rp="$(readlink -f "$p" 2>/dev/null || true)"
  [ -n "$rp" ] || return 1
  printf '%s\n' "$rp" | sed -n 's#^\(/nix/store/[^/]*-[^/]*\).*#\1#p' | head -n 1
}

add_root() {
  r="${1:-}"
  [ -n "$r" ] || return 0
  case "$r" in
    /nix/store/*)
      [ -e "$r" ] && echo "$r" >> "$TMP_RAW"
      case "$r" in
        *-lib) ;;
        *)
          # Nix frequently splits runtime libraries into sibling -lib outputs.
          if [ -e "${r}-lib" ]; then
            echo "${r}-lib" >> "$TMP_RAW"
          fi
          ;;
      esac
      ;;
  esac
}

mem_field_mb() {
  key="$1"
  awk -v k="$key" '$1==k":" {printf "%d", $2/1024; exit}' /proc/meminfo
}

report_mem() {
  tag="$1"
  echo "mem[$tag]: total=$(mem_field_mb MemTotal)MB avail=$(mem_field_mb MemAvailable)MB free=$(mem_field_mb MemFree)MB"
}

resolve_lib_path() {
  lib="$1"
  runpaths="$2"
  oldifs="$IFS"
  IFS=':'
  for d in $runpaths /lib /usr/lib; do
    [ -n "$d" ] || continue
    if [ -e "$d/$lib" ]; then
      IFS="$oldifs"
      readlink -f "$d/$lib" 2>/dev/null || true
      return 0
    fi
  done
  IFS="$oldifs"

  # BusyBox find behavior can vary; keep fallback lookup simple and robust.
  p="$(find /nix/store -name "$lib" 2>/dev/null | head -n 1 || true)"
  if [ -n "$p" ]; then
    readlink -f "$p" 2>/dev/null || true
    return 0
  fi
  return 1
}

collect_loader_roots() {
  f="$1"
  [ -n "$f" ] || return 0
  [ -x /lib/ld-musl-armhf.so.1 ] || return 0

  /lib/ld-musl-armhf.so.1 --list "$f" 2>/dev/null \
    | awk '{for(i=1;i<=NF;i++) if($i ~ /^\/nix\/store\//) print $i}' \
    | while read -r p; do
        add_root "$(store_root_of "$p" || true)"
      done
}

collect_file_deps() {
  f="$1"
  [ -n "$f" ] || return 0
  f="$(readlink -f "$f" 2>/dev/null || true)"
  [ -n "$f" ] || return 0
  [ -e "$f" ] || return 0

  grep -Fxq "$f" "$TMP_DONE_FILES" 2>/dev/null && return 0
  echo "$f" >> "$TMP_DONE_FILES"
  add_root "$(store_root_of "$f" || true)"
  collect_loader_roots "$f"

  [ -x /bin/readelf ] || return 0

  interp="$(/bin/readelf -l "$f" 2>/dev/null | sed -n 's@.*Requesting program interpreter: \(.*\)]@\1@p' | head -n 1 || true)"
  [ -n "$interp" ] && collect_file_deps "$interp"

  runpaths="$(/bin/readelf -d "$f" 2>/dev/null | sed -n 's@.*Library runpath: \[\(.*\)\]@\1@p' | head -n 1 || true)"
  if [ -n "$runpaths" ]; then
    oldifs="$IFS"
    IFS=':'
    for rp in $runpaths; do
      case "$rp" in
        /nix/store/*) add_root "$(store_root_of "$rp" || true)" ;;
      esac
    done
    IFS="$oldifs"
  fi

  needed="$(/bin/readelf -d "$f" 2>/dev/null | sed -n 's@.*Shared library: \[\(.*\)\]@\1@p' || true)"
  [ -n "$needed" ] || return 0
  for lib in $needed; do
    libp="$(resolve_lib_path "$lib" "$runpaths" || true)"
    [ -n "$libp" ] || continue
    collect_file_deps "$libp"
  done
}

collect_core_roots() {
  : > "$TMP_RAW"
  : > "$TMP_DONE_FILES"
  for cmd in cupsd lp lpadmin lpstat lpinfo ppdc gs pdftops cupsfilter; do
    p="$(command -v "$cmd" 2>/dev/null || true)"
    [ -n "$p" ] || continue
    rp="$(readlink -f "$p" 2>/dev/null || true)"
    [ -n "$rp" ] && collect_file_deps "$rp"
  done
  for p in \
    /var/cups-serverbin/lib/cups/filter/rastertobrlaser \
    /var/cups-serverbin/lib/cups/backend/usb \
    /var/cups-serverbin/lib/cups/driver/cups-driverd \
    /var/cups-serverbin/lib/cups/driver/drv
  do
    [ -e "$p" ] || continue
    rp="$(readlink -f "$p" 2>/dev/null || true)"
    [ -n "$rp" ] && collect_file_deps "$rp"
  done
  if [ -f /cups-spike.env ]; then
    # shellcheck source=/dev/null
    . /cups-spike.env
    add_root "${BRLASER_ROOT:-}"
    add_root "${CUPS_FILTERS_ROOT:-}"
  fi
  cupsd_real="$(readlink -f /bin/cupsd 2>/dev/null || true)"
  cupsd_root="$(store_root_of "$cupsd_real" || true)"
  add_root "$cupsd_root"
  if [ -n "$cupsd_root" ] && [ -d "${cupsd_root}-lib" ]; then
    add_root "${cupsd_root}-lib"
  fi
  if [ -f /etc/cups/cups-files.conf ]; then
    awk '{for(i=1;i<=NF;i++) if($i ~ /^\/nix\/store\//) print $i}' /etc/cups/cups-files.conf \
      | while read -r p; do
          add_root "$(store_root_of "$p" || true)"
        done
  fi
  sort -u "$TMP_RAW" > "$TMP_LIST"
}

collect_full_roots() {
  : > "$TMP_RAW"
  if [ ! -f /cups-spike-store-paths ]; then
    echo "error: /cups-spike-store-paths missing (rebuild with latest flake/initramfs)" >&2
    exit 1
  fi
  while read -r p; do
    [ -n "$p" ] || continue
    case "$p" in
      /nix/store/*) add_root "$p" ;;
    esac
  done < /cups-spike-store-paths
  sort -u "$TMP_RAW" > "$TMP_LIST"
}

estimate_kib() {
  total=0
  while read -r r; do
    [ -e "$r" ] || continue
    sz="$(du -sk "$r" 2>/dev/null | awk '{print $1}')"
    [ -n "$sz" ] || sz=0
    total=$((total + sz))
  done < "$TMP_LIST"
  echo "$total"
}

default_tmpfs_size() {
  kib="$1"
  # +20% copy overhead and +64 MiB headroom.
  size_kib=$(( (kib * 120) / 100 + 65536 ))
  echo "${size_kib}k"
}

is_mounted() {
  mnt="$1"
  awk -v m="$mnt" '$2==m{found=1} END{exit found?0:1}' /proc/mounts
}

setup_tmpfs() {
  size_opt="$1"
  mkdir -p "$RAM_ROOT"
  if ! is_mounted "$RAM_ROOT"; then
    mount -t tmpfs -o "size=$size_opt,mode=0755" tmpfs "$RAM_ROOT"
  fi
  mkdir -p "$RAM_ROOT/nix/store"
}

copy_roots() {
  n=0
  while read -r r; do
    [ -e "$r" ] || continue
    n=$((n + 1))
    echo "copy[$n]: $r"
    cp -a "$r" "$RAM_ROOT/nix/store/"
  done < "$TMP_LIST"
}

bind_nix() {
  src="$(awk '$2=="/nix"{print $1}' /proc/mounts | tail -n 1)"
  if [ "$src" = "$RAM_ROOT/nix" ]; then
    return 0
  fi
  mount --bind "$RAM_ROOT/nix" /nix
}

unbind_nix() {
  src="$(awk '$2=="/nix"{print $1}' /proc/mounts | tail -n 1)"
  if [ "$src" = "$RAM_ROOT/nix" ]; then
    umount /nix
  fi
}

teardown_tmpfs() {
  if is_mounted "$RAM_ROOT"; then
    umount "$RAM_ROOT"
  fi
}

status() {
  src="$(awk '$2=="/nix"{print $1 " (" $3 ")"}' /proc/mounts | tail -n 1)"
  [ -n "$src" ] || src="(unmounted)"
  echo "nix-mount: $src"
  if is_mounted "$RAM_ROOT"; then
    rsrc="$(awk -v m="$RAM_ROOT" '$2==m{print $1 " (" $3 ")"}' /proc/mounts | tail -n 1)"
    echo "ram-root: $rsrc"
  else
    echo "ram-root: not-mounted"
  fi
  report_mem "status"
}

enforce_mem_gate() {
  avail="$(mem_field_mb MemAvailable)"
  if [ "$avail" -lt "$MIN_AVAIL_MB" ]; then
    echo "FAIL: MemAvailable=${avail}MB < ${MIN_AVAIL_MB}MB gate" >&2
    return 1
  fi
  echo "PASS: MemAvailable=${avail}MB >= ${MIN_AVAIL_MB}MB gate"
}

summarize_estimate() {
  mode="$1"
  count="$(wc -l < "$TMP_LIST" | tr -d ' ')"
  kib="$(estimate_kib)"
  mib=$((kib / 1024))
  echo "mode=$mode roots=$count size=${mib}MiB"
}

run_check() {
  status
  enforce_mem_gate
  echo "[queues]"
  lpstat -h "$SOCK" -p -v || true
}

detach_sd() {
  src="$(awk '$2=="/nix"{print $1}' /proc/mounts | tail -n 1)"
  fstype="$(awk '$2=="/nix"{print $3}' /proc/mounts | tail -n 1)"
  if [ "$src" != "$RAM_ROOT/nix" ] && [ "$fstype" != "tmpfs" ]; then
    echo "error: /nix is not RAM-backed; run 'stage' first (src=$src fstype=$fstype)" >&2
    return 1
  fi

  sync
  if command -v blockdev >/dev/null 2>&1 && [ -b /dev/mmcblk0 ]; then
    blockdev --flushbufs /dev/mmcblk0 >/dev/null 2>&1 || true
  fi

  pass=0
  while [ "$pass" -lt 6 ]; do
    pass=$((pass + 1))
    remain="$(awk '$1 ~ /^\/dev\/mmcblk0p[0-9]+$/ {print $1 " " $2}' /proc/mounts)"
    [ -z "$remain" ] && break

    # Handle stacked /nix mounts: if top /nix is RAM-backed but an mmc mount
    # still exists at /nix, drop top layer once to expose lower mount.
    if echo "$remain" | awk '$2=="/nix"{found=1} END{exit found?0:1}'; then
      src="$(awk '$2=="/nix"{print $1}' /proc/mounts | tail -n 1)"
      fstype="$(awk '$2=="/nix"{print $3}' /proc/mounts | tail -n 1)"
      if [ "$src" = "$RAM_ROOT/nix" ] || [ "$fstype" = "tmpfs" ]; then
        umount /nix >/dev/null 2>&1 || true
      fi
    fi

    remain="$(awk '$1 ~ /^\/dev\/mmcblk0p[0-9]+$/ {print $1 " " $2}' /proc/mounts)"
    [ -z "$remain" ] && break
    tmp_mmc="/tmp/hbp-ram-mmc.$$.${pass}"
    echo "$remain" > "$tmp_mmc"
    while read -r dev mnt; do
      [ -n "$dev" ] || continue
      [ -n "$mnt" ] || continue
      echo "detach: unmounting $dev ($mnt)"
      umount "$mnt" >/dev/null 2>&1 || umount -l "$mnt" >/dev/null 2>&1 || \
        echo "warn: failed to unmount $dev ($mnt)" >&2
    done < "$tmp_mmc"
    rm -f "$tmp_mmc"

    # Ensure /nix is RAM-backed after detach steps.
    src="$(awk '$2=="/nix"{print $1}' /proc/mounts | tail -n 1)"
    fstype="$(awk '$2=="/nix"{print $3}' /proc/mounts | tail -n 1)"
    if [ "$src" != "$RAM_ROOT/nix" ] && [ "$fstype" != "tmpfs" ] && [ -d "$RAM_ROOT/nix/store" ]; then
      mount --bind "$RAM_ROOT/nix" /nix >/dev/null 2>&1 || true
    fi
  done

  sync
  remain="$(awk '$1 ~ /^\/dev\/mmcblk0p[0-9]+$/ {print $1 " -> " $2}' /proc/mounts)"
  if [ -n "$remain" ]; then
    echo "FAIL: mmc partitions still mounted:" >&2
    echo "$remain" >&2
    return 1
  fi
  echo "SD detach prep complete: no mmcblk0p* mounts remain."
}

ACTION="${1:-}"
MODE="${2:-core}"

case "$ACTION" in
  estimate)
    case "$MODE" in
      core) collect_core_roots ;;
      full) collect_full_roots ;;
      *) usage; exit 2 ;;
    esac
    summarize_estimate "$MODE"
    ;;
  stage)
    case "$MODE" in
      core) collect_core_roots ;;
      full) collect_full_roots ;;
      *) usage; exit 2 ;;
    esac
    summarize_estimate "$MODE"
    kib="$(estimate_kib)"
    size_opt="${HBP_RAM_SIZE:-$(default_tmpfs_size "$kib")}"
    report_mem "before-stage"
    setup_tmpfs "$size_opt"
    copy_roots
    bind_nix
    report_mem "after-stage"
    run_check
    ;;
  check)
    run_check
    ;;
  status)
    status
    ;;
  detach-sd)
    detach_sd
    ;;
  unstage)
    unbind_nix
    teardown_tmpfs
    report_mem "after-unstage"
    ;;
  *)
    usage
    exit 2
    ;;
esac
EOF
    chmod 755 /bin/cups-spike-ram-feasibility

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
    gs -q -dSAFER -dBATCH -dNOPAUSE -sDEVICE=cups -sOutputFile=/tmp/cups-hbp-test.ras -r600 -dDEVICEWIDTHPOINTS=595 -dDEVICEHEIGHTPOINTS=842 -dFIXEDMEDIA -dPDFFitPage /tmp/cups-hbp-test.ps
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
    BRLASER_BLOCK_REASON=""
    brlaser_filter_usable() {
        FILTER_BIN="/var/cups-serverbin/lib/cups/filter/rastertobrlaser"
        BRLASER_BLOCK_REASON=""
        [ -x "$FILTER_BIN" ] || { BRLASER_BLOCK_REASON="filter missing/not executable"; return 1; }
        [ -x /bin/readelf ] || { BRLASER_BLOCK_REASON="readelf unavailable for probe"; return 1; }

        # Strict runtime probe: verify interpreter, NEEDED libs, and reject relocation failures.
        INTERP_PATH="$(/bin/readelf -l "$FILTER_BIN" 2>/dev/null | sed -n 's@.*Requesting program interpreter: \(.*\)]@\1@p' | head -n 1)"
        if [ -n "$INTERP_PATH" ] && [ ! -e "$INTERP_PATH" ]; then
            BRLASER_BLOCK_REASON="missing interpreter: $INTERP_PATH"
            return 1
        fi

        RUNPATHS="$(/bin/readelf -d "$FILTER_BIN" 2>/dev/null | sed -n 's@.*Library runpath: \[\(.*\)\]@\1@p' | head -n 1)"
        NEEDED_LIBS="$(/bin/readelf -d "$FILTER_BIN" 2>/dev/null | sed -n 's@.*Shared library: \[\(.*\)\]@\1@p')"
        SEARCH_PATHS="$RUNPATHS:/lib:/usr/lib"
        for LIB in $NEEDED_LIBS; do
            FOUND=""
            for P in $(echo "$SEARCH_PATHS" | tr ':' '\n'); do
                [ -n "$P" ] || continue
                if [ -e "$P/$LIB" ]; then
                    FOUND=1
                    break
                fi
            done
            if [ -z "$FOUND" ]; then
                BRLASER_BLOCK_REASON="missing shared lib: $LIB"
                return 1
            fi
        done

        PROBE_ERR="/tmp/brlaser-probe.err"
        : > "$PROBE_ERR"
        /bin/timeout 2 "$FILTER_BIN" >/dev/null 2>"$PROBE_ERR"
        RC="$?"
        if grep -q -E "Error loading shared library|Error relocating|not found" "$PROBE_ERR"; then
            BRLASER_BLOCK_REASON="ABI mismatch (relocation/shared-lib errors)"
            return 1
        fi
        if [ "$RC" -eq 126 ] || [ "$RC" -eq 127 ]; then
            BRLASER_BLOCK_REASON="filter failed to execute (rc=$RC)"
            return 1
        fi
        return 0
    }

    provision_hbp_queue() {
        QUEUE_URI="$1"
        [ -n "$QUEUE_URI" ] || return 1

        # Optional non-raw queue via generated PPD first, then model lookup.
        if ! brlaser_filter_usable; then
            debug_echo "CUPS spike: HBP blocked: ABI mismatch; skipping test-hbp queue (${BRLASER_BLOCK_REASON})"
            return 0
        fi

        find_runtime_brlaser_ppd() {
            model="$1"
            model_dir="$CUPS_RUNTIME_DATA/model"
            [ -d "$model_dir" ] || return 0

            if [ -n "$model" ]; then
                # Fast path: direct filename match.
                ppd="$(find "$model_dir" -type f \( -iname "*${model}*.ppd" -o -iname "*${model}*.ppd.gz" \) | head -n 1)"
                [ -n "$ppd" ] && { echo "$ppd"; return 0; }

                # ppdc-generated brlaser files are often named like brl5000d.ppd, so also match content.
                ppd="$(grep -RilsF "ModelName: \"Brother $model\"" "$model_dir" 2>/dev/null | head -n 1)"
                [ -n "$ppd" ] && { echo "$ppd"; return 0; }

                ppd="$(grep -RilsF "MDL:$model;" "$model_dir" 2>/dev/null | head -n 1)"
                [ -n "$ppd" ] && { echo "$ppd"; return 0; }
            fi

            find "$model_dir" -type f \
              \( -iname 'brl*.ppd' -o -iname 'brl*.ppd.gz' -o -iname '*Brother*HL-*.ppd' -o -iname '*Brother*HL-*.ppd.gz' \) \
              | head -n 1
        }

        PPD=""
        MODEL_NAME="$(printf '%s' "$QUEUE_URI" | sed -n 's@^usb://Brother/\([^?]*\).*$@\1@p' | sed 's/%20/ /g')"
        if [ -d "$CUPS_RUNTIME_DATA/model" ]; then
            PPD="$(find_runtime_brlaser_ppd "$MODEL_NAME")"
        fi
        if [ -z "$PPD" ] && [ -x /bin/ppdc ] && [ -f "$CUPS_RUNTIME_DATA/drv/brlaser.drv" ]; then
            mkdir -p "$CUPS_RUNTIME_DATA/model"
            run_ppdc_brlaser "$CUPS_RUNTIME_DATA/model" "$CUPS_RUNTIME_DATA/drv/brlaser.drv" 10 >/dev/null 2>&1 || true
            PPD="$(find_runtime_brlaser_ppd "$MODEL_NAME")"
        fi
        if [ -n "$PPD" ]; then
            /bin/lpadmin -h /var/run/cups/cups.sock -x test-hbp >/dev/null 2>&1 || true
            /bin/lpadmin -h /var/run/cups/cups.sock -p test-hbp -E -v "$QUEUE_URI" -P "$PPD" >> /log/cups.log 2>&1 || true
            debug_echo "CUPS spike: queue test-hbp configured ppd=$PPD"
        else
            debug_echo "CUPS spike: no brlaser PPD found for non-raw queue"
        fi
        return 0
    }

    maybe_provision_hbp_async() {
        QUEUE_URI="$1"
        [ -n "$QUEUE_URI" ] || return 0
        # HBP queue pre-provisioning is optional and should not run unless explicitly enabled.
        if [ "${CUPS_SPIKE_HBP_ASYNC:-0}" = "1" ]; then
            (
                provision_hbp_queue "$QUEUE_URI"
            ) &
            debug_echo "CUPS spike: scheduling test-hbp provisioning in background"
        else
            provision_hbp_queue "$QUEUE_URI"
        fi
    }

    provision_spike_queue() {
        # Return 0 when raw queue is configured, 1 otherwise.
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
        maybe_provision_hbp_async "$QUEUE_URI"
        return 0
    }

    # Start CUPS and provision a raw queue. Prefer usb:// backend discovery, then fallback.
    if /bin/cupsd >> /log/cups.log 2>&1; then
        # Try immediate provisioning first.
        if ! provision_spike_queue; then
            # Default is OFF to avoid persistent background /nix access after SD removal
            # when running in PCL/PS-only mode.
            if [ "${CUPS_SPIKE_QUEUE_RETRY:-0}" = "1" ]; then
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
            else
                debug_echo "CUPS spike: no printer URI discovered at boot; retry loop disabled"
            fi
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
