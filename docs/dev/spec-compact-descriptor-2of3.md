# Compact Descriptor Shares (2-of-3) Spec (Draft)

## Status
Draft proposal. Not implemented.

## Goal
Reduce descriptor-share payload size for 2-of-3 multisig backups by avoiding storage of all xpubs in every descriptor shard, while still allowing full descriptor reconstruction from any 2 plates.

## Motivation
Current sharded-descriptor payloads embed full descriptor material, which increases QR density and etch complexity. In 2-of-3 backups, two recovered seeds already provide two xpubs deterministically; only the missing xpub must be recoverable from shard data.

## Scope
- Wallet type: `sortedmulti(2, ...3 keys...)` only.
- Plate format: one seed QR + one compact descriptor share QR per plate.
- Recovery target: full coordinator-importable descriptor string.

## Non-goals
- General `m-of-n` compact coding in this spec.
- Replacing existing SE1 sharded-descriptor mode.
- Backward compatibility in the same payload prefix.

## Threat model assumptions
- Plate compromise of fewer than threshold plates should not reveal the full descriptor.
- Recovery environment is offline and trusted.
- Seeds remain required for full wallet recovery.

## High-level construction
Let canonical key records be `X1`, `X2`, `X3` (equal-length byte arrays).

1. Compute parity record:
   - `P = X1 XOR X2 XOR X3`
2. Split `P` with existing Shamir byte-split (`t=2, n=3`):
   - `S1`, `S2`, `S3`
3. Plate `i` stores:
   - seed material for key `i` (existing seed QR)
   - compact share `Si`
   - compact metadata (wallet/script/path/network/order/checksum)

Recover from any two plates `i,j`:
1. Derive `Xi, Xj` from scanned seeds and descriptor metadata (path/network/script).
2. Reconstruct `P` from `Si,Sj`.
3. Recover missing key record `Xk = P XOR Xi XOR Xj`.
4. Reassemble all 3 keys in canonical order and emit full descriptor.

## Canonical key record encoding
Each key record `Xi` MUST be binary-canonical and fixed-width or length-prefixed with strict parsing.

Recommended fields:
- key fingerprint (4 bytes)
- derivation origin/path canonical bytes
- xpub serialized payload (network-aware)
- child derivation template marker (`/<0;1>/*` equivalent canonical token)

Rules:
- Same descriptor MUST always produce byte-identical `Xi` records.
- Key order MUST be canonical (descriptor order after normalization).
- Any mismatch in fingerprint/path/network MUST fail recovery.

## Compact share payload format (new prefix)
Use a new prefix, e.g. `SE2:` (do not reuse `SE1:`).

Required fields:
- `version` (u8)
- `scheme` = `compact-2of3`
- `wallet_id` (deterministic hash, short)
- `set_id` (deterministic for same canonical descriptor)
- `share_index` (1..3)
- `threshold` (=2)
- `total` (=3)
- `script_type` (e.g. `P2WSH`)
- `network` (`MAIN`/`TEST`)
- `path_template` (canonical, shared across keys)
- `key_order_fingerprints` (3 entries)
- `payload` (Shamir share bytes of `P`)
- integrity checksum/MAC (scheme-defined)

## Determinism
For identical canonical descriptor input, compact share outputs SHOULD be deterministic if deterministic set IDs are enabled in controller policy.

## Validation rules (recovery)
Reject with explicit errors for:
- non-`SE2` payload in compact mode
- mixed `wallet_id` / `set_id`
- wrong threshold/total/scheme
- duplicate share index
- checksum/integrity failure
- derived seed key record mismatch to expected fingerprint/path/network
- reconstructed `Xk` failing key-record parse or checksum

## Security notes
- One plate contains one seed + one compact share: insufficient for full descriptor by design.
- Two plates reveal two seeds and allow descriptor reconstruction (aligned with 2-of-3 policy).
- This scheme is custom and must be treated as protocol-critical code.

## Interoperability
- External wallets will not parse `SE2` directly.
- SeedEtcher recovery flow (and planned b0.4 host binaries) reconstruct full standard descriptor output for external import.

## Testing requirements
- Deterministic vectors for at least 3 fixed 2-of-3 wallets.
- Property tests for all 3 choose 2 recovery combinations.
- Negative tests: mixed sets, swapped metadata, corrupted share, corrupted seed, wrong network/path.
- Cross-tool recovery parity (controller vs host recovery binary).

## Rollout plan
1. Implement parser/encoder under new package path (no SE1 changes).
2. Add explicit mode toggle in backup flow for 2-of-3 wallets only.
3. Keep SE1 as default until physical QA confirms scan/etch gains.
4. Promote compact mode after successful field validation.

## Open questions
- Final binary encoding for `Xi` (reuse urtypes serialization vs dedicated schema).
- Integrity primitive (CRC32C vs keyed MAC in this context).
- Whether `set_id` should be deterministic or user-overridable in compact mode.
