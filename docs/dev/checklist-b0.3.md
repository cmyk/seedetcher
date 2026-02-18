# SeedEtcher b0.3 Checklist

## Goal
- Deliver the new etch-optimized plate layout with the custom rounded font and circular QR module rendering for improved transfer/etch reliability.

## Scope
- In scope:
  - New plate typography/layout using custom etching font.
  - Circular QR module rendering on plate outputs.
  - Keep descriptor/seed content and share semantics unchanged.
  - Validate print readability and recovery scan reliability.
- Out of scope:
  - New sharding formats.
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
- [ ] Add manual QA checklist for scan/readability on real laser prints.
- [x] Update docs (`docs/dev/gui.md` or dedicated layout doc) with new design constraints.
- [x] Update CHANGELOG.md
- [x] Document known scanner limits/tradeoffs for hybrid QR rendering (square islands + circular data dots).

### 5) Release prep
- [ ] Validate at least one full physical run: print -> transfer mask -> recovery scan.
- [ ] Record printer model(s), toner settings, and DPI used for acceptance.
- [ ] Freeze b0.3 layout constants after acceptance testing.
- [ ] Bumb version
