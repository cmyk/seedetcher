package gui

import (
	"encoding/hex"
	"fmt"
	"image"
	"strings"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/descriptor/shard"
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/op"
	"seedetcher.com/gui/widget"
)

// buildShardPreview precomputes descriptor shares and encoded QR payloads for
// one backup run using a fixed set_id.
func buildShardPreview(desc *urtypes.OutputDescriptor, setID [16]byte) ([]shard.Share, []string) {
	if desc == nil || len(desc.Keys) == 0 || desc.Threshold < 2 || desc.Threshold > len(desc.Keys) {
		return nil, nil
	}
	shares, err := shard.SplitPayloadBytes(desc.Encode(), shard.SplitOptions{
		Threshold: uint8(desc.Threshold),
		Total:     uint8(len(desc.Keys)),
		SetID:     setID,
	})
	if err != nil {
		return nil, nil
	}
	qrs := make([]string, len(shares))
	for i, sh := range shares {
		enc, err := shard.Encode(sh)
		if err != nil {
			return nil, nil
		}
		qrs[i] = enc
	}
	return shares, qrs
}

// ShardPreviewScreen shows each shard QR one-at-a-time and requires explicit
// next-share confirmation before printing.
type ShardPreviewScreen struct {
	Theme  *Colors
	Shares []shard.Share
	QRs    []string
	Idx    int
	OnBack func() Screen
	OnDone func() Screen
}

func (s *ShardPreviewScreen) Update(ctx *Context, ops op.Ctx) Screen {
	th := s.Theme
	if th == nil {
		th = &singleTheme
	}
	if len(s.Shares) == 0 || len(s.QRs) == 0 {
		if s.OnDone != nil {
			return s.OnDone()
		}
		return &MainMenuScreen{}
	}
	if s.Idx < 0 {
		s.Idx = 0
	}
	if s.Idx >= len(s.QRs) {
		s.Idx = len(s.QRs) - 1
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
				if s.Idx == 0 {
					if s.OnBack != nil {
						return s.OnBack()
					}
					return &MainMenuScreen{}
				}
				s.Idx--
			case Button3:
				if s.Idx >= len(s.QRs)-1 {
					if s.OnDone != nil {
						return s.OnDone()
					}
					return &MainMenuScreen{}
				}
				s.Idx++
			}
		}

		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, th.Background)
		layoutTitle(ctx, ops, dims.X, th.Text, "Descriptor Share QR")

		sh := s.Shares[s.Idx]
		wid := strings.ToUpper(hex.EncodeToString(sh.WalletID[:]))
		sid := strings.ToUpper(hex.EncodeToString(sh.SetID[:4]))
		lead := fmt.Sprintf("Share %d/%d (need %d)\nWID:%s SET:%s", sh.Index, sh.Total, sh.Threshold, wid, sid)
		lsz := widget.Labelwf(ops.Begin(), ctx.Styles.body, dims.X-88, th.Text, "%s", lead)
		op.Position(ops, ops.End(), image.Pt((dims.X-lsz.X)/2, leadingSize+8))

		qrImg := renderQRImage(s.QRs[s.Idx], 150)
		qsz := qrImg.Bounds().Size()
		qp := image.Pt((dims.X-qsz.X)/2, (dims.Y-qsz.Y)/2+10)
		op.ImageOp(ops.Begin(), qrImg, false)
		op.Position(ops, ops.End(), qp)

		note := "Back = previous / return\nCheck = next / continue"
		nsz := widget.Labelwf(ops.Begin(), ctx.Styles.debug, dims.X-88, th.Text, "%s", note)
		op.Position(ops, ops.End(), image.Pt((dims.X-nsz.X)/2, dims.Y-nsz.Y-leadingSize-8))

		layoutNavigation(ctx, inp, ops, th, dims,
			NavButton{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
			NavButton{Button: Button3, Style: StylePrimary, Icon: assets.IconCheckmark},
		)
		ctx.Frame()
	}
}
