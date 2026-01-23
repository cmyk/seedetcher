# SeedEtcher Project Overview

## 1. Purpose
SeedEtcher is a custom Raspberry Pi Zero firmware.  
It scans QR codes (seed phrases or wallet descriptors) or accepts manual input, validates them, and outputs layout plates for **laser printing → toner transfer → acid etching**.  
It is air-gapped, GUI-driven, and supports single-sig and multisig descriptors.

Seed generation is **not** included — only processing of existing seeds.

## 2. Build & Deployment
- Dev: macOS + UTM VM with Nix cross-compile → ARMv6.
- Output: `seedetcher.img` flashed to Pi Zero.
- Debugging: serial shell, screenshots, reloads via `/dev/ttyGS1`.
- PDF capture via `/dev/ttyACM1` → VM script `capture_print.sh`.

## 3. Core Flow
1. `cmd/controller/main.go` launches GUI.
2. `gui/gui.go`: 
   - Scan QR via camera (`platform_rpi.go` + libcamera + zbar).  
   - Or manual entry (validated by `bip39.go`).  
   - Confirm/preview screens.
3. `seedqr.go`: encode/decode SeedQR & CompactSeedQR.  
4. `bip39.go`: mnemonic validation, checksum, wordlist.  
5. `printer/printer.go`: PDF generation (pdfcpu), outputs via serial.  
6. User prints on laser → transfers toner to steel → acid etches.

## 4. File Map (PDF creation focus)
- `cmd/controller/main.go` – entrypoint, GUI loop.  
- `cmd/controller/platform_rpi.go` – Pi Zero hardware (camera, serial, display).  
- `gui/gui.go` – GUI flows (scan, input, confirm).  
- `printer/printer.go` – PDF generation and output.  
