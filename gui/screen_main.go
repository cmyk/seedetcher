package gui

import (
	"image"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/layout"
	"seedetcher.com/gui/op"
	"seedetcher.com/logutil"
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
		layoutNavigation(ctx, inp, ops, &singleTheme, dims, []NavButton{
			{Button: Button3, Style: StylePrimary, Icon: assets.IconHammer},
		}...)
		ctx.Frame()
	}
}

// BackupFlowScreen wraps the legacy backup flow inside the Screen loop.
type BackupFlowScreen struct {
	Theme         *Colors
	stage         backupStage
	desc          *urtypes.OutputDescriptor
	totalSeeds    int
	currentSeed   int
	printMnemonic bip39.Mnemonic
	confirmKeyIdx int
	printDesc     *urtypes.OutputDescriptor
	removeWarn    *ConfirmWarningScreen
}

type backupStage int

const (
	stageDescriptor backupStage = iota
	stageSeeds
	stageConfirm
	stagePrint
)

func (s *BackupFlowScreen) Update(ctx *Context, ops op.Ctx) Screen {
	th := s.Theme
	if th == nil {
		th = &descriptorTheme
	}
	// Require SD card removal before proceeding, matching the legacy main flow.
	if !ctx.EmptySDSlot {
		if s.removeWarn == nil {
			s.removeWarn = &ConfirmWarningScreen{
				Title: "Remove SD card",
				Body:  "Remove SD card to continue.\n\nHold button to ignore this warning.",
				Icon:  assets.IconRight,
			}
		}
		dims := ctx.Platform.DisplaySize()
		switch s.removeWarn.Layout(ctx, ops, th, dims) {
		case ConfirmYes:
			ctx.EmptySDSlot = true
			s.removeWarn = nil
		case ConfirmNo:
			s.removeWarn = nil
			return &MainMenuScreen{}
		case ConfirmNone:
			// keep showing
		}
		ctx.Frame()
		return s
	}
	switch s.stage {
	case stageDescriptor:
		return &DescriptorInputScreen{
			Theme: th,
			OnDone: func(desc *urtypes.OutputDescriptor, ok bool) Screen {
				if !ok {
					return &MainMenuScreen{}
				}
				s.desc = desc
				ctx.Keystores = make(map[uint32]bip39.Mnemonic)
				s.printDesc = desc
				if desc == nil {
					s.totalSeeds = 1
				} else {
					s.totalSeeds = len(desc.Keys)
				}
				s.currentSeed = 1
				s.printMnemonic = nil
				s.stage = stageSeeds
				return s
			},
		}
	case stageSeeds:
		return &SeedInputScreen{
			Theme:      th,
			Index:      s.currentSeed,
			Total:      s.totalSeeds,
			Descriptor: s.desc,
			AllowRestart: func() Screen {
				if maybeRestart(ctx, ops, th, func() {
					ctx.LastDescriptor = nil
					ctx.Keystores = make(map[uint32]bip39.Mnemonic)
					s.desc = nil
					s.stage = stageDescriptor
				}) {
					return s
				}
				return s
			},
			OnDone: func(mnemonic bip39.Mnemonic, mfp uint32, ok bool) Screen {
				if !ok {
					return s
				}
				ctx.Keystores[mfp] = mnemonic
				if s.desc == nil {
					s.printMnemonic = mnemonic
					s.confirmKeyIdx = 0
					s.stage = stagePrint
					return s
				}
				if len(ctx.Keystores) >= s.totalSeeds {
					s.stage = stageConfirm
				} else {
					s.currentSeed++
				}
				return s
			},
		}
	case stageConfirm:
		if s.desc == nil {
			// No descriptor present; skip confirm and go to print with the entered seed.
			s.stage = stagePrint
			return s.Update(ctx, ops)
		}
		return &WalletConfirmScreen{
			Theme:      th,
			Descriptor: *s.desc,
			Mnemonic:   ctx.Keystores[s.desc.Keys[0].MasterFingerprint],
			AllowRestart: func() Screen {
				if maybeRestart(ctx, ops, th, func() {
					ctx.LastDescriptor = nil
					ctx.Keystores = make(map[uint32]bip39.Mnemonic)
					s.desc = nil
					s.stage = stageDescriptor
				}) {
					return s
				}
				return s
			},
			OnDone: func(keyIdx int, ok bool) Screen {
				if !ok {
					return s
				}
				s.confirmKeyIdx = keyIdx
				s.stage = stagePrint
				return s
			},
		}
	case stagePrint:
		var job PrintJob
		if s.desc == nil {
			job = FromSinglesig(s.printMnemonic)
		} else {
			desc := s.desc
			if desc == nil {
				desc = s.printDesc
			}
			job = FromDescriptor(desc, ctx.Keystores[desc.Keys[0].MasterFingerprint], s.confirmKeyIdx)
		}
		return &PrintFlowScreen{
			Theme: th,
			Job:   job,
			OnSuccess: func() Screen {
				return &MainMenuScreen{}
			},
			OnRetry: func() Screen {
				s.stage = stageConfirm
				return s
			},
		}
	default:
		logutil.DebugLog("BackupFlowScreen: unknown stage, resetting")
		return &MainMenuScreen{}
	}
}
