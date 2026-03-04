# Third-Party Components and Licenses

This project is licensed under Apache-2.0 for SeedEtcher-authored code.

Release images also include third-party components that keep their own licenses.
Those licenses apply to those components.

## Included Components (Release Images)

| Component | License | Notes | Upstream |
|---|---|---|---|
| Linux kernel (raspberrypi/linux) | GPL-2.0-only | Kernel image and modules | https://github.com/raspberrypi/linux |
| Raspberry Pi firmware boot files | Raspberry Pi firmware redistribution terms | `bootcode.bin`, `start.elf`, `fixup.dat` | https://github.com/raspberrypi/firmware |
| BusyBox (static) | GPL-2.0-only | Base userland in initramfs (`/bin/*`) | https://busybox.net |
| binutils (`readelf`) | GPL-3.0-or-later | `readelf` is copied into initramfs | https://www.gnu.org/software/binutils/ |
| libcamera + camera runtime libs | See upstream libcamera licensing | Camera stack shipped in runtime libs | https://libcamera.org |
| CUPS | Apache-2.0 (with CUPS exception) | Printing system/scheduler (`cupsd`, CLI tools) | https://github.com/OpenPrinting/cups |
| cups-filters | GPL-2.0-or-later | CUPS filter stack used by print pipeline | https://github.com/OpenPrinting/cups-filters |
| Ghostscript | AGPL-3.0-or-later (or commercial) | PDF/PS/raster conversion toolchain | https://ghostscript.com/licensing/ |
| poppler-utils (`pdftops`) | GPL-2.0-or-later | PDF conversion utility used in print flow | https://poppler.freedesktop.org |
| brlaser | GPL-2.0-only | Brother laser CUPS raster filter/PPD data | https://github.com/pdewacht/brlaser |
| Martian Mono font | OFL-1.1 | Bundled TTF used by UI/print layouts | https://github.com/evilmartians/mono |
| Poppins font | OFL-1.1 | Used for generated bitmap font assets | https://github.com/itfoundry/Poppins |
| Comfortaa font | OFL-1.1 | Used for generated bitmap font assets | https://github.com/alexeiva/comfortaa |

SeedEtcher builds package versions from pinned Nix inputs (`flake.lock`), so exact versions are reproducible per release build.

## Included Components (Debug Images)

| Component | License | Notes |
|---|---|---|
| strace | LGPL-2.1-or-later and GPL-2.0-or-later | Copied into initramfs for debug/troubleshooting |

## What This Means for Releases

For each public release image:

1. Keep this file in the repository and include/update it in release documentation.
2. Keep third-party license texts available to recipients for distributed components.
3. Provide corresponding source information for GPL/AGPL components in the release context:
   - upstream source references,
   - any local patches/modifications,
   - build scripts/derivations used to produce shipped binaries.
4. Keep `flake.lock` in the tagged release so dependency revisions stay auditable/reproducible.

## Maintainer Check (Per Release)

1. Confirm release tag and commit include current `flake.lock`.
2. Confirm third-party component list above still matches what image builds include.
3. If component set changed, update this file before publishing.
4. In release notes, link to:
   - source tag/commit,
   - this third-party license file,
   - any patch locations for bundled GPL/AGPL components.

## Optional Version Audit Commands

```bash
# Build image (if needed)
nix build .#image

# Inspect closure for relevant components (versions appear in store path names)
nix path-info -r .#image | rg 'cups|cups-filters|ghostscript|brlaser-se-runtime'
```

## Notes

- This file is a practical compliance tracker, not legal advice.
- If distribution model changes (for example hosted network services using AGPL components),
  re-check obligations before release.
