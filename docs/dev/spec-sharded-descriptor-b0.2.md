# b0.2 Spec: Sharded Descriptor (Core)

Status: Draft v1 (implementation target for b0.2)

Scope:
- Defines the core data contract for descriptor sharding/recovery.
- Excludes UI layout details (tracked separately in checklist).

Goals:
- No single share reveals the full descriptor.
- Any `t` valid shares from the same set reconstruct the full descriptor.
- Recovery is offline and in-RAM only.

## 1. Security policy

- Mode: strict (Option A).
- In sharded mode, no xpub-bearing full descriptor is printed on plates.
- Non-secret hints may be printed: wallet label, network, script type, `wallet_id`, share index, threshold.
- Secret material:
  - canonical descriptor payload bytes
  - reconstructed descriptor string

## 2. Canonical descriptor payload

Input: descriptor string from existing parser flow.

Canonicalization rules (`canonical_descriptor_v1`):
- Trim leading/trailing whitespace.
- Collapse internal whitespace to parser-normalized form.
- Normalize script text formatting via parser output.
- Include descriptor checksum in canonical string.

Output payload:
- UTF-8 bytes of canonical descriptor string.

Invariant:
- Same logical descriptor always yields byte-identical canonical payload.

## 3. Share set model

For one wallet backup operation:
- `set_id`: random 16-byte identifier (new per backup set).
- `wallet_id`: 4-byte identifier derived from canonical payload:
  - `wallet_id = first4(SHA256(canonical_payload))`
- Parameters:
  - `n`: total shares
  - `t`: threshold
  - `1 <= i <= n`: share index

Constraints:
- `2 <= t <= n`
- b0.2 presets should prefer 2-of-3 and 3-of-5.

## 4. Shard container (logical fields)

Each share carries:
- `version` (uint8): `1`
- `set_id` (16 bytes)
- `wallet_id` (4 bytes)
- `network` hint (enum): `mainnet | testnet | signet | regtest`
- `script` hint (enum): e.g. `wpkh | wsh | tr | sortedmulti` (non-secret hint)
- `t` (uint8)
- `n` (uint8)
- `i` (uint8, 1-based)
- `share_bytes` (binary Shamir share payload)
- `checksum` (CRC32 or equivalent transport checksum over container fields)

Optional integrity hardening (recommended if implemented in b0.2):
- `auth_tag` (HMAC/BLAKE2 keyed tag) with key derived from `set_id`.

Mixing rules:
- Shares must match on `version`, `set_id`, `wallet_id`, `network`, `script`, `t`.
- Reject duplicates of same `i`.

## 5. Shamir split/reconstruct

Algorithm:
- GF(256) Shamir over arbitrary bytes (SSKR-style byte shares).
- Split canonical descriptor payload into `n` shares, threshold `t`.
- Reconstruct only when at least `t` valid, matching shares are present.

Failure behavior:
- `< t` shares: fail with explicit threshold error.
- Mismatch across required metadata: fail with explicit mismatch error.
- Checksum/auth failure: reject share as invalid/corrupt.

## 6. QR encoding (transport)

b0.2 transport requirement:
- Each share should fit in a single QR for typical multisig descriptors.

Container encoding:
- Binary container encoded as base32 (uppercase, no ambiguous chars) OR UR single-part.
- Prefix should include versioned type marker, e.g. `SE1:` (final prefix TBD).

If single-QR fit fails:
- Fall back to multipart UR transport (tracked as secondary path).

## 7. Recovery output

On successful reconstruction:
- Reconstruct canonical descriptor string in RAM.
- Output descriptor QR payload in standard descriptor form accepted by Sparrow.
- Optional text reveal is gated by explicit user action.
- On exit/done, wipe in-memory buffers best-effort.

## 8. Logging and persistence rules

Must not log:
- canonical descriptor payload
- share bytes
- reconstructed descriptor text

May log:
- counts/progress
- non-secret metadata (`set_id` shortened, `wallet_id`, share index)

Must not persist secret material to disk.

## 9. Acceptance criteria (core)

- Deterministic canonicalization test passes.
- Split/reconstruct round-trip passes across random descriptors.
- Reconstruction with `<t` shares fails.
- Mixed-set and mixed-wallet shares are rejected.
- Corrupted share checksum is rejected.
- Recovered descriptor imports in Sparrow and derives expected addresses.
