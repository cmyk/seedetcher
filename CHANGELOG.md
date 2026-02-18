# Changelog

## Unreleased
- (no changes yet)

## Release v0.3.0-beta.1
- b0.3 plate layout overhaul (seed + descriptor sides) with etched-first styling:
  - custom `SeedEtcher-Regular` plate font integration,
  - circular QR modules with square islands,
  - updated seed/descriptor text anchors, margins, and metadata placement.
- Invert behavior fix: plate border remains black while interior content is inverted.
- Page layout updates:
  - 4 mm inter-plate spacing,
  - top-anchored placement for partial pages,
  - no automatic plate scaling (plates remain true 90x90mm),
  - Letter layout switched to 2x2 (4 plates/page) to preserve fixed plate size.
- Pi image packaging fix: include `font/seedetcher/SeedEtcher-Regular.ttf` in initramfs.
- Host-mode print pipeline performance/memory refactor:
  - direct 1bpp plate-to-PCL streaming path (`/dev/usb/lp0`) without full-page raster buffers,
  - batched host rendering/sending to reduce peak RAM on larger jobs,
  - host default DPI set to 1200, gadget fallback path kept at 600.
- Host print-progress behavior stabilized for batched sending (continuous/monotonic progress, compose marked once).
- Test fixtures expanded with additional seed-only wallets: 12/15/18/21 words.
- Added `singlesig-longwords` seed-only fixture for plate layout stress testing.
- Docs updated to clarify host (direct 1bpp PCL) vs gadget (raster-to-PDF fallback) print paths.
- Paper-size selection is now honored end-to-end in both host and gadget print pipelines (Letter stays 2x2, A4 stays 2x3).
- Wallet label input limit reduced from 20 to 15 characters.
- Additional plate typography tuning:
  - 11pt metadata/descriptor tracking support,
  - tighter number-column tracking on seed plates,
  - wider gutter between seed index numbers and words.
- Print options UI added before printing:
  - selectable `DPI` (`1200`/`600`), `Invert` (`On`/`Off`), and `Mirror` (`On`/`Off`),
  - defaults set to `1200`, `On`, `On`,
  - options are now passed through controller print flow; non-PCL fallback remains capped at `600 DPI`.

## Release v0.2.0-beta.2
- Security dependencies bumped to address Dependabot alerts: `github.com/btcsuite/btcd` -> `v0.25.0`, `github.com/btcsuite/btcd/btcec/v2` -> `v2.3.6`, `github.com/btcsuite/btcd/btcutil` -> `v1.1.6`, and `golang.org/x/crypto` -> `v0.45.0` (plus related `x/sys`/`x/text` updates).
- Toolchain baseline updated to Go `1.24` (`go.mod`) and Nix Go toolchain pins switched to `go_1_24`; `go-deps` fixed-output hash refreshed in `flake.nix`.
- CodeQL hardening (Go): fixed unsafe integer conversion paths in descriptor shard setup and parser/formatter numeric parsing (`printer/raster.go`, `nonstandard/parse.go`, `gui/text/text.go`).
- CodeQL hardening (C): added explicit signed overflow boundary checks in `zbar/qrdec.c` where affine-step accumulation could overflow.
- Compatibility maintenance: minor Go 1.24 vet-format compatibility fix in `gui/screen_label.go`.
- Validation: `scripts/test-lite.sh`, targeted package tests, `nix build .#image`, and Pi smoke test passed.

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
