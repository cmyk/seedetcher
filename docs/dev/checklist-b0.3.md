# SeedEtcher b0.3 Checklist

## Goal
- Deliver the new etch-optimized plate layout with the custom rounded font and circular QR module rendering for improved transfer/etch reliability.

## Scope
- In scope:
  - New plate typography/layout using custom etching font.
  - Circular QR module rendering on plate outputs.
  - Adopt interoperable descriptor share encoding for 2-of-3 multisig (UR/XOR).
  - Validate print readability and recovery scan reliability.
- Out of scope:
  - Finalizing custom compact descriptor share formats (`SE1`/`SE2`) as release defaults.
  - Recovery CLI/break-glass tooling (moved to b0.4).

## Milestones

### 1) Font integration and layout update
- [x] Add/integrate custom etching font asset(s).
- [x] Wire font into plate renderer(s) used for print output.
- [x] Increase type size and rebalance spacing for available plate area.
- [x] Ensure no clipping at plate edges across A4/Letter page layouts.
- [x] Keep bitmap output canonical and match host-mode print path.

### 2) Circular QR rendering on plates
- [x] Implement hybrid QR rendering for plate output:
  - [x] data modules rendered as circular dots
  - [x] structural modules remain square (finder/alignment/timing and other required islands)
- [x] Preserve scanner reliability while using circular data modules.
- [x] Keep quiet zone and module spacing standards-compliant.
- [x] Calibrate circular data-module dot scale for etch growth headroom (current target: `0.7`) while keeping finder/alignment islands square.
- [x] Verify mirrored/inverted print flags still behave correctly.

### 3) Output parity and regression checks
- [x] Ensure captured print output matches intended plate geometry.
- [x] Verify singlesig and multisig plate outputs against current fixtures.
- [x] Confirm descriptor-share QR decode/recover still works end-to-end.
- [x] Confirm no controller crashes/regressions in print/recover flows.

### 4) Test artifacts and docs
- [x] Add visual reference fixtures for new layout (seed + descriptor plates).
- [x] Add manual QA checklist for scan/readability on real laser prints.
- [x] Add optional etch stats page (additional print page) across print paths (CLI/controller host+gadget) with clear per-plate coverage metrics (`mm²` and `%`) mapped to each printed plate side.
  - [x] Fixed physical plate model: `100x100 mm` basis with masked/unmasked margin scenarios.
  - [x] Include operator-ready PSU guidance table per plate (`Set A masked` / `Set A unmasked`) derived from exposed area.
  - [x] Include global bench defaults block (`Na2SO4 100 g/L`, `34C`, `15 mm` gap, `12 V` limit, `J=0.04 A/cm²`).
- [x] Update docs (`docs/dev/gui.md` or dedicated layout doc) with new design constraints.
- [x] Update CHANGELOG.md
- [x] Document known scanner limits/tradeoffs for hybrid QR rendering (square islands + circular data dots).

### 5) Release prep
- [ ] Validate at least one full physical run: print -> transfer mask -> etch -> recovery scan.
- [x] Record printer model(s), toner settings, and DPI used for acceptance.
- [ ] Freeze b0.3 layout constants after acceptance testing.
- [x] Bumb version

### 6) UR/XOR 2-of-3 migration (interoperability-first)
- [x] Mark `SE1`/`SE2` path as experimental-only (non-release default for 2-of-3).
- [x] Implement UR/XOR descriptor share generation for 2-of-3:
  - [x] deterministic split assignment `A`, `B`, `A⊕B`
  - [x] deterministic descriptor canonicalization before split
- [x] Implement UR/XOR descriptor share recovery for 2-of-3:
  - [x] accept any 2 shares
  - [x] reconstruct full descriptor payload deterministically
  - [x] reject mixed/invalid share sets with clear UI message
- [x] Wire UR/XOR into backup/recover GUI flow as the default 2-of-3 path.
- [x] Reuse compact single-sided 2-of-3 layout with UR/XOR payloads.
- [ ] Add test vectors and regression tests:
  - [x] stable share payload strings for fixture wallet(s)
  - [x] all pairwise recovery combinations pass (`C(3,2)=3`)
- [ ] Validate interoperability in external wallets (Sparrow, Nunchuk, BlueWallet).
- [x] Update docs/changelog for UR/XOR migration and experimental status of `SE1`/`SE2`.

### 7) UR/XOR family support (SeedHammer II parity)
- [ ] Define and document supported UR/XOR wallet families for b0.3:
  - [x] `1/1` stays singlesig-special (no UR/XOR share mode)
  - [x] `2/2`
  - [x] `2/3` (already complete)
  - [x] `2/4`
  - [x] `3/5`
  - [x] generic `m = n-1` family
- [x] Implement UR/XOR descriptor-share generation for `3/5`:
  - [x] deterministic descriptor canonicalization (same rules as `2/3`)
  - [x] deterministic part assignment per share
  - [x] stable share ordering and payload encoding
- [x] Implement UR/XOR descriptor-share recovery for `3/5`:
  - [x] accept any 3 of 5 shares
  - [x] reject duplicates/mixed sets/invalid fragments with explicit errors
  - [x] deterministic reconstruction output
- [ ] Add tests for `3/5`:
  - [x] all pairwise/combination recovery tests (`C(5,3)=10`)
  - [x] stable share payload vectors for fixture wallet(s)
  - [x] matrix regression coverage added for representative script/network families
- [x] Implement generic `m = n-1` UR/XOR support:
  - [x] generation path
  - [x] recovery path
  - [x] capability guardrails in UI/CLI (clear supported/unsupported messaging)
- [x] Fallback policy for unsupported families:
  - [x] no SE1/SE2 release fallback
  - [x] full descriptor UR per descriptor plate
  - [x] explicit warning shown during backup flow
- [x] Interop validation matrix for expanded families:
  - [x] Sparrow
  - [x] Nunchuk
  - [x] BlueWallet
- [x] Docs/changelog updates for expanded UR/XOR family support.
