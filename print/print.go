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
	// Define paper dimensions (in mm)
	var paperWidth, paperHeight float64
	switch paperSize {
	case PaperLetter:
		paperWidth, paperHeight = 216.0, 279.0 // 8.5" x 11"
	case PaperA4:
		paperWidth, paperHeight = 210.0, 297.0 // A4
	default:
		return fmt.Errorf("unsupported paper size: %s", paperSize)
	}

	// Create a new PDF document with the chosen paper size
	pdf := gofpdf.New("P", "mm", string(paperSize), "") // Portrait, mm, A4 or Letter
	pdf.AddPage()

	// Set font (use Helvetica, a default font; adjust size as needed to match SeedHammer)
	pdf.SetFont("Helvetica", "", 12) // Adjust size (e.g., 4.1mm = ~12pt, refine if needed)

	// Plate size (85x85mm, independent of paper size)
	plateSize := 85.0
	margin := 5.0 // Margin for adjustability between plates and page edges

	// Determine grid layout for numPlates (1–4)
	var rows, cols int
	switch numPlates {
	case 1:
		rows, cols = 1, 1
	case 2:
		rows, cols = 1, 2
	case 3, 4:
		rows, cols = 2, 2
	default:
		return fmt.Errorf("numPlates must be 1, 2, 3, or 4, got %d", numPlates)
	}

	// Calculate plate dimensions and positions
	plateWidth, plateHeight := plateSize, plateSize
	totalWidth := float64(cols)*plateWidth + float64(cols+1)*margin
	totalHeight := float64(rows)*plateHeight + float64(rows+1)*margin
	startX := (paperWidth - totalWidth) / 2   // Center horizontally on paper
	startY := (paperHeight - totalHeight) / 2 // Center vertically on paper

	// Generate plates (each 85x85mm) in the grid
	for i := 0; i < numPlates; i++ {
		plateX := startX + float64(i%cols)*(plateWidth+margin)
		plateY := startY + float64(i/cols)*(plateHeight+margin)

		// Variables for this plate
		var seedWordsLeft, seedWordsRight []string
		var qrData []byte
		var qrCommands []engrave.Command // Collect QR path commands

		// Process the engraving plan for this plate (assuming plan is per plate or use mnemonic)
		for cmd := range plan {
			if cmd.Line {
				// Assume lines near the top (y < 1000 in 1/10mm units) are text (seed words)
				if cmd.Coord.Y < 1000 { // Arbitrary threshold, adjust based on layout
					// Try to infer text from coordinates (simplified heuristic)
					wordIdx := len(seedWordsLeft) + len(seedWordsRight)
					word := fmt.Sprintf("Word%d", wordIdx+1) // Placeholder, improve if possible
					if strings.Contains(word, bip39.LabelFor(0)) {
						wordText := strings.ToUpper(word)
						if len(seedWordsLeft) <= len(seedWordsRight) {
							seedWordsLeft = append(seedWordsLeft, fmt.Sprintf("%d %s", wordIdx+1, wordText))
						} else {
							seedWordsRight = append(seedWordsRight, fmt.Sprintf("%d %s", wordIdx+1, wordText))
						}
					}
				} else {
					// Assume lines lower down (y > 1000) are part of the QR code
					qrCommands = append(qrCommands, cmd)
				}
			}
		}

		// If no QR commands are found, generate QR data from the mnemonic for this plate
		if len(qrCommands) == 0 {
			// For simplicity, use the same mnemonic for all plates; adjust if plates differ
			qrData = seedqr.QR(mnemonic)
			if len(qrData) == 0 {
				log.Printf("Warning: QR data is empty for mnemonic %v", mnemonic)
				continue // Skip QR rendering if data is invalid
			}
		}

		// If no seed words are found, use the mnemonic directly with numbers in two columns
		if len(seedWordsLeft)+len(seedWordsRight) == 0 {
			for i, word := range mnemonic {
				wordText := strings.ToUpper(bip39.LabelFor(word))
				if i < 12 {
					seedWordsLeft = append(seedWordsLeft, fmt.Sprintf("%d %s", i+1, wordText))
				} else {
					seedWordsRight = append(seedWordsRight, fmt.Sprintf("%d %s", i+1, wordText))
				}
			}
		}

		// Render seed phrase in two columns, centered on 85x85mm plate with margin
		lineHeight := 5.0                       // Adjust to match SeedHammer spacing (4.1mm font size ≈ 5mm line height)
		plateWidth := 85.0                      // 85mm width per plate
		plateHeight := 85.0                     // 85mm height per plate
		colWidth := (plateWidth - 2*margin) / 2 // 37.5mm per column after margins

		// Calculate starting y for columns (center vertically on plate, accounting for margins)
		totalWords := len(seedWordsLeft) + len(seedWordsRight)
		if totalWords > 0 {
			totalHeight := float64(totalWords) * lineHeight
			startY := (plateHeight-totalHeight)/2 + plateY

			// Left column, with margin
			y := startY
			for _, word := range seedWordsLeft {
				textWidth := pdf.GetStringWidth(word)
				x := plateX + margin + (colWidth-textWidth)/2 // Center in left column with margin
				pdf.Text(x, y, word)
				y += lineHeight
			}

			// Right column, with margin
			y = startY
			for _, word := range seedWordsRight {
				textWidth := pdf.GetStringWidth(word)
				x := plateX + margin + colWidth + (colWidth-textWidth)/2 // Center in right column with margin
				pdf.Text(x, y, word)
				y += lineHeight
			}
		}

		// Render QR code below the seed phrase, with margin
		if len(qrData) > 0 {
			qrCode, err := qr.Encode(string(qrData), qr.M)
			if err != nil {
				log.Printf("Warning: failed to encode QR code for mnemonic %v: %v", mnemonic, err)
				continue // Skip QR rendering on error, but continue with other plates
			}
			buf := new(bytes.Buffer)
			if err := png.Encode(buf, qrCode.Image()); err != nil {
				log.Printf("Warning: failed to generate QR image for mnemonic %v: %v", mnemonic, err)
				continue // Skip QR rendering on error
			}
			imgInfo := pdf.RegisterImageReader("qr.png", "PNG", buf)
			if imgInfo == nil {
				log.Printf("Warning: failed to register QR image for mnemonic %v", mnemonic)
				continue // Skip QR rendering if registration fails
			}
			qrWidth := 30.0                                             // Adjust QR size to fit 85mm, e.g., 30mm width
			qrX := plateX + margin + (plateWidth-2*margin-qrWidth)/2    // Center QR horizontally on plate with margins
			qrY := plateY + 60.0 + margin                               // Position QR below seed phrase (adjust to fit 85mm height, with margin)
			pdf.Image("qr.png", qrX, qrY, qrWidth, 0, false, "", 0, "") // Use image name directly
		}

		// Render title at the bottom, centered, with margin
		if title != "" {
			pdf.SetFont("Helvetica", "", 8) // Smaller font for title, adjust as needed
			titleText := strings.ToUpper(title)
			titleWidth := pdf.GetStringWidth(titleText)
			titleX := plateX + margin + (plateWidth-2*margin-titleWidth)/2 // Center title horizontally on plate with margins
			titleY := plateY + 80.0 + margin                               // Position at bottom, adjust to fit 85mm with margin
			pdf.Text(titleX, titleY, titleText)
			pdf.SetFont("Helvetica", "", 12) // Reset font for other text
		}
	}

	// Save to file
	err := pdf.OutputFileAndClose(outputPath)
	if err != nil {
		return fmt.Errorf("failed to write PDF: %v", err)
	}
	log.Printf("Generated PDF at %s from engraving plan, mnemonic, and title with %d plates on %s paper", outputPath, numPlates, paperSize)
	return nil
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
