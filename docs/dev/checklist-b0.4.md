# SeedEtcher b0.4 Checklist

## Goal
- Deliver a standalone, cross-platform, offline recovery tool focused on descriptor-share compatibility and inheritance/break-glass workflows without Pi hardware.

## Scope
- In scope:
  - Host CLI to recover descriptor payload from legacy `SE1:` shares (migration support).
  - Host CLI decode support for UR/XOR descriptor-share inputs used in b0.3 release flow.
  - Cross-platform builds (macOS, Linux, Windows).
  - Offline-first docs and reproducible release artifacts.
- Out of scope:
  - Webcam/camera scanning.
  - Network services.
  - Browser app as canonical recovery path.

## Milestones

### 1) CLI recovery command
- [ ] Add `cmd/recover/main.go`.
- [ ] Accept share input via file and stdin.
- [ ] Parse and validate `SE1:` shares.
- [ ] Reconstruct payload via `descriptor/shard`.
- [ ] Output:
  - [ ] descriptor text
  - [ ] `UR:CRYPTO-OUTPUT`
- [ ] Add clear, deterministic error messages for:
  - [ ] duplicate share index
  - [ ] mixed set IDs
  - [ ] insufficient shares
  - [ ] malformed share payload

### 2) Test vectors and verification
- [ ] Add roundtrip tests for CLI using known fixture shares.
- [ ] Add negative tests (bad/mixed/incomplete share sets).
- [ ] Verify CLI output matches controller recovery output for same inputs.

### 3) Build and release artifacts
- [ ] Add `scripts/build-recover-cli.sh`.
- [ ] Build targets:
  - [ ] `darwin/arm64`
  - [ ] `darwin/amd64`
  - [ ] `linux/amd64`
  - [ ] `linux/arm64`
  - [ ] `windows/amd64`
- [ ] Use `CGO_ENABLED=0` for portable binaries.
- [ ] Generate `SHA256SUMS` for all artifacts.

### 4) Documentation
- [ ] Add `docs/dev/recover-cli.md`:
  - [ ] offline usage workflow
  - [ ] sample commands
  - [ ] inheritance/break-glass instructions
  - [ ] compatibility limits (legacy `SE1:` is SeedEtcher-native; UR/XOR follows interoperable wallet share formats)
- [ ] Update top-level docs index to link b0.4 checklist and recovery CLI doc.

### 5) Stretch goals (if time allows)
- [ ] Optional output file formats (`.txt`, `.json`).
- [ ] Optional strict mode requiring exact threshold count.
- [ ] Optional share order normalization output for auditing.
