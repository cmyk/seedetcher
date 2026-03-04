# SeedEtcher – Self-Contained Multisig Steel Bitcoin Wallet Backups

SeedEtcher is an open-source, air-gapped system for creating durable Bitcoin backups by printing seed phrases, descriptors, and QR codes with a standard laser printer and permanently etching them into metal.
It minimizes trust and attack surface by relying on offline hardware, simple materials, and a transparent, reproducible workflow instead of expensive dedicated machines.
Once you get the hang of the workflow a double-sided plate can be done in 1.5–2h.

Starting with ***version b0.3*** prep-time and etch time halved!

The following things were substantially improved or added:

- Multisig uses descriptor-share backups (no full descriptor on a single plate). These wallet configs default to UR/XOR-compatible shares: `1/2`, `2/2`, `2/3`, `2/4`, `4/4`, `3/5`, and any `n-1/n`. This replaces b0.2's custom SE1 Shamir share method for interoperability. All other wallet types stay on full descriptor.
- All Brother lasers are supported (even host-based). PCL/PS remains the recommended way to print. HBP (host based printing) is capped to 600dpi (memory limit of pi zero)
All other brands that support true PCL or PostScript should work too. See: [printers.md](docs/printers.md)
- Print output can be sent non-inverted and non-mirrored for checking before printing to transfer paper.
- A new method (SeedEtcher Transfer Stack<sup>TM</sup>) leverages the use of silicone sheets to reliably transfer toner masks to both sides of a metal plate at once.
This means, you can also etch both sides at once!
- A new plate layout design optimizes for etching. All rounded forms, including a custom designed font face and QRs with circle modules. Also the mask area now covers the whole plate except for the side where you tape it for transfer. This means you only need to tape one side before etching.
- I designed a 3d printable etching container for optimal etching performance. No manual movement required. It will be released after geyser.io campaign, presumably.
- Improved etching method by using 30% FeCl3 at 40°C. It can be made from 40% by diluting it with distilled water.
- For folks who want to electro etch, there is an optional stats page with A/cm2 calculations that can be printed additionally. (I am still researching the optimal electro etching workflow.)

---

## The project consists of:

### 1) SeedEtcher Controller

Raspberry Pi Zero–based controller firmware that drives a standard laser printer over USB.
Scan seed and descriptor QR codes offline and print deterministic layouts for etching.

### 2) SeedEtcher Workflow

A documented, repeatable workflow for chemically etching printed layouts onto steel.

- Print to transfer paper
- Heat-transfer toner to metal
- Etch steel

[SeedEtcher-Workflow.md](docs/SeedEtcher-Workflow.md)

---

## Features
- Multisig uses descriptor-share backups (no full descriptor on a single plate). These wallet configs default to UR/XOR-compatible shares: `1/2`, `2/2`, `2/3`, `2/4`, `4/4`, `3/5`, and any `n-1/n`. Other multisig configurations output the full descriptor.
- The SeedEtcher controller has a descriptor recovery mode and is able to scan them directly from the metal plates.
- Manual mnemonic input with validation (`bip39`)
- GUI-driven, with physical button navigation
- Outputs plates layouts with words + QR codes directly via serial USB
- Laser print → toner transfer → acid etching for steel backup
- Debugging via serial shell and PDF capture on host or UART in host mode

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

## Build & Deploy for debugging (Quick Start)

(see [build-matrix.md](docs/dev/build-matrix.md) for target builds)

`nix build .#image-gadget-debug`

Flash to SD card:
``` bash
diskutil unmountDisk /dev/diskX
sudo dd if=result/seedetcher-gadget-debug.img of=/dev/rdiskX bs=1m
diskutil eject /dev/diskX
```

Run controller on Pi:
```./controller < /dev/ttyGS1 >> /log/debug.log 2>> /log/debug.log &```

---

## License
Licensed under Apache License 2.0: [LICENSE](LICENSE) and [NOTICE](NOTICE)
Third-party component licenses: [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md)

---

## Legacy
This is a fork of [seedhammer](https://github.com/seedhammer/seedhammer)
