package gui

import (
	"fmt"
	"image"

	"github.com/btcsuite/btcd/chaincfg"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/layout"
	"seedetcher.com/gui/op"
	"seedetcher.com/logutil"
	"seedetcher.com/printer"
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
	Theme         *Colors
	stage         backupStage
	desc          *urtypes.OutputDescriptor
	totalSeeds    int
	currentSeed   int
	printMnemonic bip39.Mnemonic
	confirmKeyIdx int
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
		ws := &ConfirmWarningScreen{
			Title: "Remove SD card",
			Body:  "Remove SD card to continue.\n\nHold button to ignore this warning.",
			Icon:  assets.IconRight,
		}
		for {
			dims := ctx.Platform.DisplaySize()
			switch ws.Layout(ctx, ops, th, dims) {
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
		total := s.totalSeeds
		idx := s.currentSeed
		mnemonic, ok := newMnemonicFlow(ctx, ops, th, idx, total)
		if !ok {
			confirm := &ConfirmWarningScreen{
				Title: "Restart Process?",
				Body:  "Do you want to restart and clear all scanned data?\n\nHold button to confirm.",
				Icon:  assets.IconDiscard,
			}
			if confirmWarning(ctx, ops, th, confirm) {
				ctx.LastDescriptor = nil
				ctx.Keystores = make(map[uint32]bip39.Mnemonic)
				s.desc = nil
				s.stage = stageDescriptor
				return s
			}
			return s
		}
		if !new(SeedScreen).Confirm(ctx, ops, th, mnemonic) {
			confirm := &ConfirmWarningScreen{
				Title: "Restart Process?",
				Body:  "Do you want to restart and clear all scanned data?\n\nHold button to confirm.",
				Icon:  assets.IconDiscard,
			}
			if confirmWarning(ctx, ops, th, confirm) {
				ctx.LastDescriptor = nil
				ctx.Keystores = make(map[uint32]bip39.Mnemonic)
				s.desc = nil
				s.stage = stageDescriptor
				return s
			}
			return s
		}
		mfp, err := masterFingerprintFor(mnemonic, &chaincfg.MainNetParams)
		if err != nil {
			showError(ctx, ops, th, fmt.Errorf("Failed to compute fingerprint: %v", err))
			return s
		}
		if s.desc != nil {
			if _, exists := ctx.Keystores[mfp]; exists {
				showError(ctx, ops, th, fmt.Errorf("Seed was entered already"))
				return s
			}
			if _, matched := descriptorKeyIdx(*s.desc, mnemonic, ""); !matched {
				showError(ctx, ops, th, fmt.Errorf("Seed doesn’t match wallet descriptor"))
				return s
			}
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
	case stageConfirm:
		ds := &DescriptorScreen{Descriptor: *s.desc, Mnemonic: ctx.Keystores[s.desc.Keys[0].MasterFingerprint]}
		confirmKeyIdx, ok := ds.Confirm(ctx, ops, th)
		if !ok {
			confirm := &ConfirmWarningScreen{
				Title: "Restart Process?",
				Body:  "Do you want to restart and clear all scanned data?\n\nHold button to confirm.",
				Icon:  assets.IconDiscard,
			}
			if confirmWarning(ctx, ops, th, confirm) {
				ctx.LastDescriptor = nil
				ctx.Keystores = make(map[uint32]bip39.Mnemonic)
				s.desc = nil
				s.stage = stageDescriptor
				return s
			}
			return s
		}
		s.confirmKeyIdx = confirmKeyIdx
		s.stage = stagePrint
		return s
	case stagePrint:
		var mnemonic bip39.Mnemonic
		if s.desc == nil {
			mnemonic = s.printMnemonic
		} else {
			mnemonic = ctx.Keystores[s.desc.Keys[0].MasterFingerprint]
		}
		printScreen := &PrintSeedScreen{}
		if printScreen.Print(ctx, ops, th, mnemonic, s.desc, s.confirmKeyIdx, printer.PaperA4) {
			return &MainMenuScreen{}
		}
		s.stage = stageConfirm
		return s
	default:
		logutil.DebugLog("BackupFlowScreen: unknown stage, resetting")
		return &MainMenuScreen{}
	}
}
