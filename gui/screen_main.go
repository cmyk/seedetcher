package gui

import (
	"image"

	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/layout"
	"seedetcher.com/gui/op"
	"seedetcher.com/gui/widget"
)

type program int

const (
	backupWallet program = iota
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

func mainFlow(ctx *Context, ops op.Ctx) {
	var page program
	inp := new(InputTracker)
	for {
		dims := ctx.Platform.DisplaySize()
	events:
		for {
			e, ok := inp.Next(ctx, Button3, Center, Left, Right)
			if !ok {
				break
			}
			switch e.Button {
			case Button3, Center:
				if !inp.Clicked(e.Button) {
					break
				}
				ws := &ConfirmWarningScreen{
					Title: "Remove SD card",
					Body:  "Remove SD card to continue.\n\nHold button to ignore this warning.",
					Icon:  assets.IconRight,
				}
				th := mainScreenTheme(page)
			loop:
				for !ctx.EmptySDSlot {
					res := ws.Layout(ctx, ops.Begin(), th, dims)
					dialog := ops.End()
					switch res {
					case ConfirmYes:
						break loop
					case ConfirmNo:
						continue events
					}
					drawMainScreen(ctx, ops, dims, page)
					dialog.Add(ops)
					ctx.Frame()
				}
				ctx.EmptySDSlot = true
				switch page {
				case backupWallet:
					backupWalletFlow(ctx, ops, th)
				}
			case Left:
				if !e.Pressed {
					break
				}
				page--
				if page < 0 {
					page = backupWallet
				}
			case Right:
				if !e.Pressed {
					break
				}
				page++
				if page > backupWallet {
					page = 0
				}
			}
		}
		drawMainScreen(ctx, ops, dims, page)
		layoutNavigation(inp, ops, mainScreenTheme(page), dims, []NavButton{
			{Button: Button3, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		ctx.Frame()
	}
}

func mainScreenTheme(page program) *Colors {
	switch page {
	case backupWallet:
		return &descriptorTheme
	default:
		panic("invalid page")
	}
}

func drawMainScreen(ctx *Context, ops op.Ctx, dims image.Point, page program) {
	var th *Colors
	var title string
	th = mainScreenTheme(page)
	switch page {
	case backupWallet:
		title = "SeedEtcher Backup"
	}
	op.ColorOp(ops, th.Background)

	layoutTitle(ctx, ops, dims.X, th.Text, "%s", title)

	r := layout.Rectangle{Max: dims}
	sz := layoutMainPage(ops.Begin(), th, dims.X, page)
	op.Position(ops, ops.End(), r.Center(sz))

	sz = layoutMainPager(ops.Begin(), th, page)
	_, footer := r.CutBottom(leadingSize)
	op.Position(ops, ops.End(), footer.Center(sz))

	versz := widget.Labelwf(ops.Begin(), ctx.Styles.debug, 100, th.Text, "%s", ctx.Version)
	op.Position(ops, ops.End(), r.SE(versz.Add(image.Pt(4, 0))))
	shsz := widget.Labelwf(ops.Begin(), ctx.Styles.debug, 100, th.Text, "SeedEtcher")
	op.Position(ops, ops.End(), r.SW(shsz).Add(image.Pt(3, 0)))
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
