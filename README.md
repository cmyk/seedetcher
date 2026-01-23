# SeedEtcher – Self-Contained Multisig Steel Bitcoin Wallet Backups

SeedEtcher is an open-source, air-gapped system for creating durable Bitcoin backups by printing seed phrases, descriptors, and QR codes with a standard laser printer and permanently etching them into metal.
It minimizes trust and attack surface by relying on offline hardware, simple materials, and a transparent, reproducible workflow instead of expensive dedicated machines.

The project consists of:

## 1) SeedEtcher Controller

Raspberry Pi Zero–based controller firmware that drives a standard laser printer over USB.
Scan seed and descriptor QR codes offline and print deterministic layouts for etching.

## 2) SeedEtcher Workflow

A documented, repeatable workflow for chemically etching printed layouts onto steel.

- Print to transfer paper
- Heat-transfer toner to metal
- Etch steel

[SeedEtcher-Workflow.md](docs/SeedEtcher-Workflow.md)

---

## Features
- Scan SeedQR / CompactSeedQR with Pi camera (libcamera + zbar).
- Manual mnemonic input with validation (`bip39`).
- GUI-driven, with physical button navigation.
- Outputs plates layouts with words + QR codes directly via serial USB
- Laser print → toner transfer → acid etching for steel backup.
- Debugging via serial shell and PDF capture on host.

---

## Documentation
- docs/overview.md — purpose, architecture, file map  
- docs/development.md — build, debug, printing, Nix config  

---

## Flash SeedEtcher to SD-card

Download the img file from the [release page](https://github.com/cmyk/seedetcher/releases) on github

Use [balena etcher](https://etcher.balena.io/) or via cmd line:

MacOS:

```bash 
diskutil unmountDisk /dev/diskX
sudo dd if=result/seedetcher-host.img of=/dev/rdiskX bs=1m
diskutil eject /dev/diskX
```
 ---

## Build & Deploy (Quick Start)

(see [build-matrix.md](docs/dev/build-matrix.md) for target builds)

`nix build .#image-debug`

Flash to SD card:
``` bash
diskutil unmountDisk /dev/diskX
sudo dd if=result/seedetcher-debug.img of=/dev/rdiskX bs=1m
diskutil eject /dev/diskX
```

Run controller on Pi:
./controller < /dev/ttyGS1 >> /log/debug.log 2>> /log/debug.log &

---

## License
Unlicense license: [LICENSE](LICENSE)

---

## Legacy
This is a fork of [seedhammer](https://github.com/seedhammer/seedhammer)
