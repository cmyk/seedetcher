# Release checklist (b0.1 WIP)

- [ ] GUI: fully Screen-structured backup flow
  - [ ] Descriptor input as Screen (scan/skip/reuse, validation encapsulated)
  - [ ] Seed input as Screen(s) (camera/manual, descriptor match, dup-fp guard)
  - [ ] Wallet confirm as Screen (descriptor + seed, choose key index)
  - [ ] Print flow as Screen (retry on failure, return to menu on success)
  - [x] Remove SD warning before backup (Button3 hold)
  - [x] Run loop uses Screen state machine starting at MainMenu
- [ ] Device sanity: menu → backup flow (SeedQR + manual) → print on hardware
- [ ] Docs: AGENTS.md, GUI flowchart updated
- [ ] Tests: `GOCACHE=/tmp/gocache ./scripts/test-lite.sh` clean
