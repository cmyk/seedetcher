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
	"seedetcher.com/font/vector"
	"seedetcher.com/seedqr"
)

type PaperSize string

const (
	PaperA4     PaperSize = "A4"
	PaperLetter PaperSize = "Letter"
)

// Load SeedEtcher’s vector font
var fontFace = vector.NewFace(loadFontData("vector_font.bin"))

// Load font binary data
func loadFontData(fontPath string) []byte {
	data, err := os.ReadFile(fontPath)
	if err != nil {
		log.Fatalf("Failed to load font: %v", err)
	}
	return data
}

// **Main PDF Generator**
func PrintPDF(w io.Writer, mnemonicStr string, paperSize PaperSize) error {
	var paperWidth, paperHeight float64
	switch paperSize {
	case PaperLetter:
		paperWidth, paperHeight = 216.0, 279.0
	case PaperA4:
		paperWidth, paperHeight = 210.0, 297.0
	default:
		return fmt.Errorf("unsupported paper size: %s", paperSize)
	}

	pdf := gofpdf.New("P", "mm", string(paperSize), "")
	pdf.AddPage()
	pdf.SetMargins(0, 0, 0) // Prevent clipping
	pdf.SetLineWidth(0.1)

	plateSize := 85.0
	plateX, plateY := (paperWidth-plateSize)/2, (paperHeight-plateSize)/2
	log.Printf("Plate Position: X=%.1f, Y=%.1f, Size=%.1fmm", plateX, plateY, plateSize)

	// **Parse Mnemonic**
	mnemonicWords := strings.Fields(mnemonicStr)
	if len(mnemonicWords) != 24 {
		return fmt.Errorf("mnemonic must contain exactly 24 words, got %d", len(mnemonicWords))
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

	// **Render Metadata**
	drawVectorText(pdf, plateX+5.0, plateY+3.0, "1/1")
	drawVectorText(pdf, plateX+37.0, plateY+3.0, "557CB555")
	drawVectorText(pdf, plateX+75.0, plateY+3.0, "V1")

	// **Render Seed Words**
	col1X, col2X := 5.0, 50.0
	wordY := 10.0
	for i, word := range mnemonic {
		wordStr := strings.ToUpper(bip39.LabelFor(word))
		xPos := col1X
		if i >= 12 {
			xPos = col2X
		}
		drawVectorText(pdf, plateX+xPos, plateY+wordY, fmt.Sprintf("%2d %s", i+1, wordStr))
		if i == 11 {
			wordY = 10.0
		} else {
			wordY += 5.5
		}
	}

	// **Render QR Code**
	qrContent := seedqr.QR(mnemonic)
	if len(qrContent) == 0 {
		return fmt.Errorf("failed to generate QR: empty content")
	}
	qrCode, err := qr.Encode(string(qrContent), qr.M)
	if err != nil {
		return fmt.Errorf("failed to encode QR: %v", err)
	}

	qrSize := 25.0
	qrX := plateX + (plateSize-qrSize)/2
	qrY := plateY + 35.0
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

	// **Bottom Title**
	drawVectorText(pdf, plateX+30.0, plateY+82.0, "SATOSH'S STASH")

	return pdf.Output(w)
}

// **Helper Function: Draw Text Using SeedEtcher’s Vector Font**
func drawVectorText(pdf *gofpdf.Fpdf, x, y float64, text string) {
	cursorX := x

	for _, r := range text {
		advance, segments, found := fontFace.Decode(r)
		if !found {
			continue
		}

		// Start a path for the glyph
		pdf.SetLineWidth(0.5)
		pdf.SetDrawColor(0, 0, 0)
		pdf.MoveTo(cursorX, y)

		// Iterate over segments
		for {
			segment, hasMore := segments.Next()
			if !hasMore {
				break
			}

			switch segment.Op {
			case vector.SegmentOpMoveTo:
				pdf.MoveTo(cursorX+float64(segment.Arg.X), y+float64(segment.Arg.Y))
			case vector.SegmentOpLineTo:
				pdf.LineTo(cursorX+float64(segment.Arg.X), y+float64(segment.Arg.Y))
			}
		}

		pdf.Stroke()

		// Advance cursor for the next glyph
		cursorX += float64(advance)
	}
}
