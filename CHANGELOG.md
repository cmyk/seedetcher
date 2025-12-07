# Changelog

## Unreleased
- GUI refactor: helpers split into per-screen files and run loop now starts at `MainMenuScreen` via the `Screen` state machine (no behavioral changes expected).
- Lightweight test helper remains `GOCACHE=/tmp/gocache ./scripts/test-lite.sh` (skips libcamera/wshat CGO deps).
- Fixed descriptor QR payloads: UR encoder now writes valid range derivations and QRs render with proper quiet zones, so descriptor scans work again (PDF + PNG).
