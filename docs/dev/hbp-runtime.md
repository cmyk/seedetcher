# HBP Runtime (Release)

This document describes the integrated Brother HBP runtime used in release builds.

## Scope
- HBP runtime support is integrated into standard image outputs.
- PCL/PS remain preferred when the printer supports them.
- HBP is an opt-in path for printers that do not reliably support PCL/PS.

## Runtime Components
- Bootstrap script: `/bin/cups-runtime-bootstrap`
- RAM staging helper: `/bin/cups-runtime-ram-feasibility`
- HBP print helper: `/bin/print-hbp-pdf`
- Runtime env file in initramfs: `/cups-runtime.env`

## High-Level Flow
1. Device boots normally; HBP runtime is not preloaded.
2. User chooses `Enable HBP` in the startup gate.
3. Controller runs:
   - `cups-runtime-bootstrap`
   - `cups-runtime-ram-feasibility stage core`
   - `cups-runtime-ram-feasibility detach-sd`
4. UI marks HBP runtime ready; SD can be removed.
5. HBP print jobs use `print-hbp-pdf` + CUPS queue `test-hbp`.

## Printer Language Behavior
- Preferred path: `PCL` or `PS`.
- HBP path is capped to 600 DPI.

Reason: current `brlaser` HBP path has incorrect print geometry at 1200 DPI on tested models; 600 DPI is used for correct layout.

## Memory and SD Behavior
- Runtime binaries/libs for CUPS/GS/brlaser are staged to RAM (`/run/hbp-ram-runtime`).
- `/nix` is rebound to RAM-backed content before SD detach.
- SD detach is verified so `/dev/mmcblk0p*` mounts are removed.

## Debugging and Logs (Debug Images)
- Debug images include log export tooling.
- Logs are exported to SD under `SE-LOGS-LATEST`.
- PJL snapshot is captured during error export when printer device is present.

## Known Limitations
- HBP 1200 DPI is not enabled in release path due geometry mismatch.
- HBP queue depends on CUPS runtime availability after staging.
- USB printer reconnect timing can affect first queue provisioning attempt; bootstrap includes retry behavior.

## Quick Validation Checklist
1. Build and flash a standard image (`image-debug` recommended for bring-up).
2. In UI, choose `Enable HBP` and wait for ready confirmation.
3. Remove SD card and confirm no `/dev/mmcblk0p*` mounts remain.
4. Print:
   - singlesig
   - multisig (e.g. 3/5, 7/10)
   - with and without etch stats page
5. Verify layout correctness and successful job completion.

## Historical Note
Feasibility exploration and failed approaches (early initramfs-only experiments) are tracked in the experimental branch history.
