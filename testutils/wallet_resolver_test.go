package testutils

import "testing"

func TestResolveWalletSelection_FixtureMode_Default(t *testing.T) {
	f := &Flags{}
	cfg, mnemOverride, err := ResolveWalletSelection(f)
	if err != nil {
		t.Fatalf("ResolveWalletSelection: %v", err)
	}
	if cfg.Name != "multisig" {
		t.Fatalf("fixture name=%s want multisig", cfg.Name)
	}
	if mnemOverride != "" {
		t.Fatalf("mnemonic override=%q want empty", mnemOverride)
	}
}

func TestResolveWalletSelection_FixtureMode_Explicit(t *testing.T) {
	f := &Flags{Fixture: "singlesig-nested-p2sh-p2wpkh"}
	cfg, _, err := ResolveWalletSelection(f)
	if err != nil {
		t.Fatalf("ResolveWalletSelection: %v", err)
	}
	if cfg.Name != "singlesig-nested-p2sh-p2wpkh" {
		t.Fatalf("fixture name=%s want singlesig-nested-p2sh-p2wpkh", cfg.Name)
	}
}

func TestResolveWalletSelection_ParametricMultisig_DefaultNOfM(t *testing.T) {
	f := &Flags{WalletKind: "multisig"}
	cfg, _, err := ResolveWalletSelection(f)
	if err != nil {
		t.Fatalf("ResolveWalletSelection: %v", err)
	}
	if cfg.Name != "multisig" {
		t.Fatalf("fixture name=%s want multisig", cfg.Name)
	}
}

func TestResolveWalletSelection_ParametricMultisig_3of5(t *testing.T) {
	f := &Flags{WalletKind: "multisig", NOfM: "3of5"}
	cfg, _, err := ResolveWalletSelection(f)
	if err != nil {
		t.Fatalf("ResolveWalletSelection: %v", err)
	}
	if cfg.Name != "multisig-3of5" {
		t.Fatalf("fixture name=%s want multisig-3of5", cfg.Name)
	}
}

func TestResolveWalletSelection_ParametricSinglesig(t *testing.T) {
	f := &Flags{WalletKind: "singlesig"}
	cfg, _, err := ResolveWalletSelection(f)
	if err != nil {
		t.Fatalf("ResolveWalletSelection: %v", err)
	}
	if cfg.Name != "singlesig" {
		t.Fatalf("fixture name=%s want singlesig", cfg.Name)
	}
}

func TestResolveWalletSelection_ConflictModes(t *testing.T) {
	f := &Flags{Fixture: "multisig-3of5", WalletKind: "multisig", NOfM: "3of5"}
	if _, _, err := ResolveWalletSelection(f); err == nil {
		t.Fatal("expected conflict error, got nil")
	}
}

func TestResolveWalletSelection_WordProfileLongwords(t *testing.T) {
	f := &Flags{WalletKind: "multisig", NOfM: "3of5", WordProfile: "longwords"}
	_, mnemOverride, err := ResolveWalletSelection(f)
	if err != nil {
		t.Fatalf("ResolveWalletSelection: %v", err)
	}
	if mnemOverride == "" {
		t.Fatal("expected longwords mnemonic override, got empty")
	}
}
