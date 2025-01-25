#!/bin/bash

# Function to find the SD card device automatically
find_sd_card() {
    echo "Detecting SD card..." >&2
    diskutil list | grep -i "seedetcher" | awk '{print $NF}' | sed 's/s[0-9]*$//'
}

# Build the SeedEtcher image
echo "Starting Nix build..."
nix build .#image-debug --refresh

if [ $? -ne 0 ]; then
    echo "Nix build failed. Exiting."
    exit 1
fi

# Find the generated image
IMAGE="result/seedetcher-debug.img"

if [ ! -f "$IMAGE" ]; then
    echo "Error: Built image not found at $IMAGE"
    exit 1
fi

# Automatically detect SD card device
SD_CARD=$(find_sd_card 2)

if [ -z "$SD_CARD" ]; then
    echo "No SD card detected. Please insert it and try again."
    exit 1
fi

# Confirm the detected SD card with the user
echo "Detected SD card: /dev/$SD_CARD"
read -p "Do you want to proceed with flashing? (y/n): " CONFIRM

if [[ "$CONFIRM" != "y" ]]; then
    echo "Flashing canceled."
    exit 0
fi

# Unmount the SD card before flashing
echo "Unmounting /dev/$SD_CARD..."
diskutil unmountDisk /dev/$SD_CARD

# Flash the image to the SD card
echo "Flashing image to /dev/r$SD_CARD..."
sudo dd if="$IMAGE" of="/dev/r$SD_CARD" bs=1m status=progress

# Eject the SD card after flashing
diskutil eject /dev/$SD_CARD

echo "Flashing complete. You can now remove the SD card."
