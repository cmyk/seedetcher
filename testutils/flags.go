package testutils

import "flag"

type Flags struct {
	Mnemonic               string
	Descriptor             string
	Output                 string
	PaperSize              string
	Verbose                bool
	ListFixtures           bool
	WalletType             string // deprecated alias for Fixture
	Fixture                string
	WalletKind             string
	NOfM                   string
	WordProfile            string
	SinglesigLayout        string
	BitmapDir              string
	DPI                    int
	Mirror                 bool
	Invert                 bool
	DescQRMM               float64
	PCLOut                 string
	SceneJSONOut           string
	SVGOut                 string
	GCodeOut               string
	GCodeSide              string
	LaserCalibration       string
	CalibrationAreaMM      float64
	CalibrationOffset      string
	CalibrationOffsetXMM   float64
	CalibrationOffsetYMM   float64
	CalibrationPowers      string
	CalibrationFeeds       string
	CalibrationModes       string
	FillStepTestFeeds      string
	FillStepTestSteps      string
	PowerFeedTestPowers    string
	PowerFeedTestFeeds     string
	BedMM                  float64
	PlateMM                float64
	PlateOriginXMM         float64
	PlateOriginYMM         float64
	MachineOffsetXMM       float64
	MachineOffsetYMM       float64
	LaserMode              string
	LaserMaxS              int
	LaserFeed              float64
	LaserFillStepMM        float64
	LaserFillInsetMM       float64
	LaserOutlineInsetMM    float64
	LaserFeatureShrinkMM   float64
	LaserOutlinePowerScale float64
	LaserOutlineFeedScale  float64
	LaserHatchOverscanMM   float64
	LaserMinBurnSpanMM     float64
	LaserSweepCorrection   bool
	LaserNoOutline         bool
	LaserDualOutline       bool
	LaserPassOrder         string
	LaserTextFillMode      string
	RapidFeed              float64
	WalletName             string
	EtchStatsPage          bool
	Compact2of3            bool
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
	flag.StringVar(&f.LaserCalibration, "laser-calibration", "", "Generate laser calibration scene instead of wallet scene (supported: power-grid, test-tile, line-width-tile, fill-step-test, power-feed-test, repeatability-test, sector-repeatability-test)")
	flag.Float64Var(&f.CalibrationAreaMM, "calibration-area-mm", 50.0, "Calibration scene size in mm, centered within the physical plate")
	flag.StringVar(&f.CalibrationOffset, "calibration-offset", "", "Convenience alias for calibration scene offset inside plate as x,y")
	flag.Float64Var(&f.CalibrationOffsetXMM, "calibration-offset-x", 0.0, "Calibration scene offset X inside the physical plate")
	flag.Float64Var(&f.CalibrationOffsetYMM, "calibration-offset-y", 0.0, "Calibration scene offset Y inside the physical plate")
	flag.StringVar(&f.CalibrationPowers, "calibration-powers", "40,60,80,100", "Comma-separated power values for laser calibration jobs")
	flag.StringVar(&f.CalibrationFeeds, "calibration-feeds", "400,600,800,1000", "Comma-separated feed values for laser calibration jobs")
	flag.StringVar(&f.CalibrationModes, "calibration-modes", "", "Optional comma-separated laser modes per calibration row (m3 or m4); use 1 value to apply to all rows")
	flag.StringVar(&f.FillStepTestFeeds, "fill-step-test-feeds", "", "Optional feed series for fill-step-test; accepts list (e.g. 1400,1700,2000,2300,2600) or range start:end[:count] (e.g. 1400:2600 or 1400:2600:5)")
	flag.StringVar(&f.FillStepTestSteps, "fill-step-test-steps", "", "Optional fill-step series for fill-step-test; accepts list (e.g. 0.03,0.035,0.04,0.05,0.06) or range start:end[:count] (e.g. 0.03:0.06 or 0.03:0.06:5)")
	flag.StringVar(&f.PowerFeedTestPowers, "power-feed-test-powers", "", "Optional power series for power-feed-test; accepts list (e.g. 680,720,760,800,850) or range start:end[:count] (e.g. 650:850 or 650:850:5)")
	flag.StringVar(&f.PowerFeedTestFeeds, "power-feed-test-feeds", "", "Optional feed series for power-feed-test; accepts list (e.g. 1400,1700,2000,2300,2600) or range start:end[:count] (e.g. 1400:2600 or 1400:2600:5)")
	flag.Float64Var(&f.BedMM, "bed-mm", 150.0, "Laser machine workspace size in mm (K1 default 150)")
	flag.Float64Var(&f.PlateMM, "plate-mm", 100.0, "Physical plate size in mm used for centered scene placement")
	flag.Float64Var(&f.PlateOriginXMM, "plate-origin-x", 0.0, "DEPRECATED alias for -work-origin-x")
	flag.Float64Var(&f.PlateOriginYMM, "plate-origin-y", 0.0, "DEPRECATED alias for -work-origin-y")
	flag.Float64Var(&f.PlateOriginXMM, "work-origin-x", 0.0, "Plate origin X on the calibrated work surface (bottom-left)")
	flag.Float64Var(&f.PlateOriginYMM, "work-origin-y", 0.0, "Plate origin Y on the calibrated work surface (bottom-left)")
	flag.Float64Var(&f.MachineOffsetXMM, "machine-offset-x", 0.0, "Advanced: fixed machine X correction added after work-surface coordinates; leave at 0 when using a calibrated bed cover")
	flag.Float64Var(&f.MachineOffsetYMM, "machine-offset-y", 0.0, "Advanced: fixed machine Y correction added after work-surface coordinates; leave at 0 when using a calibrated bed cover")
	flag.StringVar(&f.LaserMode, "laser-mode", "m4", "Laser mode for generated G-code (m3 constant power or m4 dynamic power)")
	flag.IntVar(&f.LaserMaxS, "laser-max-s", 80, "Laser power S value used in generated G-code (for example 0..1000, default 80)")
	flag.Float64Var(&f.LaserFeed, "laser-feed", 900.0, "Cut feed in mm/min for generated G-code (default 900)")
	flag.Float64Var(&f.LaserFillStepMM, "laser-fill-step-mm", 0.0, "Optional hatch/fill line spacing in mm for generated G-code (default renderer behavior when 0)")
	flag.Float64Var(&f.LaserFillInsetMM, "laser-fill-inset-mm", 0.0, "Optional inward inset in mm applied to fill only, leaving outline at original geometry (default 0)")
	flag.Float64Var(&f.LaserOutlineInsetMM, "laser-outline-inset-mm", 0.02, "Inward inset in mm applied to text/path outline pass to avoid fat edges from centerline tracing (default 0.02)")
	flag.Float64Var(&f.LaserFeatureShrinkMM, "laser-feature-shrink-mm", 0.0, "Optional geometry shrink in mm for text/path features to counter fat edges (default 0)")
	flag.Float64Var(&f.LaserOutlinePowerScale, "laser-outline-power-scale", 1.0, "Scale factor for outline-pass laser power (default 1.0)")
	flag.Float64Var(&f.LaserOutlineFeedScale, "laser-outline-feed-scale", 1.0, "Scale factor for outline-pass feed rate (default 1.0)")
	flag.Float64Var(&f.LaserHatchOverscanMM, "laser-hatch-overscan-mm", 0.0, "Optional lead-in/out distance in mm for hatch scanlines (laser off) to reduce edge dwell artifacts")
	flag.Float64Var(&f.LaserMinBurnSpanMM, "laser-min-burn-span-mm", 0.05, "Minimum burn span length in mm for hatch rows; shorter spans are dropped to reduce stutter artifacts")
	flag.BoolVar(&f.LaserSweepCorrection, "laser-sweep-correction", false, "Add sparse half-step correction scanlines for hatch fills to improve clipped curved edges")
	flag.BoolVar(&f.LaserNoOutline, "laser-no-outline", false, "Disable outline pass for filled primitives (diagnostic/ablation tuning)")
	flag.BoolVar(&f.LaserDualOutline, "laser-dual-outline", false, "Run a second outer outline pass for filled text/path when outline inset is active")
	flag.StringVar(&f.LaserPassOrder, "laser-pass-order", "grouped", "Pass ordering for filled text (grouped, local, or sweep)")
	flag.StringVar(&f.LaserTextFillMode, "laser-text-fill-mode", "raster", "Text fill strategy for filled text primitives (raster or contour)")
	flag.Float64Var(&f.RapidFeed, "rapid-feed", 3000.0, "Rapid move feed in mm/min for generated G-code (default 3000)")
	flag.StringVar(&f.WalletName, "wallet-name", "", "Optional wallet name to print on plates (defaults to SEEDETCHER)")
	flag.BoolVar(&f.EtchStatsPage, "etch-stats-page", false, "Append an additional etch stats page with per-plate coverage metrics")
	flag.BoolVar(&f.Compact2of3, "compact-2of3", false, "Use compact single-sided layout for sortedmulti 2-of-3 descriptor shares")
	return f
}
