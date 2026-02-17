package testutils

import (
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
)

func TestSeedOnlyFixturesParseExpectedWordCounts(t *testing.T) {
	tests := []struct {
		name     string
		words    int
		fixture  string
	}{
		{name: "seed-12", words: 12, fixture: "seed-12"},
		{name: "seed-15", words: 15, fixture: "seed-15"},
		{name: "seed-18", words: 18, fixture: "seed-18"},
		{name: "seed-21", words: 21, fixture: "seed-21"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg, ok := WalletConfigs[tc.fixture]
			if !ok {
				t.Fatalf("missing %s fixture", tc.fixture)
			}
			mnemonics, desc, err := ParseWallet(cfg, "", "")
			if err != nil {
				t.Fatalf("parse wallet: %v", err)
			}
			if desc != nil {
				t.Fatalf("expected no descriptor for %s fixture", tc.fixture)
			}
			if got, want := len(mnemonics), 1; got != want {
				t.Fatalf("mnemonic count = %d, want %d", got, want)
			}
			if got, want := len(mnemonics[0]), tc.words; got != want {
				t.Fatalf("mnemonic length = %d, want %d", got, want)
			}
		})
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
