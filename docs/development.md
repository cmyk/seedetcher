
# SeedEtcher Development Notes

When building and you added/removed pkgs, do

```bash
nix flake lock --update-input nixpkgs
nix flake update

nix build .#image-gadget-debug
```

If build fails because of has error use `--impure`
Debug builds use `--print-build-logs`


## Developemnt Environmet

Ubuntu VM on Mac.

### USB-GADET DETECTION on VM

#### Recap of Added/Modified Files & Reloading udevadm

##### 1. Added/Modified Files

##### 1.1 /etc/udev/rules.d/99-serial-settings.rules
- This is the `udev` rule that detects the Pi Zero’s USB serial interfaces and triggers the update script.
- Example rule:
  
  `ACTION=="add", SUBSYSTEM=="tty", ATTRS{idVendor}=="0525", ATTRS{idProduct}=="a4a7", KERNEL=="ttyACM*", SYMLINK+="usbzero%n", RUN+="/usr/local/bin/usbdev_checker.sh"`

##### 1.2 /usr/local/bin/usbdev_checker.sh
- This script ensures both serial devices are present before running `update_usbdevs.sh`.
- It prevents duplicate script execution.

##### 1.3 /usr/local/bin/update_usbdevs.sh
- This script assigns the detected serial devices and updates the environment variables.
- It logs device assignments and prevents duplicate messages.

**(ATTENTION: run source ~/.bashrc to update the USBDEVx shell vars)**

#### 2. How to Reload udevadm

##### Reload udev rules:

```bash 
sudo udevadm control --reload-rules
```

##### Apply changes immediately:

```bash
  sudo udevadm trigger
```

##### Check if udev triggered the script:

```bash
  journalctl -u systemd-udevd --no-pager | grep usbdev_checker.sh
```

##### Disabled ModemManager 

```bash
sudo systemctl stop ModemManager
sudo systemctl disable ModemManager
```

##### Apparmor:

```bash
sudo systemctl stop apparmor
sudo systemctl disable apparmor
sudo reboot
```


## NixOS Stuff

Install multiuser NixOS

```bash
sudo systemctl restart nix-daemon
sudo systemctl status nix-daemon

sudo nvim /etc/nix/nix.conf
>> 
extra-experimental-features = nix-command flakes
trusted-users = root <user>
keep-outputs = true
keep-derivations = true   
auto-optimise-store = true
```

If you want to remove all temporary build artifacts (like failed derivations), run:
nix-store --gc --print-dead | xargs nix-store --delete
nix build .#<package-name> --show-trace

nix-store --gc --print-dead
nix-store --gc


## Ubuntu config changes

If you want /tmp to be stored in RAM (makes it faster but non-persistent):

1️) Edit /etc/fstab:
	
	```bash
	sudo nano /etc/fstab
	```

2️) Add this line:

	```bash
	tmpfs /tmp tmpfs defaults,noatime,mode=1777 0 0
	```

3) Reboot

## GO Dependecies trouble?

`go mod tidy`

```shell
❯ nix build .#go-deps --show-trace --verbose --rebuild                                                   
checking outputs of '/nix/store/a0wafn6k91jahp9wwaqsp8izx0pi8nvi-go-deps-1.drv'...
error: hash mismatch in fixed-output derivation '/nix/store/a0wafn6k91jahp9wwaqsp8izx0pi8nvi-go-deps-1.drv':
         specified: sha256-9T8y/0OLBW+kGUISMgM1RaPy3EsM8Ip6yIy1UuAs21E=
            got:    sha256-K1aLQiZvP4p3ptJAIsD67u4C7m4WyLCzMw+kjrdcP5w=
```

Change the according line in `flake.nix` to the new hash!

## Printing

### Quick test plates (no hardware)
- Singlesig fixture (testnet): `cash zoo picture text skill steel dragon remove imitate fatal close train recipe april extra void obey sell train chaos noble rice typical below`
- Multisig fixture (2-of-3 testnet) lives in `testutils.WalletConfigs["multisig"]`.

Examples:
```bash
# Singlesig PDF + PNGs (adjust paths/flags as needed)
go run cmd/cli/main.go -w multisig -o ~/PDF -png-out ~/PDF/png -dpi 600 -desc-qr-mm 25

go run cmd/cli/main.go -w singlesig \
  -mnemonic "cash zoo picture text skill steel dragon remove imitate fatal close train recipe april extra void obey sell train chaos noble rice typical below" \
  -o ~/PDF-test \
  -png-out ~/PDF-test/png \
  -dpi 1200 \
  -desc-qr-mm 25

# Multisig PDF + PNGs
go run cmd/cli/main.go -w multisig \
  -o ~/PDF \
  -png-out ~/PDF/png \
  -dpi 600 \
  -desc-qr-mm 25
```

### CLI flags (`cmd/cli/main.go`)
- `-mnemonic` (default: empty): 12- or 24-word mnemonic phrase (space-separated)
- `-descriptor` (default: empty): raw descriptor string
- `-o` (default: `/home/cmyk/PDF`): output directory (PDF)
- `-papersize` (default: `A4`): paper size (`A4` or `Letter`)
- `-verbose` (default: `false`): verbose logging
- `-w` (default: `multisig`): wallet fixture (`singlesig`, `multisig`, `multisig-mainnet-2of3`, `multisig-3of5`, `multisig-7of10`)
- `-png-out` (default: empty): optional output directory for plate PNGs (mirrored/inverted if set)
- `-dpi` (default: `600`): raster output DPI when using `-png-out`
- `-mirror` (default: `false`): mirror raster output horizontally (toner transfer)
- `-invert` (default: `false`): invert raster output (white/black swap)
- `-desc-qr-mm` (default: `80.0`): descriptor QR size in millimeters (includes safe zone)
- `-pcl-out` (default: empty): optional output path for raw PCL
- `-wallet-name` (default: empty): optional wallet name printed on plates (defaults to `SEEDETCHER`)
- `-etch-stats-page` (default: `false`): append an additional etch stats page with per-plate coverage and PSU guidance
- `-compact-2of3` (default: `false`): use compact single-sided layout for `sortedmulti` 2-of-3 descriptor shares

### Host-mode printer check (usblp)
- `image`/`image-debug` load `usblp` automatically (CONFIG_USB_PRINTER). With a USB printer attached you should see dmesg like `usblp0: USB Bidirectional printer` and `/dev/usb/lp0` present.
- Host mode uses UART for shell (no USB gadget console). Quick probe from UART:
  ```bash
  ls -l /dev/usb/lp0
  echo -e "\033%-12345X@PJL INFO ID\r\n\033%-12345X" > /dev/usb/lp0
  timeout 2 cat /dev/usb/lp0
  ```

### Raster/PCL notes
- -png-out/-dpi/-mirror/-invert/-desc-qr-mm apply to raster plate generation; resulting PDF, PNG, and PCL outputs all reflect those settings.
- `-pcl-out <path|dir>` writes raw PCL (mirrored/inverted if flags set). If a directory or trailing `/` is provided, the file is auto-named `<wallet>.pcl` inside it.
- Send PCL over USB: `scripts/print_pcl.sh <file.pcl> [printer_dev]` (defaults `/dev/usb/lp0`, resets channel and streams with `dd bs=16k`).
- On Pi host mode (`/dev/usb/lp0`), controller printing uses direct 1bpp plate-to-PCL streaming (lower memory, faster).
- Gadget mode fallback (`/dev/ttyGS1`) still uses raster page composition + PDF serialization for capture/dev flows.
- Plate QR circular data modules are intentionally undersized (`plateQRDotScale = 0.7` in `printer/raster.go`) to leave etch-growth headroom; structural islands (finder/alignment) remain square.

## Versioning

- Canonical release tag lives in `version.Tag` (update when cutting a release).
- Optional build override via ldflags, e.g.:
  ```
  go build -ldflags "-X seedetcher.com/version.Build=$(git describe --tags --dirty --always)"
  ```
  `version.String()` prefers `Build` when set, otherwise falls back to `Tag`.
- The plate renderer uses `version.String()`.

## Shell Commands on Zero

Use `-test-createPageLayout` to run controller in headless render-test mode (no GUI), so `-w/-mnemonic/-descriptor/...` flags are applied.

```bash
./controller -test-createPageLayout -verbose -w singlesig
```

This writes `/tmp/test_output.pdf` on the device (and optional PCL if `-pcl-out` is set).
If you are running a debug hot-reload build, the active binary may be `reload-a` or `reload-b` instead of `controller`.

Restarting the Controller:

```bash
./controller < /dev/ttyGS1 >> /log/debug.log 2>> /log/debug.log &
```

### Start the controller in the background

/controller &
Press Ctrl+Z to pause (suspend) the controller.
Type bg to send it to the background.

### Command	Action
Ctrl+C					Kill the foreground process.
Ctrl+Z					Suspend (pause) the foreground process.
jobs -l					List all background jobs with IDs and statuses.
fg %<job-id>		Bring a background job to the foreground.
bg %<job-id>		Resume a suspended job in the background.
kill %<job-id>	Terminate a background job.


## Debugging on Zero

Serial Terminal to Zero
`minicom -D $USBDEV0 -b 115200 -o`

Upload the new binary with while zero is running! (Rebuild if you modified flake for changes to take effect):

```bash
nix build .#controller-debug --print-build-logs
nix run .#reload $USBDEV1
```

Keep an eye on real-time logs using:
`cat $USBDEV1`

```bash
echo "input up" > $USBDEV
echo "runes TEST" > $USBDEV
echo "screenshot" > $USBDEV
```

The available button inputs are:

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

## Notes #reload corrupting the binary sent

```bash
stty -F $USBDEV1 raw -echo # needed for transfer of binary!
echo “” > $USBDEV1 		   # delete whatever is in there
```

## Converting Fonts

`go run font/bitmap/convert.go -package comfortaa -ppem 17 font/comfortaa/Comfortaa-Bold.ttf font/comfortaa/bold17`
