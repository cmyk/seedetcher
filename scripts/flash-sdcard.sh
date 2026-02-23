#!/bin/zsh


# **************************************

# DO NOT EDIT THIS SCRIPT ON UPBUNTU!
# THIS IS RUN IN THE MAC TERMINAL!

# **************************************


# Defaults (override with -i / -r)
IMAGE_NAME="seedetcher-debug.img"
REMOTE_HOST="ubuntu"
DD_BS="16m"
SKIP_COPY=0
FLASH_ENGINE="auto" # auto|asr|dd
DISK_OVERRIDE=""

usage() {
  echo "Usage: $0 [-i image-name] [-r remote-host] [-s] [-e auto|asr|dd] [-d diskN]"
  echo "  -i image-name   file name in ~/seedetcher/result (default: seedetcher-debug.img)"
  echo "  -r remote-host  scp source host (default: ubuntu)"
  echo "  -s              skip scp; use existing ~/Downloads/<image-name>"
  echo "  -e engine       flash engine: auto (default), asr, or dd"
  echo "  -d diskN        target disk override (e.g. disk4)"
  exit 1
}

while getopts "i:r:se:d:h" opt; do
  case "$opt" in
    i) IMAGE_NAME="$OPTARG" ;;
    r) REMOTE_HOST="$OPTARG" ;;
    s) SKIP_COPY=1 ;;
    e) FLASH_ENGINE="$OPTARG" ;;
    d) DISK_OVERRIDE="$OPTARG" ;;
    h) usage ;;
    *) usage ;;
  esac
done

REMOTE_PATH="~/seedetcher/result/${IMAGE_NAME}"
LOCAL_PATH="$HOME/Downloads/${IMAGE_NAME}"


set -euo pipefail

# Primary detector for MacBook built-in SD readers (your setup).
find_sd_disk_system_profiler() {
  system_profiler SPStorageDataType SPUSBDataType 2>/dev/null \
    | awk '
      /SDXC|SD Card|SDXC Reader|Card Reader/ {inblk=1}
      inblk && /BSD Name:/ {print $3; exit}
      inblk && /^$/ {inblk=0}
    ' \
    | sed 's/s[0-9]*$//'
}

list_disks_external_physical() {
  diskutil list external physical 2>/dev/null \
    | awk '/^\/dev\/disk[0-9]+/{gsub("/dev/","",$1); print $1}'
}

find_seedetcher_named_disk() {
  diskutil list 2>/dev/null | awk '
    /^\/dev\/disk[0-9]+/ {gsub("/dev/","",$1); disk=$1}
    /Windows_FAT_32[[:space:]]+seedetcher/ {print disk; exit}
  '
}

echo "Waiting for SD card to be available..."

if [[ -n "$DISK_OVERRIDE" ]]; then
    SD_CARD="$DISK_OVERRIDE"
else
    SD_CARD="$(find_sd_disk_system_profiler || true)"
fi

if [[ -n "$SD_CARD" ]]; then
    echo "Detected candidate disk: /dev/$SD_CARD"
else
    echo "Insert SD card..."
    while [[ -z "$SD_CARD" ]]; do
      sleep 1
      SD_CARD="$(find_sd_disk_system_profiler || true)"
      [[ -z "$SD_CARD" ]] && SD_CARD="$(find_seedetcher_named_disk || true)"
      [[ -z "$SD_CARD" ]] && SD_CARD="$(list_disks_external_physical | head -n 1 || true)"
    done
    echo "Detected candidate disk: /dev/$SD_CARD"
fi


# Step 1: Copy the image from Ubuntu (optional)
if [[ "$SKIP_COPY" -eq 0 ]]; then
  echo "Copying image from ${REMOTE_HOST}..."
  scp "${REMOTE_HOST}:${REMOTE_PATH}" "${LOCAL_PATH}"
else
  echo "Skipping copy; using local image: ${LOCAL_PATH}"
fi

if [[ ! -f "${LOCAL_PATH}" ]]; then
  echo "Error: Image not found: ${LOCAL_PATH}"
  exit 1
fi

# Step 2: Identify the SD card **safely**
echo "Identifying SD card..."
if [[ -n "$DISK_OVERRIDE" ]]; then
    DISK_DEVICE="$DISK_OVERRIDE"
    CANDIDATES=("$DISK_DEVICE")
else
    CANDIDATES=("${(@f)$(list_disks_external_physical || true)}")
    if [[ "${#CANDIDATES[@]}" -eq 0 ]]; then
        DISK_DEVICE="$(find_sd_disk_system_profiler || true)"
    elif [[ "${#CANDIDATES[@]}" -eq 1 ]]; then
        DISK_DEVICE="${CANDIDATES[0]}"
    else
        echo "Multiple external disks detected:"
        for d in "${CANDIDATES[@]}"; do
            INFO="$(diskutil info /dev/$d 2>/dev/null || true)"
            NAME="$(echo "$INFO" | awk -F: '/Media Name/{sub(/^[ \t]+/, "", $2); print $2; exit}')"
            SIZE="$(echo "$INFO" | awk -F: '/Disk Size/{sub(/^[ \t]+/, "", $2); print $2; exit}')"
            echo "  - /dev/$d  ${NAME:-unknown}  ${SIZE:-unknown}"
        done
        echo
        read "DISK_DEVICE?Type disk id to flash (e.g. disk4): "
    fi
fi

# Security check: Ensure disk number is 4 or higher
DISK_NUMBER=$(echo "$DISK_DEVICE" | sed 's/disk//')

if [[ -z "$DISK_DEVICE" ]]; then
    echo "Error: No SD card found!"
    exit 1
elif [[ $DISK_NUMBER -lt 4 ]]; then
    echo "SECURITY WARNING: Detected disk$DISK_NUMBER, which is below 4. Aborting to prevent system damage!"
    exit 1
fi

# Additional safety checks (single diskutil call)
DISK_INFO="$(diskutil info /dev/$DISK_DEVICE 2>/dev/null || true)"
echo "$DISK_INFO" | grep -qE 'Internal:[[:space:]]+Yes' && echo "Note: /dev/$DISK_DEVICE is reported as internal (built-in reader path)."

MEDIA_NAME="$(echo "$DISK_INFO" | awk -F: '/Media Name/{sub(/^[ \t]+/, "", $2); print $2; exit}')"
MEDIA_SIZE="$(echo "$DISK_INFO" | awk -F: '/Disk Size/{sub(/^[ \t]+/, "", $2); print $2; exit}')"
echo "Target media: ${MEDIA_NAME:-unknown} (${MEDIA_SIZE:-unknown})"
read "CONFIRM?Type '${DISK_DEVICE}' to confirm erase/flash: "
if [[ "$CONFIRM" != "$DISK_DEVICE" ]]; then
    echo "Confirmation mismatch. Aborting."
    exit 1
fi

echo "SD card identified as: /dev/$DISK_DEVICE"
RAW_DISK_DEVICE="r${DISK_DEVICE}"
echo "Using raw device for faster flashing: /dev/${RAW_DISK_DEVICE}"

# Step 3: Unmount the SD card
echo "Unmounting /dev/$DISK_DEVICE..."
diskutil unmountDisk force /dev/$DISK_DEVICE

if [[ $? -ne 0 ]]; then
    echo "Error: Failed to unmount $DISK_DEVICE."
    exit 1
fi

# Step 4: Flash the image
echo "Flashing the image to /dev/$RAW_DISK_DEVICE..."
if [[ "$FLASH_ENGINE" == "auto" ]]; then
    if [[ -x /usr/sbin/asr ]]; then
        FLASH_ENGINE="asr"
    else
        FLASH_ENGINE="dd"
    fi
fi

if [[ "$FLASH_ENGINE" == "asr" ]]; then
    echo "Using asr restore (fast path)..."
    # --noverify avoids checksum pass and matches Etcher-like speed focus.
    sudo /usr/sbin/asr restore \
      --source "$LOCAL_PATH" \
      --target "/dev/$DISK_DEVICE" \
      --erase \
      --noprompt \
      --noverify
else
    echo "Using dd with block size: ${DD_BS}"
    sudo dd if="$LOCAL_PATH" of="/dev/$RAW_DISK_DEVICE" bs="$DD_BS" status=progress
    sync
fi

# Step 5: Eject the SD card
echo "Ejecting the SD card..."
diskutil eject /dev/$DISK_DEVICE

echo "Flashing complete. You may remove the SD card."
