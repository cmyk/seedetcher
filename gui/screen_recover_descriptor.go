package gui

import (
	"fmt"
	"image"
	"image/color"
	"sort"
	"strings"
	"time"

	"github.com/kortschak/qr"
	"seedetcher.com/bc/ur"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/descriptor/shard"
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/op"
	"seedetcher.com/gui/text"
	"seedetcher.com/gui/widget"
	"seedetcher.com/logutil"
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
		th = &singleTheme
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
	recoverStageMode
	recoverStageQR
)

type recoverDisplayMode int

const (
	recoverDisplaySingle recoverDisplayMode = iota
	recoverDisplayMultipart
)

// RecoverDescriptorFlowScreen handles descriptor recovery from sharded shares
// or plain descriptor QR input.
type RecoverDescriptorFlowScreen struct {
	Theme            *Colors
	stage            recoverStage
	recoveredUR      string
	recoveredPayload []byte
	recoveredQR      image.Image
	decodedShares    map[uint8]shard.Share
	modeChoice       int
	displayMode      recoverDisplayMode
	viewQR           image.Image
	viewNextTick     time.Time
	viewSeqNum       int
	viewSeqLen       int
}

func (s *RecoverDescriptorFlowScreen) Update(ctx *Context, ops op.Ctx) Screen {
	defer func() {
		if r := recover(); r != nil {
			logutil.DebugLog("recover screen panic: %v", r)
			showError(ctx, ops, &singleTheme, fmt.Errorf("recovery failed for scanned payload"))
			s.stage = recoverStageScan
			s.recoveredUR = ""
			s.recoveredPayload = nil
			s.recoveredQR = nil
			s.decodedShares = make(map[uint8]shard.Share)
		}
	}()

	th := s.Theme
	if th == nil {
		th = &singleTheme
	}
	if s.decodedShares == nil {
		s.decodedShares = make(map[uint8]shard.Share)
	}

	switch s.stage {
	case recoverStageScan:
		return s.scanStep(ctx, ops, th)
	case recoverStageExport:
		return s.exportStep(ctx, ops, th)
	case recoverStageMode:
		return s.modeStep(ctx, ops, th)
	case recoverStageQR:
		return s.qrStep(ctx, ops, th)
	default:
		return &MainMenuScreen{}
	}
}

func (s *RecoverDescriptorFlowScreen) scanStep(ctx *Context, ops op.Ctx, th *Colors) (next Screen) {
	next = s
	defer func() {
		if r := recover(); r != nil {
			logutil.DebugLog("recover flow panic: %v", r)
			showError(ctx, ops, th, fmt.Errorf("recovery failed for scanned payload"))
			next = s
		}
	}()

	res, ok := (&ScanScreen{
		Title: "Recover Descriptor",
		Lead:  s.scanLead(),
	}).Scan(ctx, ops)
	if !ok {
		return &MainMenuScreen{}
	}

	switch v := res.(type) {
	case urtypes.OutputDescriptor:
		showError(ctx, ops, th, fmt.Errorf("Not part of a shamir split descriptor!"))
		return s
	case []byte:
		raw := strings.TrimSpace(string(v))
		up := strings.ToUpper(raw)
		if strings.HasPrefix(up, shard.Prefix) {
			sh, err := shard.Decode(up)
			if err != nil {
				showError(ctx, ops, th, fmt.Errorf("invalid share QR: %v", err))
				return s
			}
			if base, ok := firstDecodedShare(s.decodedShares); ok && !sharesCompatible(base, sh) {
				showError(ctx, ops, th, fmt.Errorf("share set mismatch: different wallet or shard set"))
				return s
			}
			if _, exists := s.decodedShares[sh.Index]; exists {
				showError(ctx, ops, th, fmt.Errorf("share %d already scanned", sh.Index))
				return s
			}
			s.decodedShares[sh.Index] = sh
			if len(s.decodedShares) < int(sh.Threshold) {
				ctx.addToast(fmt.Sprintf("Captured share #%d (%d/%d)", sh.Index, len(s.decodedShares), sh.Threshold), 1700)
				return s
			}
			shares := make([]shard.Share, 0, len(s.decodedShares))
			for _, part := range s.decodedShares {
				shares = append(shares, part)
			}
			payload, err := shard.CombinePayloadBytes(shares)
			if err != nil {
				showError(ctx, ops, th, fmt.Errorf("combine shares failed: %v", err))
				return s
			}
			// Reconstruct canonical UR payload for coordinator import.
			recoveredUR, err := safeEncodeDescriptorUR(payload)
			if err != nil {
				showError(ctx, ops, th, fmt.Errorf("failed to encode recovered descriptor"))
				logutil.DebugLog("recover UR encode failed: %v", err)
				return s
			}
			s.recoveredUR = recoveredUR
			s.recoveredPayload = payload
			dims := ctx.Platform.DisplaySize()
			qrSize := dims.X
			if dims.Y < qrSize {
				qrSize = dims.Y
			}
			qrSize -= 8
			s.recoveredQR = renderQRImage(s.recoveredUR, qrSize)
			s.stage = recoverStageExport
			ctx.addToast("Descriptor recovered", 1200)
			return s
		}
		showError(ctx, ops, th, fmt.Errorf("Not part of a shamir split descriptor!"))
		return s
	case string:
		showError(ctx, ops, th, fmt.Errorf("Not part of a shamir split descriptor!"))
		return s
	default:
		showError(ctx, ops, th, fmt.Errorf("Not part of a shamir split descriptor!"))
		return s
	}
}

func (s *RecoverDescriptorFlowScreen) exportStep(ctx *Context, ops op.Ctx, th *Colors) Screen {
	if strings.TrimSpace(s.recoveredUR) == "" || s.recoveredQR == nil {
		s.stage = recoverStageScan
		return s
	}
	inp := new(InputTracker)
	for {
		for {
			e, ok := inp.Next(ctx, Button1, Button2, Button3)
			if !ok {
				break
			}
			if !inp.Clicked(e.Button) {
				continue
			}
			switch e.Button {
			case Button1:
				s.stage = recoverStageScan
				ctx.addToast("Scan again", 900)
				return s
			case Button2:
				// Trash wipes recovery state and restarts scanning.
				s.recoveredUR = ""
				s.recoveredPayload = nil
				s.recoveredQR = nil
				s.decodedShares = make(map[uint8]shard.Share)
				s.stage = recoverStageScan
				ctx.addToast("Recovery state deleted", 1200)
				return s
			case Button3:
				s.stage = recoverStageMode
				return s
			}
		}

		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, th.Background)
		leftPad := 6
		rightPad := 4
		navW := assets.NavBtnPrimary.Bounds().Dx()
		textW := dims.X - leftPad - navW - rightPad
		if textW < 80 {
			textW = 80
		}

		title := "Descriptor recovered"
		titleStyle := ctx.Styles.title
		titleStyle.Alignment = text.AlignStart
		tsz := widget.Labelwf(ops.Begin(), titleStyle, textW, th.Text, "%s", title)
		titleY := 8
		op.Position(ops, ops.End(), image.Pt(leftPad, titleY))

		lead := "Shares reconstructed successfully."
		leadStyle := ctx.Styles.lead
		leadStyle.Alignment = text.AlignStart
		lsz := widget.Labelwf(ops.Begin(), leadStyle, textW, th.Text, "%s", lead)
		leadY := titleY + tsz.Y + 12
		op.Position(ops, ops.End(), image.Pt(leftPad, leadY))

		note := "Check: show QR fullscreen\nBack: scan again\nTrash: delete"
		bodyStyle := ctx.Styles.body
		bodyStyle.Alignment = text.AlignStart
		widget.Labelwf(ops.Begin(), bodyStyle, textW, th.Text, "%s", note)
		noteY := leadY + lsz.Y + 14
		op.Position(ops, ops.End(), image.Pt(leftPad, noteY))

		layoutNavigation(ctx, inp, ops, th, dims,
			NavButton{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
			NavButton{Button: Button2, Style: StyleSecondary, Icon: assets.IconDiscard},
			NavButton{Button: Button3, Style: StylePrimary, Icon: assets.IconCheckmark},
		)
		ctx.Frame()
	}
}

func (s *RecoverDescriptorFlowScreen) modeStep(ctx *Context, ops op.Ctx, th *Colors) Screen {
	choice := (&ChoiceScreen{
		Title:   "Display mode",
		Lead:    "Choose output mode",
		Choices: []string{"Single QR", "Multipart UR"},
		choice:  s.modeChoice,
	})
	idx, ok := choice.Choose(ctx, ops, th)
	s.modeChoice = choice.choice
	if !ok {
		s.stage = recoverStageExport
		return s
	}
	if idx == 1 {
		s.displayMode = recoverDisplayMultipart
	} else {
		s.displayMode = recoverDisplaySingle
	}
	s.viewQR = nil
	s.viewNextTick = time.Time{}
	s.viewSeqNum = 1
	s.viewSeqLen = 0
	s.stage = recoverStageQR
	return s
}

func (s *RecoverDescriptorFlowScreen) qrStep(ctx *Context, ops op.Ctx, th *Colors) Screen {
	if strings.TrimSpace(s.recoveredUR) == "" || s.recoveredQR == nil {
		s.stage = recoverStageExport
		return s
	}
	inp := new(InputTracker)
	for {
		for {
			e, ok := inp.Next(ctx, Button1, Button2, Button3, Center, Up, Down, Left, Right, CCW, CW)
			if !ok {
				break
			}
			if e.Pressed || inp.Clicked(e.Button) {
				s.stage = recoverStageExport
				return s
			}
		}
		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, th.Background)
		s.updateViewerQR(ctx, dims)
		if s.viewQR != nil {
			op.ImageOp(ops, s.viewQR, false)
		}
		ctx.Frame()
	}
}

func (s *RecoverDescriptorFlowScreen) updateViewerQR(ctx *Context, dims image.Point) {
	now := ctx.Platform.Now()
	switch s.displayMode {
	case recoverDisplayMultipart:
		if len(s.recoveredPayload) == 0 {
			// Fallback if payload is unavailable for any reason.
			s.viewQR = renderQRImageRect(s.recoveredUR, dims.X, dims.Y)
			return
		}
		if s.viewSeqLen == 0 {
			s.viewSeqLen = chooseMultipartSeqLen(len(s.recoveredPayload))
		}
		if s.viewSeqNum <= 0 || s.viewSeqNum > s.viewSeqLen {
			s.viewSeqNum = 1
		}
		if s.viewQR == nil || s.viewNextTick.IsZero() || !now.Before(s.viewNextTick) {
			part := ur.Encode("crypto-output", s.recoveredPayload, s.viewSeqNum, s.viewSeqLen)
			s.viewQR = renderQRImageRect(part, dims.X, dims.Y)
			s.viewSeqNum++
			if s.viewSeqNum > s.viewSeqLen {
				s.viewSeqNum = 1
			}
			s.viewNextTick = now.Add(300 * time.Millisecond)
		}
		ctx.WakeupAt(s.viewNextTick)
	default:
		if s.viewQR == nil || s.viewQR.Bounds().Dx() != dims.X || s.viewQR.Bounds().Dy() != dims.Y {
			s.viewQR = renderQRImageRect(s.recoveredUR, dims.X, dims.Y)
		}
	}
}

func (s *RecoverDescriptorFlowScreen) scanLead() string {
	if len(s.decodedShares) == 0 {
		return "Scan descriptor share"
	}
	ids := make([]int, 0, len(s.decodedShares))
	threshold := 0
	for _, sh := range s.decodedShares {
		ids = append(ids, int(sh.Index))
		if int(sh.Threshold) > threshold {
			threshold = int(sh.Threshold)
		}
	}
	sort.Ints(ids)
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("#%d", id))
	}
	return fmt.Sprintf("Captured %d/%d: %s", len(ids), threshold, strings.Join(parts, " "))
}

func firstDecodedShare(m map[uint8]shard.Share) (shard.Share, bool) {
	for _, sh := range m {
		return sh, true
	}
	return shard.Share{}, false
}

func sharesCompatible(a, b shard.Share) bool {
	return a.Version == b.Version &&
		a.SetID == b.SetID &&
		a.WalletID == b.WalletID &&
		a.Network == b.Network &&
		a.Script == b.Script &&
		a.Threshold == b.Threshold &&
		a.Total == b.Total
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
	img := image.NewGray(image.Rect(0, 0, size, size))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	for y := 0; y < size; y++ {
		my := y * code.Size / size
		row := img.Pix[y*img.Stride:]
		for x := 0; x < size; x++ {
			mx := x * code.Size / size
			if code.Black(mx, my) {
				row[x] = color.Gray{Y: 0}.Y
			}
		}
	}
	return img
}

func renderQRImageRect(content string, width, height int) image.Image {
	if width < 80 {
		width = 80
	}
	if height < 80 {
		height = 80
	}
	code, err := qr.Encode(content, qr.L)
	if err != nil {
		logutil.DebugLog("recover qr rect encode failed: %v", err)
		img := image.NewGray(image.Rect(0, 0, width, height))
		for i := range img.Pix {
			img.Pix[i] = 255
		}
		return img
	}
	img := image.NewGray(image.Rect(0, 0, width, height))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	const quiet = 4
	modules := code.Size + 2*quiet
	maxSide := width
	if height < maxSide {
		maxSide = height
	}
	step := maxSide / modules
	if step < 1 {
		step = 1
	}
	qrSide := modules * step
	xOff := (width - qrSide) / 2
	yOff := (height - qrSide) / 2
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if !code.Black(x, y) {
				continue
			}
			x0 := xOff + (x+quiet)*step
			y0 := yOff + (y+quiet)*step
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

func chooseMultipartSeqLen(payloadLen int) int {
	// Keep per-frame density moderate on 240x240 while avoiding very long cycles.
	if payloadLen <= 0 {
		return 12
	}
	n := payloadLen / 22
	if payloadLen%22 != 0 {
		n++
	}
	if n < 12 {
		n = 12
	}
	if n > 48 {
		n = 48
	}
	return n
}

func safeEncodeDescriptorUR(payload []byte) (string, error) {
	var (
		out string
		err error
	)
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("ur encode panic: %v", r)
			}
		}()
		out = ur.Encode("crypto-output", payload, 1, 1)
	}()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("empty ur output")
	}
	return out, nil
}
