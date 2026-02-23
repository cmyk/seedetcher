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

## Exit Criteria
- If build/runtime complexity is too high or print path remains unreliable, mark as no-go and keep raw PCL/PS-only strategy.
