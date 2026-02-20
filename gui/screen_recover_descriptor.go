package gui

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kortschak/qr"
	"seedetcher.com/bc/ur"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/descriptor/legacy"
	"seedetcher.com/descriptor/shard"
	"seedetcher.com/descriptor/urxor2of3"
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
	recoverDisplayDescriptor recoverDisplayMode = iota
	recoverDisplayURSingle
	recoverDisplayURMultipart
)

// RecoverDescriptorFlowScreen handles descriptor recovery from sharded shares
// or plain descriptor QR input.
type RecoverDescriptorFlowScreen struct {
	Theme               *Colors
	stage               recoverStage
	recoveredUR         string
	recoveredText       string
	recoveredPayloadRaw []byte
	recoveredURPayload  []byte
	recoveredQR         image.Image
	decodedShares       map[uint8]shard.Share
	decodedURShares     map[string]struct{}
	modeChoice          int
	displayMode         recoverDisplayMode
	viewQR              image.Image
	viewNextTick        time.Time
	viewSeqNum          int
	viewSeqLen          int
	returnScreen        Screen
}

func (s *RecoverDescriptorFlowScreen) Update(ctx *Context, ops op.Ctx) Screen {
	defer func() {
		if r := recover(); r != nil {
			logutil.DebugLog("recover screen panic: %v", r)
			showError(ctx, ops, &singleTheme, fmt.Errorf("recovery failed for scanned payload"))
			s.stage = recoverStageScan
			s.recoveredUR = ""
			s.recoveredText = ""
			s.recoveredPayloadRaw = nil
			s.recoveredURPayload = nil
			s.recoveredQR = nil
			s.decodedShares = make(map[uint8]shard.Share)
			s.decodedURShares = make(map[string]struct{})
		}
	}()

	th := s.Theme
	if th == nil {
		th = &singleTheme
	}
	if s.decodedShares == nil {
		s.decodedShares = make(map[uint8]shard.Share)
	}
	if s.decodedURShares == nil {
		s.decodedURShares = make(map[string]struct{})
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
		if s.returnScreen != nil {
			return s.returnScreen
		}
		return &ActionChoiceScreen{Theme: th}
	}

	switch v := res.(type) {
	case urtypes.OutputDescriptor:
		showError(ctx, ops, th, fmt.Errorf("Not part of a descriptor share set!"))
		return s
	case []byte:
		raw := strings.TrimSpace(string(v))
		up := strings.ToUpper(raw)
		if typ, _, seqLen, ok := urxor2of3.ParseShare(raw); ok && typ == "crypto-output" && seqLen == urxor2of3.RequiredShares {
			if len(s.decodedShares) > 0 {
				showError(ctx, ops, th, fmt.Errorf("share set mismatch: mixed descriptor share formats"))
				return s
			}
			key := strings.ToLower(strings.TrimSpace(raw))
			if _, exists := s.decodedURShares[key]; exists {
				showError(ctx, ops, th, fmt.Errorf("share already scanned"))
				return s
			}
			s.decodedURShares[key] = struct{}{}
			parts := make([]string, 0, len(s.decodedURShares))
			for p := range s.decodedURShares {
				parts = append(parts, p)
			}
			payload, err := urxor2of3.Combine(parts)
			if err != nil {
				if errors.Is(err, urxor2of3.ErrInsufficientShares) {
					ctx.addToast(fmt.Sprintf("Captured share (%d/%d)", len(s.decodedURShares), urxor2of3.RequiredShares), 1700)
					return s
				}
				delete(s.decodedURShares, key)
				showError(ctx, ops, th, fmt.Errorf("combine shares failed: %v", err))
				return s
			}
			rawPayload := append([]byte(nil), payload...)
			recoveredUR, err := safeEncodeURPayload(rawPayload)
			if err != nil {
				showError(ctx, ops, th, fmt.Errorf("failed to encode recovered descriptor"))
				logutil.DebugLog("recover UR encode failed: %v", err)
				return s
			}
			s.recoveredUR = recoveredUR
			s.recoveredText = buildDescriptorText(rawPayload)
			s.recoveredPayloadRaw = rawPayload
			s.recoveredURPayload = rawPayload
			dims := ctx.Platform.DisplaySize()
			qrSize := dims.X
			if dims.Y < qrSize {
				qrSize = dims.Y
			}
			qrSize -= 8
			s.recoveredQR = renderQRImage(s.recoveredUR, qrSize)
			s.returnScreen = &ActionChoiceScreen{Theme: th}
			s.stage = recoverStageExport
			ctx.addToast("Descriptor recovered", 1200)
			return s
		}
		if strings.HasPrefix(up, shard.Prefix) {
			if len(s.decodedURShares) > 0 {
				showError(ctx, ops, th, fmt.Errorf("share set mismatch: mixed descriptor share formats"))
				return s
			}
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
			rawPayload := append([]byte(nil), payload...)
			urPayload, err := buildURPayload(rawPayload)
			if err != nil {
				showError(ctx, ops, th, fmt.Errorf("failed to build recovered descriptor"))
				logutil.DebugLog("recover payload build failed: %v", err)
				return s
			}
			recoveredUR, err := safeEncodeURPayload(urPayload)
			if err != nil {
				showError(ctx, ops, th, fmt.Errorf("failed to encode recovered descriptor"))
				logutil.DebugLog("recover UR encode failed: %v", err)
				return s
			}
			s.recoveredUR = recoveredUR
			s.recoveredText = buildDescriptorText(rawPayload)
			s.recoveredPayloadRaw = rawPayload
			s.recoveredURPayload = urPayload
			dims := ctx.Platform.DisplaySize()
			qrSize := dims.X
			if dims.Y < qrSize {
				qrSize = dims.Y
			}
			qrSize -= 8
			s.recoveredQR = renderQRImage(s.recoveredUR, qrSize)
			s.returnScreen = &ActionChoiceScreen{Theme: th}
			s.stage = recoverStageExport
			ctx.addToast("Descriptor recovered", 1200)
			return s
		}
		showError(ctx, ops, th, fmt.Errorf("Not part of a descriptor share set!"))
		return s
	case string:
		showError(ctx, ops, th, fmt.Errorf("Not part of a descriptor share set!"))
		return s
	default:
		showError(ctx, ops, th, fmt.Errorf("Not part of a descriptor share set!"))
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
			e, ok := inp.Next(ctx, Button2, Button3)
			if !ok {
				break
			}
			if !inp.Clicked(e.Button) {
				continue
			}
			switch e.Button {
			case Button2:
				// Trash wipes recovery state and restarts scanning.
				s.recoveredUR = ""
				s.recoveredText = ""
				s.recoveredPayloadRaw = nil
				s.recoveredURPayload = nil
				s.recoveredQR = nil
				s.decodedShares = make(map[uint8]shard.Share)
				s.decodedURShares = make(map[string]struct{})
				s.stage = recoverStageScan
				ctx.addToast("Recovery state deleted", 1200)
				if s.returnScreen != nil {
					return s.returnScreen
				}
				return &ActionChoiceScreen{Theme: th}
			case Button3:
				s.stage = recoverStageMode
				return s
			}
		}

		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, th.Background)
		leftPad := 10
		rightPad := 10
		textW := dims.X - leftPad - rightPad
		if textW < 80 {
			textW = 80
		}

		title := "Descriptor Recovered"
		titleStyle := ctx.Styles.title
		titleStyle.Alignment = text.AlignCenter
		tsz := widget.Labelwf(ops.Begin(), titleStyle, textW, th.Text, "%s", title)
		titleY := 8 // fixed top offset
		op.Position(ops, ops.End(), image.Pt((dims.X-tsz.X)/2, titleY))

		wid, sid := "", ""
		if base, ok := firstDecodedShare(s.decodedShares); ok {
			wid = strings.ToUpper(fmt.Sprintf("%x", base.WalletID))
			sid = strings.ToUpper(fmt.Sprintf("%x", base.SetID[:4]))
		}
		lead := "Shares reconstructed successfully."
		if wid != "" && sid != "" {
			lead = fmt.Sprintf("Shares reconstructed successfully.\nWID: %s\nSET: %s", wid, sid)
		}
		leadStyle := ctx.Styles.lead
		leadStyle.Alignment = text.AlignStart
		lsz := widget.Labelwf(ops.Begin(), leadStyle, textW, th.Text, "%s", lead)
		leadY := titleY + tsz.Y + 10 // fixed title->body spacing
		op.Position(ops, ops.End(), image.Pt(leftPad, leadY))
		_ = lsz

		layoutNavigation(ctx, inp, ops, th, dims,
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
		Choices: []string{"Descriptor QR", "UR Single", "UR Multipart"},
		choice:  s.modeChoice,
	})
	idx, ok := choice.Choose(ctx, ops, th)
	s.modeChoice = choice.choice
	if !ok {
		s.stage = recoverStageExport
		return s
	}
	switch idx {
	case 2:
		s.displayMode = recoverDisplayURMultipart
	case 1:
		s.displayMode = recoverDisplayURSingle
	default:
		s.displayMode = recoverDisplayDescriptor
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
	case recoverDisplayURMultipart:
		if len(s.recoveredURPayload) == 0 {
			// Fallback if payload is unavailable for any reason.
			s.viewQR = renderQRImageRect(s.recoveredUR, dims.X, dims.Y)
			return
		}
		if s.viewSeqLen == 0 {
			s.viewSeqLen = chooseMultipartSeqLen(len(s.recoveredURPayload))
		}
		if s.viewSeqNum <= 0 || s.viewSeqNum > s.viewSeqLen {
			s.viewSeqNum = 1
		}
		if s.viewQR == nil || s.viewNextTick.IsZero() || !now.Before(s.viewNextTick) {
			part := ur.Encode("crypto-output", s.recoveredURPayload, s.viewSeqNum, s.viewSeqLen)
			s.viewQR = renderQRImageRect(part, dims.X, dims.Y)
			s.viewSeqNum++
			if s.viewSeqNum > s.viewSeqLen {
				s.viewSeqNum = 1
			}
			s.viewNextTick = now.Add(300 * time.Millisecond)
		}
		ctx.WakeupAt(s.viewNextTick)
	case recoverDisplayURSingle:
		if s.viewQR == nil || s.viewQR.Bounds().Dx() != dims.X || s.viewQR.Bounds().Dy() != dims.Y {
			s.viewQR = renderQRImageRect(s.recoveredUR, dims.X, dims.Y)
		}
	default:
		if s.viewQR == nil || s.viewQR.Bounds().Dx() != dims.X || s.viewQR.Bounds().Dy() != dims.Y {
			content := s.recoveredText
			if strings.TrimSpace(content) == "" {
				content = s.recoveredUR
			}
			s.viewQR = renderQRImageRect(content, dims.X, dims.Y)
		}
	}
}

func (s *RecoverDescriptorFlowScreen) scanLead() string {
	if len(s.decodedURShares) > 0 {
		return fmt.Sprintf("Captured %d/%d descriptor shares", len(s.decodedURShares), urxor2of3.RequiredShares)
	}
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
	return renderQRImageRectMaxSide(content, width, height, 0)
}

func renderQRImageRectMaxSide(content string, width, height, maxSideLimit int) image.Image {
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
	if maxSideLimit > 0 && maxSideLimit < maxSide {
		maxSide = maxSideLimit
	}
	if modules <= maxSide {
		// Integer module blocks when it fits on screen.
		step := maxSide / modules
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
	// Fallback for very dense payloads: map pixels to module coordinates
	// to avoid clipping when module count exceeds display resolution.
	for y := 0; y < height; y++ {
		my := y * modules / height
		row := img.Pix[y*img.Stride:]
		for x := 0; x < width; x++ {
			mx := x * modules / width
			if mx < quiet || my < quiet || mx >= quiet+code.Size || my >= quiet+code.Size {
				continue
			}
			if code.Black(mx-quiet, my-quiet) {
				row[x] = color.Gray{Y: 0}.Y
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

func buildDescriptorText(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	v, err := urtypes.Parse("crypto-output", payload)
	if err != nil {
		return ""
	}
	desc, ok := v.(urtypes.OutputDescriptor)
	if !ok {
		return ""
	}
	return formatDescriptorText(desc)
}

func formatDescriptorText(desc urtypes.OutputDescriptor) string {
	var wrap func(urtypes.Script, string) string
	wrap = func(script urtypes.Script, inner string) string {
		switch script {
		case urtypes.P2WSH:
			return "wsh(" + inner + ")"
		case urtypes.P2SH_P2WSH:
			return "sh(wsh(" + inner + "))"
		case urtypes.P2SH:
			return "sh(" + inner + ")"
		case urtypes.P2WPKH:
			return "wpkh(" + inner + ")"
		case urtypes.P2SH_P2WPKH:
			return "sh(wpkh(" + inner + "))"
		case urtypes.P2PKH:
			return "pkh(" + inner + ")"
		case urtypes.P2TR:
			return "tr(" + inner + ")"
		default:
			return inner
		}
	}

	formatChild := func(d urtypes.Derivation) string {
		switch d.Type {
		case urtypes.WildcardDerivation:
			if d.Hardened {
				return "*'"
			}
			return "*"
		case urtypes.RangeDerivation:
			start := strconv.FormatUint(uint64(d.Index), 10)
			end := strconv.FormatUint(uint64(d.End), 10)
			if d.Hardened {
				return "<" + start + ";" + end + ">'"
			}
			return "<" + start + ";" + end + ">"
		default:
			v := strconv.FormatUint(uint64(d.Index), 10)
			if d.Hardened {
				return v + "'"
			}
			return v
		}
	}

	formatKey := func(k urtypes.KeyDescriptor) string {
		origin := ""
		if len(k.DerivationPath) > 0 {
			path := strings.TrimPrefix(k.DerivationPath.String(), "m/")
			path = strings.ReplaceAll(path, "h", "'")
			origin = fmt.Sprintf("[%08x/%s]", k.MasterFingerprint, path)
		} else {
			origin = fmt.Sprintf("[%08x]", k.MasterFingerprint)
		}
		key := k.String()
		if len(k.Children) == 0 {
			return origin + key
		}
		parts := make([]string, 0, len(k.Children))
		for _, c := range k.Children {
			parts = append(parts, formatChild(c))
		}
		return origin + key + "/" + strings.Join(parts, "/")
	}

	if desc.Type == urtypes.SortedMulti {
		keys := make([]string, 0, len(desc.Keys))
		for _, k := range desc.Keys {
			key := formatKey(k)
			if len(k.Children) == 0 {
				// Some importers reject multisig descriptors without explicit
				// account/change/index children; default to common BIP48 form.
				key += "/<0;1>/*"
			}
			keys = append(keys, key)
		}
		return wrap(desc.Script, fmt.Sprintf("sortedmulti(%d,%s)", desc.Threshold, strings.Join(keys, ",")))
	}
	if len(desc.Keys) > 0 {
		return wrap(desc.Script, formatKey(desc.Keys[0]))
	}
	return ""
}

func buildURPayload(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty descriptor payload")
	}
	v, err := urtypes.Parse("crypto-output", payload)
	if err != nil {
		return nil, err
	}
	desc, ok := v.(urtypes.OutputDescriptor)
	if !ok {
		return nil, fmt.Errorf("not a descriptor payload")
	}
	norm := legacy.NormalizeDescriptorForLegacyUR(desc)
	return norm.Encode(), nil
}

func safeEncodeDescriptorUR(payload []byte) (string, error) {
	urPayload, err := buildURPayload(payload)
	if err != nil {
		return "", err
	}
	return safeEncodeURPayload(urPayload)
}

func safeEncodeURPayload(payload []byte) (string, error) {
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
