# Changelog

## Unreleased
- GUI refactor: helpers split into per-screen files and run loop now starts at `MainMenuScreen` via the `Screen` state machine (no behavioral changes expected).
- Lightweight test helper remains `GOCACHE=/tmp/gocache ./scripts/test-lite.sh` (skips libcamera/wshat CGO deps).
- Fixed descriptor QR payloads: UR encoder now writes valid range derivations and QRs render with proper quiet zones, so descriptor scans work again (PDF + PNG).
- Printer hotplug/status: host-mode now watches `/dev/usb/lp*` (blocking inotify) with a 500 ms poll fallback, resets printer cache on events, and wakes the GUI so the print screen shows connected/disconnected + model reliably across plug/unplug cycles (with status logs in `/log/debug.log`).
- Wallet label entry: added on-device label keyboard/screen (A–Z/0–9/- capped at 20 chars), wired label through backup flow to print jobs, and apply chosen label to PCL/PDF output; simplified hint text and layout.
