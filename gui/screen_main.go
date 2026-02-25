package gui

import (
	"fmt"
	"image"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/descriptor/shard"
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/op"
	"seedetcher.com/gui/widget"
	"seedetcher.com/logutil"
	"seedetcher.com/printer"
	"seedetcher.com/version"
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
				if ctx != nil {
					ctx.SDRemovalPrepared = false
				}
				return &HBPStartupGateScreen{
					Theme: &singleTheme,
					Next: &SDCardGateScreen{
						Theme: &singleTheme,
						Next:  &ActionChoiceScreen{Theme: &singleTheme},
					},
				}
			}
		}
		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, singleTheme.Background)
		// Logo: centered horizontally and vertically.
		logoSize := assets.SeedetcherLogo.Bounds().Size()
		logoPos := image.Pt((dims.X-logoSize.X)/2, (dims.Y-logoSize.Y)/2)
		icon := ops.Begin()
		op.ImageOp(icon, assets.SeedetcherLogo, false)
		op.Position(ops, ops.End(), logoPos)
		// Version badge bottom-left.
		vlabel := fmt.Sprintf("SeedEtcher %s", version.String())
		sz := widget.Labelf(ops.Begin(), ctx.Styles.debug, singleTheme.Text, "%s", vlabel)
		op.Position(ops, ops.End(), image.Pt(6, dims.Y-sz.Y-6))

		layoutNavigation(ctx, inp, ops, &singleTheme, dims, []NavButton{
			{Button: Button3, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		ctx.Frame()
	}
}

// HBPStartupGateScreen runs before SD removal and allows optional Brother runtime prep.
type HBPStartupGateScreen struct {
	Theme *Colors
	Next  Screen
}

func (s *HBPStartupGateScreen) Update(ctx *Context, ops op.Ctx) Screen {
	th := s.Theme
	if th == nil {
		th = &singleTheme
	}
	if ctx != nil && ctx.HBPRuntimeReady {
		if s.Next != nil {
			return s.Next
		}
		return &MainMenuScreen{}
	}

	choice, ok := (&ChoiceScreen{
		Title:    "Brother HBP",
		Lead:     "Prefer PCL/PS: faster and lower RAM.\nUse HBP (600dpi cap) only if printer lacks PCL/PS.",
		Choices:  []string{"PCL/PS only", "Enable HBP"},
		LeadLeft: true,
		choice:   0,
	}).Choose(ctx, ops, th)
	if !ok {
		return &MainMenuScreen{}
	}
	if choice == 0 {
		if ctx != nil {
			ctx.HBPRuntimeReady = false
			if err := ctx.Platform.PrepareSDForRemoval(); err != nil {
				showError(ctx, ops, th, fmt.Errorf("SD removal prep failed: %v", err))
				return &MainMenuScreen{}
			}
			ctx.SDRemovalPrepared = true
		}
		if s.Next != nil {
			return s.Next
		}
		return &MainMenuScreen{}
	}

	prep := &HBPRuntimePrepareScreen{}
	if err := prep.Show(ctx, ops, th); err != nil {
		showError(ctx, ops, th, err)
		return &MainMenuScreen{}
	}
	if ctx != nil {
		ctx.HBPRuntimeReady = true
		ctx.SDRemovalPrepared = false
	}
	if s.Next != nil {
		return s.Next
	}
	return &MainMenuScreen{}
}

// BackupFlowScreen drives backup and print stages in the Screen loop.
type BackupFlowScreen struct {
	Theme         *Colors
	stage         backupStage
	desc          *urtypes.OutputDescriptor
	totalSeeds    int
	currentSeed   int
	printMnemonic bip39.Mnemonic
	confirmKeyIdx int
	printDesc     *urtypes.OutputDescriptor
	label         string
	printSeedMFP  uint32
	shardSetID    [16]byte
	shardShares   []shard.Share
}

type backupStage int

const (
	stageDescriptor backupStage = iota
	stageSeeds
	stageConfirm
	stageFingerprints
	stageShardInfo
	stageLabel
	stagePrint
)

func (s *BackupFlowScreen) Update(ctx *Context, ops op.Ctx) Screen {
	th := s.Theme
	if th == nil {
		th = &descriptorTheme
	}
	switch s.stage {
	case stageDescriptor:
		return &DescriptorInputScreen{
			Theme: th,
			OnDone: func(desc *urtypes.OutputDescriptor, ok bool) Screen {
				if !ok {
					return &ActionChoiceScreen{Theme: &singleTheme}
				}
				s.desc = desc
				ctx.Keystores = make(map[uint32]bip39.Mnemonic)
				s.label = printer.DefaultWalletLabel
				s.printDesc = desc
				if desc == nil {
					s.totalSeeds = 1
					s.stage = stageSeeds
				} else {
					s.totalSeeds = len(desc.Keys)
					if setID, ok := deriveShardSetID(desc); ok {
						s.shardSetID = setID
					} else {
						s.shardSetID = [16]byte{}
					}
					s.shardShares = buildShardShares(desc, s.shardSetID)
					s.stage = stageSeeds
				}
				s.currentSeed = 1
				s.printMnemonic = nil
				s.printSeedMFP = 0
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
					s.label = printer.DefaultWalletLabel
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
					s.printSeedMFP = mfp
					s.confirmKeyIdx = 0
					s.stage = stageFingerprints
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
			// No descriptor present; skip confirm and go to label entry before printing.
			s.stage = stageLabel
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
					s.label = printer.DefaultWalletLabel
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
				s.stage = stageFingerprints
				return s
			},
		}
	case stageFingerprints:
		var fpDesc *urtypes.OutputDescriptor
		if s.desc != nil {
			fpDesc = s.desc
		} else if s.printSeedMFP != 0 {
			tmp := &urtypes.OutputDescriptor{
				Type:      urtypes.Singlesig,
				Threshold: 1,
				Keys: []urtypes.KeyDescriptor{
					{MasterFingerprint: s.printSeedMFP},
				},
			}
			fpDesc = tmp
		}
		if fpDesc == nil {
			s.stage = stageLabel
			return s
		}
		return &FingerprintsScreen{
			Theme:      th,
			Descriptor: fpDesc,
			OnBack: func() Screen {
				if s.desc == nil {
					if maybeRestart(ctx, ops, th, func() {
						ctx.LastDescriptor = nil
						ctx.Keystores = make(map[uint32]bip39.Mnemonic)
						s.desc = nil
						s.label = printer.DefaultWalletLabel
						s.printSeedMFP = 0
						s.stage = stageDescriptor
					}) {
						return s
					}
					s.stage = stageFingerprints
					return s
				}
				s.stage = stageConfirm
				return s
			},
			OnContinue: func() Screen {
				if s.desc == nil {
					s.stage = stageLabel
					return s
				}
				s.stage = stageShardInfo
				return s
			},
		}
	case stageShardInfo:
		if s.desc == nil {
			s.stage = stageLabel
			return s
		}
		return &ShardPreviewScreen{
			Theme:  th,
			Shares: s.shardShares,
			OnBack: func() Screen {
				s.stage = stageFingerprints
				return s
			},
			OnDone: func() Screen {
				s.stage = stageLabel
				return s
			},
		}
	case stageLabel:
		return &LabelInputScreen{
			Theme:   th,
			Default: printer.DefaultWalletLabel,
			Value:   s.label,
			OnCancel: func() Screen {
				if s.desc != nil && len(s.shardShares) > 0 {
					s.stage = stageShardInfo
				} else {
					s.stage = stageFingerprints
				}
				return s
			},
			OnDone: func(label string) Screen {
				s.label = label
				s.stage = stagePrint
				return s
			},
		}
	case stagePrint:
		label := s.label
		if label == "" {
			label = printer.DefaultWalletLabel
		}
		var job PrintJob
		if s.desc == nil {
			job = FromSinglesig(s.printMnemonic, label)
		} else {
			desc := s.desc
			if desc == nil {
				desc = s.printDesc
			}
			job = FromDescriptor(desc, ctx.Keystores[desc.Keys[0].MasterFingerprint], s.confirmKeyIdx, label)
		}
		if s.desc != nil {
			printer.SetDescriptorShardSetID(&s.shardSetID)
		}
		return &PrintFlowScreen{
			Theme: th,
			Job:   job,
			OnSuccess: func() Screen {
				printer.SetDescriptorShardSetID(nil)
				return &MainMenuScreen{}
			},
			OnRetry: func() Screen {
				printer.SetDescriptorShardSetID(nil)
				s.stage = stageLabel
				return s
			},
		}
	default:
		logutil.DebugLog("BackupFlowScreen: unknown stage, resetting")
		return &MainMenuScreen{}
	}
}
