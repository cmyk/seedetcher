package gui

import (
	"fmt"
	"image"
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
	DPI       int
	Invert    bool
	Mirror    bool
	EtchStats bool
}

func (s *PrintSeedScreen) Print(ctx *Context, ops op.Ctx, th *Colors, mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paperFormat printer.PaperSize, label string) bool {
	if label == "" {
		label = printer.DefaultWalletLabel
	}
	printer.SetWalletLabel(label)
	inp := &s.inp
	paperChoice := &ChoiceScreen{
		Title:   "Select Paper Size",
		Lead:    "Choose your printer's\npaper size",
		Choices: []string{"A4", "Letter"},
	}
	choice, ok := paperChoice.Choose(ctx, ops, th)
	if !ok {
		return false
	}
	updatePrinterStatus := func() {
		if ctx != nil {
			connected, model := ctx.Platform.PrinterStatus()
			ctx.PrinterConnected = connected
			if model != "" || !connected {
				ctx.PrinterModel = model
			}
		}
	}
	selectedPaper := printer.PaperA4
	if choice == 1 {
		selectedPaper = printer.PaperLetter
	}
	opts, ok := choosePrintOptions(ctx, ops, th)
	if !ok {
		return false
	}
	for {
		updatePrinterStatus()
		for {
			e, ok := inp.Next(ctx, Button1, Button3)
			if !ok {
				break
			}
			switch e.Button {
			case Button1:
				if inp.Clicked(e.Button) {
					return false
				}
			case Button3:
				if inp.Clicked(e.Button) {
					progress := &PrintProgressScreen{}
					success, err := progress.Show(ctx, ops, th, mnemonic, desc, keyIdx, selectedPaper, opts)
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
				status = fmt.Sprintf("Printer: Connected (%s)", ctx.PrinterModel)
			} else {
				status = "Printer: Connected"
			}
		}
		lead := fmt.Sprintf("%s\nPaper: %s  DPI: %d\nInvert: %t\nMirror: %t\nEtch stats page: %t\n\nPress Print to continue.", status, selectedPaper, opts.DPI, opts.Invert, opts.Mirror, opts.EtchStats)
		if desc != nil {
			lead = fmt.Sprintf("%s\nPaper: %s  DPI: %d\nInvert: %t\nMirror: %t\nEtch stats page: %t\n\nPrinting %d wallet shares.\nPress Print to continue.", status, selectedPaper, opts.DPI, opts.Invert, opts.Mirror, opts.EtchStats, len(desc.Keys))
		}
		layoutBodyLeftUnderTitle(ctx, ops, dims, th.Text, titleRect, lead)
		layoutNavigation(ctx, inp, ops, th, dims, []NavButton{
			{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
			{Button: Button3, Style: StylePrimary, Icon: assets.IconPrint},
		}...)
		ctx.Frame()
	}
}

func choosePrintOptions(ctx *Context, ops op.Ctx, th *Colors) (printOptions, bool) {
	out := printOptions{
		DPI:       1200,
		Invert:    true,
		Mirror:    true,
		EtchStats: false,
	}
	dpiChoice := &ChoiceScreen{
		Title:   "Print DPI",
		Lead:    "Choose print resolution",
		Choices: []string{"1200", "600"},
	}
	choice, ok := dpiChoice.Choose(ctx, ops, th)
	if !ok {
		return out, false
	}
	if choice == 1 {
		out.DPI = 600
	}
	invertChoice := &ChoiceScreen{
		Title:   "Invert",
		Lead:    "Invert plate output?",
		Choices: []string{"On", "Off"},
	}
	choice, ok = invertChoice.Choose(ctx, ops, th)
	if !ok {
		return out, false
	}
	out.Invert = choice == 0
	mirrorChoice := &ChoiceScreen{
		Title:   "Mirror",
		Lead:    "Mirror plate output?",
		Choices: []string{"On", "Off"},
	}
	choice, ok = mirrorChoice.Choose(ctx, ops, th)
	if !ok {
		return out, false
	}
	out.Mirror = choice == 0
	statsChoice := &ChoiceScreen{
		Title:   "Etch Stats Page",
		Lead:    "Append etch stats page?",
		Choices: []string{"Off", "On"},
	}
	choice, ok = statsChoice.Choose(ctx, ops, th)
	if !ok {
		return out, false
	}
	out.EtchStats = choice == 1
	return out, true
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
			DPI:           float64(printOpts.DPI),
			Mirror:        printOpts.Mirror,
			Invert:        printOpts.Invert,
			EtchStatsPage: printOpts.EtchStats,
		}
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
			lastStage = p.stage
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
				label = "Composing pages..."
			case printer.StageSend:
				label = "Sending to printer..."
			}
		}
		sz := widget.Labelwf(ops.Begin(), ctx.Styles.lead, dims.X-16, th.Text, "%s", label)
		op.Position(ops, ops.End(), content.Center(sz).Add(image.Pt(0, assets.ProgressCircle.Bounds().Dy()/2+17)))

		layoutNavigation(ctx, &s.inp, ops, th, dims)
		ctx.Frame()

		if finished && !finishedAt.IsZero() && ctx.Platform.Now().Sub(finishedAt) >= time.Second {
			return printErr == nil, printErr
		}
	}
}
