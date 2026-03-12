package testutils

import "flag"

type Flags struct {
	Mnemonic        string
	Descriptor      string
	Output          string
	PaperSize       string
	Verbose         bool
	WalletType      string // deprecated alias for Fixture
	Fixture         string
	WalletKind      string
	NOfM            string
	WordProfile     string
	SinglesigLayout string
	BitmapDir       string
	DPI             int
	Mirror          bool
	Invert          bool
	DescQRMM        float64
	PCLOut          string
	SceneJSONOut    string
	SVGOut          string
	WalletName      string
	EtchStatsPage   bool
	Compact2of3     bool
}

func DefineFlags() *Flags {
	f := &Flags{}
	flag.StringVar(&f.Mnemonic, "mnemonic", "", "12- or 24-word mnemonic phrase (space-separated)")
	flag.StringVar(&f.Descriptor, "descriptor", "", "Raw descriptor string")
	flag.StringVar(&f.Output, "o", "/home/cmyk/PDF", "Output directory")
	flag.StringVar(&f.PaperSize, "papersize", "A4", "Paper size (A4 or Letter)")
	flag.BoolVar(&f.Verbose, "verbose", false, "Enable verbose logging")
	flag.StringVar(&f.WalletType, "w", "multisig", "DEPRECATED alias for -fixture")
	flag.StringVar(&f.Fixture, "fixture", "", "Named wallet fixture (seed-12, seed-15, seed-18, seed-21, singlesig, singlesig-longwords, singlesig-nested-p2sh-p2wpkh, multisig, multisig-mainnet-2of3, multisig-nested-2of3, multisig-2of2, multisig-2of4, multisig-3of4, multisig-3of5, multisig-4of7, multisig-5of7, multisig-7of10)")
	flag.StringVar(&f.WalletKind, "wallet-type", "", "Parametric wallet type (singlesig or multisig)")
	flag.StringVar(&f.NOfM, "n-of-m", "", "Parametric multisig threshold and key-count (for example 3of5)")
	flag.StringVar(&f.WordProfile, "word-profile", "normal", "Seed word profile (normal or longwords)")
	flag.StringVar(&f.SinglesigLayout, "singlesig-layout", "seed-info", "Singlesig render layout (seed-info, seed-only, or seed-desc)")
	flag.StringVar(&f.BitmapDir, "png-out", "", "Optional output directory for 600dpi plate PNGs (mirrored/inverted if set)")
	flag.IntVar(&f.DPI, "dpi", 600, "Raster output DPI when using -png-out")
	flag.BoolVar(&f.Mirror, "mirror", false, "Mirror raster output horizontally (toner transfer)")
	flag.BoolVar(&f.Invert, "invert", false, "Invert raster output (white/black swap)")
	flag.Float64Var(&f.DescQRMM, "desc-qr-mm", 80.0, "Maximum descriptor QR size in millimeters")
	flag.StringVar(&f.PCLOut, "pcl-out", "", "Optional output path for raw PCL (bitmap raster)")
	flag.StringVar(&f.SceneJSONOut, "scene-json-out", "", "Optional output path for plate scene JSON (seed-side foundation)")
	flag.StringVar(&f.SVGOut, "svg-out", "", "Optional output directory for per-plate scene SVGs (seed-side foundation)")
	flag.StringVar(&f.WalletName, "wallet-name", "", "Optional wallet name to print on plates (defaults to SEEDETCHER)")
	flag.BoolVar(&f.EtchStatsPage, "etch-stats-page", false, "Append an additional etch stats page with per-plate coverage metrics")
	flag.BoolVar(&f.Compact2of3, "compact-2of3", false, "Use compact single-sided layout for sortedmulti 2-of-3 descriptor shares")
	return f
}
