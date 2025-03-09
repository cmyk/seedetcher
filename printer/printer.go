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
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/logutil"
	"seedetcher.com/seedqr"
)

// PaperSize defines the supported paper formats for printing.
type PaperSize string

const (
	PaperA4     PaperSize = "A4"     // A4 paper size (210x297mm)
	PaperLetter PaperSize = "Letter" // Letter paper size (216x279mm)
)

// Load Fonts
var martianMono = "font/martianmono/MartianMono_Condensed-Regular.ttf" // Path to the UTF-8 TrueType font file

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

// createSeedPlate generates an 85x85mm PDF plate for a seed phrase with the original layout.
func createSeedPlate(mnemonic bip39.Mnemonic, shareNum int, totalShares int) (*gofpdf.Fpdf, *bytes.Buffer, error) {
	pdf := gofpdf.NewCustom(&gofpdf.InitType{UnitStr: "mm", Size: gofpdf.SizeType{Wd: 85, Ht: 85}})
	pdf.AddPage()
	pdf.SetMargins(0, 0, 0)
	pdf.SetLineWidth(0.2)

	var fontName = "MartianMono"
	fontData := loadFontData(martianMono)
	if fontData == nil {
		pdf.SetFont("Courier", "", 8)
	} else {
		pdf.AddUTF8FontFromBytes(fontName, "", fontData)
		if pdf.Err() {
			pdf.SetFont("Courier", "", 8)
		} else {
			pdf.SetFont(fontName, "", 8)
		}
	}

	plateSize := 85.0
	pdf.Rect(0, 0, plateSize, plateSize, "D")

	pdf.SetFont(fontName, "", 5)
	// Fix: Use 1/1 if totalShares > 1 but no descriptor context (singlesig case)
	if totalShares > 1 {
		totalShares = 1 // Singlesig without descriptor
	}
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

		pdf.SetFont(fontName, "", 5)
		pdf.Text(40.0, 5.0, fingerprintHex)
		pdf.Text(70.0, 5.0, "V1")

		qrContent := seedqr.QR(mnemonic)
		if len(qrContent) > 0 {
			qrCode, err := qr.Encode(string(qrContent), qr.M)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to encode QR: %v", err)
			}
			qrSize := 25.0
			qrX := 45.0
			qrY := plateSize - qrSize - 10.0
			pixelSize := qrSize / float64(qrCode.Size)
			for y := 0; y < qrCode.Size; y++ {
				for x := 0; x < qrCode.Size; x++ {
					if qrCode.Black(x, y) {
						startX := qrX + (float64(x) * pixelSize)
						startY := qrY + (float64(y) * pixelSize)
						pdf.Rect(startX, startY, pixelSize, pixelSize, "F")
					}
				}
			}
		}

		pdf.SetFont(fontName, "", 5)
		pdf.Text((plateSize-pdf.GetStringWidth("SATOSHI'S STASH"))/2, plateSize-5.0, "SATOSHI'S STASH")
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, nil, fmt.Errorf("failed to generate PDF: %v", err)
	}
	return pdf, &buf, nil
}

// createDescriptorPlate generates an 85x85mm PDF plate for a descriptor with all details and a square QR code.
func createDescriptorPlate(desc *urtypes.OutputDescriptor, keyIdx int, shareNum int, totalShares int) (*gofpdf.Fpdf, error) {
	pdf := gofpdf.NewCustom(&gofpdf.InitType{UnitStr: "mm", Size: gofpdf.SizeType{Wd: 85, Ht: 85}})
	pdf.AddPage()
	pdf.SetMargins(10, 10, 10) // 10mm margins
	pdf.SetLineWidth(0.2)

	var fontName = "MartianMono"
	fontData := loadFontData(martianMono)
	if fontData == nil {
		pdf.SetFont("Courier", "", 8)
	} else {
		pdf.AddUTF8FontFromBytes(fontName, "", fontData)
		if pdf.Err() {
			pdf.SetFont("Courier", "", 8)
		} else {
			pdf.SetFont(fontName, "", 8)
		}
	}

	plateSize := 85.0
	pdf.Rect(0, 0, plateSize, plateSize, "D")

	pdf.SetFont(fontName, "", 5)
	shareText := fmt.Sprintf("%d/%d", shareNum, totalShares)
	pdf.Text(5.0, 5.0, shareText)

	pdf.SetFont(fontName, "", 8)
	pdf.SetXY(20.0, 8.0)
	key := desc.Keys[keyIdx]
	allText := fmt.Sprintf("Type:%v/Script:%s/Threshold:%d/Keys:%d/Key%d:%s", desc.Type, strings.Replace(desc.Script.String(), " ", "", -1), desc.Threshold, len(desc.Keys), keyIdx+1, key.String())
	lines := pdf.SplitText(allText, 75.0) // 65mm width within 10mm margins
	y := 10.0                             // distance from top edge
	for _, line := range lines {
		pdf.Text(5.0, y, line)
		y += 5.0 // Adjust line spacing
	}

	// pdf.SetFont(fontName, "", 8)
	// pdf.SetXY(4.0, 10.0)
	// key := desc.Keys[keyIdx]
	// allText := fmt.Sprintf("Type:%v/Script:%s/Threshold:%d/Keys:%d\nKey%d:%s", desc.Type, strings.Replace(desc.Script.String(), " ", "", -1), desc.Threshold, len(desc.Keys), keyIdx+1, key.String())
	// strings.TrimRight(allText, "\r\n")                // UTF-8 bug fix to removing trailing newlines
	// pdf.MultiCell(77.0, 4.0, allText, "", "W", false) // "W" for word wrap, no hyphens

	// QR at bottom, 10mm from edge
	qrContent := createDescriptorQR(desc)
	if len(qrContent) == 0 {
		return nil, fmt.Errorf("failed to generate descriptor QR: empty content")
	}
	qrCode, err := qr.Encode(qrContent, qr.M)
	if err != nil {
		return nil, fmt.Errorf("failed to encode descriptor QR: %v", err)
	}
	textHeight := int(y)
	availableHeight := int(plateSize - float64(textHeight) - 5.0) // 5mm top + 5mm bottom margin
	qrSize := min(availableHeight, 75)                            // Max 75mm within 10mm margins
	qrX := (plateSize - float64(qrSize)) / 2                      // Left margin
	qrY := plateSize - float64(qrSize) - 5
	pixelSize := float64(qrSize) / float64(qrCode.Size)
	fmt.Printf("pixelSize: %f /", pixelSize)
	for y := 0; y < qrCode.Size; y++ {
		for x := 0; x < qrCode.Size; x++ {
			if qrCode.Black(x, y) {
				startX := qrX + (float64(x) * pixelSize)
				startY := qrY + (float64(y) * pixelSize)
				pdf.Rect(startX, startY, pixelSize, pixelSize, "F")
			}
		}
	}

	return pdf, nil
}

// createDescriptorQR constructs a QR string for the descriptor.
func createDescriptorQR(desc *urtypes.OutputDescriptor) string {
	if desc == nil {
		return ""
	}
	parts := []string{fmt.Sprintf("Type: %v", desc.Type)}
	parts = append(parts, fmt.Sprintf("Script: %s", desc.Script.String()))
	parts = append(parts, fmt.Sprintf("Threshold: %d", desc.Threshold))
	parts = append(parts, fmt.Sprintf("Keys: %d", len(desc.Keys)))
	for i, key := range desc.Keys {
		parts = append(parts, fmt.Sprintf("Key %d: %s", i+1, key.String()))
	}
	return strings.Join(parts, "\n")
}

// PrintPDF generates individual 85x85mm PDFs and returns paths to the generated plates.
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
	} else if totalShares > 1 {
		totalShares = 1
		logutil.DebugLog("No descriptor, forcing totalShares to %d (singlesig)", totalShares)
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
			logutil.DebugLog("Generating descriptor plate %d", i+1)
			pdf, err := createDescriptorPlate(desc, keyIdx, i+1, totalShares)
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

// createPageLayout merges the generated plates into an A4 PDF with a 2x3 layout and writes to w.
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
