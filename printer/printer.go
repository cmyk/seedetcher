package printer

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/jung-kurt/gofpdf"
	"github.com/kortschak/qr"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/logutil"
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
	logutil.DebugLog("Attempting to load font from %s", fontPath)
	data, err := os.ReadFile(fontPath)
	if err != nil {
		logutil.DebugLog("Failed to load font: %v", err)
		return nil // Return nil instead of crashing
	}
	logutil.DebugLog("Font data loaded, size: %d bytes", len(data))
	return data
}

// PrintPDF renders the backup plate layout as a PDF with fixed positions for seed words, QR, and metadata, matching SeedHammer style.
func printPDFToBuffer(w io.Writer, mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paperFormat PaperSize) error {
	var paperWidth, paperHeight float64
	switch paperFormat {
	case PaperA4:
		paperWidth, paperHeight = 216.0, 279.0 // 8.5x11 inches in mm
	case PaperLetter:
		paperWidth, paperHeight = 210.0, 297.0 // A4 in mm
	default:
		logutil.DebugLog("unsupported paper size: %s", paperFormat)
	}
	logutil.DebugLog("Creating PDF with size %fx%f mm", paperWidth, paperHeight)
	pdf := gofpdf.New("P", "mm", string(paperFormat), "")
	pdf.AddPage()
	logutil.DebugLog("Page added")
	pdf.SetMargins(0, 0, 0) // Prevent clipping
	pdf.SetLineWidth(0.2)   // Thicker border for visibility

	// Try to load and add the Martian Mono font as UTF-8 TrueType
	var fontName string = "MartianMono"
	fontData := loadFontData(martianMono)
	if fontData == nil {
		logutil.DebugLog("Font data is nil, falling back to Courier")
		pdf.SetFont("Courier", "", 8) // Fallback to built-in font
	} else {
		pdf.AddUTF8FontFromBytes(fontName, "", fontData)
		if pdf.Err() {
			logutil.DebugLog("Font load failed: %v", pdf.Error())
			pdf.SetFont("Courier", "", 8) // Fallback on error
		} else {
			logutil.DebugLog("Font loaded")
			pdf.SetFont(fontName, "", 8)
		}
	}

	plateSize := 85.0                                                     // 85x85mm plate
	plateX, plateY := (paperWidth-plateSize)/2, (paperHeight-plateSize)/2 // Center on page
	pdf.Rect(plateX, plateY, plateSize, plateSize, "D")                   // Add visible border with 5mm margins

	// Calculate fingerprint for the mnemonic using seedetcher.com/bip39 and btcd
	var words []string
	for _, w := range mnemonic {
		if w != -1 { // Skip placeholder values
			words = append(words, bip39.LabelFor(w))
		}
	}
	seed := bip39.MnemonicSeed(mnemonic, "") // Corrected to handle single return value
	if seed == nil {                         // Check if seed is nil or handle error appropriately
		return fmt.Errorf("invalid mnemonic: failed to generate seed")
	}

	// Generate master key from seed using BIP-32 (btcd)
	masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		return fmt.Errorf("failed to derive master key: %v", err)
	}

	// Derive the master public key
	masterPubKey, err := masterKey.Neuter()
	if err != nil {
		return fmt.Errorf("failed to derive master public key: %v", err)
	}

	// Calculate fingerprint (first 4 bytes of HASH160 of the public key)
	pubKey, err := masterPubKey.ECPubKey() // Use ECPubKey instead of SerializedPubKey
	if err != nil {
		return fmt.Errorf("failed to get public key: %v", err)
	}
	fingerprint := btcutil.Hash160(pubKey.SerializeCompressed())[:4] // Use SerializeCompressed for public key
	fingerprintHex := fmt.Sprintf("%X", fingerprint)                 // Convert to uppercase hex

	// Set font for metadata/title (5pt, matching "SATOSHI'S STASH")
	pdf.SetFont(fontName, "", 5) // 5pt for metadata/title

	// Render metadata (top, 5mm margins, with calculated fingerprint)
	pdf.Text(plateX+5.0, plateY+5.0, "1/1")           // Page (top-left, 5mm margin)
	pdf.Text(plateX+40.0, plateY+5.0, fingerprintHex) // Center fingerprint (40mm from left, 5mm margin)
	pdf.Text(plateX+70.0, plateY+5.0, "V1")           // Version (top-right, 5mm margin)

	// Reset font for seed words
	pdf.SetFont(fontName, "", 8) // 8pt for words

	// Render seed words (16 on left, 8 on right for 24 words, or 12 on left for 12 words, 5mm margin, 4mm spacing, stacked vertically, moved down by 5mm)
	wordYLeft := plateY + 15.0  // Start left column 15mm from top (5mm margin + 10mm padding, down by 5mm)
	wordYRight := plateY + 15.0 // Start right column at same Y for alignment, down by 5mm
	for i, word := range mnemonic {
		if word == -1 { // Skip placeholder values
			continue
		}
		wordStr := strings.ToUpper(bip39.LabelFor(word))
		if i < 16 { // First 16 words on left column
			xPos := plateX + 12.0 // Left column, 5mm from left (equal margin)
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

	// Render descriptor data if provided (optional, for metadata or title)
	if desc != nil {
		// Add descriptor title or key info (e.g., keyIdx) as metadata or title if applicable
		descTitle := desc.Title
		if descTitle == "" {
			descTitle = fmt.Sprintf("Share %d/%d", keyIdx+1, len(desc.Keys))
		}
		pdf.Text(plateX+32.0, plateY+80.0, descTitle) // Add below words, centered, 5mm from bottom
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
	qrSize := 25.0                          // 25mm, matching your previous request
	qrX := plateX + 45.0                    // Align left edge of QR with left edge of right column’s words (45mm from left)
	qrY := plateY + plateSize - qrSize - 10 // Align bottom of QR with bottom
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
	logutil.DebugLog("Preparing to write PDF")
	err = pdf.Output(w) // Fixed: Use = instead of :=
	logutil.DebugLog("PDF write completed with err: %v", err)
	return err
}

// Update PrintPDF to convert based on capabilities
func PrintPDF(w io.Writer, mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paperFormat PaperSize, supportsPCL, supportsPostScript bool) error {
	var buf bytes.Buffer
	if err := printPDFToBuffer(&buf, mnemonic, desc, keyIdx, paperFormat); err != nil {
		return err
	}

	if supportsPostScript {
		cmd := exec.Command("pdftops", "-", "-") // Convert PDF to PostScript
		cmd.Stdin = &buf
		cmd.Stdout = w
		return cmd.Run()
	} else {
		cmd := exec.Command("gs", "-dBATCH", "-dNOPAUSE", "-sDEVICE=pcld5", "-sOutputFile=-", "-") // Convert PDF to PCL5
		cmd.Stdin = &buf
		cmd.Stdout = w
		return cmd.Run()
	}
}
