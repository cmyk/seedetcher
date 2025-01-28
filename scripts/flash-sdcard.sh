#!/bin/bash

# Check for root privileges
if [[ $EUID -ne 0 ]]; then
  echo "Please run this script as root (use sudo)." >&2
  exit 1
fi

# Check if the image exists
IMAGE="result/seedetcher-debug.img"
if [[ ! -f $IMAGE ]]; then
  echo "Error: Image file '$IMAGE' not found!" >&2
  exit 1
fi

# List available disks and prompt for SD card selection
echo "Available Disks:"
diskutil list
echo "Enter the disk identifier for your SD card (e.g., disk2):"
read -r DISK

# Confirm the disk
DISK_PATH="/dev/$DISK"
echo "You selected $DISK_PATH. Are you sure? This will erase all data on the SD card! (yes/no)"
read -r CONFIRM
if [[ $CONFIRM != "yes" ]]; then
  echo "Aborted."
  exit 0
fi

# Unmount the SD card
echo "Unmounting $DISK_PATH..."
diskutil unmountDisk "$DISK_PATH"
if [[ $? -ne 0 ]]; then
  echo "Failed to unmount $DISK_PATH. Aborting." >&2
  exit 1
fi

# Write the image to the SD card
echo "Flashing $IMAGE to $DISK_PATH..."
sudo dd if="$IMAGE" of="${DISK_PATH/\/dev\//\/dev\/r}" bs=1m status=progress
if [[ $? -ne 0 ]]; then
  echo "Failed to flash the image. Aborting." >&2
  exit 1
fi

# Eject the SD card
echo "Ejecting $DISK_PATH..."
diskutil eject "$DISK_PATH"
if [[ $? -eq 0 ]]; then
  echo "SD card flashed and ejected successfully!"
else
  echo "SD card flashed, but ejection failed. Please remove it manually."
fi