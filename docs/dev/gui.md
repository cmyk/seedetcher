# GUI Flow (current)

```mermaid
flowchart TD
    A[MainMenuScreen<br>singleTheme] -->|Button3| B[BackupFlowScreen<br>descriptorTheme]
    B -->|If SD present| C[ConfirmWarning: Remove SD<br>Hold Button3 to continue<br>Button1 to cancel]
    C -->|Yes| D
    C -->|No| A
    D[backupWalletFlow<br>legacy inline] --> E[Descriptor input<br>Scan/Skip/Re-use<br>validate descriptor+dups]
    E --> F[Seeds loop<br>Scan or manual per key<br>confirm seed<br>check vs descriptor<br>no dup fingerprints]
    F --> G[Confirm wallet<br>Descriptor+seed<br>choose key index]
    G --> H[Add wallet label]
    H --> I[PrintSeedScreen<br>Paper A4]
    I --> A
```

Manual entry UX: if the seed is invalid or mismatched, the confirm screen leaves you on the same seed input with your entered words prefilled for correction (no restart).

Notes:
- `Run` enters the Screen state machine at `MainMenuScreen`.
- Colors: `singleTheme` on menu; `descriptorTheme` for backup flow and warnings.
- All helper logic lives alongside screens (`gui/screen_*.go` and `gui/screen_helpers.go`).

Planned refactor steps:
- Replace `backupWalletFlow` with explicit `Screen` structs: Descriptor input → Seed entry/confirm → Wallet confirm → Print.
- Keep testing on device via `nix run .#reload $USBDEV1` after each step.
