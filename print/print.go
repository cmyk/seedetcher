package print

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/jung-kurt/gofpdf"
	"github.com/kortschak/qr"
	"seedetcher.com/bip39"
	"seedetcher.com/seedqr"
)

type PaperSize string

const (
	PaperA4     PaperSize = "A4"
	PaperLetter PaperSize = "Letter"
)

// Load Fonts
var martianMono = "font/martianmono/MartianMono_Condensed-Regular.ttf" // Path to the UTF-8 TrueType font file

// Load font binary data
func loadFontData(fontPath string) []byte {
	data, err := os.ReadFile(fontPath)
	if err != nil {
		log.Fatalf("Failed to load font from %s: %v", fontPath, err)
	}
	return data
}

// PrintPDF renders the backup plate layout as a PDF with fixed positions for seed words, QR, and metadata, matching SeedHammer style.
func PrintPDF(w io.Writer, mnemonicStr string, paperSize PaperSize) error {
	var paperWidth, paperHeight float64
	switch paperSize {
	case PaperLetter:
		paperWidth, paperHeight = 216.0, 279.0 // 8.5x11 inches in mm
	case PaperA4:
		paperWidth, paperHeight = 210.0, 297.0 // A4 in mm
	default:
		return fmt.Errorf("unsupported paper size: %s", paperSize)
	}

	pdf := gofpdf.New("P", "mm", string(paperSize), "")
	pdf.AddPage()
	pdf.SetMargins(0, 0, 0) // Prevent clipping
	pdf.SetLineWidth(0.2)   // Thicker border for visibility

	// Try to load and add the Martian Mono font as UTF-8 TrueType
	var fontName string = "MartianMono"
	fontData := loadFontData(martianMono)
	pdf.AddUTF8FontFromBytes(fontName, "", fontData) // Call directly
	if pdf.Err() {                                   // Check for error using pdf.Err() as bool
		log.Printf("Failed to add MartianMono font as UTF-8: %v. Falling back to Courier.", pdf.Error())
		fontName = "Courier"
		pdf.SetFont(fontName, "", 8) // 8pt for words
	} else {
		pdf.SetFont(fontName, "", 8) // Use MartianMono at 8pt for words
	}

	plateSize := 85.0                                                     // 85x85mm plate
	plateX, plateY := (paperWidth-plateSize)/2, (paperHeight-plateSize)/2 // Center on page
	pdf.Rect(plateX, plateY, plateSize, plateSize, "D")                   // Add visible border with 5mm margins

	// Parse mnemonic (expecting 12 or 24 words)
	mnemonicWords := strings.Fields(mnemonicStr)
	if len(mnemonicWords) != 12 && len(mnemonicWords) != 24 {
		return fmt.Errorf("mnemonic must contain 12 or 24 words, got %d", len(mnemonicWords))
	}
	mnemonic := make(bip39.Mnemonic, len(mnemonicWords))
	for i, w := range mnemonicWords {
		word, ok := bip39.ClosestWord(w)
		if !ok {
			return fmt.Errorf("invalid word: %s", w)
		}
		mnemonic[i] = word
	}
	if !mnemonic.Valid() {
		return fmt.Errorf("invalid mnemonic")
	}

	// Set font for metadata/title (5pt, matching "SATOSHI'S STASH")
	pdf.SetFont(fontName, "", 5) // 5pt for metadata/title

	// Render metadata (top, 5mm margins, matching your image)
	pdf.Text(plateX+5.0, plateY+5.0, "1/1")       // Page (top-left, 5mm margin)
	pdf.Text(plateX+40.0, plateY+5.0, "557CB555") // Center checksum (40mm from left, 5mm margin)
	pdf.Text(plateX+70.0, plateY+5.0, "V1")       // Version (top-right, 5mm margin)

	// Reset font for seed words
	pdf.SetFont(fontName, "", 8) // 8pt for words

	// Render seed words (16 on left, 8 on right for 24 words, 5mm margin, 4mm spacing, stacked vertically, moved down by 5mm)
	wordYLeft := plateY + 13.0  // Start left column 15mm from top (5mm margin + 10mm padding, down by 5mm)
	wordYRight := plateY + 13.0 // Start right column at same Y for alignment, down by 5mm
	for i, word := range mnemonic {
		wordStr := strings.ToUpper(bip39.LabelFor(word))
		if i < 16 { // First 16 words on left column
			xPos := plateX + 15.0 // Left column, 5mm from left (equal margin)
			pdf.Text(xPos, wordYLeft, fmt.Sprintf("%2d %s", i+1, wordStr))
			if i < 15 { // Increment for left column, excluding last word
				wordYLeft += 4.0 // 4mm spacing
			}
		} else if len(mnemonic) == 24 { // Remaining 8 words on right column for 24 words, stacked vertically
			xPos := plateX + 45.0 // Right column, 45mm from left (maintain 5mm padding from right)
			pdf.Text(xPos, wordYRight, fmt.Sprintf("%2d %s", i+1, wordStr))
			if i < 23 { // Increment for right column, excluding last word
				wordYRight += 4.0 // 4mm spacing, matching left column
			}
		}
	}

	// Render QR code (bottom right corner, aligned with right column’s words left edge, bottom aligned with left column’s bottom, 25mm size, moved down by 5mm)
	qrContent := seedqr.QR(mnemonic)
	if len(qrContent) == 0 {
		return fmt.Errorf("failed to generate QR: empty content")
	}
	qrCode, err := qr.Encode(string(qrContent), qr.M) // Medium error correction
	if err != nil {
		return fmt.Errorf("failed to encode QR: %v", err)
	}
	qrSize := 25.0              // 25mm, matching your previous request
	qrX := plateX + 45.0        // Align left edge of QR with left edge of right column’s words (45mm from left)
	qrY := (wordYLeft - qrSize) // Align bottom of QR with bottom of left column’s last word (wordYLeft for 16th word), moved down by 5mm
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

	// Render title (bottom, centered, 5mm margin, matching "SATOSHI'S STASH")
	pdf.SetFont(fontName, "", 5)                                                                                  // 5pt for title
	pdf.Text(plateX+(plateSize-pdf.GetStringWidth("SATOSHI'S STASH"))/2, plateY+plateSize-5.0, "SATOSHI'S STASH") // Bottom-center, 5mm margin

	return pdf.Output(w)
}
