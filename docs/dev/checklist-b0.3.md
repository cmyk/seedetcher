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
- [ ] Add/integrate custom etching font asset(s).
- [ ] Wire font into plate renderer(s) used for print output.
- [ ] Increase type size and rebalance spacing for available plate area.
- [ ] Ensure no clipping at plate edges across A4/Letter page layouts.
- [ ] Keep bitmap output canonical and match host-mode print path.

### 2) Circular QR rendering on plates
- [ ] Implement hybrid QR rendering for plate output:
  - [ ] data modules rendered as circular dots
  - [ ] structural modules remain square (finder/alignment/timing and other required islands)
- [ ] Preserve scanner reliability while using circular data modules.
- [ ] Keep quiet zone and module spacing standards-compliant.
- [ ] Verify mirrored/inverted print flags still behave correctly.

### 3) Output parity and regression checks
- [ ] Ensure captured print output matches intended plate geometry.
- [ ] Verify singlesig and multisig plate outputs against current fixtures.
- [ ] Confirm descriptor-share QR decode/recover still works end-to-end.
- [ ] Confirm no controller crashes/regressions in print/recover flows.

### 4) Test artifacts and docs
- [ ] Add visual reference fixtures for new layout (seed + descriptor plates).
- [ ] Add manual QA checklist for scan/readability on real laser prints.
- [ ] Update docs (`docs/dev/gui.md` or dedicated layout doc) with new design constraints.
- [ ] Document known scanner limits/tradeoffs for hybrid QR rendering (square islands + circular data dots).

### 5) Release prep
- [ ] Validate at least one full physical run: print -> transfer mask -> recovery scan.
- [ ] Record printer model(s), toner settings, and DPI used for acceptance.
- [ ] Freeze b0.3 layout constants after acceptance testing.
