//go:build debug

package gui

import (
	"fmt"

	"github.com/btcsuite/btcd/chaincfg"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/gui/op"
	"seedetcher.com/printer"
	"seedetcher.com/testutils"
)

type loadTestWalletScreen struct {
	Theme *Colors
}

type testWalletFixture struct {
	Name string
	Key  string
}

var debugTestWalletFixtures = []testWalletFixture{
	{Name: "Singlesig", Key: "singlesig"},
	{Name: "Multisig 3/5", Key: "multisig-3of5"},
	{Name: "Multisig 7/10", Key: "multisig-7of10"},
}

func debugLoadTestWalletEnabled(ctx *Context) bool {
	return ctx != nil && ctx.Platform != nil && ctx.Platform.Debug()
}

func newLoadTestWalletScreen(th *Colors) Screen {
	return &loadTestWalletScreen{Theme: th}
}

func (s *loadTestWalletScreen) Update(ctx *Context, ops op.Ctx) Screen {
	th := s.Theme
	if th == nil {
		th = &singleTheme
	}
	choices := make([]string, 0, len(debugTestWalletFixtures))
	for _, f := range debugTestWalletFixtures {
		choices = append(choices, f.Name)
	}
	cs := &ChoiceScreen{
		Title:   "Load Test Wallet",
		Lead:    "Choose wallet type",
		Choices: choices,
	}
	choice, ok := cs.Choose(ctx, ops, th)
	if !ok {
		return &ActionChoiceScreen{Theme: th}
	}
	if choice < 0 || choice >= len(debugTestWalletFixtures) {
		return &ActionChoiceScreen{Theme: th}
	}
	flow, keystores, err := buildDebugLoadedBackupFlow(debugTestWalletFixtures[choice].Key, th)
	if err != nil {
		showError(ctx, ops, th, err)
		return &ActionChoiceScreen{Theme: th}
	}
	// Mirror the manual-scan state handoff before entering post-scan flow.
	ctx.Keystores = keystores
	ctx.LastDescriptor = flow.desc
	flow.printDesc = flow.desc
	return flow
}

func buildDebugLoadedBackupFlow(fixtureKey string, th *Colors) (*BackupFlowScreen, map[uint32]bip39.Mnemonic, error) {
	cfg, ok := testutils.WalletConfigs[fixtureKey]
	if !ok {
		return nil, nil, fmt.Errorf("unknown test wallet fixture: %s", fixtureKey)
	}
	mnemonics, desc, err := testutils.ParseWallet(cfg, "", "")
	if err != nil {
		return nil, nil, fmt.Errorf("load test wallet: %w", err)
	}
	if desc == nil {
		return nil, nil, fmt.Errorf("fixture %s has no descriptor", fixtureKey)
	}
	keystores, err := deriveFixtureKeystores(desc, mnemonics)
	if err != nil {
		return nil, nil, err
	}
	flow := &BackupFlowScreen{
		Theme:       th,
		stage:       stageConfirm,
		desc:        desc,
		totalSeeds:  len(desc.Keys),
		currentSeed: len(desc.Keys),
		label:       printer.DefaultWalletLabel,
		printDesc:   desc,
	}
	if setID, ok := deriveShardSetID(desc); ok {
		flow.shardSetID = setID
	}
	flow.shardShares = buildShardShares(desc, flow.shardSetID)
	// Stash derived seeds keyed by MFP into descriptor order.
	flow.printMnemonic = keystores[desc.Keys[0].MasterFingerprint]
	return flow, keystores, nil
}

func deriveFixtureKeystores(desc *urtypes.OutputDescriptor, mnemonics []bip39.Mnemonic) (map[uint32]bip39.Mnemonic, error) {
	if desc == nil {
		return nil, fmt.Errorf("missing descriptor")
	}
	network := &chaincfg.MainNetParams
	if len(desc.Keys) > 0 && desc.Keys[0].Network != nil {
		network = desc.Keys[0].Network
	}
	keystores := make(map[uint32]bip39.Mnemonic, len(mnemonics))
	for _, m := range mnemonics {
		mfp, err := masterFingerprintFor(m, network)
		if err != nil {
			return nil, fmt.Errorf("derive test wallet fingerprint: %w", err)
		}
		keystores[mfp] = m
	}
	for _, k := range desc.Keys {
		if _, ok := keystores[k.MasterFingerprint]; !ok {
			return nil, fmt.Errorf("fixture seed set does not match descriptor (%08x missing)", k.MasterFingerprint)
		}
	}
	return keystores, nil
}
