package testutils

import "flag"

type Flags struct {
	Mnemonic   string
	Descriptor string
	Output     string
	PaperSize  string
	Verbose    bool
	WalletType string
	BitmapDir  string
	DPI        int
	Mirror     bool
	Invert     bool
	DescQRMM   float64
	PCLOut     string
}

func DefineFlags() *Flags {
	f := &Flags{}
	flag.StringVar(&f.Mnemonic, "mnemonic", "", "12- or 24-word mnemonic phrase (space-separated)")
	flag.StringVar(&f.Descriptor, "descriptor", "", "Raw descriptor string")
	flag.StringVar(&f.Output, "o", "/home/cmyk/PDF", "Output directory")
	flag.StringVar(&f.PaperSize, "papersize", "A4", "Paper size (A4 or Letter)")
	flag.BoolVar(&f.Verbose, "verbose", false, "Enable verbose logging")
	flag.StringVar(&f.WalletType, "w", "multisig", "Wallet type (singlesig or multisig)")
	flag.StringVar(&f.BitmapDir, "png-out", "", "Optional output directory for 600dpi plate PNGs (mirrored/inverted if set)")
	flag.IntVar(&f.DPI, "dpi", 600, "Raster output DPI when using -png-out")
	flag.BoolVar(&f.Mirror, "mirror", false, "Mirror raster output horizontally (toner transfer)")
	flag.BoolVar(&f.Invert, "invert", false, "Invert raster output (white/black swap)")
	flag.Float64Var(&f.DescQRMM, "desc-qr-mm", 75.0, "Maximum descriptor QR size in millimeters")
	flag.StringVar(&f.PCLOut, "pcl-out", "", "Optional output path for raw PCL (bitmap raster) instead of PDF capture")
	return f
}
