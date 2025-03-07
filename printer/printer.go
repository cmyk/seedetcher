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

// PrintPDF generates individual 85x85mm PDFs and merges them into an A4/Letter PDF.
func CreatePlates(w io.Writer, mnemonics []bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paperFormat PaperSize, supportsPCL, supportsPostScript bool) error {
	logutil.DebugLog("Starting CreatePlates with %d mnemonics, desc=%v, keyIdx=%d, paperFormat=%s", len(mnemonics), desc != nil, keyIdx, paperFormat)
	if len(mnemonics) != 3 {
		return fmt.Errorf("expected exactly 3 mnemonics, got %d", len(mnemonics))
	}

	// Default totalShares to number of mnemonics if desc is nil
	totalShares := len(mnemonics)
	if desc != nil && len(desc.Keys) > 0 {
		totalShares = len(desc.Keys)
		logutil.DebugLog("Calculated totalShares: %d based on desc.Keys length: %d", totalShares, len(desc.Keys))
	} else {
		logutil.DebugLog("Descriptor is nil or Keys is empty, defaulting totalShares to %d (number of mnemonics)", totalShares)
	}

	tempDir := "/tmp/seedetcher-plates-test"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory %s: %v", tempDir, err)
	}
	logutil.DebugLog("Using directory: %s", tempDir)

	for i := 0; i < totalShares; i++ {
		mnemonic := mnemonics[i%len(mnemonics)] // Cycle through mnemonics if totalShares > 3
		logutil.DebugLog("Generating seed plate %d", i+1)
		seedPDF, buf, err := createSeedPlate(mnemonic, i+1, totalShares)
		if err != nil {
			return fmt.Errorf("failed to create seed plate %d: %v", i+1, err)
		}
		seedFile := filepath.Join(tempDir, fmt.Sprintf("seed_%d.pdf", i))
		if buf.Len() == 0 {
			return fmt.Errorf("seed PDF %d is empty after Output", i+1)
		}
		if err := os.WriteFile(seedFile, buf.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write seed PDF %d to %s: %v", i+1, seedFile, err)
		}
		logutil.DebugLog("Generated seed plate %d at %s, size: %d bytes", i+1, seedFile, buf.Len())
		logutil.DebugLog("First 100 bytes of seed_%d.pdf: %x", i, buf.Bytes()[:min(100, buf.Len())])
		seedPDF.Close()
	}

	numDescPlates := 0
	if desc != nil && len(desc.Keys) > 0 {
		numDescPlates = totalShares
		logutil.DebugLog("Generating %d descriptor plates for totalShares %d", numDescPlates, totalShares)
		for i := 0; i < numDescPlates; i++ {
			logutil.DebugLog("Generating descriptor plate %d", i+1)
			var descFile string
			var buf bytes.Buffer
			descPDF, err := createDescriptorPlate(desc, keyIdx, i+1, totalShares)
			if err != nil {
				return fmt.Errorf("failed to create descriptor plate %d: %v", i+1, err)
			}
			if err := descPDF.Output(&buf); err != nil {
				return fmt.Errorf("failed to generate descriptor PDF %d: %v", i+1, err)
			}
			if buf.Len() == 0 {
				return fmt.Errorf("descriptor PDF %d is empty after Output", i+1)
			}
			descFile = filepath.Join(tempDir, fmt.Sprintf("desc_%d.pdf", i))
			if err := os.WriteFile(descFile, buf.Bytes(), 0644); err != nil {
				return fmt.Errorf("failed to write descriptor PDF %d to %s: %v", i+1, descFile, err)
			}
			logutil.DebugLog("Generated descriptor plate %d at %s, size: %d bytes", i+1, descFile, buf.Len())
			logutil.DebugLog("First 100 bytes of desc_%d.pdf: %x", i, buf.Bytes()[:min(100, buf.Len())])
			descPDF.Close()
		}
	} else {
		logutil.DebugLog("Descriptor is nil or Keys is empty, skipping descriptor plate generation")
	}

	return nil
}

// createPageLayout merges the generated plates into an A4 PDF with a 2x3 layout and writes to w.
func CreatePageLayout(w io.Writer, tempDir string, paperFormat PaperSize) error {
	logutil.DebugLog("Starting CreatePageLayout with tempDir: %s, paperFormat: %s", tempDir, paperFormat)

	// Collect seed and descriptor plates separately
	var seedFiles, descFiles []string
	for i := 0; i < 3; i++ {
		seedFile := filepath.Join(tempDir, fmt.Sprintf("seed_%d.pdf", i))
		if _, err := os.Stat(seedFile); err == nil {
			seedFiles = append(seedFiles, seedFile)
		}
	}
	if _, err := os.Stat(filepath.Join(tempDir, "desc_0.pdf")); err == nil {
		files, err := filepath.Glob(filepath.Join(tempDir, "desc_*.pdf"))
		if err != nil {
			return fmt.Errorf("failed to glob descriptor files: %v", err)
		}
		descFiles = append(descFiles, files...)
	}
	logutil.DebugLog("Found %d seed files: %v", len(seedFiles), seedFiles)
	logutil.DebugLog("Found %d desc files: %v", len(descFiles), descFiles)

	if len(seedFiles) == 0 && len(descFiles) == 0 {
		logutil.DebugLog("No PDF files found to merge")
		return fmt.Errorf("no PDF files generated to merge")
	}

	// Arrange files for 2x3 grid: seed plates in left column, descriptor plates in right column
	// Position: 1 2  (seed_0, desc_0)
	//           3 4  (seed_1, desc_1)
	//           5 6  (seed_2, desc_2)
	allFiles := make([]string, 0, 6)
	for i := 0; i < 3; i++ {
		if i < len(seedFiles) {
			allFiles = append(allFiles, seedFiles[i]) // Left column: seed
		} else {
			allFiles = append(allFiles, "") // Empty slot if no seed plate
		}
		if i < len(descFiles) {
			allFiles = append(allFiles, descFiles[i]) // Right column: desc
		} else {
			allFiles = append(allFiles, "") // Empty slot if no desc plate
		}
	}
	logutil.DebugLog("Arranged files for NUp: %v", allFiles)

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

	// Concatenate all PDFs into a single temporary PDF
	tempConcatFile := filepath.Join(tempDir, "concat.pdf")
	if err := api.MergeCreateFile(allFiles, tempConcatFile, false, nil); err != nil {
		logutil.DebugLog("Failed to merge PDFs: %v", err)
		return fmt.Errorf("failed to merge PDFs: %v", err)
	}
	defer os.Remove(tempConcatFile)
	if info, err := os.Stat(tempConcatFile); err == nil {
		logutil.DebugLog("Size of concat.pdf: %d bytes", info.Size())
	} else {
		logutil.DebugLog("Failed to stat concat.pdf: %v", err)
	}

	// Arrange in 3x2 grid on one page
	tempNUpFile := filepath.Join(tempDir, "nup.pdf")
	nupConfig := model.DefaultNUpConfig()
	nupConfig.PageSize = pageSize
	nupConfig.Grid = &types.Dim{Width: 2, Height: 3} // 3 rows, 2 columns
	nupConfig.UserDim = true                         // Force page size to be respected
	nupConfig.PageDim = types.PaperSize[pageSize]    // Set A4/Letter dimensions
	if err := api.NUpFile([]string{tempConcatFile}, tempNUpFile, nil, nupConfig, nil); err != nil {
		logutil.DebugLog("Failed to create NUp layout: %v", err)
		return fmt.Errorf("failed to create NUp layout: %v", err)
	}
	defer os.Remove(tempNUpFile)
	if info, err := os.Stat(tempNUpFile); err == nil {
		logutil.DebugLog("Size of nup.pdf: %d bytes", info.Size())
	} else {
		logutil.DebugLog("Failed to stat nup.pdf: %v", err)
	}

	// Write the NUp PDF to w
	nupBytes, err := os.ReadFile(tempNUpFile)
	if err != nil {
		return fmt.Errorf("failed to read NUp PDF: %v", err)
	}
	logutil.DebugLog("nupBytes length before write: %d", len(nupBytes))
	if _, err := w.Write(nupBytes); err != nil {
		return fmt.Errorf("failed to write NUp PDF: %v", err)
	}

	logutil.DebugLog("Wrote NUp PDF to output, size: %d bytes", len(nupBytes))
	return nil
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
