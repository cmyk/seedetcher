package gui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/chaincfg"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/op"
	"seedetcher.com/logutil"
	"seedetcher.com/printer"
	"seedetcher.com/seedqr"
)

// backupWalletFlow drives the main backup/print flow.
func backupWalletFlow(ctx *Context, ops op.Ctx, th *Colors) {
	logutil.DebugLog("backupWalletFlow: Starting")

descLoop:
	for {
		desc, ok := inputDescriptorFlow(ctx, ops, th)
		logutil.DebugLog("backupWalletFlow: After inputDescriptorFlow, desc=%v, ok=%v", desc != nil, ok)
		if !ok {
			logutil.DebugLog("backupWalletFlow: inputDescriptorFlow failed, exiting")
			return
		}
		if desc == nil {
			// Singlesig path
			for {
				mnemonic, ok := newMnemonicFlow(ctx, ops, th, 1, 1) // Singlesig: 1/1
				if !ok {
					logutil.DebugLog("backupWalletFlow: newMnemonicFlow failed")
					continue descLoop
				}
				logutil.DebugLog("backupWalletFlow: Seed flow done")
				if !new(SeedScreen).Confirm(ctx, ops, th, mnemonic) {
					logutil.DebugLog("backupWalletFlow: SeedScreen.Confirm failed")
					continue descLoop
				}
				logutil.DebugLog("backupWalletFlow: Seed confirmed")
				mfp, err := masterFingerprintFor(mnemonic, &chaincfg.MainNetParams)
				if err != nil {
					logutil.DebugLog("backupWalletFlow: Fingerprint error: %v", err)
					showError(ctx, ops, th, fmt.Errorf("Failed to compute fingerprint: %v", err))
					continue descLoop
				}
				ctx.Keystores[mfp] = mnemonic
				logutil.DebugLog("backupWalletFlow: Keystore updated, printing singlesig")
				printScreen := &PrintSeedScreen{}
				if printScreen.Print(ctx, ops, th, mnemonic, nil, 0, printer.PaperA4) {
					logutil.DebugLog("backupWalletFlow: Print succeeded")
					return
				}
				logutil.DebugLog("backupWalletFlow: Print failed")
				continue descLoop
			}
		}

		logutil.DebugLog("backupWalletFlow: Descriptor present with %d keys", len(desc.Keys))
		totalSeeds := len(desc.Keys)
		for i := 1; i <= totalSeeds; i++ {
		seedLoop:
			for {
				mnemonic, ok := newMnemonicFlow(ctx, ops, th, i, totalSeeds)
				if !ok {
					logutil.DebugLog("backupWalletFlow: newMnemonicFlow failed, retrying seed %d", i)
					confirm := &ConfirmWarningScreen{
						Title: "Restart Process?",
						Body:  "Do you want to restart and clear all scanned data?\n\nHold button to confirm.",
						Icon:  assets.IconDiscard,
					}
					if confirmWarning(ctx, ops, th, confirm) {
						logutil.DebugLog("backupWalletFlow: User confirmed restart, clearing data")
						ctx.LastDescriptor = nil
						ctx.Keystores = make(map[uint32]bip39.Mnemonic)
						continue descLoop
					}
					logutil.DebugLog("backupWalletFlow: User declined restart, continuing seed input")
					continue seedLoop
				}
				logutil.DebugLog("backupWalletFlow: Seed flow done for seed %d", i)
				if !new(SeedScreen).Confirm(ctx, ops, th, mnemonic) {
					logutil.DebugLog("backupWalletFlow: SeedScreen.Confirm failed for seed %d", i)
					confirm := &ConfirmWarningScreen{
						Title: "Restart Process?",
						Body:  "Do you want to restart and clear all scanned data?\n\nHold button to confirm.",
						Icon:  assets.IconDiscard,
					}
					if confirmWarning(ctx, ops, th, confirm) {
						logutil.DebugLog("backupWalletFlow: User confirmed restart, clearing data")
						ctx.LastDescriptor = nil
						ctx.Keystores = make(map[uint32]bip39.Mnemonic)
						continue descLoop
					}
					logutil.DebugLog("backupWalletFlow: User declined restart, continuing seed input")
					continue seedLoop
				}
				logutil.DebugLog("backupWalletFlow: Seed confirmed for seed %d", i)
				mfp, err := masterFingerprintFor(mnemonic, &chaincfg.MainNetParams)
				if err != nil {
					logutil.DebugLog("backupWalletFlow: Fingerprint error: %v", err)
					showError(ctx, ops, th, fmt.Errorf("Failed to compute fingerprint: %v", err))
					continue seedLoop
				}
				if _, exists := ctx.Keystores[mfp]; exists {
					logutil.DebugLog("backupWalletFlow: Duplicate seed %.8x detected", mfp)
					showError(ctx, ops, th, fmt.Errorf("Seed was entered already"))
					continue seedLoop
				}
				_, matched := descriptorKeyIdx(*desc, mnemonic, "")
				if !matched {
					logutil.DebugLog("backupWalletFlow: Seed fingerprint %.8x doesn’t match descriptor", mfp)
					showError(ctx, ops, th, fmt.Errorf("Seed doesn’t match wallet descriptor"))
					continue seedLoop
				}
				ctx.Keystores[mfp] = mnemonic
				logutil.DebugLog("backupWalletFlow: Keystore updated, seeds scanned: %d/%d", len(ctx.Keystores), len(desc.Keys))
				break seedLoop
			}
			if len(ctx.Keystores) >= len(desc.Keys) {
				break
			}
		}

	confirmLoop:
		for {
			ds := &DescriptorScreen{Descriptor: *desc, Mnemonic: ctx.Keystores[desc.Keys[0].MasterFingerprint]}
			confirmKeyIdx, ok := ds.Confirm(ctx, ops, th)
			logutil.DebugLog("backupWalletFlow: Confirm returned keyIdx=%d, ok=%v", confirmKeyIdx, ok)
			if !ok {
				logutil.DebugLog("backupWalletFlow: Descriptor not confirmed, prompting restart")
				confirm := &ConfirmWarningScreen{
					Title: "Restart Process?",
					Body:  "Do you want to restart and clear all scanned data?\n\nHold button to confirm.",
					Icon:  assets.IconDiscard,
				}
				if confirmWarning(ctx, ops, th, confirm) {
					logutil.DebugLog("backupWalletFlow: User confirmed restart, clearing data")
					ctx.LastDescriptor = nil
					ctx.Keystores = make(map[uint32]bip39.Mnemonic)
					continue descLoop
				}
				logutil.DebugLog("backupWalletFlow: User declined restart, returning to confirm")
				continue confirmLoop
			}
			logutil.DebugLog("backupWalletFlow: All %d seeds collected, printing with keyIdx=%d", len(desc.Keys), confirmKeyIdx)
			printScreen := &PrintSeedScreen{}
			if printScreen.Print(ctx, ops, th, ds.Mnemonic, desc, confirmKeyIdx, printer.PaperA4) {
				logutil.DebugLog("backupWalletFlow: Print succeeded")
				return
			}
			logutil.DebugLog("backupWalletFlow: Print failed")
			continue confirmLoop // Back to Confirm Wallet, not descLoop
		}
	}
}

func inputDescriptorFlow(ctx *Context, ops op.Ctx, th *Colors) (*urtypes.OutputDescriptor, bool) {
	originalDesc := ctx.LastDescriptor // Save original
	cs := &ChoiceScreen{
		Title:   "Descriptor",
		Lead:    "Choose input method",
		Choices: []string{"SCAN", "SKIP"},
	}
	if ctx.LastDescriptor != nil {
		cs.Choices = append(cs.Choices, "RE-USE")
	}
	for {
		choice, ok := cs.Choose(ctx, ops, th)
		if !ok {
			logutil.DebugLog("inputDescriptorFlow: Choose returned false")
			ctx.LastDescriptor = originalDesc // Restore on back
			return nil, false
		}
		switch choice {
		case 0: // Scan
			res, ok := (&ScanScreen{
				Title: "Scan",
				Lead:  "Descriptor",
			}).Scan(ctx, ops)
			if !ok {
				logutil.DebugLog("inputDescriptorFlow: Scan returned false")
				ctx.LastDescriptor = originalDesc // Restore on back
				return nil, false
			}
			desc, ok := res.(urtypes.OutputDescriptor)
			if !ok {
				logutil.DebugLog("inputDescriptorFlow: Scan result not descriptor")
				showError(ctx, ops, th, fmt.Errorf("Failed to parse descriptor"))
				continue
			}
			desc.Title = sanitizeTitle(desc.Title)
			logutil.DebugLog("inputDescriptorFlow: Returning desc with %d keys", len(desc.Keys))
			ctx.LastDescriptor = &desc
			return &desc, true
		case 1: // Skip
			logutil.DebugLog("inputDescriptorFlow: Skipping descriptor")
			ctx.LastDescriptor = originalDesc
			return nil, true
		case 2: // Re-use
			logutil.DebugLog("inputDescriptorFlow: Re-using last descriptor")
			return ctx.LastDescriptor, true
		}
	}
}

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
				if !mn.Valid() {
					logutil.DebugLog("newMnemonicFlow: Invalid mnemonic")
					showErr(&ErrorScreen{
						Title: "Invalid Seed",
						Body:  "The seed phrase is invalid.",
					})
					continue
				}
				logutil.DebugLog("newMnemonicFlow: Returning seed")
				return mn, true
			}
		}
	}
}
