#!/bin/zsh


# **************************************

# DO NOT EDIT THIS SCRIPT ON UPBUNTU!
# THIS IS RUN IN THE MAC TERMINAL!

# **************************************


# Defaults (override with -i)
IMAGE_NAME="seedetcher-debug.img"
REMOTE_HOST="ubuntu"

usage() {
  echo "Usage: $0 [-i image-name]"
  echo "  -i image-name   file name in ~/seedetcher/result (default: seedetcher-debug.img)"
  exit 1
}

while getopts "i:h" opt; do
  case "$opt" in
    i) IMAGE_NAME="$OPTARG" ;;
    h) usage ;;
    *) usage ;;
  esac
done

REMOTE_PATH="~/seedetcher/result/${IMAGE_NAME}"
LOCAL_PATH="$HOME/Downloads/${IMAGE_NAME}"


echo "Waiting for SD card to be mounted..."

# Try detecting SD card immediately
SD_CARD=$(system_profiler SPStorageDataType SPUSBDataType | grep -A10 -B12 SDXC | awk '/BSD Name:/{print $3}' | sed 's/s[0-9]*$//')

if [[ -n "$SD_CARD" ]]; then
    echo "SD card was already mounted: /dev/$SD_CARD"
else
    # Wait for SD card using fswatch
    fswatch -1 /Volumes | while read change; do
        SD_CARD=$(system_profiler SPStorageDataType SPUSBDataType | grep -A10 -B12 SDXC | awk '/BSD Name:/{print $3}' | sed 's/s[0-9]*$//')
        
        if [[ -n "$SD_CARD" ]]; then
            echo "SD card detected: /dev/$SD_CARD"
            break
        fi
    done
fi


# Step 1: Copy the image from Ubuntu
echo "Copying image from Ubuntu..."
scp ${REMOTE_HOST}:${REMOTE_PATH} ${LOCAL_PATH}

if [[ $? -ne 0 ]]; then
    echo "Error: SCP failed."
    exit 1
fi

# Step 2: Identify the SD card **safely**
echo "Identifying SD card..."
DISK_DEVICE=$(system_profiler SPStorageDataType SPUSBDataType | grep -A10 -B12 SDXC | awk '/BSD Name:/{print $3}' | sed 's/s[0-9]*$//')

# Security check: Ensure disk number is 4 or higher
DISK_NUMBER=$(echo $DISK_DEVICE | sed 's/disk//')

if [[ -z "$DISK_DEVICE" ]]; then
    echo "Error: No SD card found!"
    exit 1
elif [[ $DISK_NUMBER -lt 4 ]]; then
    echo "SECURITY WARNING: Detected disk$DISK_NUMBER, which is below 4. Aborting to prevent system damage!"
    exit 1
fi

echo "SD card identified as: /dev/$DISK_DEVICE"

# Step 3: Unmount the SD card
echo "Unmounting /dev/$DISK_DEVICE..."
diskutil unmountDisk /dev/$DISK_DEVICE

if [[ $? -ne 0 ]]; then
    echo "Error: Failed to unmount $DISK_DEVICE."
    exit 1
fi

# Step 4: Flash the image
echo "Flashing the image to /dev/$DISK_DEVICE..."
sudo dd if=$LOCAL_PATH of=/dev/$DISK_DEVICE bs=4M status=progress

if [[ $? -ne 0 ]]; then
    echo "Error: Flashing failed."
    exit 1
fi

# Step 5: Eject the SD card
echo "Ejecting the SD card..."
diskutil eject /dev/$DISK_DEVICE

if [[ $? -ne 0 ]]; then
    echo "Error: Failed to eject SD card."
    exit 1
fi

echo "Flashing complete. You may remove the SD card."
