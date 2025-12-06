# SeedEtcher – Quick Operator Notes

## Purpose
- Air‑gapped Pi Zero firmware (fork of SeedHammer) to scan SeedQR/CompactSeedQR or manual mnemonics, validate them, and produce PDF seed plates for toner-transfer → acid etch. Seed generation is out of scope.

## What Works (last confirmed Mar 2024)
- Scan QR seed phrases on the Pi.
- Generate and capture PDFs from the Zero via USB using `scripts/capture_print.sh`.

## Environments
- Dev host: macOS; builds run inside an Ubuntu VM with Nix.
- Target: Raspberry Pi Zero (ARMv6, musl).

## Build → Flash
- Build image in Ubuntu VM: `nix build .#image-debug --impure` (outputs `result/seedetcher-debug.img`).
- Flash from mac: `scripts/flash-sdcard.sh` pointing at the built image.

## Run/Debug on Pi
- Start controller: `./controller < /dev/ttyGS1 >> /log/debug.log 2>> /log/debug.log &`
- Capture print (PDF over USB serial): `scripts/capture_print.sh`
- Test page layout without GUI: `./controller -test-createPageLayout ...` (see flags in `cmd/controller/main.go` and `testutils`).

## Testing
- Lightweight, hardware-free: `GOCACHE=/tmp/gocache ./scripts/test-lite.sh` (skips libcamera/wshat CGO deps; mostly sanity-builds packages).
- Full hardware-dependent tests are not wired up; avoid `driver/libcamera` locally unless deps installed.
- GUI loop now runs via the `Screen` state machine starting at `MainMenuScreen`; add new UI by implementing `Screen` and wiring transitions instead of expanding the old `mainFlow`.

## Host CLI (no hardware)
- Generate plates locally: `go run cmd/cli/main.go -w singlesig|multisig -o ~/outdir [-verbose]`

## Key Directories
- `cmd/controller/` – GUI entrypoint; `platform_rpi.go` (camera/display/printer), `platform_dummy.go` for non-ARM.
- `cmd/cli/` – host PDF generator.
- `gui/` – UI flow, layout, widgets.
- `seedqr/` – SeedQR & CompactSeedQR encode/decode.
- `bip39/`, `bc/` – mnemonic and descriptor parsing/validation.
- `printer/` – plate rendering and page layout (pdfcpu/gofpdf).
- `driver/` – libcamera, zbar, DRM LCD, wshat buttons (no engraver stack).
- `scripts/` – flash, capture print, printer helpers.
- `docs/` – overview and development notes (build/debug/Nix tips).

## Hardware/IO Notes
- Printer is accessed via `/dev/usb/lp0` (see `scripts/print_file.sh`, `scripts/query_printer.sh`).
- Camera uses libcamera + zbar; display via DRM LCD driver; input via wshat buttons/joystick.

## State / Gaps
- No automated tests run recently; manual flows verified up to QR→PDF capture.
- Flake pin may need hash bumps when deps change; see `docs/development.md` for update commands.
