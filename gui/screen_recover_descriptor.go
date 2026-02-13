package gui

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"github.com/kortschak/qr"
	"seedetcher.com/bc/ur"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/descriptor/shard"
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/op"
	"seedetcher.com/gui/widget"
	"seedetcher.com/logutil"
	"seedetcher.com/nonstandard"
)

// SDCardGateScreen enforces SD-card removal before entering sensitive flows.
type SDCardGateScreen struct {
	Theme *Colors
	Next  Screen
	warn  *ConfirmWarningScreen
}

func (s *SDCardGateScreen) Update(ctx *Context, ops op.Ctx) Screen {
	if ctx.EmptySDSlot {
		if s.Next != nil {
			return s.Next
		}
		return &MainMenuScreen{}
	}
	th := s.Theme
	if th == nil {
		th = &descriptorTheme
	}
	if s.warn == nil {
		s.warn = &ConfirmWarningScreen{
			Title: "Remove SD card",
			Body:  "Remove SD card to continue.\n\nHold button to ignore this warning.",
			Icon:  assets.IconRight,
		}
	}
	dims := ctx.Platform.DisplaySize()
	switch s.warn.Layout(ctx, ops, th, dims) {
	case ConfirmYes:
		ctx.EmptySDSlot = true
		if s.Next != nil {
			return s.Next
		}
		return &MainMenuScreen{}
	case ConfirmNo:
		return &MainMenuScreen{}
	case ConfirmNone:
		ctx.Frame()
		return s
	}
	return s
}

type recoverStage int

const (
	recoverStageScan recoverStage = iota
	recoverStageExport
)

// RecoverDescriptorFlowScreen handles descriptor recovery from sharded shares
// or plain descriptor QR input.
type RecoverDescriptorFlowScreen struct {
	Theme         *Colors
	stage         recoverStage
	recovered     *urtypes.OutputDescriptor
	recoveredUR   string
	recoveredQR   image.Image
	decodedShares map[uint8]shard.Share
}

func (s *RecoverDescriptorFlowScreen) Update(ctx *Context, ops op.Ctx) Screen {
	th := s.Theme
	if th == nil {
		th = &descriptorTheme
	}
	if s.decodedShares == nil {
		s.decodedShares = make(map[uint8]shard.Share)
	}

	switch s.stage {
	case recoverStageScan:
		return s.scanStep(ctx, ops, th)
	case recoverStageExport:
		return s.exportStep(ctx, ops, th)
	default:
		return &MainMenuScreen{}
	}
}

func (s *RecoverDescriptorFlowScreen) scanStep(ctx *Context, ops op.Ctx, th *Colors) Screen {
	res, ok := (&ScanScreen{
		Title: "Recover Descriptor",
		Lead:  "Scan descriptor or share",
	}).Scan(ctx, ops)
	if !ok {
		return &MainMenuScreen{}
	}

	switch v := res.(type) {
	case urtypes.OutputDescriptor:
		return s.loadRecoveredDescriptor(ctx, v)
	case []byte:
		shareText := strings.TrimSpace(string(v))
		if strings.HasPrefix(strings.ToUpper(shareText), shard.Prefix) {
			sh, err := shard.Decode(strings.ToUpper(shareText))
			if err != nil {
				showError(ctx, ops, th, fmt.Errorf("invalid share QR: %v", err))
				return s
			}
			if _, exists := s.decodedShares[sh.Index]; exists {
				showError(ctx, ops, th, fmt.Errorf("share %d already scanned", sh.Index))
				return s
			}
			s.decodedShares[sh.Index] = sh
			if len(s.decodedShares) < int(sh.Threshold) {
				ctx.addToast(fmt.Sprintf("Share %d/%d", len(s.decodedShares), sh.Threshold), 1500)
				return s
			}
			shares := make([]shard.Share, 0, len(s.decodedShares))
			for _, part := range s.decodedShares {
				shares = append(shares, part)
			}
			descText, err := shard.Combine(shares)
			if err != nil {
				showError(ctx, ops, th, fmt.Errorf("combine shares failed: %v", err))
				return s
			}
			desc, err := nonstandard.OutputDescriptor([]byte(descText))
			if err != nil {
				showError(ctx, ops, th, fmt.Errorf("parsed descriptor invalid: %v", err))
				return s
			}
			return s.loadRecoveredDescriptor(ctx, desc)
		}
		showError(ctx, ops, th, fmt.Errorf("unsupported QR payload"))
		return s
	default:
		showError(ctx, ops, th, fmt.Errorf("unsupported QR payload"))
		return s
	}
}

func (s *RecoverDescriptorFlowScreen) loadRecoveredDescriptor(ctx *Context, desc urtypes.OutputDescriptor) Screen {
	s.recovered = &desc
	s.recoveredUR = ur.Encode("crypto-output", desc.Encode(), 1, 1)
	s.recoveredQR = renderQRImage(s.recoveredUR, 180)
	s.stage = recoverStageExport
	ctx.addToast("Descriptor recovered", 1200)
	return s
}

func (s *RecoverDescriptorFlowScreen) exportStep(ctx *Context, ops op.Ctx, th *Colors) Screen {
	if s.recovered == nil || s.recoveredQR == nil {
		s.stage = recoverStageScan
		return s
	}
	inp := new(InputTracker)
	for {
		for {
			e, ok := inp.Next(ctx, Button1, Button2)
			if !ok {
				break
			}
			if !inp.Clicked(e.Button) {
				continue
			}
			switch e.Button {
			case Button1:
				// Back keeps the recovered QR visible for re-scan.
				ctx.addToast("QR stays visible", 900)
				return s
			case Button2:
				// Trash wipes recovery state and restarts scanning.
				s.recovered = nil
				s.recoveredUR = ""
				s.recoveredQR = nil
				s.decodedShares = make(map[uint8]shard.Share)
				s.stage = recoverStageScan
				ctx.addToast("Recovery state deleted", 1200)
				return s
			}
		}

		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, th.Background)
		layoutTitle(ctx, ops, dims.X, th.Text, "Recovered Descriptor QR")

		lead := "Scan with your coordinator, then choose:"
		sz := widget.Labelwf(ops.Begin(), ctx.Styles.body, dims.X-16, th.Text, "%s", lead)
		op.Position(ops, ops.End(), image.Pt((dims.X-sz.X)/2, leadingSize+8))

		qrSize := s.recoveredQR.Bounds().Size()
		qrPos := image.Pt((dims.X-qrSize.X)/2, (dims.Y-qrSize.Y)/2)
		op.ImageOp(ops, s.recoveredQR, false)
		op.Position(ops, ops.End(), qrPos)

		note := "Back = show QR again\nTrash = delete and restart"
		nsz := widget.Labelwf(ops.Begin(), ctx.Styles.debug, dims.X-16, th.Text, "%s", note)
		op.Position(ops, ops.End(), image.Pt((dims.X-nsz.X)/2, dims.Y-nsz.Y-leadingSize-6))

		layoutNavigation(ctx, inp, ops, th, dims,
			NavButton{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
			NavButton{Button: Button2, Style: StylePrimary, Icon: assets.IconDiscard},
		)
		ctx.Frame()
	}
}

func renderQRImage(content string, size int) image.Image {
	if size < 80 {
		size = 80
	}
	code, err := qr.Encode(content, qr.L)
	if err != nil {
		logutil.DebugLog("recover qr encode failed: %v", err)
		img := image.NewGray(image.Rect(0, 0, size, size))
		for i := range img.Pix {
			img.Pix[i] = 255
		}
		return img
	}
	const quiet = 4
	modules := code.Size + 2*quiet
	step := size / modules
	if step < 1 {
		step = 1
	}
	actual := modules * step
	img := image.NewGray(image.Rect(0, 0, actual, actual))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if !code.Black(x, y) {
				continue
			}
			x0 := (x + quiet) * step
			y0 := (y + quiet) * step
			for yy := y0; yy < y0+step; yy++ {
				row := img.Pix[yy*img.Stride:]
				for xx := x0; xx < x0+step; xx++ {
					row[xx] = color.Gray{Y: 0}.Y
				}
			}
		}
	}
	return img
}
