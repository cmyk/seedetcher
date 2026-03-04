
# SeedEtcher Development Notes

When building and you added/removed pkgs, do

```bash
nix flake lock --update-input nixpkgs
nix flake update

nix build .#image-gadget-debug
```

If build fails because of has error use `--impure`
Debug builds use `--print-build-logs`

## Local Machine Notes

Host/VM-specific setup notes are intentionally kept outside this repository.

- keep private notes in your own local/private repo (example: `~/seedetcher-private-notes`)
- optional symlink from this repo: `.tmp/private-notes`

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
- `-w` (default: `multisig`): wallet fixture (`seed-12`, `seed-15`, `seed-18`, `seed-21`, `singlesig`, `singlesig-longwords`, `singlesig-nested-p2sh-p2wpkh`, `multisig`, `multisig-mainnet-2of3`, `multisig-nested-2of3`, `multisig-2of2`, `multisig-2of4`, `multisig-3of4`, `multisig-3of5`, `multisig-4of7`, `multisig-5of7`, `multisig-7of10`)
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

### Release image builds (`mkRelease`)

Set the release tag in `version/version.go` and run:

```bash
nix run .#mkRelease
```

This builds from the current checkout by default (works on forks), then writes:

```text
release/seedetcher-vX.Y.Z.img
```

To override version explicitly:

```bash
nix run .#mkRelease -- vX.Y.Z
```

To build/stamp from a different flake ref, set:

```bash
SE_RELEASE_FLAKE=github:seedetcher/seedetcher/vX.Y.Z nix run .#mkRelease -- vX.Y.Z
```

### Reproducibility check

Release images are intended to be deterministic. Verify your local build hash against the published artifact hash:

```bash
sha256sum release/seedetcher-vX.Y.Z.img
# or on macOS:
shasum -a 256 release/seedetcher-vX.Y.Z.img
```

### Third-party license release check

Release images include third-party printing/runtime components (CUPS, cups-filters,
Ghostscript, brlaser). Before publishing a release:

1. Review/update [`THIRD_PARTY_LICENSES.md`](../THIRD_PARTY_LICENSES.md).
2. Ensure `flake.lock` is committed in the release tag.
3. In release notes, link to:
   - the source tag/commit,
   - `THIRD_PARTY_LICENSES.md`,
   - any local patch/build changes for bundled GPL/AGPL components.

Minimal release-note snippet:

```md
## Source and Licensing
- Source tag: <TAG>
- Third-party licenses: THIRD_PARTY_LICENSES.md
- GPL/AGPL local patches: none
```

For the complete release flow, see:
`docs/dev/release-checklist.md`

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
