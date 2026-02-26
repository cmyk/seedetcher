# Supported Printers

**Important:** For SeedEtcher a laser printer MUST have USB. All printers in this list do.
PCL capability is preferable, since you won't need to load the extra HBP support (which is heavy on a zero).
A printer without Wi-Fi is preferable (air-gapped security). Wi-Fi can be shut off, however.

Brother's suffix meanings:
D = Duplex
W = Wireless
N = Network
C = Color

Therefore: 
- DN = duplex, network
- DW = duplex, wireless


This table is primarily generated from `spike/brlaser-root.tar.gz` (`brlaser.drv`) and kept as one list ordered by model name.
Capabilities are initially derived from brlaser data (including 1284DeviceID CMD tokens) and then manually corrected where verified by vendor specs or real-world testing.
Some rows are manual additions for known models not present in the current `brlaser.drv` export.
`PCL`/`PS`/`HBP` may be manually corrected for models with known support not reflected in `CMD:`.

| Brand | Model Name | USB | PCL | PS | HBP | Tested |
|---|---|---|---|---|---|---|
| Brother | DCP-1510 series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-1600 series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-1610W series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-7010 | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-7020 | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-7030 | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-7040 | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-7055 | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-7055W | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-7060D | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-7065DN | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-7070DW | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-7080 | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-7080D | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-8065DN | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-B7500D series | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-L2500D series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-L2510D series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-L2520D series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-L2520DW series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-L2537DW | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-L2540DW series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-L2550DW series | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | DCP-L2560DW series | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| FX | DocuPrint P265 dw | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | FAX-2820 | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | FAX-2840 | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-1110 series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-1200 series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-2030 series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-2130 series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-2140 series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-2220 series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-2230 series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-2240 series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-2240D series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-2250DN series | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | HL-2260 | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-2260D | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-2270DW series | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | HL-2280DW | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | HL-5030 series | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | HL-5040 series | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | HL-5140 series | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | HL-5370DW series | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | HL-5450DN series | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | HL-L2300D series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-L2305 series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-L2310D series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-L2320D series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-L2335D series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-L2340D series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-L2350DW series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-L2360D series | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | HL-L2370DN series | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | HL-L2375DW series | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | HL-L2380DW series | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | HL-L2390DW | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-L2400D (alias of HL-L2400DW) | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-L2400DW | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-L2402D | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-L2405W | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | HL-L5000D series | 🟢 | 🟢 | 🟢 | 🟢 | 🟢 |
| Lenovo | LJ2650DN | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-1810 series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-1910W series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-7240 | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-7320 | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-7340 | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-7360N | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-7365DN | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-7420 | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-7440N | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-7460DN | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-7860DW | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-8440 | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-8710DW | 🟢 | 🟢 | 🟢 | 🟢 | 🔴 |
| Brother | MFC-8860DN | 🟢 | 🟢 | 🟢 | 🟢 | 🔴 |
| Brother | MFC-9140CDN (manual entry) | 🟢 | 🟢 | 🟢 | 🟢 | 🔴 |
| Brother | MFC-9160 | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-L2690DW | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-L2700DN series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-L2700DW series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-L2710DN series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-L2710DW series | 🟢 | 🔴 | 🔴 | 🟢 | 🔴 |
| Brother | MFC-L2750DW series | 🟢 | 🟢 | 🔴 | 🟢 | 🔴 |

