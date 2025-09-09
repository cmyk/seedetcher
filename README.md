# SeedEtcher

SeedEtcher is a custom Raspberry Pi Zero firmware forked from [SeedHammer](https://github.com/seedhammer/seedhammer).  
It scans QR codes (seed phrases or wallet descriptors) or accepts manual input, validates them, and outputs **PDF seed plates** for laser printing → toner transfer → acid etching.

It is **air-gapped**, runs on a Pi Zero with camera & buttons, and supports **single-sig and multisig descriptors**.  
Seed generation is *not* included — only processing of existing seeds.

---

## Features
- Scan SeedQR / CompactSeedQR with Pi camera (libcamera + zbar).
- Manual mnemonic input with validation (`bip39`).
- GUI-driven, with physical button navigation.
- Outputs PDF plates (A4/Letter) with words + QR codes.
- Laser print → toner transfer → acid etching for steel backup.
- Debugging via serial shell and PDF capture on host.

---

## Documentation
- docs/overview.md — purpose, architecture, file map  
- docs/development.md — build, debug, printing, Nix config  

---

## Build & Deploy (Quick Start)

On Ubuntu VM:
nix build .#image-debug --impure
scp ubuntu:~/seedetcher/result/seedetcher-debug.img ~/Downloads/

Flash to SD card:
diskutil unmountDisk /dev/diskX
sudo dd if=result/seedetcher-debug.img of=/dev/rdiskX bs=1m
diskutil eject /dev/diskX

Run controller on Pi:
./controller < /dev/ttyGS1 >> /log/debug.log 2>> /log/debug.log &

---

## License
MIT (see LICENSE file)

---

## Legacy
The original SeedHammer README is kept as README_seedhammer.md
