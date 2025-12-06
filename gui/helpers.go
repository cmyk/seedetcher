package gui

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip32"
	"seedetcher.com/bip39"
)

const maxTitleLen = 18

func sanitizeTitle(title string) string {
	title = strings.ToUpper(title)
	var b strings.Builder
	for _, r := range title {
		if b.Len() >= maxTitleLen {
			break
		}
		if !utf8.ValidRune(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func deriveMasterKey(m bip39.Mnemonic, net *chaincfg.Params) (*hdkeychain.ExtendedKey, bool) {
	seed := bip39.MnemonicSeed(m, "")
	mk, err := hdkeychain.NewMaster(seed, net)
	// Err is only non-nil if the seed generates an invalid key, or we made a mistake.
	// According to [0] the odds of encountering a seed that generates an invalid key by chance is 1 in 2^127.
	//
	// [0] https://bitcoin.stackexchange.com/questions/53180/bip-32-seed-resulting-in-an-invalid-private-key
	return mk, err == nil
}

func masterFingerprintFor(m bip39.Mnemonic, network *chaincfg.Params) (uint32, error) {
	mk, ok := deriveMasterKey(m, network)
	if !ok {
		return 0, errors.New("failed to derive mnemonic master key")
	}
	mfp, _, err := bip32.Derive(mk, urtypes.Path{0})
	if err != nil {
		return 0, err
	}
	return mfp, nil
}

func validateDescriptor(desc urtypes.OutputDescriptor) error {
	keys := make(map[string]bool)
	for _, k := range desc.Keys {
		xpub := k.String()
		if keys[xpub] {
			return &errDuplicateKey{Fingerprint: k.MasterFingerprint}
		}
		keys[xpub] = true
	}
	return nil
}
