# Changelog

## Unreleased
- (no changes yet)

## Release v0.2.0-beta.1
- Build target rename: image outputs are now host-first by default (`image`, `image-debug`), with gadget variants moved to explicit names (`image-gadget`, `image-gadget-debug`); docs and flashing examples updated to match new artifact names.
- Backup GUI flow update (multisig): `Confirm wallet` -> `Fingerprints` (paged, 5/page) -> `Descriptor shares` summary (`t/n`, `WID`, `SET`) -> `Wallet label` -> `Paper size` -> `Print`.
- Backup GUI consistency: fingerprints paging now uses the same back/check navigation pattern as other review screens, with directional arrows shown only when movement is possible.
- Printer regression fix: singlesig descriptor side no longer routes through shard splitting (`t=1,n=1`), so `cmd/cli` singlesig generation works again and emits plain descriptor QR.
- Sharded descriptor recovery/export: added fullscreen display-mode chooser (`Single QR` vs animated `Multipart UR`) with overlay-free viewer; recovery now hardens non-`SE1` inputs with explicit user-facing errors instead of crashes.
- Logging hardening: removed descriptor-content debug logging in QR scan path to avoid leaking descriptor payload details in logs.
- Interop validation: recovered animated multipart `UR:CRYPTO-OUTPUT` descriptor was verified importable in Sparrow with a `7/10` test wallet.
- Added larger multisig fixtures for stress testing: `-w multisig-3of5` and `-w multisig-7of10` in `testutils`.
- Practical guidance: current etched-plate reliability target is `n <= 10` shares pending additional physical QA.
- Release/migration note for b0.2: multisig backups now print sharded descriptor shares only (`SE1:`); full descriptor plate mode is removed in strict policy. Recovery requires scanning at least `t` shares and exporting descriptor QR to coordinator.
- Output unification: canonical plate/page rendering is now bitmap-based; both PDF and PCL are serialized from the same composed raster pages to keep host test artifacts aligned with printer jobs.
- CLI now renders once and fans out outputs from the same page set (PDF always, optional PNG plates, optional PCL via `-pcl-out`).
- Controller print path now uses the canonical bitmap pipeline for both host-mode PCL and non-PCL fallback PDF serialization.
- `-invert` now works as a real switch in raster rendering (no longer forced on when omitted).
- Controller `--test-createPageLayout` test mode now exercises the canonical bitmap pipeline and writes `/tmp/test_output.pdf` (plus optional PCL via `-pcl-out`).
- Legacy vector-PDF helpers in `printer/printer.go` are now marked deprecated.

## Release v0.1.0-beta.1
- Screen saver now absorbs button presses when active so the first input only wakes it instead of triggering the underlying screen action.
- GUI refactor: helpers split into per-screen files and run loop now starts at `MainMenuScreen` via the `Screen` state machine (no behavioral changes expected).
- Lightweight test helper remains `GOCACHE=/tmp/gocache ./scripts/test-lite.sh` (skips libcamera/wshat CGO deps).
- Fixed descriptor QR payloads: UR encoder now writes valid range derivations and QRs render with proper quiet zones, so descriptor scans work again (PDF + PNG).
- Printer hotplug/status: host-mode now watches `/dev/usb/lp*` (blocking inotify) with a 500 ms poll fallback, resets printer cache on events, and wakes the GUI so the print screen shows connected/disconnected + model reliably across plug/unplug cycles (with status logs in `/log/debug.log`).
- Wallet label entry: added on-device label keyboard/screen (A–Z/0–9/- capped at 20 chars), wired label through backup flow to print jobs, and apply chosen label to PCL/PDF output; simplified hint text and layout.
- Versioning: added `version` package with release `Tag` and optional build override; plates now print `version.String()` instead of a hardcoded "V1".
- Host prod hardening: `image-host` no longer enables the UART console/getty; debug host image keeps UART.
- Screen saver now absorbs button presses when active so the first input only wakes it instead of triggering the underlying screen action.
