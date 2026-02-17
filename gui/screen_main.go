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
				return &SDCardGateScreen{
					Theme: &singleTheme,
					Next:  &ActionChoiceScreen{Theme: &singleTheme},
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
	label         string
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
					s.confirmKeyIdx = 0
					s.stage = stageLabel
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
		if s.desc == nil {
			s.stage = stageLabel
			return s
		}
		return &FingerprintsScreen{
			Theme:      th,
			Descriptor: s.desc,
			OnBack: func() Screen {
				s.stage = stageConfirm
				return s
			},
			OnContinue: func() Screen {
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
				if s.desc != nil {
					s.stage = stageShardInfo
				} else {
					s.stage = stageSeeds
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
				s.stage = stageConfirm
				return s
			},
		}
	default:
		logutil.DebugLog("BackupFlowScreen: unknown stage, resetting")
		return &MainMenuScreen{}
	}
}
