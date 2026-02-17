package testutils

import (
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
)

func TestSeed12FixtureParsesAs12WordSeedOnly(t *testing.T) {
	cfg, ok := WalletConfigs["seed-12"]
	if !ok {
		t.Fatal("missing seed-12 fixture")
	}
	mnemonics, desc, err := ParseWallet(cfg, "", "")
	if err != nil {
		t.Fatalf("parse wallet: %v", err)
	}
	if desc != nil {
		t.Fatal("expected no descriptor for seed-12 fixture")
	}
	if got, want := len(mnemonics), 1; got != want {
		t.Fatalf("mnemonic count = %d, want %d", got, want)
	}
	if got, want := len(mnemonics[0]), 12; got != want {
		t.Fatalf("mnemonic length = %d, want %d", got, want)
	}
}

func TestMultisigMainnet2of3FixtureParsesAsMainnetXpub(t *testing.T) {
	cfg, ok := WalletConfigs["multisig-mainnet-2of3"]
	if !ok {
		t.Fatal("missing multisig-mainnet-2of3 fixture")
	}
	_, desc, err := ParseWallet(cfg, "", "")
	if err != nil {
		t.Fatalf("parse wallet: %v", err)
	}
	if desc == nil {
		t.Fatal("expected descriptor")
	}
	if got, want := desc.Threshold, 2; got != want {
		t.Fatalf("threshold = %d, want %d", got, want)
	}
	if got, want := len(desc.Keys), 3; got != want {
		t.Fatalf("len(keys) = %d, want %d", got, want)
	}
	for i, k := range desc.Keys {
		if k.Network.Name != chaincfg.MainNetParams.Name {
			t.Fatalf("key %d network = %s, want %s", i, k.Network.Name, chaincfg.MainNetParams.Name)
		}
		if got := k.String(); !strings.HasPrefix(got, "xpub") {
			t.Fatalf("key %d is not xpub: %s", i, got)
		}
		dp := k.DerivationPath
		if len(dp) != 4 {
			t.Fatalf("key %d derivation path length = %d, want 4", i, len(dp))
		}
		if dp[0] != hdkeychain.HardenedKeyStart+48 ||
			dp[1] != hdkeychain.HardenedKeyStart+0 ||
			dp[2] != hdkeychain.HardenedKeyStart+0 ||
			dp[3] != hdkeychain.HardenedKeyStart+2 {
			t.Fatalf("key %d derivation path = %v, want m/48h/0h/0h/2h", i, dp)
		}
	}
}
