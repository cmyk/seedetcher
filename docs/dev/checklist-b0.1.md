# Release checklist (b0.1 WIP)

- [X] GUI: fully Screen-structured backup flow
  - [x] Descriptor input as Screen (scan/skip/reuse, validation encapsulated)
  - [x] Seed input as Screen (camera/manual, descriptor match, dup-fp guard)
  - [x] Wallet confirm as Screen (descriptor + seed, choose key index)
  - [x] Print flow as Screen (retry on failure, return to menu on success)
  - [x] Remove SD warning before backup (Button3 hold)
  - [x] Run loop uses Screen state machine starting at MainMenu
- [x] Device sanity: menu → backup flow (SeedQR + manual) → print on hardware
- [x] Docs: AGENTS.md, GUI flowchart updated
- [x] Tests: `GOCACHE=/tmp/gocache ./scripts/test-lite.sh` clean
- [ ] GUI dedupe pass (refactor/gui-dedupe)
  - [x] Extract restart-confirm helper (reuse across descriptor/seed/confirm)
  - [x] Extract seed validation helper (dup fp / descriptor mismatch) with typed errors
  - [x] Tidy print job plumbing (desc/mnemonic/keyIdx holder)
