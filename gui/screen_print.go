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

func (s *PrintSeedScreen) Print(ctx *Context, ops op.Ctx, th *Colors, mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paperFormat printer.PaperSize) bool {
	inp := &s.inp
	paperChoice := &ChoiceScreen{
		Title:   "Select Paper Size",
		Lead:    "Choose your printer's paper size",
		Choices: []string{"A4", "Letter"},
	}
	choice, ok := paperChoice.Choose(ctx, ops, th)
	if !ok {
		return false
	}
	selectedPaper := printer.PaperA4
	if choice == 1 {
		selectedPaper = printer.PaperLetter
	}
	for {
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
					success, err := progress.Show(ctx, ops, th, mnemonic, desc, keyIdx, selectedPaper)
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
		layoutTitle(ctx, ops, dims.X, th.Text, "%s", title)
		lead := fmt.Sprintf("Paper size: %s\n\nEnsure your printer is connected before printing.\nPress Print to continue.", selectedPaper)
		if desc != nil {
			lead = fmt.Sprintf("Paper size: %s\n\nEnsure your printer is connected before printing share %d/%d.\nPress Print to continue.", selectedPaper, keyIdx+1, len(desc.Keys))
		}
		sz := widget.Labelwf(ops.Begin(), ctx.Styles.lead, dims.X-16, th.Text, "%s", lead)
		op.Position(ops, ops.End(), dims.Div(2).Sub(sz.Div(2)))
		layoutNavigation(inp, ops, th, dims, []NavButton{
			{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
			{Button: Button3, Style: StylePrimary, Icon: assets.IconHammer},
		}...)
		ctx.Frame()
	}
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
		layoutNavigation(&s.inp, ops, th, dims, []NavButton{
			{Button: Button2, Style: StyleSecondary, Icon: assets.IconHammer, Progress: 0}, // Print Again
			{Button: Button3, Style: StylePrimary, Icon: assets.IconDiscard, Progress: 0},  // Delete Seed
		}...)
		ctx.Frame()
	}
}

type PrintProgressScreen struct {
	inp InputTracker
}

func (s *PrintProgressScreen) Show(ctx *Context, ops op.Ctx, th *Colors, mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paperFormat printer.PaperSize) (bool, error) {
	start := ctx.Platform.Now()
	duration := 2 * time.Second
	var printErr error
	go func() {
		if err := ctx.Platform.CreatePlates(ctx, mnemonic, desc, keyIdx); err != nil {
			logutil.DebugLog("Print failed in progress: %v", err)
			printErr = err
		}
		logutil.DebugLog("PrintPDF completed in goroutine")
	}()

	for {
		for {
			e, ok := s.inp.Next(ctx, Button1)
			if !ok {
				break
			}
			if e.Button == Button1 && s.inp.Clicked(e.Button) {
				return false, fmt.Errorf("print canceled")
			}
		}
		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, th.Background)
		layoutTitle(ctx, ops, dims.X, th.Text, "Printing")

		elapsed := ctx.Platform.Now().Sub(start)
		progress := float32(elapsed.Seconds() / duration.Seconds())
		if progress >= 1.0 || printErr != nil {
			logutil.DebugLog("Progress complete or error: success=%v, err=%v", printErr == nil, printErr)
			return printErr == nil, printErr
		}

		ctx.WakeupAt(ctx.Platform.Now().Add(100 * time.Millisecond))

		content := layout.Rectangle{Max: dims}.Shrink(leadingSize, 0, leadingSize, 0)
		op.Offset(ops, content.Center(assets.ProgressCircle.Bounds().Size()))
		(&ProgressImage{
			Progress: progress,
			Src:      assets.ProgressCircle,
		}).Add(ops)
		op.ColorOp(ops, th.Text)
		sz := widget.Labelf(ops.Begin(), ctx.Styles.progress, th.Text, "%d%%", int(progress*100))
		op.Position(ops, ops.End(), content.Center(sz))

		labelSz := widget.Labelwf(ops.Begin(), ctx.Styles.lead, dims.X-16, th.Text, "Printing...")
		op.Position(ops, ops.End(), content.Center(labelSz).Add(image.Pt(0, assets.ProgressCircle.Bounds().Dy()/2+12)))

		layoutNavigation(&s.inp, ops, th, dims, []NavButton{
			{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
		}...)
		ctx.Frame()
	}
}
