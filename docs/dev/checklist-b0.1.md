# Release checklist (b0.1 WIP)

- [X] GUI: fully Screen-structured backup flow
  - [x] Descriptor input as Screen (scan/skip/reuse, validation encapsulated)
  - [x] Seed input as Screen (camera/manual, descriptor match, dup-fp guard)
  - [x] Wallet confirm as Screen (descriptor + seed, choose key index)
  - [x] Print flow as Screen (retry on failure, return to menu on success)
  - [x] Remove SD warning before backup (Button3 hold)
  - [x] Run loop uses Screen state machine starting at MainMenu
- [x] Device sanity: menu → backup flow (SeedQR + manual) → print on hardware
- [x] Docs: AGENTS.md, GUI flowchart updated
- [x] Tests: `GOCACHE=/tmp/gocache ./scripts/test-lite.sh` clean
- [ ] GUI dedupe pass (refactor/gui-dedupe)
  - [x] Extract restart-confirm helper (reuse across descriptor/seed/confirm)
  - [x] Extract seed validation helper (dup fp / descriptor mismatch) with typed errors
  - [x] Tidy print job plumbing (desc/mnemonic/keyIdx holder)
- [x] Docs: build matrix (controllers/images, USB roles) — see `docs/dev/build-matrix.md`
- [ ] Printing without Ghostscript (Brother HL-L5000D, prefer PCL5e)
  - [x] Render plates to bitmap in Go at printer DPI with mirror flag
  - [x] PCL5e raster writer (start/end job, set resolution/page, stream rows)
  - [ ] Controller flag for raw PCL vs PDF capture (not needed; host mode defaults to PCL)
- [x] Boot/USB logistics
  - [x] Add host-mode image targets (dr_mode=host, no g_serial) and document log/shell access path (UART when in host mode) — image outputs now exposed as (`image`, `image-debug`); still needs device verification
  - [x] Make init/gadget bring-up tolerant: do not block boot if no USB host on OTG (powerbank + printer on data port)
  - [x] Confirm/restrict shell access: use `controller-debug` only for dev; release image runs non-debug binary (no gadget shell)
- [ ] Printing workflow notes
  - [x] Add CLI flag to emit raw PCL to file (e.g., `-pcl-out`/`-mirror`) for host-side testing
  - [x] Document host print test: `lp` or `cat out.pcl > /dev/usb/lp0` on Ubuntu before deploying to Zero
- [x] Docs update checkpoint (after implementing above)
- [x] Zero host-mode printing (usblp)
  - [x] Switch OTG to host mode (disable gadget overlay/modules; use `dr_mode=host` in dtoverlay)
  - [x] Ensure kernel has `usblp` (CONFIG_USB_PRINTER) built/loaded; auto-load at boot if modular
  - [x] Verify `/dev/usb/lp0` appears with printer attached; document UART/alt shell for host mode

## slice name feature/input-label
- [x] Add label keyboard for A–Z/0–9/-, uppercase only, max 20 chars with live preview
- [x] Add LabelInputScreen (title/lead, prefill default, back/clear/confirm controls)
- [x] Wire label state into backup flow before print; persist across retries
- [x] Apply chosen label to printing path (PCL/PDF) instead of hardcoded default
- [x] QA: test-lite script and manual flow: enter label, enforce limits, verify printed footer
