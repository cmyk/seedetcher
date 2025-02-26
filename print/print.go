// package print handles generating PDF output for printing seed phrases and QR codes based on engraving plans.
package print

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/jung-kurt/gofpdf"
	"seedetcher.com/backup"
	"seedetcher.com/bip39"
	"seedetcher.com/engrave"
	"seedetcher.com/font/constant"
	"seedetcher.com/seedqr"

	"image/png"

	"github.com/kortschak/qr"
)

type PaperSize string

const (
	PaperA4     PaperSize = "A4"     // 210mm x 297mm
	PaperLetter PaperSize = "Letter" // 216mm x 279mm
)

func GeneratePDF(outputPath string, plan engrave.Plan, mnemonic bip39.Mnemonic, title string, numPlates int, paperSize PaperSize) error {
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
	pdf.SetFont("Helvetica", "", 12)

	plateSize := 85.0
	margin := 5.0

	for i := 0; i < numPlates; i++ {
		plateX := (paperWidth - plateSize) / 2
		plateY := (paperHeight - plateSize) / 2

		seedWords := extractSeedWordsFromPlan(plan)
		qrData := seedqr.QR(mnemonic)

		y := plateY + margin
		for _, word := range seedWords {
			pdf.Text(plateX+margin, y, word)
			y += 5.0
		}

		qrCode, _ := qr.Encode(string(qrData), qr.M)
		buf := new(bytes.Buffer)
		png.Encode(buf, qrCode.Image())
		pdf.RegisterImageReader("qr.png", "PNG", buf)
		pdf.Image("qr.png", plateX+20, plateY+60, 40, 0, false, "", 0, "")

		if title != "" {
			pdf.SetFont("Helvetica", "", 8)
			pdf.Text(plateX, plateY+80, strings.ToUpper(title))
			pdf.SetFont("Helvetica", "", 12)
		}
	}

	return pdf.OutputFileAndClose(outputPath)
}

func extractSeedWordsFromPlan(plan engrave.Plan) []string {
	var words []string
	for cmd := range plan {
		if cmd.Line && cmd.Coord.Y < 1000 { // Detects seed words by engraving position
			wordIdx := len(words) + 1
			word := fmt.Sprintf("%d %s", wordIdx, bip39.LabelFor(bip39.Word(wordIdx)))
			words = append(words, strings.ToUpper(word))
		}
	}
	return words
}

func PrintPCL(w io.Writer, mnemonic bip39.Mnemonic, qrData []byte, numPlates int, paperSize PaperSize) error {
	// Validate numPlates
	if numPlates < 1 || numPlates > 4 {
		return fmt.Errorf("numPlates must be between 1 and 4, got %d", numPlates)
	}

	// Generate QR data from mnemonic if not provided
	if len(qrData) == 0 {
		qrData = seedqr.QR(mnemonic)
		if len(qrData) == 0 {
			return fmt.Errorf("failed to generate QR data for mnemonic %v", mnemonic)
		}
	}

	// Generate engraving plan for the seed using backup.EngraveSeed for SquarePlate
	plate := backup.Seed{
		Title:             "SATOSHI STASH", // Match SeedHammer title
		KeyIdx:            0,
		Mnemonic:          mnemonic,
		Keys:              1,
		MasterFingerprint: 0x12345678, // Example, adjust as needed
		Font:              constant.Font,
		Size:              backup.SquarePlate, // Use 85x85mm plate
	}
	params := engrave.Params{Millimeter: 100} // 1mm = 100 units
	log.Printf("QR Data: %s", string(qrData))
	plan, err := backup.EngraveSeed(params, plate)
	if err != nil {
		return fmt.Errorf("failed to generate engraving plan: %v", err)
	}

	// Generate PDF from the plan, mnemonic, title, number of plates, and paper size
	outputPath := "/home/cmyk/PDF/test.pdf"
	if err := GeneratePDF(outputPath, plan, mnemonic, plate.Title, numPlates, paperSize); err != nil {
		return err
	}

	// Optionally, print directly to printer if available
	if printer, err := os.OpenFile("/dev/usb/lp0", os.O_WRONLY, 0); err == nil {
		defer printer.Close()
		pdfData, err := os.ReadFile(outputPath)
		if err != nil {
			return fmt.Errorf("failed to read PDF: %v", err)
		}
		if _, err := printer.Write(pdfData); err != nil {
			return fmt.Errorf("failed to print PDF: %v", err)
		}
		log.Printf("Sent PDF to printer at /dev/usb/lp0")
	}
	return nil
}
