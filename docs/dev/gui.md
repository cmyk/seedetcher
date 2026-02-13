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
    M -->|Yes multisig| S1[Shard params<br>Derive t and n from descriptor]
    S1 --> S2[Shard review<br>wallet_id/set_id]
    S2 --> F2
    F2[Seeds loop<br>Scan or manual per key<br>Confirm and no duplicates] --> G1[Confirm wallet<br>choose key index]
    G1 --> H2[Add wallet label]
    H2 --> P2[Print flow with shard QR per plate]
    P2 --> A

    F1 --> H3[Add wallet label]
    H3 --> P3[Print flow singlesig]
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
- Multisig backup uses sharded descriptor mode only in b0.2.
- Singlesig backup stays non-sharded.
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
