# GUI Flow (current)

```mermaid
flowchart TD
    A[MainMenuScreen<br>singleTheme]
    A --> G[ConfirmWarning Remove SD card<br>Hold Button3 to continue<br>Button1 to cancel]

    G -->|Continue| C{Choose route}
    G -->|Cancel| A
    C -->|Backup Wallet| B[BackupFlowScreen<br>descriptorTheme]
    C -->|Recover Descriptor| R0[RecoverDescriptorIntro]

    D[Descriptor input<br>Scan or Skip or Reuse<br>Validate descriptor and duplicates] --> M{Descriptor present}
    B --> D
    M -->|No singlesig| F1[Seed input confirm<br>1 of 1]
    M -->|Yes descriptor scanned| F2
    F2[Seeds loop<br>Scan or manual per key<br>Confirm and no duplicates] --> G1[Confirm wallet<br>choose key index]
    G1 --> FP[Fingerprints review<br>All cosigner fingerprints<br>7 per page]
    FP --> S2{Descriptor shares exist?}
    S2 -->|Yes| S3[Descriptor shares review<br>t/n summary]
    S2 -->|No| H2
    S3 --> H2[Add wallet label]
    H2 --> MODE{Wallet mode selector}
    MODE -->|Singlesig descriptor| SM[Singlesig layout<br>Seed Only / Seed + Info / Seed + Descr QR]
    MODE -->|2 of 3 multisig| CM[Compact 2/3<br>Off or On]
    MODE -->|Other wallets| PS
    SM --> PS[Select paper size]
    CM --> PS
    PS --> PO[Print settings<br>DPI, Invert, Mirror, Etch stats page]
    PO --> P2[Print flow with descriptor shares per plate]
    P2 --> A

    F1 --> FP1[Fingerprints review<br>single seed fingerprint]
    FP1 --> H3[Add wallet label]
    H3 --> SM1[Singlesig layout<br>Seed Only / Seed + Info / Seed + Descr QR]
    SM1 --> PS1[Select paper size]
    PS1 --> PO1[Print settings<br>DPI, Invert, Mirror, Etch stats page]
    PO1 --> P3[Print flow singlesig]
    P3 --> A

    R0 --> R1[Scan input]
    R1 --> R2[Collect descriptor shares<br>Progress k of t]
    R2 --> R3[Validate format/set/duplicates]
    R3 -->|k gte t| R5[Reconstruct descriptor in RAM]
    R5 --> R6
```

Manual entry UX: if the seed is invalid or mismatched, the confirm screen leaves you on the same seed input with your entered words prefilled for correction (no restart).

Notes:
- `Run` enters the Screen state machine at `MainMenuScreen`.
- Colors: `singleTheme` on menu; `descriptorTheme` for backup flow and warnings.
- All helper logic lives alongside screens (`gui/screen_*.go` and `gui/screen_helpers.go`).
- Multisig backup uses sharded descriptor mode only since since v0.2.0-beta.1.
- Singlesig backup stays non-sharded.
- Backup review sequence for multisig is:
  - `Confirm wallet` -> `Fingerprints` -> optional `Descriptor shares` -> `Wallet label` -> wallet-mode selector (`Compact 2/3` when eligible, `Singlesig layout` for singlesig descriptor) -> `Paper size` -> print settings -> `Print`.
- Backup review sequence for singlesig (descriptor skipped) is:
  - `Seed input` -> `Fingerprints` -> `Wallet label` -> `Singlesig layout` -> `Paper size` -> print settings -> `Print`.
  - Back from `Fingerprints` opens `Restart Process?`; decline returns to `Fingerprints`.
- `Fingerprints` uses page navigation (left/right arrows) and keeps back/check nav buttons (`7` entries/page).
- In backup descriptor scan, UR/XOR 2-of-3 fragments show `x/2` capture progress (not `%`).
- Print setup order is wallet-mode selector (if applicable) -> `Paper size` -> `DPI` -> `Invert` -> `Mirror` -> `Etch stats page`.
- When `Etch stats page` is enabled, one additional stats page is appended after plate pages:
  - area/coverage table per printed plate side (`mm²` and `%`),
  - per-plate PSU current guide (`Set A masked` / `Set A unmasked`) using bench defaults.
- Recovery mode is descriptor-share recovery; plain descriptor QR input is rejected with an explicit message.
- Recovery QR screen copy:
  - Title: `Recovered Descriptor QR`
  - Body: `Scan with your coordinator, then choose:`
  - `Back = show QR again`
  - `Trash = delete and restart`

Implementation note:
- Active flow uses explicit `Screen` structs (`MainMenuScreen` -> `BackupFlowScreen` stages).
- Keep testing on device via `nix run .#reload $USBDEV1` after each step.
