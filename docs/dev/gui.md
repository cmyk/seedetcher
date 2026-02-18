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
    M -->|Yes multisig| F2
    F2[Seeds loop<br>Scan or manual per key<br>Confirm and no duplicates] --> G1[Confirm wallet<br>choose key index]
    G1 --> FP[Fingerprints review<br>All cosigner fingerprints<br>5 per page]
    FP --> S2[Descriptor shares review<br>t/n + wallet_id/set_id]
    S2 --> H2[Add wallet label]
    H2 --> PS[Select paper size]
    PS --> PO[Select print options<br>DPI, Invert, Mirror]
    PO --> P2[Print flow with descriptor shares per plate]
    P2 --> A

    F1 --> H3[Add wallet label]
    H3 --> PO1[Select print options<br>DPI, Invert, Mirror]
    PO1 --> P3[Print flow singlesig]
    P3 --> A

    R0 --> R1[Scan input]
    R1 --> RX{Input type}
    RX -->|Plain descriptor QR| R4[Validate descriptor]
    R4 --> R6[Export/confirm screen]
    R6 --> A

    RX -->|Sharded share QR| R2[Collect shares<br>Progress k of t]
    R2 --> R3[Validate set version network<br>Checksum and duplicate checks]
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
  - `Confirm wallet` -> `Fingerprints` -> `Descriptor shares` -> `Wallet label` -> `Paper size` -> `Print options` -> `Print`.
- `Fingerprints` uses page navigation (left/right arrows) and keeps back/check nav buttons.
- Print options screen exposes `DPI`, `Invert`, and `Mirror` prior to print submission.
- Recovery mode accepts both sharded shares and plain descriptor QR input.
- Plain descriptor QR input bypasses shard threshold accumulation and goes directly to export/confirm.
- Recovery QR screen copy:
  - Title: `Recovered Descriptor QR`
  - Body: `Scan with your coordinator, then choose:`
  - `Back = show QR again`
  - `Trash = delete and restart`

Implementation note:
- Active flow already uses explicit `Screen` structs (`MainMenuScreen` -> `BackupFlowScreen` stages).
- `backupWalletFlow` in `gui/screen_backup.go` is legacy/helper code and should not be expanded for new work.
- Keep testing on device via `nix run .#reload $USBDEV1` after each step.
