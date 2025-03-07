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
	if len(mnemonics) != 3 {
		return nil, nil, "", fmt.Errorf("expected exactly 3 mnemonics, got %d", len(mnemonics))
	}

	// Set totalShares based on descriptor keys, default to mnemonics if no descriptor
	totalShares := len(mnemonics)
	if desc != nil && len(desc.Keys) > 0 {
		totalShares = len(desc.Keys)
		logutil.DebugLog("Calculated totalShares: %d based on desc.Keys length: %d", totalShares, len(desc.Keys))
	} else {
		logutil.DebugLog("Descriptor is nil or Keys is empty, defaulting totalShares to %d (number of mnemonics)", totalShares)
	}

	tempDir := "/tmp/seedetcher-plates-test"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, nil, "", fmt.Errorf("failed to create temp directory %s: %v", tempDir, err)
	}
	logutil.DebugLog("Using directory: %s", tempDir)

	// Generate seed and descriptor plates for each unique share
	seedPaths := make([]string, totalShares)
	descPaths := make([]string, totalShares)
	for i := 0; i < totalShares; i++ {
		mnemonic := mnemonics[i%len(mnemonics)] // Cycle through mnemonics if totalShares > 3
		logutil.DebugLog("Generating seed plate %d", i+1)
		seedPDF, buf, err := createSeedPlate(mnemonic, i+1, totalShares)
		if err != nil {
			return nil, nil, "", fmt.Errorf("failed to create seed plate %d: %v", i+1, err)
		}
		seedFile := filepath.Join(tempDir, fmt.Sprintf("seed_%d.pdf", i))
		if buf.Len() == 0 {
			return nil, nil, "", fmt.Errorf("seed PDF %d is empty after Output", i+1)
		}
		if err := os.WriteFile(seedFile, buf.Bytes(), 0644); err != nil {
			return nil, nil, "", fmt.Errorf("failed to write seed PDF %d to %s: %v", i+1, seedFile, err)
		}
		logutil.DebugLog("Generated seed plate %d at %s, size: %d bytes", i+1, seedFile, buf.Len())
		logutil.DebugLog("First 100 bytes of seed_%d.pdf: %x", i, buf.Bytes()[:min(100, buf.Len())])
		seedPDF.Close()
		seedPaths[i] = seedFile

		// Generate descriptor plate if desc exists
		if desc != nil && len(desc.Keys) > 0 {
			logutil.DebugLog("Generating descriptor plate %d", i+1)
			var descFile string
			var buf bytes.Buffer
			descKeyIdx := i % len(desc.Keys) // Cycle keyIdx if totalShares > len(desc.Keys)
			descPDF, err := createDescriptorPlate(desc, descKeyIdx, i+1, totalShares)
			if err != nil {
				return nil, nil, "", fmt.Errorf("failed to create descriptor plate %d: %v", i+1, err)
			}
			if err := descPDF.Output(&buf); err != nil {
				return nil, nil, "", fmt.Errorf("failed to generate descriptor PDF %d: %v", i+1, err)
			}
			if buf.Len() == 0 {
				return nil, nil, "", fmt.Errorf("descriptor PDF %d is empty after Output", i+1)
			}
			descFile = filepath.Join(tempDir, fmt.Sprintf("desc_%d.pdf", i))
			if err := os.WriteFile(descFile, buf.Bytes(), 0644); err != nil {
				return nil, nil, "", fmt.Errorf("failed to write descriptor PDF %d to %s: %v", i+1, descFile, err)
			}
			if info, err := os.Stat(descFile); err != nil {
				return nil, nil, "", fmt.Errorf("failed to stat descriptor PDF %d at %s: %v", i+1, descFile, err)
			} else {
				logutil.DebugLog("Generated descriptor plate %d at %s, size: %d bytes", i+1, descFile, info.Size())
				logutil.DebugLog("First 100 bytes of desc_%d.pdf: %x", i, buf.Bytes()[:min(100, buf.Len())])
			}
			descPaths[i] = descFile
		} else {
			descPaths[i] = "" // Blank slot if no descriptor
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

	// Map paperFormat to dimensions
	var pageSize string
	switch paperFormat {
	case PaperA4:
		pageSize = "A4"
	case PaperLetter:
		pageSize = "Letter"
	default:
		return fmt.Errorf("unsupported paper size: %v", paperFormat)
	}

	// Paginate into 2x3 grids (6 slots per page)
	slotsPerPage := 6       // 2x3 grid
	numPages := (3 + 2) / 3 // Always 1 page for singlesig (3 shares), more for multisig
	logutil.DebugLog("Total shares: %d, generating %d pages", totalShares, numPages)

	for page := 0; page < numPages; page++ {
		// Determine slots for this page (always 3 shares for singlesig, more for multisig)
		startIdx := 0                 // Singlesig uses first plate, duplicated
		endIdx := min(3, totalShares) // 3 shares per page for singlesig, actual shares for multisig
		pageShares := endIdx - startIdx
		logutil.DebugLog("Page %d: shares %d to %d (%d shares)", page+1, startIdx+1, endIdx, pageShares)

		// Arrange files for this page: duplicate first seed and desc for singlesig, use all for multisig
		allFiles := make([]string, 0, slotsPerPage)
		if totalShares == 1 { // Singlesig case, duplicate first plate
			for i := 0; i < 3; i++ {
				allFiles = append(allFiles, seedPaths[0]) // Repeat seed 3 times
				allFiles = append(allFiles, descPaths[0]) // Repeat desc 3 times (or "" if no desc)
			}
		} else { // Multisig case, use actual shares
			for i := startIdx; i < endIdx; i++ {
				allFiles = append(allFiles, seedPaths[i]) // Left column: seed
				allFiles = append(allFiles, descPaths[i]) // Right column: desc
			}
		}
		// Pad with empty slots if needed
		for i := len(allFiles); i < slotsPerPage; i++ {
			allFiles = append(allFiles, "")
		}
		logutil.DebugLog("Page %d files: %v", page+1, allFiles)

		// Concatenate PDFs for this page with cleanup
		tempConcatFile := filepath.Join(tempDir, fmt.Sprintf("concat_page_%d.pdf", page))
		if err := api.MergeCreateFile(allFiles, tempConcatFile, false, nil); err != nil {
			os.Remove(tempConcatFile) // Clean up on error
			logutil.DebugLog("Failed to merge PDFs for page %d: %v", page+1, err)
			return fmt.Errorf("failed to merge PDFs for page %d: %v", page+1, err)
		}
		defer os.Remove(tempConcatFile)
		if info, err := os.Stat(tempConcatFile); err == nil {
			logutil.DebugLog("Size of concat_page_%d.pdf: %d bytes", page, info.Size())
		} else {
			logutil.DebugLog("Failed to stat concat_page_%d.pdf: %v", page, err)
		}

		// Arrange in 2x3 grid
		tempNUpFile := filepath.Join(tempDir, fmt.Sprintf("nup_page_%d.pdf", page))
		nupConfig := model.DefaultNUpConfig()
		nupConfig.PageSize = pageSize
		nupConfig.Grid = &types.Dim{Width: 2, Height: 3} // 3 rows, 2 columns
		nupConfig.UserDim = true                         // Force page size to be respected
		nupConfig.PageDim = types.PaperSize[pageSize]    // Set A4/Letter dimensions
		if err := api.NUpFile([]string{tempConcatFile}, tempNUpFile, nil, nupConfig, nil); err != nil {
			os.Remove(tempNUpFile) // Clean up on error
			logutil.DebugLog("Failed to create NUp layout for page %d: %v", page+1, err)
			return fmt.Errorf("failed to create NUp layout for page %d: %v", page+1, err)
		}
		defer os.Remove(tempNUpFile)
		if info, err := os.Stat(tempNUpFile); err == nil {
			logutil.DebugLog("Size of nup_page_%d.pdf: %d bytes", page, info.Size())
		} else {
			logutil.DebugLog("Failed to stat nup_page_%d.pdf: %v", page, err)
		}

		// Append to w
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
