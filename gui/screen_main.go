package gui

import (
	"image"

	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/layout"
	"seedetcher.com/gui/op"
)

// MainMenuScreen renders the landing page and routes into the backup flow.
type MainMenuScreen struct{}

func (s *MainMenuScreen) Update(ctx *Context, ops op.Ctx) Screen {
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
				return s // No-op back on root
			case Button3:
				// Enter backup/print flow as its own screen.
				return &BackupFlowScreen{Theme: &singleTheme}
			}
		}
		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, singleTheme.Background)
		layoutTitle(ctx, ops, dims.X, singleTheme.Text, "SeedEtcher")
		// Simple center icon/title; reuse existing assets/layout helpers.
		center := layout.Rectangle{Max: dims}.Center(image.Pt(assets.Hammer.Bounds().Dx(), assets.Hammer.Bounds().Dy()))
		icon := ops.Begin()
		op.ImageOp(icon, assets.Hammer, false)
		op.Position(ops, ops.End(), center)
		layoutNavigation(inp, ops, &singleTheme, dims, []NavButton{
			{Button: Button3, Style: StylePrimary, Icon: assets.IconHammer},
		}...)
		ctx.Frame()
	}
}

// BackupFlowScreen wraps the legacy backup flow inside the Screen loop.
type BackupFlowScreen struct {
	Theme *Colors
}

func (s *BackupFlowScreen) Update(ctx *Context, ops op.Ctx) Screen {
	th := s.Theme
	if th == nil {
		th = &descriptorTheme
	}
	// Require SD card removal before proceeding, matching the legacy main flow.
	if !ctx.EmptySDSlot {
		ws := &ConfirmWarningScreen{
			Title: "Remove SD card",
			Body:  "Remove SD card to continue.\n\nHold button to ignore this warning.",
			Icon:  assets.IconRight,
		}
		for {
			dims := ctx.Platform.DisplaySize()
			switch ws.Layout(ctx, ops.Begin(), th, dims) {
			case ConfirmYes:
				ctx.EmptySDSlot = true
				goto startFlow
			case ConfirmNo:
				return &MainMenuScreen{}
			}
			ctx.Frame()
		}
	}
startFlow:
	backupWalletFlow(ctx, ops, th)
	return &MainMenuScreen{}
}
