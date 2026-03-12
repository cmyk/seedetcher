# SeedEtcher b0.4 Checklist

## Goal
- Add a direct laser-ablation path (Acmer K1 / GRBL) from SeedEtcher plate data while preserving current print behavior.
- Move toward a single source of truth for layout: vector-first `PlateScene`, then backend renderers.

## Scope
- In scope:
  - Single-plate laser output (`100x100mm`), no paper/cutbox pagination in laser mode.
  - Direct GRBL streaming in host mode over USB serial.
  - Vector-first layout model and renderer split.
  - Maintain visual morphology parity with current etched print style (dot QR + rounded islands + font geometry).
- Out of scope (initial b0.4):
  - LightBurn dependency/workflow.
  - Multi-plate nesting/auto-arrange in one bed job.
  - Camera/G-code preview UI.

## Milestones

### 1) Architecture foundation (single source of truth)
- [x] Define `PlateScene` domain model (mm-based coordinates):
  - [x] primitives: path, circle, rounded-rect, text outline, transform/group
  - [x] layer tags: `mask`, `guide` (guide disabled by default)
  - [ ] deterministic origin and bounds (`100x100mm`)
- [x] Enforce OOP boundaries in Go:
  - [x] domain objects: `PlateDocument`, `PlateScene`, primitive structs
  - [x] builder interfaces: `SceneBuilder` (layout only)
  - [x] renderer interfaces: `SceneRenderer` (output only)
  - [x] concrete builders/renderers in separate files/packages
  - [x] no backend-specific branches inside scene builders
- [ ] Define renderer interfaces:
  - [ ] `SceneRasterRenderer` (for print path parity checks)
  - [x] `SceneGCodeRenderer` (for GRBL output)
  - [x] optional `SceneSVGRenderer` (debug only)
- [ ] Add visual/debug CLI outputs from the same scene:
  - [x] `-scene-json-out <file>` (canonical scene dump for diff/tests)
  - [x] `-svg-out <dir>` (human visual inspection)
  - [ ] `-png-out <dir>` from scene raster renderer (quick morphology parity)
- [ ] Keep current print pipeline untouched in this phase.

### 2) PlateScene builder (layout migration)
- [x] Build scene for seed plate from existing layout rules.
- [x] Build scene for descriptor plate from existing layout rules.
- [x] Add explicit layout-variant architecture (avoid branchy/procedural drift):
  - [x] define `LayoutSpec` interface (implemented shape):
    - [x] `Name()`
    - [x] `Supports(RenderContext, LayoutSelection)`
    - [x] `Apply(RenderContext, LayoutSelection)`
    - [x] `RenderBitmaps(...)`
    - [x] `BuildScenes(...)`
  - [x] define `LayoutRegistry`/selector:
    - [x] maps flags/context to exactly one active layout spec
    - [x] no scattered `if compact2of3/...` checks in render loops
  - [ ] first concrete specs:
    - [x] `ClassicLayoutSpec`
    - [x] `Compact2of3LayoutSpec`
    - [ ] `SinglesigSeedInfoLayout`
    - [ ] `SinglesigSeedDescLayout`
  - [x] ensure scene and raster backends consume the same selected layout plan.
- [ ] Preserve current styling semantics:
  - [ ] QR data modules as circles (dot scale parity)
  - [ ] registration/finder islands as rounded squares
  - [ ] exact text anchors/margins/tracking behavior
- [x] Add scene-level geometry snapshot tests for key fixtures.

### 3) Direct G-code backend (GRBL)
- [ ] Add CLI output mode for laser:
  - [x] `-gcode-out <dir>`
  - [x] `-side seed|desc|both`
  - [x] `-plate-mm` (default `100`)
  - [ ] laser params:
    - [x] `-laser-max-s`
    - [x] `-laser-feed`
    - [x] `-rapid-feed`
  - [x] no-send mode by default (generate files first, stream later)
- [ ] Emit GRBL-safe preamble/footer:
  - [x] `G21`, `G90`, `M4`/`M5`, sane feed defaults
  - [x] configurable power scaling `S`
- [ ] Implement fill/trace strategy for closed vector regions.
- [ ] Add deterministic output tests for small canonical plate fixtures.

### 4) USB serial transport to K1 (host mode)
- [ ] Add GRBL serial transport (`/dev/ttyACM*` or `/dev/ttyUSB*`):
  - [ ] line-by-line send with `ok`/`error` ack handling
  - [ ] timeout and retry policy
  - [ ] alarm/reset handling (`?`, soft reset, unlock flow)
- [ ] Ensure transport is isolated from printer `/dev/usb/lp*` path.
- [ ] Add a dry-run mode that writes G-code only (no device send).

### 5) Validation and parity
- [ ] Visual parity tests (scene vs current raster) for:
  - [ ] singlesig
  - [ ] multisig 2-of-3
  - [ ] multisig 3-of-5
- [ ] Physical validation on K1:
  - [ ] seed side readability (words + QR)
  - [ ] descriptor side readability
  - [ ] scan success in Sparrow/Seed tools
- [ ] Regression check: existing PCL/PS/HBP print flows unchanged.
- [ ] After `v0.3.0-beta.2`, backport HBP model/PPD hotfix (`hotfix/b0.3-hbp-model-scaling`) into:
  - [ ] `release/0.4.0-beta`
  - [ ] `feature/b0.4-plate-scene-foundation`
  - [ ] `docs/wallet-code-standard`

### 6) Documentation and release
- [ ] Add `docs/dev/laser-grbl.md`:
  - [ ] machine setup
  - [ ] CLI examples
  - [ ] safe power/feed starting profiles
  - [ ] troubleshooting (`error`, `alarm`, mis-scale, offsets)
- [ ] Update `docs/development.md` with laser output flags.
- [ ] Add changelog entry when laser path is user-ready.

## Stretch goals
- [ ] Real-time progress reporting for GRBL send in UI.
- [x] Optional SVG export from `PlateScene` for inspection/debug.
- [ ] Optional path optimization pass (travel reduction).
