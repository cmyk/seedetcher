package gui

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/layout"
	"seedetcher.com/gui/op"
	"seedetcher.com/gui/widget"
	"seedetcher.com/logutil"
	"seedetcher.com/printer"
)

type PrintSeedScreen struct {
	inp InputTracker
}

type printOptions struct {
	DPI         int
	Invert      bool
	Mirror      bool
	EtchStats   bool
	Compact2of3 bool
	Singlesig   printer.SinglesigLayoutMode
	PrinterLang printer.PrinterLanguage
}

type printSetupState struct {
	PaperChoice     int
	SinglesigChoice int
	DPIChoice       int
	InvertChoice    int
	MirrorChoice    int
	StatsChoice     int
	CompactChoice   int
	PrinterLang     int
}

var lastPrintSetupState = printSetupState{
	PaperChoice:     0,
	SinglesigChoice: 1,
	DPIChoice:       0,
	InvertChoice:    0,
	MirrorChoice:    0,
	StatsChoice:     0,
	CompactChoice:   0,
	PrinterLang:     0,
}

func loadPrintSetupState() printSetupState {
	return lastPrintSetupState
}

func savePrintSetupState(s printSetupState) {
	lastPrintSetupState = s
}

func (s *PrintSeedScreen) Print(ctx *Context, ops op.Ctx, th *Colors, mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paperFormat printer.PaperSize, label string) bool {
	if label == "" {
		label = printer.DefaultWalletLabel
	}
	printer.SetWalletLabel(label)
	inp := &s.inp
	state := loadPrintSetupState()
	setupSteps := make([]string, 0, 8)
	if isSinglesigDescriptor(desc) {
		setupSteps = append(setupSteps, "singlesig")
	}
	if isCompact2of3Eligible(desc) {
		setupSteps = append(setupSteps, "compact")
	}
	setupSteps = append(setupSteps, "paper", "dpi", "invert", "mirror", "stats", "printerlang")
	stepIdx := 0

	updatePrinterStatus := func() {
		if ctx != nil {
			connected, model := ctx.Platform.PrinterStatus()
			ctx.PrinterConnected = connected
			if model != "" || !connected {
				ctx.PrinterModel = model
			}
		}
	}
	chooseWithInitial := func(title, lead string, choices []string, initial int) (int, bool) {
		cs := &ChoiceScreen{
			Title:   title,
			Lead:    lead,
			Choices: choices,
			choice:  initial,
		}
		return cs.Choose(ctx, ops, th)
	}

	inSetup := true
	for {
		if inSetup {
			step := setupSteps[stepIdx]
			var ok bool
			switch step {
			case "paper":
				var next int
				next, ok = chooseWithInitial("Select Paper Size", "Choose paper size", []string{"A4", "Letter"}, state.PaperChoice)
				if ok {
					state.PaperChoice = next
				}
			case "singlesig":
				var next int
				next, ok = chooseSinglesigLayoutOption(ctx, ops, th, state.SinglesigChoice)
				if ok {
					state.SinglesigChoice = next
				}
			case "dpi":
				var next int
				next, ok = chooseWithInitial("Print DPI", "Choose print resolution", []string{"1200", "600"}, state.DPIChoice)
				if ok {
					state.DPIChoice = next
				}
			case "invert":
				var next int
				next, ok = chooseWithInitial("Invert", "Invert plate output?", []string{"On", "Off"}, state.InvertChoice)
				if ok {
					state.InvertChoice = next
				}
			case "mirror":
				var next int
				next, ok = chooseWithInitial("Mirror", "Mirror plate output?", []string{"On", "Off"}, state.MirrorChoice)
				if ok {
					state.MirrorChoice = next
				}
			case "stats":
				var next int
				next, ok = chooseWithInitial("Etch Stats Page", "Append etch stats page?", []string{"Off", "On"}, state.StatsChoice)
				if ok {
					state.StatsChoice = next
				}
			case "compact":
				var next int
				next, ok = chooseWithInitial("Compact 2/3", "Use compact single-sided\n2-of-3 layout?", []string{"Off", "On"}, state.CompactChoice)
				if ok {
					state.CompactChoice = next
				}
			case "printerlang":
				var next int
				next, ok = choosePrinterLanguageOption(ctx, ops, th, state.PrinterLang, ctx != nil && ctx.HBPRuntimeReady)
				if ok {
					state.PrinterLang = next
				}
			default:
				ok = true
			}
			if !ok {
				if stepIdx == 0 {
					return false
				}
				stepIdx--
				continue
			}
			if stepIdx < len(setupSteps)-1 {
				savePrintSetupState(state)
				stepIdx++
				continue
			}
			savePrintSetupState(state)
			inSetup = false
			continue
		}

		selectedPaper := printer.PaperA4
		if state.PaperChoice == 1 {
			selectedPaper = printer.PaperLetter
		}
		opts := printOptions{
			DPI:         1200,
			Invert:      state.InvertChoice == 0,
			Mirror:      state.MirrorChoice == 0,
			EtchStats:   state.StatsChoice == 1,
			Compact2of3: state.CompactChoice == 1,
			Singlesig:   printer.SinglesigLayoutSeedWithInfo,
			PrinterLang: printer.PrinterLangPCL,
		}
		if state.DPIChoice == 1 {
			opts.DPI = 600
		}
		switch state.SinglesigChoice {
		case 0:
			opts.Singlesig = printer.SinglesigLayoutSeedOnly
		case 2:
			opts.Singlesig = printer.SinglesigLayoutSeedWithDescriptorQR
		}
		if state.PrinterLang == 1 {
			opts.PrinterLang = printer.PrinterLangPS
		}
		if state.PrinterLang == 2 {
			opts.PrinterLang = printer.PrinterLangBrotherHBP
		}

		updatePrinterStatus()
		for {
			e, ok := inp.Next(ctx, Button1, Button3)
			if !ok {
				break
			}
			switch e.Button {
			case Button1:
				if inp.Clicked(e.Button) {
					inSetup = true
					stepIdx = len(setupSteps) - 1
					break
				}
			case Button3:
				if inp.Clicked(e.Button) {
					printOpts := opts
					if opts.PrinterLang == printer.PrinterLangBrotherHBP {
						if ctx == nil || !ctx.HBPRuntimeReady {
							s.showError(ctx, ops, th, fmt.Errorf("Brother HBP runtime is not prepared.\nReturn to start screen and enable HBP before SD removal"))
							continue
						}
						if printOpts.DPI != 600 {
							printOpts.DPI = 600
							s.showNotice(ctx, ops, th, "HBP mode uses 600 DPI.\nDPI was set to 600 for this print.")
						}
					}
					if ctx != nil && ctx.HBPRuntimeReady && opts.PrinterLang == printer.PrinterLangPS && printOpts.DPI > 600 {
						pages := estimateJobPages(desc, selectedPaper, printOpts)
						if pages > 1 {
							printOpts.DPI = 600
							s.showNotice(ctx, ops, th, fmt.Sprintf("PS 1200 DPI is currently limited to one-page jobs.\nThis job has %d pages, so DPI was set to 600.", pages))
						}
					}
					progress := &PrintProgressScreen{}
					success, err := progress.Show(ctx, ops, th, mnemonic, desc, keyIdx, selectedPaper, printOpts)
					if err != nil && err.Error() != "print canceled" {
						s.showError(ctx, ops, th, err)
					}
					if err != nil && err.Error() == "print canceled" {
						continue
					}
					result := &PrintResultScreen{success: success}
					if result.Show(ctx, ops, th, mnemonic, desc, keyIdx, selectedPaper) {
						continue
					}
					return true
				}
			}
		}
		if inSetup {
			continue
		}
		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, th.Background)
		title := "Print Seed"
		if desc != nil {
			title = "Print Wallet Share"
		}
		titleRect := layoutTitle(ctx, ops, dims.X, th.Text, "%s", title)
		status := "Printer: Not connected"
		if ctx.PrinterConnected {
			if ctx.PrinterModel != "" {
				status = fmt.Sprintf("Printer: %s", ctx.PrinterModel)
			} else {
				status = "Printer: Connected"
			}
		}
		showCompactLine := isCompact2of3Eligible(desc)
		showSinglesigLine := isSinglesigDescriptor(desc)
		effectiveDPI := opts.DPI
		if opts.PrinterLang == printer.PrinterLangBrotherHBP {
			effectiveDPI = 600
		} else if ctx != nil && ctx.HBPRuntimeReady && opts.PrinterLang == printer.PrinterLangPS && opts.DPI > 600 {
			pages := estimateJobPages(desc, selectedPaper, opts)
			if pages > 1 {
				effectiveDPI = 600
			}
		}
		lead := fmt.Sprintf("%s\nPaper:%s @%d dpi\nInvert: %s, Mirror: %s\nEtch stats page: %s\nPrinter lang: %s", status, selectedPaper, effectiveDPI, onOff(opts.Invert), onOff(opts.Mirror), onOff(opts.EtchStats), printerLangLabel(opts.PrinterLang))
		if showCompactLine {
			lead += fmt.Sprintf("\nCompact 2/3: %s", onOff(opts.Compact2of3))
		}
		if showSinglesigLine {
			lead += fmt.Sprintf("\nSinglesig layout: %s", singlesigLayoutLabel(opts.Singlesig))
		}
		walletShares := 1
		if desc != nil {
			walletShares = len(desc.Keys)
		}
		maxSlotsPerPage := 6
		if selectedPaper == printer.PaperLetter {
			maxSlotsPerPage = 4
		}
		slotsPerShare := 2
		if desc == nil {
			slotsPerShare = 1
		}
		if showCompactLine && opts.Compact2of3 {
			slotsPerShare = 1
		}
		if showSinglesigLine && opts.Singlesig != printer.SinglesigLayoutSeedWithDescriptorQR {
			slotsPerShare = 1
		}
		sharesPerPage := maxSlotsPerPage / slotsPerShare
		if sharesPerPage < 1 {
			sharesPerPage = 1
		}
		totalPages := (walletShares + sharesPerPage - 1) / sharesPerPage
		statsSuffix := ""
		if opts.EtchStats {
			statsSuffix = " (+1)"
		}
		jobLabel := "seed shares"
		if desc != nil {
			jobLabel = "wallet shares"
		}
		lead += fmt.Sprintf("\n\nPrinting %d %s\nTotal pages: %d%s", walletShares, jobLabel, totalPages, statsSuffix)
		layoutBodyLeftUnderTitle(ctx, ops, dims, th.Text, titleRect, lead)
		layoutNavigation(ctx, inp, ops, th, dims, []NavButton{
			{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
			{Button: Button3, Style: StylePrimary, Icon: assets.IconPrint},
		}...)
		ctx.Frame()
	}
}

func chooseSinglesigLayoutOption(ctx *Context, ops op.Ctx, th *Colors, initialChoice int) (int, bool) {
	inp := new(InputTracker)
	choice := initialChoice
	if choice < 0 || choice > 2 {
		choice = 1
	}
	labels := []string{
		"Seed Only",
		"Seed + Info",
		"Seed + Descr QR",
	}
	details := []string{
		"1-sided, 2 copies",
		"1-sided, 2 copies",
		"2-sided, 2 copies",
	}

	for {
		for {
			e, ok := inp.Next(ctx, Button1, Button3, Center, Up, Down, CCW, CW)
			if !ok {
				break
			}
			switch e.Button {
			case Button1:
				if inp.Clicked(e.Button) {
					return 0, false
				}
			case Button3, Center:
				if inp.Clicked(e.Button) {
					return choice, true
				}
			case Up, CCW:
				if e.Pressed && choice > 0 {
					choice--
				}
			case Down, CW:
				if e.Pressed && choice < len(labels)-1 {
					choice++
				}
			}
		}

		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, th.Background)
		titleRect := layoutTitle(ctx, ops, dims.X, th.Text, "Singlesig layout")
		infoRect := layoutBodyLeftUnderTitle(ctx, ops, dims, th.Text, titleRect, "Choose print layout:")

		choicesMinY := infoRect.Max.Y + 4
		choicesMaxY := dims.Y - leadingSize - 30
		if choicesMaxY < choicesMinY+10 {
			choicesMaxY = choicesMinY + 10
		}
		content := layout.Rectangle(image.Rect(16, choicesMinY, dims.X-16, choicesMaxY))

		children := make([]struct {
			Size image.Point
			W    op.CallOp
		}, len(labels))
		maxW := 0
		for i, c := range labels {
			style := ctx.Styles.button
			col := th.Text
			if i == choice {
				col = th.Background
			}
			sz := widget.Labelf(ops.Begin(), style, col, "%s", c)
			ch := ops.End()
			children[i].Size = sz
			children[i].W = ch
			if sz.X > maxW {
				maxW = sz.X
			}
		}
		inner := ops.Begin()
		h := 0
		for i, c := range children {
			xoff := (maxW - c.Size.X) / 2
			pos := image.Pt(xoff, h)
			txt := c.W
			if i == choice {
				bg := image.Rectangle{Max: c.Size}
				bg.Min.X -= xoff
				bg.Max.X += xoff
				assets.ButtonFocused.Add(inner.Begin(), bg, true)
				op.ColorOp(inner, th.Text)
				txt.Add(inner)
				txt = inner.End()
			}
			op.Position(inner, txt, pos)
			h += c.Size.Y
		}
		op.Position(ops, ops.End(), content.Center(image.Pt(maxW, h)))
		descRect := image.Rectangle{
			Min: image.Pt(10, choicesMaxY+4),
			Max: image.Pt(dims.X-10, dims.Y-leadingSize-6),
		}
		if descRect.Dy() > 0 {
			style := ctx.Styles.body
			widget.Labelwf(ops.Begin(), style, descRect.Dx(), th.Text, "%s", details[choice])
			op.Position(ops, ops.End(), descRect.Min)
		}
		layoutNavigation(ctx, inp, ops, th, dims, []NavButton{
			{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
			{Button: Button3, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		ctx.Frame()
	}
}

func choosePrinterLanguageOption(ctx *Context, ops op.Ctx, th *Colors, initialChoice int, hbpReady bool) (int, bool) {
	choices := []string{"PCL", "PS"}
	if hbpReady {
		choices = append(choices, "Brother HBP")
	}
	choice := initialChoice
	if choice < 0 || choice >= len(choices) {
		choice = 0
	}
	cs := &ChoiceScreen{
		Title:   "Printer Language",
		Lead:    "Choose language",
		Choices: choices,
		choice:  choice,
	}
	return cs.Choose(ctx, ops, th)
}

func isCompact2of3Eligible(desc *urtypes.OutputDescriptor) bool {
	if desc == nil {
		return false
	}
	return desc.Type == urtypes.SortedMulti && desc.Threshold == 2 && len(desc.Keys) == 3
}

func isSinglesigDescriptor(desc *urtypes.OutputDescriptor) bool {
	if desc == nil {
		return false
	}
	return desc.Type == urtypes.Singlesig && len(desc.Keys) == 1
}

func singlesigLayoutLabel(mode printer.SinglesigLayoutMode) string {
	switch mode {
	case printer.SinglesigLayoutSeedOnly:
		return "Seed Only"
	case printer.SinglesigLayoutSeedWithDescriptorQR:
		return "Seed + Descr QR"
	default:
		return "Seed + Info"
	}
}

func onOff(v bool) string {
	if v {
		return "On"
	}
	return "Off"
}

func printerLangLabel(lang printer.PrinterLanguage) string {
	if lang == printer.PrinterLangPS {
		return "PS"
	}
	if lang == printer.PrinterLangBrotherHBP {
		return "HBP"
	}
	return "PCL"
}

func estimateJobPages(desc *urtypes.OutputDescriptor, paper printer.PaperSize, opts printOptions) int {
	walletShares := 1
	if desc != nil {
		walletShares = len(desc.Keys)
	}
	maxSlotsPerPage := 6
	if paper == printer.PaperLetter {
		maxSlotsPerPage = 4
	}
	slotsPerShare := 2
	if desc == nil {
		slotsPerShare = 1
	}
	if isCompact2of3Eligible(desc) && opts.Compact2of3 {
		slotsPerShare = 1
	}
	if isSinglesigDescriptor(desc) && opts.Singlesig != printer.SinglesigLayoutSeedWithDescriptorQR {
		slotsPerShare = 1
	}
	sharesPerPage := maxSlotsPerPage / slotsPerShare
	if sharesPerPage < 1 {
		sharesPerPage = 1
	}
	totalPages := (walletShares + sharesPerPage - 1) / sharesPerPage
	if totalPages < 1 {
		totalPages = 1
	}
	if opts.EtchStats {
		totalPages++
	}
	return totalPages
}

func (s *PrintSeedScreen) showError(ctx *Context, ops op.Ctx, th *Colors, err error) {
	logutil.DebugLog("showError called with error: %v", err)
	errScr := NewErrorScreen(err)
	for {
		dims := ctx.Platform.DisplaySize()
		dismissed := errScr.Layout(ctx, ops, th, dims)
		if dismissed {
			logutil.DebugLog("Error screen dismissed")
			break
		}
		ctx.Frame()
	}
}

func (s *PrintSeedScreen) showNotice(ctx *Context, ops op.Ctx, th *Colors, msg string) {
	n := &ErrorScreen{Title: "Notice", Body: msg}
	for {
		dims := ctx.Platform.DisplaySize()
		if n.Layout(ctx, ops, th, dims) {
			break
		}
		ctx.Frame()
	}
}

type PrintResultScreen struct {
	success bool
	inp     InputTracker
}

func (s *PrintResultScreen) Show(ctx *Context, ops op.Ctx, th *Colors, mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paperFormat printer.PaperSize) bool {
	for {
		for {
			e, ok := s.inp.Next(ctx, Button2, Button3)
			if !ok {
				break
			}
			if !s.inp.Clicked(e.Button) {
				continue
			}
			switch e.Button {
			case Button2: // Print Again
				return true
			case Button3: // Delete Seed and Go Home
				return false
			}
		}
		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, th.Background)
		layoutTitle(ctx, ops, dims.X, th.Text, "Print Result")
		msg := "Print completed successfully."
		if !s.success {
			msg = "Print failed. Check printer connection."
		}
		sz := widget.Labelwf(ops.Begin(), ctx.Styles.lead, dims.X-16, th.Text, "%s", msg)
		op.Position(ops, ops.End(), dims.Div(2).Sub(sz.Div(2)))
		layoutNavigation(ctx, &s.inp, ops, th, dims, []NavButton{
			{Button: Button2, Style: StyleSecondary, Icon: assets.IconPrint, Progress: 0}, // Print Again
			{Button: Button3, Style: StylePrimary, Icon: assets.IconDiscard, Progress: 0}, // Delete Seed
		}...)
		ctx.Frame()
	}
}

type PrintProgressScreen struct {
	inp InputTracker
}

type HBPRuntimePrepareScreen struct {
	inp InputTracker
}

var lastHBPPrepareDuration = 32 * time.Second

func (s *HBPRuntimePrepareScreen) Show(ctx *Context, ops op.Ctx, th *Colors) error {
	if ctx == nil {
		return fmt.Errorf("missing UI context")
	}

	done := make(chan error, 1)
	go func() {
		done <- ctx.Platform.PrepareHBPForSDRemoval()
	}()

	var (
		finished bool
		prepErr  error
	)
	startedAt := ctx.Platform.Now()
	for {
		if !finished {
			select {
			case prepErr = <-done:
				finished = true
			default:
			}
		}

		for {
			e, ok := s.inp.Next(ctx, Button3)
			if !ok {
				break
			}
			if finished && prepErr == nil && s.inp.Clicked(e.Button) {
				return nil
			}
		}

			dims := ctx.Platform.DisplaySize()
			op.ColorOp(ops, th.Background)
			layoutTitle(ctx, ops, dims.X, th.Text, "Preparing HBP")
			status := "Preparing Brother HBP runtime..."
			if finished && prepErr == nil {
				status = "Brother HBP is ready. SD can be removed safely."
			}

			if !finished {
				barW := dims.X - 48
				if barW < 120 {
					barW = dims.X - 24
				}
				barH := 10
				barX := (dims.X - barW) / 2
				barY := dims.Y/2 - barH/2
				barRect := image.Rect(barX, barY, barX+barW, barY+barH)

			track := color.NRGBA{R: th.Text.R, G: th.Text.G, B: th.Text.B, A: 70}
			op.ClipOp(barRect).Add(ops.Begin())
			op.ColorOp(ops, track)
			barBg := ops.End()
			barBg.Add(ops)

			eta := lastHBPPrepareDuration
			if eta < 5*time.Second {
				eta = 5 * time.Second
			}
			elapsed := ctx.Platform.Now().Sub(startedAt)
			progress := float32(elapsed) / float32(eta)
			if progress > 0.95 {
				progress = 0.95
			}
			if progress < 0 {
				progress = 0
			}
			fillW := int(float32(barW) * progress)
			if fillW < 4 {
				fillW = 4
			}
			fillRect := image.Rect(barX, barY, barX+fillW, barY+barH)
			op.ClipOp(fillRect).Add(ops.Begin())
			op.ColorOp(ops, th.Text)
				barFill := ops.End()
				barFill.Add(ops)
			}

			r := layout.Rectangle{Max: dims}
			_, bottom := r.CutTop(leadingSize)
			_, lead := bottom.CutBottom(leadingSize)
			sz := widget.Labelwf(ops.Begin(), ctx.Styles.lead, dims.X-16, th.Text, "%s", status)
			op.Position(ops, ops.End(), lead.Center(sz).Add(image.Pt(0, 12)))

			if finished && prepErr != nil {
				return prepErr
			}
		if finished && prepErr == nil {
			duration := ctx.Platform.Now().Sub(startedAt)
			if duration > 0 {
				lastHBPPrepareDuration = duration
			}
			layoutNavigation(ctx, &s.inp, ops, th, dims, []NavButton{
				{Button: Button3, Style: StylePrimary, Icon: assets.IconCheckmark},
			}...)
		}
		if !finished {
			ctx.WakeupAt(ctx.Platform.Now().Add(200 * time.Millisecond))
		}
		ctx.Frame()
	}
}

func (s *PrintProgressScreen) Show(ctx *Context, ops op.Ctx, th *Colors, mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paperFormat printer.PaperSize, printOpts printOptions) (bool, error) {
	var (
		printErr error
		done     = make(chan struct{})
	)
	finished := false
	var finishedAt time.Time
	type progressUpdate struct {
		stage   printer.PrintStage
		current int64
		total   int64
	}
	progressCh := make(chan progressUpdate, 8)
	stageState := make(map[printer.PrintStage]progressUpdate)
	progressVal := float32(0)
	lastStage := printer.StagePrepare
	if ctx != nil {
		ctx.PrintProgress = func(stage printer.PrintStage, current, total int64) {
			if total <= 0 {
				return
			}
			select {
			case progressCh <- progressUpdate{stage: stage, current: current, total: total}:
			default:
			}
		}
		defer func() { ctx.PrintProgress = nil }()
	}
	go func() {
		opts := printer.RasterOptions{
			DPI:             float64(printOpts.DPI),
			Mirror:          printOpts.Mirror,
			Invert:          printOpts.Invert,
			PrinterLang:     printOpts.PrinterLang,
			SinglesigLayout: printOpts.Singlesig,
			EtchStatsPage:   printOpts.EtchStats,
		}
		printer.SetCompactDescriptor2of3Enabled(printOpts.Compact2of3)
		defer printer.SetCompactDescriptor2of3Enabled(false)
		printErr = ctx.Platform.CreatePlates(ctx, mnemonic, desc, keyIdx, paperFormat, opts)
		close(done)
	}()

	for {
		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, th.Background)
		layoutTitle(ctx, ops, dims.X, th.Text, "Printing")

		if !finished {
			select {
			case <-done:
				finished = true
				finishedAt = ctx.Platform.Now()
				if progressVal < 1 {
					progressVal = 1
				}
			default:
			}
		}

		select {
		case p := <-progressCh:
			stageState[p.stage] = p
			if p.stage >= lastStage {
				lastStage = p.stage
			}
			// Mark earlier stages complete if we reached a later stage.
			ordered := []printer.PrintStage{printer.StagePrepare, printer.StageCompose, printer.StageSend}
			for _, st := range ordered {
				if st == p.stage {
					break
				}
				if _, ok := stageState[st]; !ok {
					stageState[st] = progressUpdate{stage: st, current: 1, total: 1}
				}
			}
			// Compute overall progress as the average of stage fractions.
			sum := float32(0)
			for _, st := range ordered {
				if upd, ok := stageState[st]; ok && upd.total > 0 {
					f := float32(upd.current) / float32(upd.total)
					if f < 0 {
						f = 0
					}
					if f > 1 {
						f = 1
					}
					sum += f
				}
			}
			if len(ordered) > 0 {
				progressVal = sum / float32(len(ordered))
			}
		default:
		}

		ctx.WakeupAt(ctx.Platform.Now().Add(100 * time.Millisecond))

		content := layout.Rectangle{Max: dims}.Shrink(leadingSize, 0, leadingSize, 0)
		op.Offset(ops, content.Center(assets.ProgressCircle.Bounds().Size()))
		(&ProgressImage{Progress: progressVal, Src: assets.ProgressCircle}).Add(ops)
		op.ColorOp(ops, th.Text)
		percentLabel := fmt.Sprintf("%d%%", int(progressVal*100+0.5))
		pctSz := widget.Labelwf(ops.Begin(), ctx.Styles.lead, assets.ProgressCircle.Bounds().Dx(), th.Text, "%s", percentLabel)
		op.Position(ops, ops.End(), content.Center(pctSz))
		label := "Preparing..."
		if upd, ok := stageState[lastStage]; ok {
			switch lastStage {
			case printer.StagePrepare:
				label = fmt.Sprintf("Rendering plates %d/%d", upd.current, upd.total)
			case printer.StageCompose:
				label = fmt.Sprintf("Composing pages %d/%d", upd.current, upd.total)
			case printer.StageSend:
				label = "Sending job to printer"
			}
		}
				sz := widget.Labelwf(ops.Begin(), ctx.Styles.lead, dims.X-16, th.Text, "%s", label)
				r := layout.Rectangle{Max: dims}
				_, bottom := r.CutTop(leadingSize)
				_, lead := bottom.CutBottom(leadingSize)
				op.Position(ops, ops.End(), lead.Center(sz).Add(image.Pt(0, 12)))

		layoutNavigation(ctx, &s.inp, ops, th, dims)
		ctx.Frame()

		if finished && !finishedAt.IsZero() && ctx.Platform.Now().Sub(finishedAt) >= time.Second {
			return printErr == nil, printErr
		}
	}
}
