
SeedEtcher Development Notes


nix build .#image-debug

IF BUILD FAILS BECAUSE OF HASH ERROR use --impure

WHEN BUILDING and you added/removed pkgs, do
nix flake lock --update-input nixpkgs
nix flake update

nix build .#image-debug --impure

scp ubuntu:~/seedetcher/result/seedetcher-debug.img ~/Downloads/

nix build .#image-debug --impure --refresh --print-build-logs

nix build .#image-debug --store ssh://ubuntu --impure --print-build-logs

To check initramfs contents:
——————————
nix build .#initramfs-debug
mkdir -p tmp
cd tmp
zcat ../result/initramfs.cpio.gz | cpio -idmv
ls -l bin/sh



If you want to remove all temporary build artifacts (like failed derivations), run:
nix-store --gc --print-dead | xargs nix-store --delete
nix build .#<package-name> --show-trace

nix-store --gc --print-dead
nix-store --gc

￼

DEBUGGING
=========================================
Test PDF creation
on VM:
go run cmd/cli/main.go -w multisig -verbose -o /home/cmyk/PDF 

on Pi:
--test-createPlageLayout is needed to access the controller's flags!
./reload-a --test-createPageLayout -verbose -w singlesig
(remember to use the running instance of the controller! If you reloaded the controller, it will be either reload-a or reload-b)

Restarting the Controller:
./controller < /dev/ttyGS1 >> /log/debug.log 2>> /log/debug.log &


MUTOOL
===============================================

mutool convert -O resolution=600,colorspace=mono,spacing=3 -o output.pcl  multisig.pdf




NIX STUFF

Single User nix.conf:
nano ~/.config/nix/nix.conf


If it fails, continue with
nix build .#image-debug --impure --keep-going

If packages aren't available for the build system (Mac):
export NIXPKGS_ALLOW_UNSUPPORTED_SYSTEM=1 

Or use nix develop (doesn't work on Mac, because the binaries are for linux)

How to Speed Up the Rebuild (Going Forward)
	1.	Enable Local Nix Cache to Prevent Future Rebuilds:
	•	You can enable a local cache to store build results and reuse them in future builds:
nix build .#image-debug --keep-going --option keep-outputs true --option keep-derivations true


GO Dependecies trouble?
=============================================

go mod tidy

❯ nix build .#go-deps --show-trace --verbose --rebuild                                                   
checking outputs of '/nix/store/a0wafn6k91jahp9wwaqsp8izx0pi8nvi-go-deps-1.drv'...
error: hash mismatch in fixed-output derivation '/nix/store/a0wafn6k91jahp9wwaqsp8izx0pi8nvi-go-deps-1.drv':
         specified: sha256-9T8y/0OLBW+kGUISMgM1RaPy3EsM8Ip6yIy1UuAs21E=
            got:    sha256-K1aLQiZvP4p3ptJAIsD67u4C7m4WyLCzMw+kjrdcP5w=

Change line 569 in flake.nix to the new hash!


OLD!!! nix is installed in single user mode on Ubuntu.
=============================================
nvim ~/.config/nix/nix.conf

extra-experimental-features = nix-command flakes
trusted-users = root cmyk
keep-outputs = true
keep-derivations = true   
auto-optimise-store = true


nix-env --delete-generations +10
~



NEW!!!!
INSTALLED MULTIUSER 28.2.25
=================================

sudo systemctl restart nix-daemon
sudo systemctl status nix-daemon

sudo nvim /etc/nix/nix.conf
>> 
extra-experimental-features = nix-command flakes
trusted-users = root cmyk
keep-outputs = true
keep-derivations = true   
auto-optimise-store = true



UNINTSTALLING NIX (SINGLE USER LINUX)
================================
sudo mv /etc/bash.bashrc.backup-before-nix /etc/bash.bashrc
sudo mv /etc/zshrc.backup-before-nix /etc/zshrc

rm -rf .local/share/nix
rm -rf .local/state/nix
rm -rf .config/nix

UBUNTU CONFIGURATION CHANGES
================================
If you want /tmp to be stored in RAM (makes it faster but non-persistent):
1️⃣ Edit /etc/fstab:

sudo nano /etc/fstab

2️⃣ Add this line:
tmpfs /tmp tmpfs defaults,noatime,mode=1777 0 0

3️⃣ Reboot






Unmount sd card
diskutil unmountDisk /dev/diskX

sudo dd if=result/seedetcher-debug.img of=/dev/rdiskX bs=1m

diskutil eject /dev/disk5

Find the USB device 
ls /dev/cu.usbmodem*
ls /dev/tty.usbmodem* // probably better!

Serial Terminal to Zero
minicom -D /dev/cu.usbmodem101 -b 115200 -o

Set env var: export USBDEV=/dev/tty.usbmodem101

Upload the new binary with while zero is running! (Rebuild if you modified flake for changes to take effect):
nix build .#image-debug --impure --refresh --print-build-logs
nix run .#reload $USBDEV1

Keep an eye on real-time logs using:
cat $USBDEV

echo "input up" > $USBDEV

echo "runes TEST" > $USBDEV

echo "screenshot" > $USBDEV

Based on the SeedHammer documentation, the available button inputs are:
	•	Joystick (left side):
	•	up
	•	down
	•	left
	•	right
	•	center (pressing the joystick)
	•	Right-side buttons:
	•	b1 (top button)
	•	b2 (middle button)
	•	b3 (bottom button)



Shell Commands on Zero
==========================================
# Start the controller in the background
/controller &
Press Ctrl+Z to pause (suspend) the controller.
Type bg to send it to the background.


Command	Action
Ctrl+C			Kill the foreground process.
Ctrl+Z			Suspend (pause) the foreground process.
jobs -l			List all background jobs with IDs and statuses.
fg %<job-id>	Bring a background job to the foreground.
bg %<job-id>	Resume a suspended job in the background.
kill %<job-id>	Terminate a background job.



Printing
==========================================

magick /Users/cmyk/Documents/GitHub/seedetcher/backup/testdata/plate-2-side-1-1-of-1-words-24-grayscale.png \
    -density 100 -resize 393x393! -monochrome pcl:/Users/cmyk/Documents/GitHub/seedetcher/backup/testdata/output-fixed.pcl

lp -d Brother_HL_L5000D_series -o raw /Users/cmyk/Documents/GitHub/seedetcher/backup/testdata/output-fixed.pcl

==========================================
Getting USB Serial connection working

Check if pi is running Getty on USB serial port:
sudo systemctl status serial-getty@ttyGS0

MUST BE STARTED. DISABLED BY DEFAULT!!
If it's not:
sudo systemctl enable serial-getty@ttyGS0
sudo systemctl start serial-getty@ttyGS0


SERIAL STUFF
====================================
Added 
sudo nano /etc/udev/rules.d/99-serial-settings.rules
With
ACTION=="add", SUBSYSTEM=="tty", KERNEL=="ttyACM0", RUN+="/bin/stty -F /dev/ttyACM0 115200 raw -echo"

Check baudrate:
stty -F /dev/ttyACM0




USB-GADET DETECTION on VM
=============================================================================
# Recap of Added/Modified Files & Reloading udevadm

## 1. Added/Modified Files

### 1.1 /etc/udev/rules.d/99-serial-settings.rules
- This is the `udev` rule that detects the Pi Zero’s USB serial interfaces and triggers the update script.
- Example rule:
  
  ACTION=="add", SUBSYSTEM=="tty", ATTRS{idVendor}=="0525", ATTRS{idProduct}=="a4a7", KERNEL=="ttyACM*", SYMLINK+="usbzero%n", RUN+="/usr/local/bin/usbdev_checker.sh"

### 1.2 /usr/local/bin/usbdev_checker.sh
- This script ensures both serial devices are present before running `update_usbdevs.sh`.
- It prevents duplicate script execution.

### 1.3 /usr/local/bin/update_usbdevs.sh
- This script assigns the detected serial devices and updates the environment variables.
- It logs device assignments and prevents duplicate messages.

----------------------------

## 2. How to Reload udevadm

### Reload udev rules:
  sudo udevadm control --reload-rules

### Apply changes immediately:
  sudo udevadm trigger

### Check if udev triggered the script:
  journalctl -u systemd-udevd --no-pager | grep usbdev_checker.sh

----------------------------


Disabled ModemManager 

sudo systemctl stop ModemManager
sudo systemctl disable ModemManager

APPARMOR!

sudo systemctl stop apparmor
sudo systemctl disable apparmor
sudo reboot


NOTES ON #reload corrupting the binary sent
=========================================
stty -F $USBDEV1 raw -echo # needed for transfer of binary!
echo “” > $USBDEV1 		   # delete whatever is in there


GUI
==================================


Confirm Seed > Descriptor > Engrave Plate

=====================
Grok 3
=====================

github access token:
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

git clone https://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@github.com/cmyk/seedetcher.git




Analysis and Next Steps
Now that I have all the files, I’ll analyze them to help you implement laser printer testing for SeedEtcher, printing a seed phrase and QR code in a 10x10cm area on A4 or letter paper. Here’s a high-level plan based on your setup:
Key Observations
	•	seedetcher/cmd/controller/main.go initializes a Platform (via Init() from platform_rpi.go) and runs the GUI loop using gui.Run, which uses /dev/ttyGS1 for output (as seen in debug_rpi.go).
	•	The Platform interface (gui/gui.go) defines methods like Engraver() and EngraverParams(), currently used for engraving via mjolnir. We’ll replace this with a printer output method.
	•	backup/backup.go and engrave/engrave.go generate the seed phrase and QR code layouts, which we’ll adapt for PCL (Printer Command Language) output.
	•	Your setup uses /dev/ttyGS0 for Busybox shell and /dev/ttyGS1 for controller I/O, ideal for testing via a virtual printer on your Ubuntu host.
Implementation Plan
	1	Modify seedetcher/cmd/controller/platform_rpi.go:
	◦	Add a new method to Platform (e.g., Printer() io.Writer) to handle output to /dev/ttyGS1 or /dev/usb/lp0.
	◦	Update Engraver to optionally return a printer interface or coexist with engraving.
	2	Adapt engrave/engrave.go:
	◦	Convert Plan (moves/lines) into PCL commands for text and QR codes, maintaining the 10x10cm layout from backup/testdata.
	◦	Use PCL raster graphics for the QR code and position text (seed phrase) at 50mm x 50mm, QR at 50mm x 70mm (within 100x100mm).
	3	Update gui/gui.go:
	◦	Add a new screen or option (e.g., PrintSeedScreen) to trigger printing instead of engraving, using the new printer output.
	◦	Reuse backup/backup.go for seed and descriptor data, but output to PCL instead of mjolnir commands.
	4	Testing on Ubuntu:
	◦	Capture /dev/ttyACM1 (mapped to /dev/ttyGS1) output on your host, save to test.pcl, and send to cups-pdf for virtual PDF testing.
	◦	Verify the 10x10cm layout in ~/PDF/test.pdf.
	5	Real Printer Integration:
	◦	Switch output to /dev/usb/lp0 for physical printing, ensuring USB printer support in flake.nix (CONFIG_USB_PRINTER=y).





Memory Constraints
Issue: My context memory fills up with long chats, risking a crash.
Tips:
Start Fresh: If I fail, create a new chat and paste only the latest files/changes (e.g., updated print.go and cmd/cli/main.go).
Summarize: Tell me, “Use the latest print.go and cmd/cli/main.go from [timestamp]” instead of re-explaining everything.
Focus: Ask one thing at a time (like now), keeping replies short.



https://www.seedetcher.com/src/LICENSE
https://www.seedetcher.com/src/README.md
https://www.seedetcher.com/src/capture_print.sh
https://www.seedetcher.com/src/flake.lock
https://www.seedetcher.com/src/flake.nix
https://www.seedetcher.com/src/go.mod
https://www.seedetcher.com/src/go.sum
https://www.seedetcher.com/src/init.sh
https://www.seedetcher.com/src/test.html
https://www.seedetcher.com/src/testgodf.go
https://www.seedetcher.com/src/address/address.go
https://www.seedetcher.com/src/address/address_test.go
https://www.seedetcher.com/src/backup/backup.go
https://www.seedetcher.com/src/backup/backup_test.go
https://www.seedetcher.com/src/backup/testdata/testseed.txt
https://www.seedetcher.com/src/backup/testdata/sample_seed.txt
https://www.seedetcher.com/src/bc/bytewords/bytewords.go
https://www.seedetcher.com/src/bc/bytewords/bytewords_test.go
https://www.seedetcher.com/src/bc/fountain/fountain.go
https://www.seedetcher.com/src/bc/fountain/fountain_test.go
https://www.seedetcher.com/src/bc/ur/ur.go
https://www.seedetcher.com/src/bc/ur/ur_test.go
https://www.seedetcher.com/src/bc/urtypes/urtypes.go
https://www.seedetcher.com/src/bc/urtypes/urtypes_test.go
https://www.seedetcher.com/src/bc/xoshiro256/xoshiro.go
https://www.seedetcher.com/src/bc/xoshiro256/xoshiro_test.go
https://www.seedetcher.com/src/bip32/bip32.go
https://www.seedetcher.com/src/bip32/bip32_test.go
https://www.seedetcher.com/src/bip39/bip39.go
https://www.seedetcher.com/src/bip39/bip39_test.go
https://www.seedetcher.com/src/cmd/markers/main.go
https://www.seedetcher.com/src/cmd/cli/main.go
https://www.seedetcher.com/src/cmd/controller/debug.go
https://www.seedetcher.com/src/cmd/controller/debug_rpi.go
https://www.seedetcher.com/src/cmd/controller/logger.go
https://www.seedetcher.com/src/cmd/controller/main.go
https://www.seedetcher.com/src/cmd/controller/platform_dummy.go
https://www.seedetcher.com/src/cmd/controller/platform_rpi.go
https://www.seedetcher.com/src/driver/qr_driver.go
https://www.seedetcher.com/src/driver/blockchain_driver.go
https://www.seedetcher.com/src/driver/libcamera/rpi_libcamera.go
https://www.seedetcher.com/src/driver/libcamera/rpi_libcamera_test.go
https://www.seedetcher.com/src/driver/libcamera/libcamera_config.json
https://www.seedetcher.com/src/driver/libcamera/rpi_camera_test.sh
https://www.seedetcher.com/src/engrave/engrave.go
https://www.seedetcher.com/src/engrave/engrave_test.go
https://www.seedetcher.com/src/engrave/testdata/test_seed.txt
https://www.seedetcher.com/src/engrave/testdata/etch_sample.txt
https://www.seedetcher.com/src/font/Arial.ttf
https://www.seedetcher.com/src/font/CosmicFont.ttf
https://www.seedetcher.com/src/gui/gui.go
https://www.seedetcher.com/src/gui/gui_test.go
https://www.seedetcher.com/src/gui/index.html
https://www.seedetcher.com/src/gui/style.css
https://www.seedetcher.com/src/gui/script.js
https://www.seedetcher.com/src/gui/assets/arrow-down.bin
https://www.seedetcher.com/src/gui/assets/arrow-down.png
https://www.seedetcher.com/src/gui/assets/arrow-left.bin
https://www.seedetcher.com/src/gui/assets/arrow-left.png
https://www.seedetcher.com/src/gui/assets/arrow-right.bin
https://www.seedetcher.com/src/gui/assets/arrow-right.png
https://www.seedetcher.com/src/gui/assets/arrow-up.bin
https://www.seedetcher.com/src/gui/assets/arrow-up.png
https://www.seedetcher.com/src/gui/assets/button-focused.bin
https://www.seedetcher.com/src/gui/assets/button-focused.png
https://www.seedetcher.com/src/gui/assets/camera-corners.bin
https://www.seedetcher.com/src/gui/assets/camera-corners.png
https://www.seedetcher.com/src/gui/assets/circle-filled.bin
https://www.seedetcher.com/src/gui/assets/circle-filled.png
https://www.seedetcher.com/src/gui/assets/circle.bin
https://www.seedetcher.com/src/gui/assets/circle.png
https://www.seedetcher.com/src/gui/assets/embed.go
https://www.seedetcher.com/src/gui/assets/gen.go
https://www.seedetcher.com/src/gui/assets/generator.go
https://www.seedetcher.com/src/gui/assets/hammer.bin
https://www.seedetcher.com/src/gui/assets/hammer.png
https://www.seedetcher.com/src/gui/assets/icon-back.bin
https://www.seedetcher.com/src/gui/layout/layout.go
https://www.seedetcher.com/src/gui/op/op.go
https://www.seedetcher.com/src/gui/op/op_test.go
https://www.seedetcher.com/src/gui/saver/saver.go
https://www.seedetcher.com/src/gui/saver/saver_test.go
https://www.seedetcher.com/src/gui/text/text.go
https://www.seedetcher.com/src/gui/text/text_test.go
https://www.seedetcher.com/src/gui/widget/label.go
https://www.seedetcher.com/src/image/seedqr.png
https://www.seedetcher.com/src/image/cosmos_bg.jpg
https://www.seedetcher.com/src/nonstandard/custom_crypto.go
https://www.seedetcher.com/src/nonstandard/experimental_vis.go
https://www.seedetcher.com/src/patches/go-qrcode.patch
https://www.seedetcher.com/src/patches/gofpdf.patch
https://www.seedetcher.com/src/scripts/flash-sdcard.sh
https://www.seedetcher.com/src/zbar/scan.go
https://www.seedetcher.com/src/zbar/scan_test.go


Converting Fonts

go run font/bitmap/convert.go -package comfortaa -ppem 17 font/comfortaa/Comfortaa-Bold.ttf font/comfortaa/bold17




go run test_pcl.go
gpcl6 -sDEVICE=pdfwrite -o ~/PDF/test.pdf test.pcl


24 words
go run ./cmd/cli -mnemonic "abandon ability able about above absent absorb abstract absurd abuse access accident account accuse achieve acid acoustic acquire across act action actor actress blanket" -papersize A4 -o ./plates

12 words
go run ./cmd/cli -mnemonic "shy east submit check grocery crumble jazz shrimp word myself shoe brisk" -papersize A4 -o ./plates



	1	Summarize and Archive: I can provide a concise summary of our conversation so far, capturing the key decisions and final print.go code. This allows us to start fresh while preserving the critical information. You can save the summary for reference, reducing the need to retain the full chat history here.
	2	Focus on New Tasks: If you have new requirements or tasks related to SeedEtcher (e.g., integrating with cmd/cli/main.go, GUI, or Pi controller), I can help with those without revisiting old details, keeping the conversation lean.
	3	Clear Context: Since the layout and print.go are finalized, we can reset the context to focus on new topics. I’ll remember the final print.go and key settings (e.g., 16/8 word columns, 5mm margins, fonts, QR position) but won’t carry forward unnecessary chat history.
Summary of Our Conversation (for Reference)
	•	Goal: Create a PDF backup plate for SeedEtcher with a layout matching SeedHammer’s style (85x85mm, 12/24 words, QR, metadata, title) for a Brother HL-L5000D printer.
	•	Final print.go:



Long-Term Plan
Dev Mode: Keep go-deps hashless while debugging. It’s flexible—deps update as you change go.mod.

Release Mode: Once you’re stable, add the outputHash back for reproducibility. Run nix build .#go-deps, fail it with a dummy hash, grab the real one from the error, and lock it in.


