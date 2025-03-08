package testutils

import "flag"

type Flags struct {
	Mnemonic   string
	Descriptor string
	Output     string
	PaperSize  string
	Verbose    bool
	WalletType string
}

func DefineFlags() *Flags {
	f := &Flags{}
	flag.StringVar(&f.Mnemonic, "mnemonic", "", "12- or 24-word mnemonic phrase (space-separated)")
	flag.StringVar(&f.Descriptor, "descriptor", "", "Raw descriptor string")
	flag.StringVar(&f.Output, "o", "/home/cmyk/PDF", "Output directory")
	flag.StringVar(&f.PaperSize, "papersize", "A4", "Paper size (A4 or Letter)")
	flag.BoolVar(&f.Verbose, "verbose", false, "Enable verbose logging")
	flag.StringVar(&f.WalletType, "w", "multisig", "Wallet type (singlesig or multisig)")
	return f
}
