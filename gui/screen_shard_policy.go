package gui

import (
	"encoding/hex"
	"fmt"
	"strings"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/descriptor/shard"
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/op"
)

// ShardedPolicyScreen confirms that multisig sharding parameters are derived
// from the descriptor (read-only) for b0.2.
type ShardedPolicyScreen struct {
	Theme      *Colors
	Descriptor *urtypes.OutputDescriptor
	SetID      [16]byte
	Shares     []shard.Share
	OnBack     func() Screen
	OnContinue func() Screen
}

func (s *ShardedPolicyScreen) Update(ctx *Context, ops op.Ctx) Screen {
	th := s.Theme
	if th == nil {
		th = &singleTheme
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
		title := layoutTitle(ctx, ops, dims.X, th.Text, "Sharding")

		walletID := "N/A"
		setID := strings.ToUpper(hex.EncodeToString(s.SetID[:4]))
		if len(s.Shares) > 0 {
			walletID = strings.ToUpper(hex.EncodeToString(s.Shares[0].WalletID[:]))
		}
		body := fmt.Sprintf("Using descriptor values:\n\nt = %d\nn = %d\nWID: %s\nSET: %s", desc.Threshold, len(desc.Keys), walletID, setID)
		layoutBodyLeftUnderTitle(ctx, ops, dims, th.Text, title, body)

		layoutNavigation(ctx, inp, ops, th, dims,
			NavButton{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
			NavButton{Button: Button3, Style: StylePrimary, Icon: assets.IconCheckmark},
		)
		ctx.Frame()
	}
}
