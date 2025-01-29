#!/bin/bash


#### TEST MESSAGE TO TEST WEBHOOK
# Exit on error
set -e

# Variables
VM_USER="cmyk"          # Update this if your VM username is different
VM_HOST="ubuntu"          # Update this if your VM hostname is different
VM_IMG_PATH="~/seedetcher/result/seedetcher-debug.img"
LOCAL_IMG_PATH="/tmp/seedetcher-debug.img"

# Step 1: Fetch the .img file from the VM
echo "Fetching image from VM..."
scp "${VM_USER}@${VM_HOST}:${VM_IMG_PATH}" "${LOCAL_IMG_PATH}"

# Check if the image was successfully transferred
if [[ ! -f "${LOCAL_IMG_PATH}" ]]; then
  echo "Error: Failed to fetch the image from the VM." >&2
  exit 1
fi

echo "Image fetched successfully to ${LOCAL_IMG_PATH}."

# Step 2: List available disks
echo "Available disks on your Mac:"
diskutil list

# Prompt for the disk identifier
echo "Enter the disk identifier for your SD card (e.g., disk2):"
read -r DISK

# Confirm the disk path
DISK_PATH="/dev/${DISK}"
echo "You selected ${DISK_PATH}. Are you sure? This will erase all data on the SD card! (yes/no)"
read -r CONFIRM
if [[ "${CONFIRM}" != "yes" ]]; then
  echo "Aborted."
  exit 0
fi

# Step 3: Request sudo access
echo "Requesting sudo access to flash the SD card..."
sudo -v

# Step 4: Unmount the disk
echo "Unmounting ${DISK_PATH}..."
sudo diskutil unmountDisk "${DISK_PATH}"

# Step 5: Flash the image
echo "Flashing ${LOCAL_IMG_PATH} to ${DISK_PATH}..."
sudo dd if="${LOCAL_IMG_PATH}" of="/dev/r${DISK}" bs=1m status=progress

# Step 6: Eject the disk
echo "Ejecting ${DISK_PATH}..."
sudo diskutil eject "${DISK_PATH}"

echo "SD card flashed and ejected successfully!"