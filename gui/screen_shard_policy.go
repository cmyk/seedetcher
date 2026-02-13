package gui

import (
	"fmt"
	"image"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/op"
	"seedetcher.com/gui/widget"
)

// ShardedPolicyScreen confirms that multisig sharding parameters are derived
// from the descriptor (read-only) for b0.2.
type ShardedPolicyScreen struct {
	Theme      *Colors
	Descriptor *urtypes.OutputDescriptor
	OnBack     func() Screen
	OnContinue func() Screen
}

func (s *ShardedPolicyScreen) Update(ctx *Context, ops op.Ctx) Screen {
	th := s.Theme
	if th == nil {
		th = &descriptorTheme
	}
	desc := s.Descriptor
	if desc == nil {
		if s.OnContinue != nil {
			return s.OnContinue()
		}
		return &MainMenuScreen{}
	}

	inp := new(InputTracker)
	for {
		for {
			e, ok := inp.Next(ctx, Button1, Button3)
			if !ok {
				break
			}
			if !inp.Clicked(e.Button) {
				continue
			}
			switch e.Button {
			case Button1:
				if s.OnBack != nil {
					return s.OnBack()
				}
				return &MainMenuScreen{}
			case Button3:
				if s.OnContinue != nil {
					return s.OnContinue()
				}
				return &MainMenuScreen{}
			}
		}

		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, th.Background)
		layoutTitle(ctx, ops, dims.X, th.Text, "Sharded Descriptor")

		body := fmt.Sprintf(
			"Multisig descriptor shares are derived from the wallet descriptor.\n\nThreshold (t): %d\nTotal shares (n): %d\n\nNo manual t/n selection in b0.2.",
			desc.Threshold,
			len(desc.Keys),
		)
		sz := widget.Labelwf(ops.Begin(), ctx.Styles.body, dims.X-16, th.Text, "%s", body)
		op.Position(ops, ops.End(), image.Pt((dims.X-sz.X)/2, leadingSize+10))

		layoutNavigation(ctx, inp, ops, th, dims,
			NavButton{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
			NavButton{Button: Button3, Style: StylePrimary, Icon: assets.IconCheckmark},
		)
		ctx.Frame()
	}
}
