# Release checklist (b0.2 Descriptor Hardening — Shamir Descriptor Shards)

**DRAFT (implementation-scoped)**

Goal: No single plate reveals the wallet descriptor. Descriptor is recoverable offline via SeedEtcher by scanning ≥t shards and exporting the full descriptor as QR.

Reference spec: `docs/dev/spec-sharded-descriptor-b0.2.md`

## Locked decisions for b0.2
- Policy: **Option A (strict)** for sharded mode.
- Shard scheme: **GF(256) Shamir for arbitrary bytes (SSKR-style split/reconstruct)**.
- Canonical descriptor payload: normalized descriptor string **with checksum included**.
- Multisig backup uses **sharded descriptor only** (no legacy full-descriptor plate mode).
- Singlesig backup does not require descriptor sharding.
- Recovery reconstructs descriptor in RAM only and exports QR; no secret persistence/logging.
- **Release-blocking addendum:** descriptor shard `SETID` must be deterministic/reproducible for identical descriptor input (no random default), so users can regenerate identical plate layouts for reprint.

## 0) Security model (must be explicit)
- [x] Define threat model: "plate compromised" => attacker must NOT learn wallet descriptor/xpub set
- [x] Define what remains non-secret on plates (e.g., wallet label, network, script type hint) and why
- [x] Define what is secret: full descriptor string (incl xpubs/paths)
- [x] Decide policy for xpub presence on plates:
  - [x] Option A (strict): no xpubs anywhere except reconstructed descriptor in recovery mode
  - [x] Option B (pragmatic): not selected for b0.2 (explicitly rejected)

## 1) Shard scheme + encoding
- [x] Pick scheme:
  - [x] Use SLIP-39 (Shamir) style encoding OR not selected for b0.2
  - [x] Use SSKR / GF(256) Shamir for arbitrary bytes
- [x] Define shard metadata (must be in every shard):
  - [x] wallet_id (short hash / fingerprint)
  - [x] group_id / set_id (random per wallet)
  - [x] threshold t, share index i, total n (or infer n)
  - [x] version + network (main/test) + script type (wsh/wpkh/tr) as non-secret hints
  - [x] checksum/MAC for integrity (detect typos + wrong shares)
- [x] Canonicalize descriptor before splitting (must be deterministic):
  - [x] Normalize whitespace
  - [x] Ensure checksum handling is consistent (store with/without checksum; document it)
- [x] Decide maximum QR size / encoding (base32/base64/UR):
  - [x] Confirm shard fits as single QR for typical multisig descriptors (validated with `multisig-3of5` and `multisig-7of10`; practical cap currently treated as `n <= 10` for etch reliability)
  - [x] If not: define UR/multipart strategy for shards and for reconstructed descriptor

## 2) UI/UX changes (controller)
- [x] Enforce descriptor policy in backup flow:
  - [x] Multisig: sharded descriptor only (no legacy full-descriptor option)
  - [x] Singlesig: keep non-sharded flow
- [x] Sharded descriptor creation screens:
  - [x] Derive n and t from descriptor (read-only confirmation, no user choice)
  - [x] Generate wallet_id + set_id; show confirmation
  - [x] Display each shard as QR and/or print it per plate
  - [x] Ensure shards are shown/printed one-at-a-time with explicit “Next share” action
- [x] Validation:
  - [x] Refuse to mix shards with different wallet_id/set_id
  - [x] Refuse wrong threshold / version mismatch
  - [x] Detect invalid checksum/MAC

## 3) Plate / print layout changes
- [x] Define what each plate contains (recommended):
  - [x] Seed phrase / key material for that plate (existing)
  - [x] Descriptor shard QR for that plate (new)
  - [x] Wallet label (non-secret)
  - [x] wallet_id + share index i + threshold t (human-readable)
- [x] Remove full descriptor from plate layout when sharded mode is used
- [x] Add clear on-plate warning text:
  - [x] “Descriptor is sharded — need t shares to recover”
- [x] QA: printing pipeline supports shard QR (contrast, size, error correction)

## 4) Recovery mode (SeedEtcher as reconstructor)
- [x] Add MainMenu entry: “Recover Descriptor”
- [x] Recovery flow:
  - [x] Reject plain descriptor QR with explicit message in shard recovery flow
  - [x] Scan share 1
  - [x] Scan share 2..t (progress indicator)
  - [x] Validate all shares (wallet_id/set_id/version/network)
  - [x] Reconstruct full descriptor (in RAM only)
  - [x] Display reconstructed descriptor as QR (single or UR animated)
  - [x] Optional: show descriptor text behind “hold-to-reveal” (explicitly deferred by policy; QR-only export in b0.2)
  - [x] “Done” exits and wipes RAM state
  - [x] If input is plain descriptor QR, show non-sharded warning and continue scanning
- [x] No persistence:
  - [x] Do not write descriptor/shares to disk
  - [x] Do not log secret material

## 5) Interop targets (Sparrow etc.)
- [x] Verify Sparrow import path for descriptor QR (what exact payload it expects)
- [x] Ensure QR payload matches standard descriptor format Sparrow accepts
- [x] If multipart/UR is needed:
  - [x] Confirm target wallets support it; otherwise keep single-QR as requirement

## 6) Tests
- [x] Unit: descriptor canonicalization is stable
- [x] Unit: split/reconstruct round-trip for many random descriptors
- [x] Unit: wrong-share detection (wallet_id mismatch, checksum fail)
- [x] Unit: threshold behavior (need <t fails; >=t succeeds)
- [x] Integration: simulated scan flow reconstructs descriptor and renders QR
- [x] Regression: multisig backup path offers sharded descriptor only

## 7) Docs
- [x] Update workflow doc: explain why descriptor is secret + sharded
- [x] Add recovery instructions (cold-room guidance):
  - [x] “Scan t shards -> export descriptor QR -> import in Sparrow”
- [x] Add attack-surface notes:
  - [x] “Recovery QR reveals wallet structure; treat as sensitive”
- [x] Add troubleshooting: mixed shares, checksum fail, QR too large

## 8) Release gates
- [x] End-to-end hardware test:
  - [x] Create sharded wallet -> print plates -> recover descriptor -> import in Sparrow
  - [x] Derive addresses match
- [x] test-lite clean
- [x] No secret material in logs (grep quick check; descriptor-content debug log removed from scan path)
- [x] Tag + signed release notes mention descriptor hardening + migration notes (release notes prepared in changelog)
- [ ] Reproducibility gate: re-running backup for the same descriptor yields identical descriptor shard QR payloads (`WID` + `SET` + share contents) across sessions.
- [ ] Bump `version/version.go` release tag before creating the release tag.
- [ ] Update `CHANGELOG.md` (move relevant notes out of `Unreleased`) before tagging the release.
