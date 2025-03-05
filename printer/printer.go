package printer

import (
	"bytes"
	"fmt"
	"io"
	"os"
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

// PrintPDF generates a single 85x85mm PDF plate for a seed phrase, centered on A4/Letter.
func PrintPDF(w io.Writer, mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paperFormat PaperSize, supportsPCL, supportsPostScript bool) error {
	logutil.DebugLog("Starting PrintPDF for single seed plate")
	pdf, err := CreateSeedPlatePDF(mnemonic, desc, keyIdx, paperFormat)
	if err != nil {
		return fmt.Errorf("failed to create seed plate PDF: %v", err)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return fmt.Errorf("failed to generate PDF: %v", err)
	}
	_, err = w.Write(buf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to write PDF to printer: %v", err)
	}
	return nil
}

// CreateSeedPlatePDF generates a single 85x85mm PDF plate for a seed phrase, centered on A4/Letter.
func CreateSeedPlatePDF(mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paperFormat PaperSize) (*gofpdf.Fpdf, error) {
	pdf := gofpdf.New("P", "mm", string(paperFormat), "")
	pdf.AddPage()
	pdf.SetMargins(0, 0, 0)
	pdf.SetLineWidth(0.2)

	var fontName string = "MartianMono"
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
	plateX, plateY := (210.0-plateSize)/2, (297.0-plateSize)/2
	pdf.Rect(plateX, plateY, plateSize, plateSize, "D")

	pdf.SetFont(fontName, "", 5)
	pdf.Text(plateX+5.0, plateY+5.0, "1/1")
	var words []string
	for _, w := range mnemonic {
		if w != -1 {
			words = append(words, bip39.LabelFor(w))
		}
	}
	seed := bip39.MnemonicSeed(mnemonic, "")
	if seed == nil {
		return nil, fmt.Errorf("invalid mnemonic: failed to generate seed")
	}

	masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		return nil, fmt.Errorf("failed to derive master key: %v", err)
	}
	masterPubKey, err := masterKey.Neuter()
	if err != nil {
		return nil, fmt.Errorf("failed to derive master public key: %v", err)
	}
	pubKey, err := masterPubKey.ECPubKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get public key: %v", err)
	}
	fingerprint := btcutil.Hash160(pubKey.SerializeCompressed())[:4]
	fingerprintHex := fmt.Sprintf("%X", fingerprint)
	pdf.Text(plateX+40.0, plateY+5.0, fingerprintHex)
	pdf.Text(plateX+70.0, plateY+5.0, "V1")

	pdf.SetFont(fontName, "", 8)
	wordYLeft := plateY + 15.0
	wordYRight := plateY + 15.0
	for i, word := range mnemonic {
		if word == -1 {
			continue
		}
		wordStr := strings.ToUpper(bip39.LabelFor(word))
		if i < 16 {
			xPos := plateX + 12.0
			pdf.Text(xPos, wordYLeft, fmt.Sprintf("%2d %s", i+1, wordStr))
			if i < 15 {
				wordYLeft += 4.0
			}
		} else if len(mnemonic) == 24 {
			xPos := plateX + 45.0
			pdf.Text(xPos, wordYRight, fmt.Sprintf("%2d %s", i+1, wordStr))
			if i < 23 {
				wordYRight += 4.0
			}
		}
	}

	if desc != nil {
		descTitle := desc.Title
		if descTitle == "" {
			descTitle = fmt.Sprintf("Share %d/%d", keyIdx+1, len(desc.Keys))
		}
		pdf.Text(plateX+32.0, plateY+80.0, descTitle)
	}

	qrContent := seedqr.QR(mnemonic)
	if len(qrContent) == 0 {
		return nil, fmt.Errorf("failed to generate QR: empty content")
	}
	qrCode, err := qr.Encode(string(qrContent), qr.M)
	if err != nil {
		return nil, fmt.Errorf("failed to encode QR: %v", err)
	}
	qrSize := 25.0
	qrX := plateX + 45.0
	qrY := plateY + plateSize - qrSize - 10
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

	pdf.SetFont(fontName, "", 5)
	pdf.Text(plateX+(plateSize-pdf.GetStringWidth("SATOSHI'S STASH"))/2, plateY+plateSize-5.0, "SATOSHI'S STASH")

	return pdf, nil
}
