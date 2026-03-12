package testutils

import (
	"fmt"
	"strings"
)

const (
	longwordsMnemonic = "abstract accident acoustic announce artefact attitude bachelor broccoli business category champion cinnamon congress consider convince cupboard daughter december decorate decrease describe dinosaur disagree begin"
)

var parametricMultisigFixtureByNOfM = map[string]string{
	"2of2":  "multisig-2of2",
	"2of3":  "multisig",
	"2of4":  "multisig-2of4",
	"3of4":  "multisig-3of4",
	"3of5":  "multisig-3of5",
	"4of7":  "multisig-4of7",
	"5of7":  "multisig-5of7",
	"7of10": "multisig-7of10",
}

// ResolveWalletSelection determines the wallet fixture and mnemonic override to use.
// It supports either fixture mode (-fixture / -w) or parametric mode
// (-wallet-type with optional -n-of-m).
func ResolveWalletSelection(f *Flags) (WalletConfig, string, error) {
	fixtureName := strings.TrimSpace(f.Fixture)
	legacyW := strings.TrimSpace(f.WalletType)
	walletKind := strings.ToLower(strings.TrimSpace(f.WalletKind))
	nOfM := strings.ToLower(strings.TrimSpace(f.NOfM))
	wordProfile := strings.ToLower(strings.TrimSpace(f.WordProfile))

	if fixtureName != "" && legacyW != "" && legacyW != "multisig" && fixtureName != legacyW {
		return WalletConfig{}, "", fmt.Errorf("conflicting -fixture (%s) and -w (%s)", fixtureName, legacyW)
	}
	if fixtureName == "" && legacyW != "" && legacyW != "multisig" {
		fixtureName = legacyW
	}

	paramMode := walletKind != "" || nOfM != ""
	if paramMode && fixtureName != "" {
		return WalletConfig{}, "", fmt.Errorf("cannot combine fixture mode (-fixture/-w) with parametric mode (-wallet-type/-n-of-m)")
	}

	if wordProfile != "" && wordProfile != "normal" && wordProfile != "longwords" {
		return WalletConfig{}, "", fmt.Errorf("invalid -word-profile: %s (allowed: normal, longwords)", f.WordProfile)
	}

	if !paramMode {
		if fixtureName == "" {
			fixtureName = legacyW
			if fixtureName == "" {
				fixtureName = "multisig"
			}
		}
		cfg, ok := WalletConfigs[fixtureName]
		if !ok {
			return WalletConfig{}, "", fmt.Errorf("unknown fixture: %s", fixtureName)
		}
		override := ""
		if wordProfile == "longwords" {
			override = longwordsMnemonic
		}
		return cfg, override, nil
	}

	if walletKind == "" {
		walletKind = "multisig"
	}
	switch walletKind {
	case "singlesig":
		if nOfM != "" {
			return WalletConfig{}, "", fmt.Errorf("-n-of-m is only valid for multisig")
		}
		cfg := WalletConfigs["singlesig"]
		override := ""
		if wordProfile == "longwords" {
			override = longwordsMnemonic
		}
		return cfg, override, nil
	case "multisig":
		if nOfM == "" {
			nOfM = "2of3"
		}
		fixture, ok := parametricMultisigFixtureByNOfM[nOfM]
		if !ok {
			return WalletConfig{}, "", fmt.Errorf("unsupported -n-of-m for parametric multisig: %s", nOfM)
		}
		cfg := WalletConfigs[fixture]
		override := ""
		if wordProfile == "longwords" {
			override = longwordsMnemonic
		}
		return cfg, override, nil
	default:
		return WalletConfig{}, "", fmt.Errorf("invalid -wallet-type: %s (allowed: singlesig, multisig)", f.WalletKind)
	}
}
