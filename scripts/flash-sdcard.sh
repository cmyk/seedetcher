#!/bin/bash

# ✔ Commits & pushes code changes from Mac
# ✔ Pulls latest code on the VM (ensures up-to-date build)
# ✔ Builds the #debug image on the VM
# ✔ Transfers the built image from VM to Mac
# ✔ Lists available disks and confirms SD card selection
# ✔ Unmounts, flashes, and ejects the SD card

# Exit on error
set -e

# Variables
VM_USER="cmyk"          
VM_HOST="ubuntu"        
VM_IMG_PATH="~/seedetcher/result/seedetcher-debug.img"
LOCAL_IMG_PATH="/tmp/seedetcher-debug.img"

# Step 1: Commit & Push Changes from Mac
echo "🛠️ Checking for changes..."
cd ~/Documents/GitHub/seedetcher || exit 1

if [[ -n $(git status --porcelain) ]]; then
  echo "🔄 Changes detected, committing..."
  git add -A
  git commit -m "Auto-build: $(date '+%Y-%m-%d %H:%M:%S')"
  git push origin main
else
  echo "✅ No changes to commit."
fi

# Step 2: Pull Changes on VM & Wait for Completion
echo "🔄 Pulling latest changes on VM..."
ssh "${VM_USER}@${VM_HOST}" << 'EOF'
cd ~/seedetcher
for attempt in {1..5}; do
    git pull origin main && echo "✅ Git pull successful" && exit 0
    echo "⚠️  Git pull failed. Retrying in 5 seconds..."
    sleep 5
done
echo "❌ Git pull failed after multiple attempts."
exit 1
EOF

# Step 3: Build Debug Image on VM (Wait for Completion)
echo "🛠️ Building debug image on VM..."
ssh "${VM_USER}@${VM_HOST}" << 'EOF'
cd ~/seedetcher
echo "🚀 Starting image build..."
if nix build .#image-debug --impure --print-build-logs; then
    echo "✅ Build completed successfully!"
else
    echo "❌ Build failed!" >&2
    exit 1
fi
EOF

# Step 4: Fetch the .img file from the VM
echo "📦 Fetching image from VM..."
scp "${VM_USER}@${VM_HOST}:${VM_IMG_PATH}" "${LOCAL_IMG_PATH}"

if [[ ! -f "${LOCAL_IMG_PATH}" ]]; then
  echo "❌ Error: Failed to fetch the image from the VM." >&2
  exit 1
fi

echo "✅ Image fetched successfully to ${LOCAL_IMG_PATH}."

# Step 5: List available disks
echo "📝 Available disks on your Mac:"
diskutil list

# Prompt for the disk identifier
echo "💾 Enter the disk identifier for your SD card (e.g., disk2):"
read -r DISK

# Confirm the disk path
DISK_PATH="/dev/${DISK}"
echo "⚠️  You selected ${DISK_PATH}. Are you sure? This will erase all data on the SD card! (yes/no)"
read -r CONFIRM
if [[ "${CONFIRM}" != "yes" ]]; then
  echo "❌ Aborted."
  exit 0
fi

# Step 6: Request sudo access
echo "🔑 Requesting sudo access to flash the SD card..."
sudo -v

# Step 7: Unmount the disk
echo "📤 Unmounting ${DISK_PATH}..."
sudo diskutil unmountDisk "${DISK_PATH}"

# Step 8: Flash the image
echo "⚡ Flashing ${LOCAL_IMG_PATH} to ${DISK_PATH}..."
sudo dd if="${LOCAL_IMG_PATH}" of="/dev/r${DISK}" bs=1m status=progress

# Step 9: Eject the disk
echo "💨 Ejecting ${DISK_PATH}..."
sudo diskutil eject "${DISK_PATH}"

echo "✅ SD card flashed and ejected successfully!"