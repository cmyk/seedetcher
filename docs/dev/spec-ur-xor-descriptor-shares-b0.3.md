# SeedEtcher Spec: UR/XOR Descriptor Shares (b0.3)

## Goal
- Use interoperable UR/XOR descriptor shares wherever SeedHammer-compatible schemes exist.
- Avoid non-interoperable default formats in release flow.

## Scope
- In scope:
  - `sortedmulti` UR/XOR share generation + recovery for supported families.
  - Deterministic descriptor canonicalization before share generation.
  - Explicit fallback for unsupported families.
- Out of scope:
  - New custom descriptor-share formats (`SE1`/`SE2`) as release defaults.

## Supported Families (release path)
- UR/XOR supported:
  - `n-1/n` (for example `2/3`, `4/5`, `9/10`)
  - `2/4`
  - `3/5`
- Singlesig `1/1`:
  - no descriptor share-splitting mode; descriptor QR behavior remains singlesig-specific.

## Unsupported Families
- For multisig families outside supported UR/XOR schemes (for example `7/10`):
  - do **not** use SE1/SE2 fallback.
  - print full descriptor `UR:CRYPTO-OUTPUT` on each descriptor plate.
  - show an explicit `WARNING` in backup flow:
    - descriptor sharding not supported for this `m/n`;
    - full descriptor QR will be printed on each descriptor plate.

## Canonicalization
- Before UR/XOR split:
  - canonicalize sortedmulti keys deterministically;
  - normalize missing children for sortedmulti as needed (`/<0;1>/*`), then apply UR export compatibility rules;
  - encode canonical descriptor payload bytes.

## Recovery Rules
- UR/XOR sets:
  - accept valid fragments from same set;
  - reject mixed or invalid sets with explicit UI errors;
  - reconstruct deterministic descriptor payload.
- Full-UR fallback sets:
  - any one full descriptor UR is sufficient to reconstruct descriptor payload.

## Determinism and Reprint
- For same canonical descriptor input and same supported UR/XOR family:
  - share payloads should be deterministic.
- For unsupported families in fallback mode:
  - descriptor UR payload is deterministic for same canonical descriptor.

## Tests
- Table-driven descriptor-share matrix across:
  - scripts: `P2WSH`, `P2SH-P2WSH`
  - networks: `main`, `test`
  - representative families: `2/3`, `2/4`, `3/5`, `n-1/n` (`4/5`), fallback (`7/10`)
- Assertions:
  - UR/XOR families produce multipart UR fragments and reconstruct correctly.
  - Unsupported families produce full single-part descriptor UR (not SE1, not multipart).

