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
- [ ] Add an experimental image target with CUPS+GS components enabled.
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

## Exit Criteria
- If build/runtime complexity is too high or print path remains unreliable, mark as no-go and keep raw PCL/PS-only strategy.
