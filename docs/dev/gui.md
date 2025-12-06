# GUI Flow (current)

```
MainMenuScreen (singleTheme)
  Button3 → BackupFlowScreen

BackupFlowScreen (descriptorTheme by default)
  If SD present: ConfirmWarning "Remove SD card" (hold Button3 to continue, Button1 to cancel → MainMenu)
  → backupWalletFlow (legacy inline)
    - Descriptor input (scan/skip/reuse)
    - For each key: Seed input (scan or manual), confirm seed, duplicate check against descriptor
    - Confirm wallet (descriptor + seed), choose key index to print
    - PrintSeedScreen (A4)
  → returns to MainMenuScreen
```

Notes:
- `Run` enters the Screen state machine at `MainMenuScreen`.
- Colors: `singleTheme` on menu; `descriptorTheme` for backup flow and warnings.
- Remove-SD warning uses `ConfirmWarningScreen` before entering the flow.
- All helper logic lives alongside screens (`gui/screen_*.go` and `gui/screen_helpers.go`).

Planned refactor steps:
- Replace `backupWalletFlow` with explicit `Screen` structs: Descriptor input → Seed entry/confirm → Wallet confirm → Print.
- Keep testing on device via `nix run .#reload $USBDEV1` after each step.
