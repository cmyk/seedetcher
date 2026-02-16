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

// buildShardShares precomputes descriptor share metadata for one backup run
// using a fixed set_id.
func buildShardShares(desc *urtypes.OutputDescriptor, setID [16]byte) []shard.Share {
	if desc == nil || len(desc.Keys) == 0 || desc.Threshold < 2 || desc.Threshold > len(desc.Keys) {
		return nil
	}
	shares, err := shard.SplitPayloadBytes(desc.Encode(), shard.SplitOptions{
		Threshold: uint8(desc.Threshold),
		Total:     uint8(len(desc.Keys)),
		SetID:     setID,
	})
	if err != nil {
		return nil
	}
	return shares
}

// ShardPreviewScreen shows descriptor shard policy summary before print setup.
type ShardPreviewScreen struct {
	Theme  *Colors
	Shares []shard.Share
	OnBack func() Screen
	OnDone func() Screen
}

func (s *ShardPreviewScreen) Update(ctx *Context, ops op.Ctx) Screen {
	th := s.Theme
	if th == nil {
		th = &singleTheme
	}
	if len(s.Shares) == 0 {
		if s.OnDone != nil {
			return s.OnDone()
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
				if s.OnDone != nil {
					return s.OnDone()
				}
				return &MainMenuScreen{}
			}
		}

		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, th.Background)
		title := layoutTitle(ctx, ops, dims.X, th.Text, "Descriptor Shares")

		sh := s.Shares[0]
		wid := strings.ToUpper(hex.EncodeToString(sh.WalletID[:]))
		sid := strings.ToUpper(hex.EncodeToString(sh.SetID[:4]))
		body := fmt.Sprintf(
			"Need %d of %d descriptor shares to recover.\n\nWID: %s\nSET: %s\n\nContinue to wallet label and print setup.",
			sh.Threshold,
			sh.Total,
			wid,
			sid,
		)
		layoutBodyLeftUnderTitle(ctx, ops, dims, th.Text, title, body)

		layoutNavigation(ctx, inp, ops, th, dims,
			NavButton{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
			NavButton{Button: Button3, Style: StylePrimary, Icon: assets.IconCheckmark},
		)
		ctx.Frame()
	}
}
