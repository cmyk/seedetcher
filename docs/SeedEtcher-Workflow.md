# SeedEtcher Workflow

A word of warning:\
This process is involved. It’s not a machine that you flip on and off and be done with it.\
But the idea was to create a process that doesn’t require a $500 machine. Most of the items you need you might already have. Also, you won’t create multisig backups very often.
So, investing a bit more time instead of money might be for you.
This workflow has many unknown variables (like the laser printer or iron you use). I tried to rule out as many variables as possible. Still, your mileage may vary. Do not expect this to work on your first try.

Here's a video of the process: https://youtu.be/O1ZcKIli9hk?si=wur4efhf88QD2LMY \
The video DOES NOT REPLACE this guide. Please do read this guide for best results.

What you need:

- [ ] Raspi Pi Zero with screen and cam (same hardware as SeedSigner)
- [ ] micro SD-Card
- [ ] SeedEtcher Firmware
- [ ] Laser Printer (air gapped), I only tested Brother HL series, avoid eco toners, needs to understand PCL
- [ ] Micro-USB male to USB-A female ([amazon](https://a.co/d/drLFF49))
- [ ] Steel Plates, 10x10cm (make sure they are really flat). You can get them on ebay or amazon or cut your own.
- [ ] Iron (for ironing clothes)
- [ ] 0.5-2mm thick silicone sheet ([amazon](https://a.co/d/2F59LSZ)) for better heat transfer from iron plate
- [ ] Transfer Paper ([amazon](https://a.co/d/dmR4RUL))
- [ ] Anti-etching pens ([amazon](https://a.co/d/5DnOhRR)), stop out ground e.g. from [Lascaux](https://lascaux.ch/en/products/brushes-printmaking-sets-various/lascaux-etching?shp3_product=1704) or [Charbonnel](https://intaglioprintmaker.com/product/charbonnel-lamour-black-covering-varnish/) or nail polish
- [ ] Packaging or electrical tape
- [ ] Acetone
- [ ] Isopropyl Alcohol
- [ ] Ferric Chloride Etching 40% solution ([amazon](https://a.co/d/h497Xaa))\
Note: FeCl is for etching brass, copper and steel. It does not work for titanium! Etching titanium with hydrofluoric acid is not recommended unless you have a lab and know what you are doing.
- [ ] Nitrile Gloves, eye protection
- [ ] Baking Soda (sodium bicarbonate, NaHCO₃), NOT baking powder (contains acids + starch)

## Flash SeedEtcher to SD-card

Use [balena etcher](https://etcher.balena.io) or via cmd line:

MacOS:

```bash 
diskutil unmountDisk /dev/diskX
sudo dd if=result/seedetcher.img of=/dev/rdiskX bs=1m
diskutil eject /dev/diskX
```

## Load Descriptor and Seedphrases

There is multiple ways of doing this and it is beyond the scope of this guide.\
SeedEtcher just needs a QR of the descriptor. The seedphrase(s) can be input via QR or manually.
You generally use a coordinator like [sparrow](https://www.sparrowwallet.com) to create the descriptor.
Note: Sparrow just needs the xpubs of the seedphrases for this and not the actual seedphrase(s).
Example: Create the 3 seedphrases for a 2/3 multisig on [SeedSigner](https://seedsigner.com). Use sparrow to create the descriptor.
Scan the descriptor from sparrow with SeedEtcher and then each seedphrase QR.

## Descriptor Sharding (b0.2)

For multisig backups, SeedEtcher now prints descriptor shares (`SE1:`) instead of a full descriptor on each plate.

- No single plate reveals the full descriptor.
- You must scan at least `t` descriptor shares to reconstruct and export the descriptor QR.
- For this flow, descriptor-share threshold matches the wallet signing threshold: an m-of-n wallet uses t=m descriptor shares for recovery.
- Singlesig backup flow is unchanged (no descriptor sharding required).
- Recovery QR is sensitive: once reconstructed, treat it like wallet metadata and keep cameras/devices away.

### Backup flow (on device)

For multisig backups, the on-device review/setup flow is:

1. Confirm wallet
2. Fingerprints review (all cosigner fingerprints, paged)
3. Descriptor shares summary (`t/n`, `WID`, `SET`)
4. Wallet label
5. Paper size
6. Print

### Recovery (cold-room flow)

1. Open `Recover Descr.` on SeedEtcher.
2. Scan descriptor share QRs until threshold is reached.
3. Choose `Single QR` or `Multipart UR`.
4. Scan the exported descriptor QR with Sparrow.
5. Verify first receive/change addresses before trusting the backup.

### Troubleshooting

- Mixed share sets:
  - Error: `share set mismatch: different wallet or shard set`
  - Cause: shares from different backups mixed together.
- Checksum/invalid share:
  - Error includes `invalid share QR` or `combine shares failed`.
  - Cause: damaged/partial scan or wrong share.
- QR too dense:
  - Use `Multipart UR` on the 240x240 recovery screen.
  - Current practical etched-plate target is `n <= 10`.
- Sparrow red derivation/network fields:
  - Ensure Sparrow is in Testnet mode for testnet descriptors.
  - Keep xpub/tpub serialization and coin-type path consistent.

## Printing the Layouts

Connect the dataport of the Pi Zero (it’s the one closer to the center) to the printer’s USB port. Connect a power source to the other port. SeedEtcher sends a bitmap via this USB serial connection using PCL that most laser printers understand.
Tip: Print the layout to paper first to check.
Use the manual feed to print onto the transfer paper. Make sure it prints onto the glossy side!

### Printer Settings

- Set resolution to highest (bitmap is sent at 600dpi)
- Shut off toner saving options.
- If it has a silent mode option, turn it on (prints slower which is good).
- Set density to 0 (neutral, not +, not -).

### A Note on Printing and Security

SeedEtcher is an air-gapped workflow. Therefore, your printer should obviously also be air-gapped. You should not use a networked printer for this. Albeit, it is very unlikely that an attacker would be able to extract the print layout, it is not impossible. So, keep that in mind.
Also, no cameras should be present where you do this process. That includes cell phones.

## Transferring Laser Print to Steel Plate

Important: Do not touch the printed surface! Oils from your fingers will prevent the toner from sticking to the metal.

Note: When doing 2-sided plates, keep in mind that you have to etch one side at a time. You cannot transfer toner to both sides because the heat would destroy the other side.

Cut the print with a 1-2mm edge around the black square. It’s easier to get off this way after transferring. 
Pre-heat the iron to around 170°C. (the temperature has to be between 150°C and 180°C). Tip: Use a thermometer to figure out how hot your iron gets.

Use a scotch brite, steel wool or 600 grit sand paper to thoroughly clean the plate. If you like the brushed metal look, sand it that way. Then thoroughly clean it with warm water and dish soap. Then clean it with acetone. Optionally, clean it with isopropyl alcohol after the acetone. Do not touch it after that. Oil on surface is your enemy. Let it dry.
Tape the transfer paper with the laser print face down onto the steel plate. Masking tape works well for this, since it is easily removable after transfer. Only tape one side, preferebly the side where there is just black (left or right side). A tiny strip is enough. ONLY use masking tape for holding the transfer paper, not for etch masking!

Put 2 layers of a thin cotton rag (no texture!) over the plate.
TIP (highly recommended!): Get some 0.5-2mm thick silicone sheet ([amazon](https://a.co/d/2F59LSZ)). Cut it to 10x10cm and use that instead of the cotton rag. Pressure and heat distribution is better with silicone.

Put the plate on a wood block on the floor. Some household paper folded 4 times underneath the plate helps to keep it from sliding. Your contraption for this should be stable, no wiggling.

Set a timer to 180 seconds. Start the timer. Press the iron onto the plate with increasing pressure, covering the whole plate with the iron for 60s. Do not slide the iron!
Lift, press down on left 3rd of plate, 30s. Then right side 30s. Then top 30s. Then bottom.
Pressure is important. Do it on the floor where you can really lean onto the iron. But be careful to not slide while pressing! And please, do not break your wife’s iron.

Optional: Put a stack of steel plates on top of the hot plate (heat sink). It seems to help moving the heat off the transfer paper quickly, causing the toner to be released onto the steel fully.
Let it cool off completely! The transfer paper should buckle and lift off the metal all by itself. The transfer paper should come off without any toner sticking to it. 
Don’t be frustrated if it doesn’t work the first time. This takes practice.
Common culprits: 
- not enough pressure (most of the toner sticks to the paper)
- not enough heat (most of the toner sticks to the paper)
- plate not clean enough (toner doesn’t stick everywhere)
- touching the print toner surface or plate (oils!)

Bake plate in oven.
Pre-heat to 170°C, no airflow. 
Bake for 8 minutes. 
This reflows the toner and makes it stick even more to the plate.

Repairs:\
If the transfer wasn’t perfect you can do repairs by using nail polish or stop out ground and a small brush or anti-etching pens ([amazon](https://a.co/d/5DnOhRR))


## Etching

You’ll need:

- [ ] Container to hold Ferric Chloride. Food containers made from HDPE work well. Choose a size that allows to fully submerge the metal plate in 1L of solution. Tip: Test with water first.
- [ ] Plastic bowl with 1L of 20–25°C warm water and 1–2 tablespoons (15–30 g) of baking soda (NaHCO₃) dissolved into it.
- [ ] Gloves and eye protection
- [ ] Timer
- [ ] Close access to running water

Warning:
Ferric Chloride stains EVERYTHING it comes in contact with. Don’t let it drip into your kitchen sink, you’ll ruin the sink.
Always first put the plate into the baking soda solution bowl before rinsing it with water. This prevents acid carryover into sink and flash rust.

1. Prepare the plate. You have to mask off the other side that should not be etched. Make sure you mask it off properly or etchant will get to it. Normal packaging tape or electrical tape works. Avoid masking tape! (it’s not water proof)\
Tip: make a holding flap from tape, so you can hold the plate easily.
2. Make sure the etchant is around 20-25°C. Too warm will make the etchant too aggressive and it will attack the toner mask more quickly. Too cold (10-15°C) and it etches much slower.
3. Set timer to 5 minutes and start. Submerge the plate fully into the FeCl, ideally keep it vertical. Get the FeCl moving slightly by either moving the container or the plate. Don’t go crazy, a slight movement of the fluid every 30s is enough.
4. Take the plate out, let it drip off, submerge it into the baking soda bowl. This neutralises the acidic ferric-chloride residue.
5. Rinse the plate under running water (no hot water!). \
You can use a very soft brush to clean the plate carefully from etching remains. Just be careful to not destroy the mask!\
This prevents the neutralizer to mess with your ferric chloride.
1. Repeat steps 3–5 three to four times. This depends on how deep you want to etch and how well your toner mask is holding up.\
Important: Never put the plate back into the acid right after neutralizing it. The etchant solution will be slowly destroyed by this.\
And obviously: Do not etch unattended!

One liter of FeCl should last you for plenty of plates. I etched 16 plates and it still works fine.\
When etch times double: replace.
Note: The etchant solution does not expire, but it loses strength as iron salts build up. 
So, re-use the etchant and when it’s done dispose of it properly.

## A note on Electric Etching

You could use salt water and 12V/1amp to etch.\
However, I do strongly advise you NOT to do that. Etching stainless steel with salt water can produce chlorine gas and other toxic chlorine compounds.\
You do NOT want chlorine gas in your lungs. There are hundreds of youtube videos on etching like this, and none of them cares to give you that warning.
Etching copper or brass this way is fine.

## Post processing

Remove the toner with acetone.

Optionally wipe it down with vinegar (prevents corrosion).

If the etching started to etch surfaces that should have been masked, you can often correct it by using 1200 or finer grit sandpaper with a sanding block.
Carefully sand the etched plate until the undesired etching errors are mostly gone.

Do not keep failed prints or transfer sheets: destroy immediately!

And lastly: Please do test your backup before calling it done.



