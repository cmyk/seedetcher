package printer

import (
	"os"
	"strings"

	"github.com/kortschak/qr"
	"seedetcher.com/bc/ur"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/descriptor/legacy"
	"seedetcher.com/logutil"
)

// PaperSize defines the supported paper formats for printing.
type PaperSize string

const (
	PaperA4     PaperSize = "A4"     // A4 paper size (210x297mm)
	PaperLetter PaperSize = "Letter" // Letter paper size (216x279mm)
)

var (
	martianMono                = "font/martianmono/MartianMono_Condensed-Regular.ttf"
	martianMonoMedium          = "font/martianmono/static/MartianMono-Medium.ttf"
	descriptorQRSizeMM float64 = 0.0
	descriptorQRECC            = qr.L
)

// loadFontData reads font bytes from disk for raster rendering.
func loadFontData(fontPath string) []byte {
	logutil.DebugLog("Attempting to load font from %s", fontPath)
	data, err := os.ReadFile(fontPath)
	if err != nil {
		logutil.DebugLog("Failed to load font: %v", err)
		return nil
	}
	logutil.DebugLog("Font data loaded, size: %d bytes", len(data))
	return data
}

// createDescriptorQR constructs a canonical UR descriptor payload.
func createDescriptorQR(desc *urtypes.OutputDescriptor) string {
	if desc == nil {
		return ""
	}
	normalized := legacy.NormalizeDescriptorForLegacyUR(*desc)
	// Encode as UR:crypto-output so scan path uses the standard descriptor parser.
	return ur.Encode("crypto-output", normalized.Encode(), 1, 1)
}

// DescriptorQRPayload returns the canonical descriptor QR payload for a full
// descriptor (single-part UR:crypto-output).
func DescriptorQRPayload(desc *urtypes.OutputDescriptor) string {
	return createDescriptorQR(desc)
}

func derivationPathForKey(key urtypes.KeyDescriptor, script urtypes.Script) string {
	normalize := func(path string) string {
		path = strings.ReplaceAll(path, "H", "'")
		path = strings.ReplaceAll(path, "h", "'")
		return path
	}
	if len(key.DerivationPath) > 0 {
		return normalize(key.DerivationPath.String())
	}
	return normalize(script.DerivationPath().String())
}

// SetDescriptorQRSize overrides the maximum descriptor QR size in millimeters.
// Zero or negative values are ignored.
func SetDescriptorQRSize(mm float64) {
	if mm > 0 {
		descriptorQRSizeMM = mm
	}
}
