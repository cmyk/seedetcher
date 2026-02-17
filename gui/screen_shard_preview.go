package gui

import (
	"encoding/hex"
	"fmt"
	"math"
	"strings"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/descriptor/shard"
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/op"
)

// buildShardShares precomputes descriptor share metadata for one backup run
// using a fixed set_id.
func buildShardShares(desc *urtypes.OutputDescriptor, setID [16]byte) []shard.Share {
	threshold8, total8, ok := descriptorSplitParams(desc)
	if !ok {
		return nil
	}
	shares, err := shard.SplitPayloadBytes(desc.Encode(), shard.SplitOptions{
		Threshold: threshold8,
		Total:     total8,
		SetID:     setID,
	})
	if err != nil {
		return nil
	}
	return shares
}

func deriveShardSetID(desc *urtypes.OutputDescriptor) ([16]byte, bool) {
	threshold8, total8, ok := descriptorSplitParams(desc)
	if !ok {
		return [16]byte{}, false
	}
	return shard.DeriveSetID(desc.Encode(), threshold8, total8), true
}

func descriptorSplitParams(desc *urtypes.OutputDescriptor) (uint8, uint8, bool) {
	if desc == nil {
		return 0, 0, false
	}
	total := len(desc.Keys)
	threshold := desc.Threshold
	if total == 0 || threshold < 2 || threshold > total {
		return 0, 0, false
	}
	if total > math.MaxUint8 || threshold > math.MaxUint8 {
		return 0, 0, false
	}
	return uint8(threshold), uint8(total), true
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
