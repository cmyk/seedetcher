# Build Matrix and USB Modes

Quick reference for which controller/image outputs to use, what they include, and how USB is configured.

## Controller binaries

| Package             | Go tags           | Extras                                                                 |
|---------------------|-------------------|------------------------------------------------------------------------|
| `controller`        | `netgo`           | Minimal production binary.                                            |
| `controller-debug`  | `netgo,debug`     | Serial console on `/dev/ttyGS1`, hot-reload command, screenshots to FAT, debug button scripting. |

Build directly if needed: `nix build .#controller` or `nix build .#controller-debug`.

## Images

| Image output               | Controller used    | USB role                         | Notes                                                         |
|----------------------------|--------------------|----------------------------------|---------------------------------------------------------------|
| `image`                    | `controller`       | Gadget (`dwc2,g_serial`)         | Console on `ttyGS0`/HDMI; no debug hooks.                     |
| `image-debug`              | `controller-debug` | Gadget (`dwc2,g_serial`)         | Adds serial console + reload flow via `/dev/ttyGS1`.          |
| `image-host`               | `controller`       | Host (`dr_mode=host`, `usblp`)   | Direct USB-printer use (`/dev/usb/lp0`); no gadget shell; UART console disabled. |
| `image-host-debug`         | `controller-debug` | Host (`dr_mode=host`, `usblp`)   | Host-mode + debug controller; debug console via UART (no gadget shell).     |

Build commands (examples):
- `nix build .#image` → `result/seedetcher.img`
- `nix build .#image-debug` → `result/seedetcher-debug.img`
- `nix build .#image-host` → `result/seedetcher-host.img`
- `nix build .#image-host-debug` → `result/seedetcher-host-debug.img`

Flash via `./scripts/flash-sdcard.sh -i seedetcher-host-debug.img` from macOS, pointing at the built image.

## Reload workflow (debug gadget images)
- Requires `image-debug` running in gadget mode (serial gadget active).
- From host: `nix run .#reload /dev/ttyACM0` (device path may vary).
- Uses the debug controller hooks to stream a new binary over `/dev/ttyGS1`.

## Choosing an image
- Development with serial shell/hot reload: use `image-debug`.
- Production/field use without shell: use `image`.
- Printing directly to USB printer (no host PC in the loop): use `image-host`/`image-host-debug` (switches OTG to host and loads `usblp` so `/dev/usb/lp0` appears).

## Host-mode TODOs (tracked in checklist)
- Add host-mode images that set `dr_mode=host` (no `g_serial` in `cmdline.txt`) and ensure `usblp` auto-loads.
- Document how to access logs/shell in host mode (likely UART).
