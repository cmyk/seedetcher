package gui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/chaincfg"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/gui/op"
	"seedetcher.com/logutil"
	"seedetcher.com/seedqr"
)

func newMnemonicFlow(ctx *Context, ops op.Ctx, th *Colors, current, total int) (bip39.Mnemonic, bool) {
	cs := &ChoiceScreen{
		Title:   fmt.Sprintf("Input Seed %d/%d", current, total), // Display "Seed X/Y"
		Lead:    "Choose input method",
		Choices: []string{"CAMERA", "KEYBOARD"},
	}
	showErr := func(errScreen *ErrorScreen) {
		for {
			dims := ctx.Platform.DisplaySize()
			dismissed := errScreen.Layout(ctx, ops.Begin(), th, dims)
			d := ops.End()
			if dismissed {
				break
			}
			cs.Draw(ctx, ops, th, dims)
			d.Add(ops)
			ctx.Frame()
		}
	}
outer:
	for {
		choice, ok := cs.Choose(ctx, ops, th)
		if !ok {
			return nil, false
		}
		switch choice {
		case 0: // Camera.
			res, ok := (&ScanScreen{
				Title: fmt.Sprintf("Scan Seed %d/%d", current, total), // Update ScanScreen title
				Lead:  "SeedQR or Mnemonic",
			}).Scan(ctx, ops)
			if !ok {
				continue
			}
			if b, ok := res.([]byte); ok {
				if sqr, ok := seedqr.Parse(b); ok {
					res = sqr
				} else if sqr, err := bip39.ParseMnemonic(strings.ToLower(string(b))); err == nil || errors.Is(err, bip39.ErrInvalidChecksum) {
					res = sqr
				}
			}
			seed, ok := res.(bip39.Mnemonic)
			if !ok {
				showErr(&ErrorScreen{
					Title: "Invalid Seed",
					Body:  "The scanned data does not represent a seed.",
				})
				continue
			}
			return seed, true

		case 1: // Keyboard.
			cs := &ChoiceScreen{
				Title:   fmt.Sprintf("Input Seed %d/%d", current, total), // Update keyboard choice title
				Lead:    "Choose number of words",
				Choices: []string{"12 WORDS", "24 WORDS"},
			}
			for {
				words, ok := cs.Choose(ctx, ops, th)
				if !ok {
					continue outer
				}
				mn := emptyMnemonic([]int{12, 24}[words])
				inputWordsFlow(ctx, ops, th, mn, 0)
				if isEmptyMnemonic(mn) {
					continue outer
				}
				logutil.DebugLog("newMnemonicFlow: inputWordsFlow returned")
				logutil.DebugLog("newMnemonicFlow: Returning seed")
				return mn, true
			}
		}
	}
}

// SeedInputScreen wraps seed capture/validation for one seed in the flow.
type SeedInputScreen struct {
	Theme        *Colors
	Index        int
	Total        int
	Descriptor   *urtypes.OutputDescriptor
	OnDone       func(mnemonic bip39.Mnemonic, mfp uint32, ok bool) Screen
	AllowRestart func() Screen
	Draft        bip39.Mnemonic
}

func (s *SeedInputScreen) Update(ctx *Context, ops op.Ctx) Screen {
	th := s.Theme
	if th == nil {
		th = &descriptorTheme
	}
	var (
		mnemonic bip39.Mnemonic
		ok       bool
	)
	if s.Draft != nil {
		inputWordsFlow(ctx, ops, th, s.Draft, firstEmptyWord(s.Draft))
		mnemonic = s.Draft
		ok = true
	} else {
		mnemonic, ok = newMnemonicFlow(ctx, ops, th, s.Index, s.Total)
		if !ok {
			if s.AllowRestart != nil {
				return s.AllowRestart()
			}
			return &MainMenuScreen{}
		}
	}
	if !new(SeedScreen).Confirm(ctx, ops, th, mnemonic) {
		// Back out of confirm: honor restart handler if provided.
		if s.AllowRestart != nil {
			return s.AllowRestart()
		}
		return &MainMenuScreen{}
	}
	network := &chaincfg.MainNetParams
	if s.Descriptor != nil && len(s.Descriptor.Keys) > 0 {
		network = s.Descriptor.Keys[0].Network
	}
	mfp, err := validateSeedAgainstDescriptor(s.Descriptor, mnemonic, ctx.Keystores, network)
	if err != nil {
		switch err {
		case errSeedDuplicate:
			showError(ctx, ops, th, fmt.Errorf("Seed was entered already"))
		case errSeedMismatch:
			showError(ctx, ops, th, fmt.Errorf("Seed doesn’t match wallet descriptor"))
		default:
			showError(ctx, ops, th, fmt.Errorf("Failed to compute fingerprint: %v", err))
		}
		s.Draft = mnemonic
		return s
	}
	// Success path: clear draft.
	s.Draft = nil
	if s.OnDone != nil {
		return s.OnDone(mnemonic, mfp, true)
	}
	return &MainMenuScreen{}
}

func firstEmptyWord(m bip39.Mnemonic) int {
	for i, w := range m {
		if w == -1 {
			return i
		}
	}
	return 0
}
