# CUPS+GS on musl Spike (Experimental)

## Goal
Evaluate whether adding CUPS + Ghostscript to SeedEtcher's musl image is feasible on Pi Zero for HBP/GDI printer support.

## Scope
- Throwaway experiment branch only.
- No release commitment from this doc.

## Questions
1. Can we build an image variant with CUPS + Ghostscript in current Nix/musl flow?
2. Can we print end-to-end on an HBP model (Brother HL-L2400D)?
3. What are the runtime costs on Pi Zero?

## Plan
- [x] Add isolated experimental image targets:
  - `image-cups-spike`
  - `image-cups-spike-debug`
- [ ] Boot on Pi Zero and verify services start cleanly.
- [ ] Confirm USB printer discovery path for HL-L2400D.
- [ ] Run one controlled print job end-to-end.
- [ ] Capture metrics:
  - [ ] image size delta vs release image
  - [ ] idle RAM usage
  - [ ] boot-to-ready time
  - [ ] print latency (job start -> first page out)
- [ ] Record failure modes and debugging notes.
- [ ] Decide go/no-go with clear recommendation.

## Success Criteria
- Build succeeds reproducibly.
- HL-L2400D prints a known test page from SeedEtcher path.
- Resource overhead is documented with hard numbers.

## Build Commands
```bash
# Production-like host-mode spike image
nix build .#image-cups-spike --impure

# Debug variant for shell/log access
nix build .#image-cups-spike-debug --impure
```

## Early Observation (Host VM)
- First full build attempt is heavy even before Pi testing:
  - reached deep cross toolchain compilation (`armv6l-unknown-linux-musleabihf-binutils`)
  - 6m14s elapsed before manual stop
  - peak RSS during run: ~1.8 GB
- Interpretation: feasible to attempt, but expect long first-build latency and large dependency closure.

## Runtime Finding (Pi Zero, initramfs-only image)
- Result: **no-go for CUPS+GS inside current initramfs design**.
- Symptom on UART boot log:
  - `rootfs image is not initramfs (write error); looks like an initrd`
  - `/initrd.image: incomplete write (-28 != ...)`
- `-28` (`ENOSPC`) indicates initrd/initramfs payload is too large for this boot path.
- Practical outcome:
  - boot degrades (repeating `/proc` errors / unstable startup),
  - shell/controller behavior is unreliable.
- Conclusion:
  - Do not pursue "full CUPS+GS in initramfs" further on this architecture.
  - If CUPS is still desired, test it only in an SD-card rootfs image model (persistent root filesystem), not ram-only initramfs.

## Rootfs Follow-up (current branch)
- Spike now uses an SD-backed ext4 partition for CUPS/GS closure data:
  - keep initramfs lean (controller boot path unchanged),
  - add `disk.img2` ext4 partition,
  - mount `/dev/mmcblk0p2` at `/nix` during init in spike mode.
- Requirement: kernel must include `EXT4_FS` (disabled in baseline minimal config, enabled on this spike branch).
- Requirement: kernel must include basic socket networking (`NET`, `INET`, `UNIX`) for `cupsd` listeners; otherwise `socket(...)=ENOSYS` and scheduler exits.
- New risk to validate: image size growth and boot-time mount reliability.

## Current Boot Behavior (OOB on spike image)
- In `cups-spike` images, init now:
  - mounts `/dev/mmcblk0p2` on `/nix`,
  - creates minimal `/etc/passwd` + `/etc/group` entries required by CUPS,
  - prepares CUPS runtime dirs (`/run/cups`, `/var/run/cups`, `/var/cache/cups`, `/var/spool/cups/tmp`),
  - writes a minimal `cups-files.conf`,
  - copies CUPS `ServerBin` to writable storage and fixes backend ownership/perms expected by scheduler,
  - starts `cupsd`,
  - provisions a raw queue `test` on `file:/dev/usb/lp0`.

## Runtime Findings So Far
- `cupsd`, `gs`, and `pdftops` run on Pi Zero from the rootfs-backed spike image.
- CUPS scheduler can listen on unix socket `/var/run/cups/cups.sock`.
- Direct CUPS `usb://...` backend path was unstable in this environment.
- Direct writes to `/dev/usb/lp0` print successfully.
- CUPS raw queue jobs are accepted/completed but do not produce printed output reliably in this setup.
- CUPS rewrites `file:///dev/usb/lp0` to `///dev/usb/lp0` in queue config on this target, which aligns with observed non-printing behavior.
- CUPS warns raw queues are deprecated (separate from the functional failure above).

## Decision
- **NO-GO for release integration** (current musl spike implementation).
- Keep existing direct PCL/PS path as the production strategy.
- Keep this branch as a reference experiment only.

## Exit Criteria
- If build/runtime complexity is too high or print path remains unreliable, mark as no-go and keep raw PCL/PS-only strategy.

## HBP USB Backend Follow-up (`experimental/hbp-usb-backend`)

### Why this follow-up exists
- HBP support remains high-value for end users because low-cost laser printers often lack native PCL/PS support.
- The previous spike proved that:
  - direct `/dev/usb/lp0` writes work,
  - CUPS stack can run,
  - but `raw + file:///dev/usb/lp0` did not yield physical prints.
- This branch focuses specifically on CUPS USB/backend-driven paths rather than `file:///` raw fallback.

### Scope (branch-local)
- Keep all work isolated to experimental branch.
- Do not change release behavior unless a complete end-to-end HBP print path is proven stable.
- Continue preserving direct PCL/PS production path.

### Working hypotheses to test
1. `usb://` backend path can be made reliable with correct runtime permissions/config.
2. A non-raw filter/driver chain is required for real HBP output.
3. If backend+filter path is viable, one model-specific proof (HL-L2400D or equivalent) is enough to decide whether to productize.

### Test checklist
- [ ] Confirm backend discovery works (`lpinfo -v`) on target image.
- [ ] Submit one controlled job via `usb://...` queue and verify physical page output.
- [ ] Confirm repeated jobs (>=3) print without scheduler/backend stalls.
- [ ] Capture logs for successful path (`/var/log/cups/error_log`) and record required config.
- [ ] Measure overhead:
  - [ ] boot-to-ready delta vs non-spike image
  - [ ] idle RAM delta
  - [ ] first-page latency
- [ ] Decide go/no-go for integrating HBP path into release branch.

### Acceptance criteria for "Go"
- At least one HBP printer prints SeedEtcher-generated content from Pi host mode with no manual shell setup.
- Config is reproducible after reboot.
- Resource overhead is documented and acceptable for Pi Zero constraints.
