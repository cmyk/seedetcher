package gui

import (
	"image"

	"github.com/btcsuite/btcd/chaincfg"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/layout"
	"seedetcher.com/gui/op"
)

type DescriptorScreen struct {
	Descriptor urtypes.OutputDescriptor
	Mnemonic   bip39.Mnemonic
}

func (s *DescriptorScreen) Confirm(ctx *Context, ops op.Ctx, th *Colors) (int, bool) {
	showErr := func(errScreen *ErrorScreen) {
		for {
			dims := ctx.Platform.DisplaySize()
			dismissed := errScreen.Layout(ctx, ops.Begin(), th, dims)
			d := ops.End()
			if dismissed {
				break
			}
			s.Draw(ctx, ops, th, dims)
			d.Add(ops)
			ctx.Frame()
		}
	}
	inp := new(InputTracker)
	for {
		for {
			e, ok := inp.Next(ctx, Button1, Button2, Button3)
			if !ok {
				break
			}
			switch e.Button {
			case Button1:
				if inp.Clicked(e.Button) {
					return 0, false
				}
			case Button2:
				if !inp.Clicked(e.Button) {
					break
				}
				ShowAddressesScreen(ctx, ops, th, s.Descriptor)
			case Button3:
				if !inp.Clicked(e.Button) {
					break
				}
				if err := validateDescriptor(s.Descriptor); err != nil {
					showErr(NewErrorScreen(err))
					continue
				}
				keyIdx, ok := descriptorKeyIdx(s.Descriptor, s.Mnemonic, "")
				if !ok {
					// Passphrase protected seeds don't match the descriptor, so allow singlesig ignore.
					if len(s.Descriptor.Keys) == 1 {
						confirm := &ConfirmWarningScreen{
							Title: "Unknown Wallet",
							Body:  "The wallet does not match the seed.\n\nIf it is passphrase protected, long press to confirm.",
							Icon:  assets.IconCheckmark,
						}
					loop:
						for {
							dims := ctx.Platform.DisplaySize()
							res := confirm.Layout(ctx, ops.Begin(), th, dims)
							d := ops.End()
							switch res {
							case ConfirmYes:
								return 0, true
							case ConfirmNo:
								break loop
							}
							s.Draw(ctx, ops, th, dims)
							d.Add(ops)
							ctx.Frame()
						}
					} else {
						showErr(&ErrorScreen{
							Title: "Unknown Wallet",
							Body:  "The wallet does not match the seed or is passphrase protected.",
						})
					}
					continue
				}
				return keyIdx, true
			}
		}

		dims := ctx.Platform.DisplaySize()
		s.Draw(ctx, ops, th, dims)
		layoutNavigation(ctx, inp, ops, th, dims, []NavButton{
			{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
			{Button: Button2, Style: StyleSecondary, Icon: assets.IconInfo},
			{Button: Button3, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		ctx.Frame()
	}
}

func (s *DescriptorScreen) Draw(ctx *Context, ops op.Ctx, th *Colors, dims image.Point) {
	const infoSpacing = 8

	desc := s.Descriptor
	op.ColorOp(ops, th.Background)

	// Title.
	r := layout.Rectangle{Max: dims}
	layoutTitle(ctx, ops, dims.X, th.Text, "Confirm Wallet")

	btnw := assets.NavBtnPrimary.Bounds().Dx()
	body := r.Shrink(leadingSize, btnw, 0, btnw)

	{
		ops := ops.Begin()
		var bodytxt richText

		bodyst := ctx.Styles.body
		subst := ctx.Styles.subtitle
		if desc.Title != "" {
			bodytxt.Add(ops, subst, body.Dx(), th.Text, "Title")
			bodytxt.Add(ops, bodyst, body.Dx(), th.Text, "%s", desc.Title)
			bodytxt.Y += infoSpacing
		}
		bodytxt.Add(ops, subst, body.Dx(), th.Text, "Type")
		testnet := any("") // TinyGo allocates without explicit interface conversion.
		if len(desc.Keys) > 0 && desc.Keys[0].Network != &chaincfg.MainNetParams {
			testnet = " (testnet)"
		}
		switch desc.Type {
		case urtypes.Singlesig:
			bodytxt.Add(ops, bodyst, body.Dx(), th.Text, "Singlesig%s", testnet)
		default:
			bodytxt.Add(ops, bodyst, body.Dx(), th.Text, "%d-of-%d multisig%s", desc.Threshold, len(desc.Keys), testnet)
		}
		bodytxt.Y += infoSpacing
		bodytxt.Add(ops, subst, body.Dx(), th.Text, "Script")
		bodytxt.Add(ops, bodyst, body.Dx(), th.Text, "%s", desc.Script.String())
	}

	op.Position(ops, ops.End(), body.Min.Add(image.Pt(0, scrollFadeDist)))
}
