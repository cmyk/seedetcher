# SeedEtcher – Self-Contained Multisig Steel Bitcoin Wallet Backups

SeedEtcher is an open-source, air-gapped system for creating durable Bitcoin backups by printing seed phrases, descriptors, and QR codes with a standard laser printer and permanently etching them into metal.
It minimizes trust and attack surface by relying on offline hardware, simple materials, and a transparent, reproducible workflow instead of expensive dedicated machines.

Starting with version b0.3 the follwing things were substantially improved:

- All brother lasers are supported (even host-based). PCL/PS remains the recommended way to print. HBP (host based printing) is capped to 600dpi (memory limit of pi zero)
All other brands that support true PCL or PostScript should work too. See: [printers.md](docs/printers.md)
- A new method leverages the use of silicone sheets to reliably transfer toner masks to both sides of a metal plate at once.
This means, you can also etch both sides at once!
- A new 


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
- Multisig uses descriptor-share backups (no full descriptor on a single plate). Starting with b0.3 these wallet configs default to UR/XOR-compatible shares: `1/2`, `2/2`, `2/3`, `2/4`, `4/4`, `3/5`, and any `n-1/n`. 
other multisig configurations output the full descriptor.
- The SeedEtcher controller has a descriptor recovery mode. TODO: cross-platform binaries will make this inheritance-friendly.
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
sudo dd if=result/seedetcher.img of=/dev/rdiskX bs=1m
diskutil eject /dev/diskX
```
 ---

## Build & Deploy (Quick Start)

(see [build-matrix.md](docs/dev/build-matrix.md) for target builds)

`nix build .#image-gadget-debug`

Flash to SD card:
``` bash
diskutil unmountDisk /dev/diskX
sudo dd if=result/seedetcher-gadget-debug.img of=/dev/rdiskX bs=1m
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
