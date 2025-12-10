package gui

import (
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
)

// PrintJob carries the data needed to render a plate.
type PrintJob struct {
	Mnemonic   bip39.Mnemonic
	Descriptor *urtypes.OutputDescriptor
	KeyIdx     int
	Label      string
}

// FromSinglesig creates a print job for a single-seed path.
func FromSinglesig(m bip39.Mnemonic, label string) PrintJob {
	return PrintJob{Mnemonic: m, KeyIdx: 0, Label: label}
}

// FromDescriptor creates a print job from a descriptor and selected key index.
func FromDescriptor(desc *urtypes.OutputDescriptor, mnemonic bip39.Mnemonic, keyIdx int, label string) PrintJob {
	return PrintJob{Mnemonic: mnemonic, Descriptor: desc, KeyIdx: keyIdx, Label: label}
}
