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
func PrintPDF(w io.Writer, mnemonics []bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paperFormat PaperSize, supportsPCL, supportsPostScript bool) error {
	logutil.DebugLog("Starting PrintPDF with %d mnemonics, desc=%v, keyIdx=%d, paperFormat=%s", len(mnemonics), desc != nil, keyIdx, paperFormat)
	if len(mnemonics) != 3 {
		return fmt.Errorf("expected exactly 3 mnemonics, got %d", len(mnemonics))
	}

	totalShares := 1
	if desc != nil && len(desc.Keys) > 1 {
		totalShares = len(desc.Keys)
	}
	logutil.DebugLog("Total shares: %d", totalShares)

	// Use a fixed temporary directory
	tempDir := "/tmp/seedetcher-plates-test"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory %s: %v", tempDir, err)
	}
	logutil.DebugLog("Using directory: %s", tempDir)

	// Generate and save seed plate PDFs
	for i, mnemonic := range mnemonics {
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
		// Avoid memory leak by closing the PDF
		seedPDF.Close()
	}

	// Generate and save descriptor plate PDFs
	for i := 0; i < 3; i++ {
		var descFile string
		var buf bytes.Buffer
		if desc != nil {
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
			descPDF.Close()
		} else {
			pdf := gofpdf.NewCustom(&gofpdf.InitType{
				UnitStr: "mm",
				Size:    gofpdf.SizeType{Wd: 85, Ht: 85},
			})
			pdf.AddPage()
			pdf.SetMargins(0, 0, 0)
			pdf.SetLineWidth(0.2)
			pdf.Rect(0, 0, 85.0, 85.0, "D")
			pdf.SetFont("Courier", "", 8)
			pdf.Text(5.0, 5.0, "No Descriptor")
			if err := pdf.Output(&buf); err != nil {
				return fmt.Errorf("failed to generate placeholder PDF %d: %v", i+1, err)
			}
			if buf.Len() == 0 {
				return fmt.Errorf("placeholder PDF %d is empty after Output", i+1)
			}
			descFile = filepath.Join(tempDir, fmt.Sprintf("desc_%d.pdf", i))
			if err := os.WriteFile(descFile, buf.Bytes(), 0644); err != nil {
				return fmt.Errorf("failed to write placeholder PDF %d to %s: %v", i+1, descFile, err)
			}
			pdf.Close()
		}
		logutil.DebugLog("Generated descriptor plate %d at %s, size: %d bytes", i+1, descFile, buf.Len())
		logutil.DebugLog("First 100 bytes of desc_%d.pdf: %x", i, buf.Bytes()[:min(100, buf.Len())])
	}

	return nil
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
