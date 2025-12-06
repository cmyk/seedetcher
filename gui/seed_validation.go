package gui

import (
	"errors"

	"github.com/btcsuite/btcd/chaincfg"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
)

var (
	errSeedDuplicate = errors.New("seed duplicate")
	errSeedMismatch  = errors.New("seed mismatch descriptor")
)

// validateSeedAgainstDescriptor computes the master fingerprint and validates it
// against the descriptor and existing keystores. It returns the fingerprint or an error.
func validateSeedAgainstDescriptor(desc *urtypes.OutputDescriptor, mnemonic bip39.Mnemonic, keystores map[uint32]bip39.Mnemonic, network *chaincfg.Params) (uint32, error) {
	mfp, err := masterFingerprintFor(mnemonic, network)
	if err != nil {
		return 0, err
	}
	if desc != nil {
		if _, exists := keystores[mfp]; exists {
			return 0, errSeedDuplicate
		}
		if _, matched := descriptorKeyIdx(*desc, mnemonic, ""); !matched {
			return 0, errSeedMismatch
		}
	}
	return mfp, nil
}
