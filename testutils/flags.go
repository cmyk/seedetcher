package testutils

import "flag"

type Flags struct {
	Mnemonic             string
	Descriptor           string
	Output               string
	PaperSize            string
	Verbose              bool
	ListFixtures         bool
	WalletType           string // deprecated alias for Fixture
	Fixture              string
	WalletKind           string
	NOfM                 string
	WordProfile          string
	SinglesigLayout      string
	BitmapDir            string
	DPI                  int
	Mirror               bool
	Invert               bool
	DescQRMM             float64
	PCLOut               string
	SceneJSONOut         string
	SVGOut               string
	GCodeOut             string
	GCodeSide            string
	LaserCalibration     string
	CalibrationAreaMM    float64
	CalibrationOffset    string
	CalibrationOffsetXMM float64
	CalibrationOffsetYMM float64
	CalibrationPowers    string
	CalibrationFeeds     string
	BedMM                float64
	PlateMM              float64
	PlateOriginXMM       float64
	PlateOriginYMM       float64
	LaserMaxS            int
	LaserFeed            float64
	RapidFeed            float64
	WalletName           string
	EtchStatsPage        bool
	Compact2of3          bool
}

func DefineFlags() *Flags {
	f := &Flags{}
	flag.StringVar(&f.Mnemonic, "mnemonic", "", "12- or 24-word mnemonic phrase (space-separated)")
	flag.StringVar(&f.Descriptor, "descriptor", "", "Raw descriptor string")
	flag.StringVar(&f.Output, "o", "/home/cmyk/PDF", "Output directory")
	flag.StringVar(&f.PaperSize, "papersize", "A4", "Paper size (A4 or Letter)")
	flag.BoolVar(&f.Verbose, "verbose", false, "Enable verbose logging")
	flag.BoolVar(&f.ListFixtures, "list-fixtures", false, "List available fixtures and exit")
	flag.StringVar(&f.WalletType, "w", "multisig", "DEPRECATED alias for -fixture")
	flag.StringVar(&f.Fixture, "fixture", "", "Named wallet fixture (use -list-fixtures)")
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
	flag.StringVar(&f.GCodeOut, "gcode-out", "", "Optional output directory for per-plate scene G-code (file-only laser prototype)")
	flag.StringVar(&f.GCodeSide, "side", "both", "G-code scene side selection (seed, desc, or both)")
	flag.StringVar(&f.LaserCalibration, "laser-calibration", "", "Generate laser calibration scene instead of wallet scene (supported: power-grid)")
	flag.Float64Var(&f.CalibrationAreaMM, "calibration-area-mm", 50.0, "Calibration scene size in mm, centered within the physical plate")
	flag.StringVar(&f.CalibrationOffset, "calibration-offset", "", "Convenience alias for calibration scene offset inside plate as x,y")
	flag.Float64Var(&f.CalibrationOffsetXMM, "calibration-offset-x", 0.0, "Calibration scene offset X inside the physical plate")
	flag.Float64Var(&f.CalibrationOffsetYMM, "calibration-offset-y", 0.0, "Calibration scene offset Y inside the physical plate")
	flag.StringVar(&f.CalibrationPowers, "calibration-powers", "40,60,80,100", "Comma-separated power values for laser calibration jobs")
	flag.StringVar(&f.CalibrationFeeds, "calibration-feeds", "400,600,800,1000", "Comma-separated feed values for laser calibration jobs")
	flag.Float64Var(&f.BedMM, "bed-mm", 150.0, "Laser machine workspace size in mm (K1 default 150)")
	flag.Float64Var(&f.PlateMM, "plate-mm", 100.0, "Physical plate size in mm used for centered scene placement")
	flag.Float64Var(&f.PlateOriginXMM, "plate-origin-x", 0.0, "Plate origin X in machine coordinates (bottom-left)")
	flag.Float64Var(&f.PlateOriginYMM, "plate-origin-y", 0.0, "Plate origin Y in machine coordinates (bottom-left)")
	flag.IntVar(&f.LaserMaxS, "laser-max-s", 80, "Laser power S value used in generated G-code (for example 0..1000, default 80)")
	flag.Float64Var(&f.LaserFeed, "laser-feed", 900.0, "Cut feed in mm/min for generated G-code (default 900)")
	flag.Float64Var(&f.RapidFeed, "rapid-feed", 3000.0, "Rapid move feed in mm/min for generated G-code (default 3000)")
	flag.StringVar(&f.WalletName, "wallet-name", "", "Optional wallet name to print on plates (defaults to SEEDETCHER)")
	flag.BoolVar(&f.EtchStatsPage, "etch-stats-page", false, "Append an additional etch stats page with per-plate coverage metrics")
	flag.BoolVar(&f.Compact2of3, "compact-2of3", false, "Use compact single-sided layout for sortedmulti 2-of-3 descriptor shares")
	return f
}
