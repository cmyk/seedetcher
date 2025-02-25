// package print handles generating PCL output for printing seed phrases and QR codes.
package print

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"strings"

	"github.com/kortschak/qr"
	"seedetcher.com/bip39"
	"seedetcher.com/font/bitmap"
	"seedetcher.com/font/comfortaa"
)

func loadFont(fontFace *bitmap.Face, text string, fontSize int, dpi int) (*image.Gray, error) {
	scale := float64(dpi) / 300.0
	scaledFontSize := int(float64(fontSize) * scale)

	// Estimate width based on scaled font size (use 10/10 for better fit)
	img := image.NewGray(image.Rect(0, 0, len(text)*scaledFontSize*10/10, scaledFontSize))
	y := 0
	for _, r := range text {
		glyphImg, _, ok := fontFace.Glyph(r)
		if !ok {
			log.Printf("Warning: No glyph for rune %c", r)
			continue
		}
		bounds := glyphImg.Bounds()
		log.Printf("Glyph for '%c': bounds %v", r, bounds)
		for py := 0; py < bounds.Dy(); py++ {
			for px := 0; px < bounds.Dx(); px++ {
				c := glyphImg.At(px, py)
				if a, ok := c.(color.Alpha); ok {
					// Use alpha as grayscale (0 = black, 255 = white)
					gray := uint8(255 - a.A) // Invert for black-on-white
					if gray < 128 {
						img.SetGray(px, y+py, color.Gray{Y: 0}) // Black
						log.Printf("Set black pixel at (%d, %d) for '%c'", px, y+py, r)
					} else {
						img.SetGray(px, y+py, color.Gray{Y: 255}) // White
					}
				} else if g, ok := c.(color.Gray); ok {
					// Handle existing Gray pixels
					if g.Y < 128 {
						img.SetGray(px, y+py, color.Gray{Y: 0}) // Black
						log.Printf("Set black pixel at (%d, %d) for '%c'", px, y+py, r)
					} else {
						img.SetGray(px, y+py, color.Gray{Y: 255}) // White
					}
				} else {
					log.Printf("Warning: Unexpected color type for glyph at (%d, %d): %T", px, py, c)
				}
			}
		}
		// Use GlyphAdvance for accurate spacing, scaled for DPI
		adv, ok := fontFace.GlyphAdvance(r)
		if ok {
			y += int(float64(int(adv)*scaledFontSize/64) * scale)
		} else {
			y += int(float64(bounds.Dy()) * scale) // Fallback to height
		}
		log.Printf("Advanced y to %d for '%c'", y, r)
	}
	log.Printf("Loaded font for '%s': width %d, height %d", text, img.Bounds().Dx(), img.Bounds().Dy())
	return img, nil
}

func rasterToPCL(img *image.Gray, dpi int) string {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	if width == 0 || height == 0 {
		log.Printf("Warning: Empty image in rasterToPCL, width: %d, height: %d", width, height)
		return ""
	}

	// Start PCL raster with specified DPI, ensure single raster
	pcl := fmt.Sprintf("\033*r%dA", dpi/300) // 300 DPI = 1
	pcl += fmt.Sprintf("\033*t%04dW", width) // Set raster width
	log.Printf("Raster width: %d, height: %d, DPI: %d", width, height, dpi)
	// Transfer raster data, limit to A4 height (1190 units at 1/10mm), ensure valid pixels
	for y := 0; y < height && y < 1190/10; y++ {
		var row []byte
		hasBlack := false
		for x := 0; x < width; x += 8 {
			byteVal := byte(0)
			for bit := 0; bit < 8 && x+bit < width; bit++ {
				c := img.GrayAt(x+bit, y)
				if c.Y < 128 { // Black pixel
					byteVal |= 1 << (7 - bit)
					hasBlack = true
				} else if c.Y > 128 { // Ensure white pixels are not misinterpreted
					// No action for white (255), already 0 in byteVal
				}
				// Log a sample pixel for debugging
				if x == 0 && y == 0 {
					log.Printf("Pixel at (0,0) grayscale value: %d", c.Y)
				}
			}
			if hasBlack { // Only include rows with black pixels
				row = append(row, byteVal)
			}
		}
		length := len(row)
		if length == 0 {
			log.Printf("Warning: Empty row at y=%d (skipping)", y)
			continue
		}
		lengthBytes := []byte(fmt.Sprintf("%03d", length))
		pcl += "\033*b"
		pcl += string(lengthBytes)
		pcl += "W"
		pcl += string(row)
		log.Printf("Wrote row %d, length %d, data: %v, hasBlack: %v", y, length, row, hasBlack)
	}
	pcl += "\033*rB" // End raster, no form feed
	return pcl
}

func PrintPCL(w io.Writer, mnemonic bip39.Mnemonic, qrData []byte) error {
	log.Printf("Starting PCL generation for mnemonic: %v", mnemonic)
	var words []string
	for _, w := range mnemonic {
		words = append(words, bip39.LabelFor(w))
	}
	seed := strings.Join(words, " ")
	log.Printf("Seed phrase: %s", seed)

	io.WriteString(w, "\033E") // Reset printer
	log.Printf("Wrote reset command: \\033E")
	io.WriteString(w, "\033&l26A") // A4, portrait
	log.Printf("Wrote A4 command: \\033&l26A")
	io.WriteString(w, "\033&f3") // 1/10mm units
	log.Printf("Wrote units command: \\033&f3")

	params := struct {
		Millimeter int
	}{
		Millimeter: 100, // 1mm = 100 units (1/10mm)
	}
	plateDims := image.Pt(850, 1190) // A4 in 1/10mm (width x height for portrait)
	innerMargin := params.Millimeter * 10
	seedFontSize := 20 // Increase for better visibility

	seedFont := comfortaa.Bold17

	wordsSlice := strings.Split(seed, " ")
	maxWords := len(wordsSlice)
	maxCol1, maxCol2 := 12, 12 // Default for 24 words, adjust for 12 words
	if maxWords <= 12 {
		maxCol1 = maxWords / 2
		maxCol2 = maxWords - maxCol1
	}

	// Column 1 (300 DPI, centered vertically)
	col1Y := (plateDims.Y / 2) - (maxCol1*seedFontSize*11/10)/2
	col1Cmds := wordColumnPCL(seedFont, seedFontSize, wordsSlice, 0, maxCol1, innerMargin, col1Y, 300)
	for i, cmd := range col1Cmds {
		if cmd == "" {
			log.Printf("Warning: Empty column 1 command at index %d", i)
		} else {
			log.Printf("Writing column 1 command %d: %s", i, cmd)
			io.WriteString(w, cmd)
		}
	}

	// Column 2 (300 DPI, centered vertically, right margin)
	col2X := params.Millimeter * 44
	col2Cmds := wordColumnPCL(seedFont, seedFontSize, wordsSlice, maxCol1, maxCol1+maxCol2, col2X, col1Y, 300)
	for i, cmd := range col2Cmds {
		if cmd == "" {
			log.Printf("Warning: Empty column 2 command at index %d", i)
		} else {
			log.Printf("Writing column 2 command %d: %s", i, cmd)
			io.WriteString(w, cmd)
		}
	}

	// Metadata (Version, MFP, Page, Title) at top (300 DPI, single line)
	log.Printf("Writing metadata commands...")
	metaY := params.Millimeter * 10 // Top margin
	metaTexts := []string{"V1", fmt.Sprintf("%.8x", 0x12345678), "Page 1", "SeedEtcher"}
	metaX := innerMargin
	for i, text := range metaTexts {
		metaImg, _ := loadFont(seedFont, text, seedFontSize, 300)
		metaPCL := rasterToPCL(metaImg, 300)
		if metaPCL != "" {
			io.WriteString(w, fmt.Sprintf("%s\033&a%dh%dV", metaPCL, metaX, metaY))
			log.Printf("Wrote metadata %d: %s at %d,%d", i, text, metaX, metaY)
			metaY += seedFontSize * 11 / 10 // Line spacing, ensure within A4
			if metaY > plateDims.Y-innerMargin {
				break
			}
		}
	}

	// QR code (300 DPI, centered, single page)
	qr, err := qr.Encode(string(qrData), qr.M)
	if err != nil {
		log.Printf("QR encoding failed: %v", err)
		return err
	}
	dim := qr.Size
	if dim == 0 {
		log.Printf("Warning: QR code dimensions are zero")
		return fmt.Errorf("invalid QR code size")
	}
	qrX := params.Millimeter*60 - dim*10/2 // Center horizontally
	qrY := (plateDims.Y - dim*10) / 2      // Center vertically
	io.WriteString(w, fmt.Sprintf("\033&a%dh%dV\033*r1A", qrX, qrY))
	log.Printf("Wrote QR position and raster start: \\033&a%dh%dV\\033*r1A", qrX, qrY)
	for y := 0; y < dim; y++ {
		var row []byte
		for x := 0; x < dim; x += 8 {
			byteVal := byte(0)
			for bit := 0; bit < 8 && x+bit < dim; bit++ {
				if qr.Black(x+bit, y) {
					byteVal |= 1 << (7 - bit)
				}
			}
			row = append(row, byteVal)
		}
		length := len(row)
		if length == 0 {
			log.Printf("Warning: Empty QR row %d", y)
			continue
		}
		lengthBytes := []byte(fmt.Sprintf("%03d", length))
		io.WriteString(w, "\033*b")
		w.Write(lengthBytes)
		io.WriteString(w, "W")
		w.Write(row)
		log.Printf("Wrote QR row %d, length %d, data: %v", y, length, row)
	}
	io.WriteString(w, "\033*rB") // End raster, no form feed
	log.Printf("Wrote end raster: \\033*rB")
	return nil
}

func wordColumnPCL(fontFace *bitmap.Face, fontSize int, words []string, start, end, x, y int, dpi int) []string {
	var cmds []string
	// Ensure end doesn’t exceed slice length
	if end > len(words) {
		end = len(words)
	}
	for i := start; i < end; i++ {
		if i >= len(words) { // Safety check
			break
		}
		word := strings.ToUpper(words[i])
		num := fmt.Sprintf("%2d ", i+1)

		// Rasterize number
		numImg, _ := loadFont(fontFace, num, fontSize, dpi)
		numPCL := rasterToPCL(numImg, dpi)
		numX := x
		numY := y + (i-start)*(fontSize*11/10)
		cmds = append(cmds, fmt.Sprintf("%s\033&a%dh%dV", numPCL, numX, numY))

		// Rasterize word
		wordImg, _ := loadFont(fontFace, word, fontSize, dpi)
		wordPCL := rasterToPCL(wordImg, dpi)
		wordWidth := 0
		for _, r := range word {
			adv, ok := fontFace.GlyphAdvance(r)
			if ok {
				wordWidth += int(adv) * fontSize / 64 // Convert fixed.Int26_6 to pixels
			}
		}
		wordX := numX + (len(num) * fontSize * 6 / 10)
		cmds = append(cmds, fmt.Sprintf("%s\033&a%dh%dV", wordPCL, wordX, numY))
	}
	return cmds
}
