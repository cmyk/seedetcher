package printer

import (
	"testing"

	"seedetcher.com/testutils"
)

func TestSelectLayout_ClassicDefault(t *testing.T) {
	cfg := testutils.WalletConfigs["multisig-3of5"]
	_, desc, err := testutils.ParseWallet(cfg, "", "")
	if err != nil {
		t.Fatalf("parse wallet: %v", err)
	}
	sel := SelectLayout(RenderContext{
		MnemonicCount:   len(cfg.Mnemonics),
		Desc:            desc,
		SinglesigLayout: SinglesigLayoutSeedWithInfo,
		Compact2of3:     false,
	})
	if sel.Name != "classic" || sel.UseCompact2of3 {
		t.Fatalf("unexpected selection: %+v", sel)
	}
}

func TestSelectLayout_Compact2of3(t *testing.T) {
	cfg := testutils.WalletConfigs["multisig"]
	_, desc, err := testutils.ParseWallet(cfg, "", "")
	if err != nil {
		t.Fatalf("parse wallet: %v", err)
	}
	sel := SelectLayout(RenderContext{
		MnemonicCount:   len(cfg.Mnemonics),
		Desc:            desc,
		SinglesigLayout: SinglesigLayoutSeedWithInfo,
		Compact2of3:     true,
	})
	if sel.Name != "compact-2of3" || !sel.UseCompact2of3 {
		t.Fatalf("unexpected selection: %+v", sel)
	}
}

func TestSelectLayout_SinglesigSeedDesc_NoInfo(t *testing.T) {
	cfg := testutils.WalletConfigs["singlesig"]
	_, desc, err := testutils.ParseWallet(cfg, "", "")
	if err != nil {
		t.Fatalf("parse wallet: %v", err)
	}
	sel := SelectLayout(RenderContext{
		MnemonicCount:   len(cfg.Mnemonics),
		Desc:            desc,
		SinglesigLayout: SinglesigLayoutSeedWithDescriptorQR,
		Compact2of3:     false,
	})
	if !sel.IncludeSinglesigDescriptorQR || sel.IncludeSinglesigInfo {
		t.Fatalf("unexpected singlesig flags: %+v", sel)
	}
}
