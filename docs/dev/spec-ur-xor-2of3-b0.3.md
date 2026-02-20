# SeedEtcher Spec: UR/XOR Descriptor Shares (2-of-3, b0.3)

## Goal
- Replace custom descriptor share defaults with an interoperability-first 2-of-3 scheme based on UR-compatible XOR partitioning.
- Keep compact single-sided plate layout work, but change descriptor payload semantics to UR/XOR.

## Scope
- In scope:
  - Sortedmulti 2-of-3 descriptor share generation and recovery.
  - Deterministic assignment of descriptor parts to shares.
  - Integration into existing backup/recover GUI flow.
- Out of scope:
  - 3-of-5 and 7-of-10 UR/XOR generalization.
  - SE1/SE2 removal from repository (kept experimental).

## Canonicalization
- Before split, canonicalize descriptor payload deterministically:
  - normalize sortedmulti key order;
  - normalize missing children for sortedmulti (`/<0;1>/*`) as needed;
  - encode descriptor payload bytes from canonical structure.

## 2-of-3 Share Assignment
- Split canonical descriptor payload into two byte parts `A` and `B`:
  - deterministic partition rule (fixed for all implementations);
  - odd-byte handling must be deterministic and documented.
- Build third part as XOR:
  - `C = A ⊕ B` (byte-wise over padded/equalized part length).
- Share mapping:
  - Share 1 -> `A`
  - Share 2 -> `B`
  - Share 3 -> `C`
- Any two shares must recover full canonical payload.

## Encoding
- Encode each share payload via existing UR stack (`bc/ur`, bytewords/fountain-compatible framing).
- Keep share metadata minimal; avoid embedding non-essential plaintext that reduces privacy.

## Recovery
- Accept any 2 of 3 shares from the same set.
- Recover missing part using XOR algebra:
  - `A = B ⊕ C`
  - `B = A ⊕ C`
  - `A || B` from shares 1+2.
- Reconstruct canonical descriptor payload and parse/validate before export.
- Reject mixed-format sets (e.g., SE1/SE2 mixed with UR/XOR set) with explicit UI errors.

## UI/Flow
- For 2-of-3 backup:
  - use UR/XOR shares by default;
  - keep compact single-sided layout (seed QR + descriptor share QR).
- For 2-of-3 recovery:
  - recover descriptor from any two UR/XOR shares;
  - keep existing export options (QR single/multipart behavior unchanged unless separately specified).

## Determinism & Reprint
- Same canonical descriptor input must produce byte-identical share payloads.
- Determinism is required for exact reprint workflows.

## Tests
- Golden payload strings for fixture wallet(s).
- Recovery matrix test for all 3 pair combinations.
- Canonicalization invariance tests:
  - key reorder input yields identical shares;
  - omitted children yields identical shares after normalization.
- End-to-end GUI flow regression for backup -> recover -> export.

## Release Positioning
- UR/XOR 2-of-3 is release-path for b0.3.
- `SE1`/`SE2` remain experimental and non-default.
