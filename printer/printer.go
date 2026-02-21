package printer

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/jung-kurt/gofpdf/v2"
	"github.com/kortschak/qr"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"seedetcher.com/bc/ur"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/descriptor/legacy"
	"seedetcher.com/logutil"
	"seedetcher.com/seedqr"
	"seedetcher.com/version"
)

// PaperSize defines the supported paper formats for printing.
type PaperSize string

const (
	PaperA4     PaperSize = "A4"     // A4 paper size (210x297mm)
	PaperLetter PaperSize = "Letter" // Letter paper size (216x279mm)
)

// Load Fonts
var martianMono = "font/martianmono/MartianMono_Condensed-Regular.ttf" // Path to the UTF-8 TrueType font file
var martianMonoMedium = "font/martianmono/static/MartianMono-Medium.ttf"
var descriptorQRSizeMM = 0.0 // Max descriptor QR size in mm (0 = no cap)
var descriptorQRECC = qr.L   // Error correction level for descriptor QR

// Load font binary data
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

// PlateData holds the data needed to generate individual plates.
type PlateData struct {
	Mnemonic     bip39.Mnemonic
	Desc         *urtypes.OutputDescriptor
	KeyIdx       int
	ShareNum     int
	TotalShares  int
	IsDescriptor bool
}

// createSeedPlate generates a square PDF plate for a seed phrase with the original layout.
func createSeedPlate(mnemonic bip39.Mnemonic, shareNum int, totalShares int) (*gofpdf.Fpdf, *bytes.Buffer, error) {
	pdf := gofpdf.NewCustom(&gofpdf.InitType{UnitStr: "mm", Size: gofpdf.SizeType{Wd: plateSizeMM, Ht: plateSizeMM}})
	pdf.AddPage()
	pdf.SetMargins(0, 0, 0)
	pdf.SetLineWidth(0.2)

	var (
		fontName       = "MartianMono"
		fontNameMedium = "MartianMonoMedium"
		mediumName     = fontName
	)
	fontData := loadFontData(martianMono)
	fontDataMedium := loadFontData(martianMonoMedium)
	if fontData == nil {
		pdf.SetFont("Courier", "", 8)
	} else {
		pdf.AddUTF8FontFromBytes(fontName, "", fontData)
		if pdf.Err() {
			pdf.SetFont("Courier", "", 8)
		} else {
			pdf.SetFont(fontName, "", 8)
			if fontDataMedium != nil {
				pdf.AddUTF8FontFromBytes(fontNameMedium, "", fontDataMedium)
				if !pdf.Err() {
					mediumName = fontNameMedium
				}
			}
		}
	}

	plateSize := plateSizeMM
	pdf.Rect(0, 0, plateSize, plateSize, "D")

	pdf.SetFont(mediumName, "", 6)
	shareText := fmt.Sprintf("%d/%d", shareNum, totalShares)
	pdf.Text(5.0, 5.0, shareText)

	// Revert font size to 8pt and leading to 4mm
	pdf.SetFont(fontName, "", 8)
	yLeft := 15.0
	for i := 0; i < 16 && i < len(mnemonic); i++ {
		if mnemonic[i] == -1 {
			continue
		}
		wordStr := strings.ToUpper(bip39.LabelFor(mnemonic[i]))
		pdf.Text(12.0, yLeft, fmt.Sprintf("%2d %s", i+1, wordStr))
		yLeft += 4.0
	}
	yRight := 15.0
	for i := 16; i < 24 && i < len(mnemonic); i++ {
		if mnemonic[i] == -1 {
			continue
		}
		wordStr := strings.ToUpper(bip39.LabelFor(mnemonic[i]))
		pdf.Text(45.0, yRight, fmt.Sprintf("%2d %s", i+1, wordStr))
		yRight += 4.0
	}

	seed := bip39.MnemonicSeed(mnemonic, "")
	if seed != nil {
		masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create master key: %v", err)
		}
		masterPubKey, err := masterKey.Neuter()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to neuter master key: %v", err)
		}
		pubKey, err := masterPubKey.ECPubKey()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get EC pub key: %v", err)
		}
		fingerprint := btcutil.Hash160(pubKey.SerializeCompressed())[:4]
		fingerprintHex := fmt.Sprintf("%X", fingerprint)

		pdf.SetFont(mediumName, "", 6)
		pdf.Text(40.0, 5.0, fingerprintHex)
		pdf.Text(70.0, 5.0, version.String())

		qrContent := seedqr.QR(mnemonic)
		if len(qrContent) > 0 {
			qrCode, err := qr.Encode(string(qrContent), qr.M)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to encode QR: %v", err)
			}
			qrSize := 28.0
			// Align QR bottom to the 16th word baseline (yLeft base + 15*4mm).
			qrY := (15.0 + float64(15)*4.0) - qrSize
			const quiet = 4
			step := qrSize / float64(qrCode.Size+2*quiet)
			offset := float64(quiet) * step
			qrX := 46.0 - offset
			for y := 0; y < qrCode.Size; y++ {
				for x := 0; x < qrCode.Size; x++ {
					if !qrCode.Black(x, y) {
						continue
					}
					startX := qrX + offset + (float64(x) * step)
					startY := qrY + offset + (float64(y) * step)
					pdf.Rect(startX, startY, step, step, "F")
				}
			}
		}

		pdf.SetFont(mediumName, "", 6)
		label := walletLabel()
		pdf.Text((plateSize-pdf.GetStringWidth(label))/2, plateSize-3.0, label)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, nil, fmt.Errorf("failed to generate PDF: %v", err)
	}
	return pdf, &buf, nil
}

// createDescriptorPlate generates a square PDF plate for a descriptor with all details and a square QR code.
func createDescriptorPlate(desc *urtypes.OutputDescriptor, keyIdx int, shareNum int, totalShares int) (*gofpdf.Fpdf, error) {
	pdf := gofpdf.NewCustom(&gofpdf.InitType{UnitStr: "mm", Size: gofpdf.SizeType{Wd: plateSizeMM, Ht: plateSizeMM}})
	pdf.AddPage()
	pdf.SetMargins(10, 10, 10) // 10mm margins
	pdf.SetLineWidth(0.2)

	var (
		fontName       = "MartianMono"
		fontNameMedium = "MartianMonoMedium"
		mediumName     = fontName
	)
	fontData := loadFontData(martianMono)
	fontDataMedium := loadFontData(martianMonoMedium)
	if fontData == nil {
		pdf.SetFont("Courier", "", 8)
	} else {
		pdf.AddUTF8FontFromBytes(fontName, "", fontData)
		if pdf.Err() {
			pdf.SetFont("Courier", "", 8)
		} else {
			pdf.SetFont(fontName, "", 8)
			if fontDataMedium != nil {
				pdf.AddUTF8FontFromBytes(fontNameMedium, "", fontDataMedium)
				if !pdf.Err() {
					mediumName = fontNameMedium
				}
			}
		}
	}

	plateSize := plateSizeMM
	pdf.Rect(0, 0, plateSize, plateSize, "D")

	pdf.SetFont(mediumName, "", 6)
	shareText := fmt.Sprintf("%d/%d", shareNum, totalShares)
	pathStr := derivationPathForKey(desc.Keys[keyIdx], desc.Script)
	pathWidth := pdf.GetStringWidth(fmt.Sprintf("Path:%s", pathStr))
	pdf.Text(5.0, 5.0, shareText)
	pdf.Text(plateSize-pathWidth-5.0, 5.0, fmt.Sprintf("Path:%s", pathStr))

	pdf.SetFont(fontName, "", 8)
	pdf.SetXY(20.0, 8.0)
	key := desc.Keys[keyIdx]
	allText := fmt.Sprintf("Type:%v/Script:%s/Threshold:%d/Keys:%d/Key%d:%s", desc.Type, strings.Replace(desc.Script.String(), " ", "", -1), desc.Threshold, len(desc.Keys), keyIdx+1, key.String())
	lines := pdf.SplitText(allText, plateSize-10.0) // 5mm margins
	lineHeightMM := pdf.PointConvert(8)             // current font size height in mm
	lineSpacing := 3.5                              // mm between baselines
	y := 10.0                                       // baseline of first line
	for i, line := range lines {
		pdf.Text(5.0, y, line)
		if i < len(lines)-1 {
			y += lineSpacing
		}
	}
	// QR at bottom, 10mm from edge
	qrContent := createDescriptorQR(desc)
	if len(qrContent) == 0 {
		return nil, fmt.Errorf("failed to generate descriptor QR: empty content")
	}
	qrCode, err := qr.Encode(qrContent, descriptorQRECC)
	if err != nil {
		return nil, fmt.Errorf("failed to encode descriptor QR: %v", err)
	}
	textLines := float64(len(lines))
	textBlockHeight := lineHeightMM
	if textLines > 1 {
		textBlockHeight += (textLines - 1) * lineSpacing
	}
	textBottom := 7.0 + textBlockHeight
	qrGap := 2.0    // gap between text block and QR
	qrBottom := 0.0 // bottom margin
	qrSize := plateSize - textBottom - qrGap - qrBottom
	if qrSize > descriptorQRSizeMM && descriptorQRSizeMM > 0 {
		qrSize = descriptorQRSizeMM
	}
	if qrSize < 5 {
		qrSize = 5 // Prevent too-small QR
	}
	qrX := (plateSize - qrSize) / 2 // Left margin
	qrY := textBottom + qrGap
	const quiet = 4
	step := qrSize / float64(qrCode.Size+2*quiet)
	offset := float64(quiet) * step
	for y := 0; y < qrCode.Size; y++ {
		for x := 0; x < qrCode.Size; x++ {
			if !qrCode.Black(x, y) {
				continue
			}
			startX := qrX + offset + (float64(x) * step)
			startY := qrY + offset + (float64(y) * step)
			pdf.Rect(startX, startY, step, step, "F")
		}
	}

	return pdf, nil
}

// createDescriptorQR constructs a QR string for the descriptor.
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

// Deprecated: legacy vector-PDF plate generator.
// Use CreatePlateBitmaps + ComposePages + WritePDFRaster/WritePCL instead.
func CreatePlates(w io.Writer, mnemonics []bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, supportsPCL, supportsPostScript bool) ([]string, []string, string, error) {
	logutil.DebugLog("Starting CreatePlates with %d mnemonics, desc=%v, keyIdx=%d", len(mnemonics), desc != nil, keyIdx)
	tempDir := filepath.Join(os.TempDir(), "seedetcher-plates")
	if err := os.Mkdir(tempDir, 0700); err != nil && !os.IsExist(err) {
		return nil, nil, "", fmt.Errorf("failed to create temp dir %s: %v", tempDir, err)
	}
	logutil.DebugLog("Using directory: %s", tempDir)
	// No defer os.RemoveAll(tempDir) here—caller will handle cleanup

	totalShares := len(mnemonics)
	if desc != nil && len(desc.Keys) > 0 {
		totalShares = len(desc.Keys)
		logutil.DebugLog("Calculated totalShares: %d based on desc.Keys length: %d", totalShares, len(desc.Keys))
	}

	seedPaths := make([]string, totalShares)
	descPaths := make([]string, totalShares)
	for i := 0; i < totalShares; i++ {
		mnemonic := mnemonics[i%len(mnemonics)]
		logutil.DebugLog("Generating seed plate %d", i+1)
		_, seedBuf, err := createSeedPlate(mnemonic, i+1, totalShares)
		if err != nil {
			return nil, nil, tempDir, fmt.Errorf("failed to generate seed plate %d: %v", i, err)
		}
		seedFile := filepath.Join(tempDir, fmt.Sprintf("seed_%d.pdf", i))
		if err := os.WriteFile(seedFile, seedBuf.Bytes(), 0644); err != nil {
			return nil, nil, tempDir, fmt.Errorf("failed to write seed plate %d: %v", i, err)
		}
		seedPaths[i] = seedFile
		logutil.DebugLog("Generated seed plate %d at %s, size: %d bytes", i+1, seedFile, seedBuf.Len())
		if desc != nil && len(desc.Keys) > 0 {
			descKeyIdx := i % len(desc.Keys) // rotate keys per plate
			logutil.DebugLog("Generating descriptor plate %d", i+1)
			pdf, err := createDescriptorPlate(desc, descKeyIdx, i+1, totalShares)
			if err != nil {
				return nil, nil, tempDir, fmt.Errorf("failed to generate descriptor plate %d: %v", i, err)
			}
			descFile := filepath.Join(tempDir, fmt.Sprintf("desc_%d.pdf", i))
			if err := pdf.OutputFileAndClose(descFile); err != nil {
				return nil, nil, tempDir, fmt.Errorf("failed to write descriptor plate %d: %v", i, err)
			}
			descPaths[i] = descFile
			if info, err := os.Stat(descFile); err == nil {
				logutil.DebugLog("Generated descriptor plate %d at %s, size: %d bytes", i+1, descFile, info.Size())
			}
		} else {
			descPaths[i] = ""
		}
	}
	return seedPaths, descPaths, tempDir, nil
}

// Deprecated: legacy vector-PDF n-up page layout path.
// Use CreatePlateBitmaps + ComposePages + WritePDFRaster/WritePCL instead.
func CreatePageLayout(w io.Writer, tempDir string, paperFormat PaperSize, seedPaths, descPaths []string) error {
	logutil.DebugLog("Starting CreatePageLayout with tempDir: %s", tempDir)

	if len(seedPaths) != len(descPaths) {
		return fmt.Errorf("mismatch in seed and desc paths: %d vs %d", len(seedPaths), len(descPaths))
	}
	totalShares := len(seedPaths)
	if totalShares == 0 {
		logutil.DebugLog("No plates to merge")
		return fmt.Errorf("no plates to merge")
	}

	var pageSize string
	switch paperFormat {
	case PaperA4:
		pageSize = "A4"
	case PaperLetter:
		pageSize = "Letter"
	default:
		return fmt.Errorf("unsupported paper size: %v", paperFormat)
	}

	slotsPerPage := 6
	numPages := (totalShares*2 + slotsPerPage - 1) / slotsPerPage
	logutil.DebugLog("Total shares: %d, generating %d pages", totalShares, numPages)

	for page := 0; page < numPages; page++ {
		startIdx := page * (slotsPerPage / 2)
		endIdx := min(startIdx+(slotsPerPage/2), totalShares)
		pageShares := endIdx - startIdx
		logutil.DebugLog("Page %d: shares %d to %d (%d shares)", page+1, startIdx+1, endIdx, pageShares)

		allFiles := make([]string, 0, slotsPerPage)
		hasDesc := false
		for _, path := range descPaths[startIdx:endIdx] {
			if path != "" {
				hasDesc = true
				break
			}
		}
		if hasDesc {
			for i := startIdx; i < endIdx; i++ {
				allFiles = append(allFiles, seedPaths[i], descPaths[i])
			}
			// No padding with empty strings here
		} else {
			for i := startIdx; i < endIdx; i++ {
				allFiles = append(allFiles, seedPaths[i])
			}
		}
		logutil.DebugLog("Page %d files (before filter): %v", page+1, allFiles)

		// Filter out empty strings
		var filteredFiles []string
		for _, f := range allFiles {
			if f != "" {
				filteredFiles = append(filteredFiles, f)
			}
		}
		logutil.DebugLog("Page %d files (after filter): %v", page+1, filteredFiles)

		// Debug: Verify files exist before merge
		for _, f := range filteredFiles {
			if info, err := os.Stat(f); err == nil {
				logutil.DebugLog("Before merge: %s exists, size: %d bytes", f, info.Size())
			} else {
				logutil.DebugLog("Before merge: %s error: %v", f, err)
			}
		}

		tempConcatFile := filepath.Join(tempDir, fmt.Sprintf("concat_page_%d.pdf", page))
		logutil.DebugLog("Attempting to merge into %s", tempConcatFile)
		if err := api.MergeCreateFile(filteredFiles, tempConcatFile, false, nil); err != nil {
			logutil.DebugLog("Failed to merge PDFs for page %d: %v", page+1, err)
			os.Remove(tempConcatFile)
			return fmt.Errorf("failed to merge PDFs for page %d: %v", page+1, err)
		}
		logutil.DebugLog("Merged PDFs into %s", tempConcatFile)
		if info, err := os.Stat(tempConcatFile); err == nil {
			logutil.DebugLog("Size of concat_page_%d.pdf: %d bytes", page, info.Size())
		} else {
			logutil.DebugLog("Failed to stat merged file %s: %v", tempConcatFile, err)
		}
		defer os.Remove(tempConcatFile)

		tempNUpFile := filepath.Join(tempDir, fmt.Sprintf("nup_page_%d.pdf", page))
		nupConfig := model.DefaultNUpConfig()
		nupConfig.PageSize = pageSize
		nupConfig.Grid = &types.Dim{Width: 2, Height: 3}
		nupConfig.UserDim = true
		nupConfig.PageDim = types.PaperSize[pageSize]
		if err := api.NUpFile([]string{tempConcatFile}, tempNUpFile, nil, nupConfig, nil); err != nil {
			logutil.DebugLog("Failed to create NUp layout for page %d: %v", page+1, err)
			os.Remove(tempNUpFile)
			return fmt.Errorf("failed to create NUp layout for page %d: %v", page+1, err)
		}
		logutil.DebugLog("Created NUp layout at %s", tempNUpFile)
		if info, err := os.Stat(tempNUpFile); err == nil {
			logutil.DebugLog("Size of nup_page_%d.pdf: %d bytes", page, info.Size())
		}
		defer os.Remove(tempNUpFile)

		nupBytes, err := os.ReadFile(tempNUpFile)
		if err != nil {
			return fmt.Errorf("failed to read NUp PDF for page %d: %v", page+1, err)
		}
		logutil.DebugLog("nupBytes length for page %d before write: %d", page+1, len(nupBytes))
		if _, err := w.Write(nupBytes); err != nil {
			return fmt.Errorf("failed to write NUp PDF for page %d: %v", page+1, err)
		}
	}

	logutil.DebugLog("Wrote %d pages to output", numPages)
	return nil
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
