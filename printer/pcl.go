package printer

import (
	"fmt"
	"image"
	"image/draw"
	"io"
	"math"

	xdraw "golang.org/x/image/draw"
)

// ComposePages assembles plate bitmaps into A4/Letter pages (2x3 grid), matching the PDF layout.
// Mirrors/inversion should be handled at the plate level via RasterOptions.
func ComposePages(seedPlates, descPlates []*image.Paletted, paper PaperSize, dpi float64) ([]*image.Paletted, error) {
	if len(seedPlates) == 0 {
		return nil, fmt.Errorf("no seed plates to compose")
	}
	pageWmm, pageHmm, ok := paperDimsMM(paper)
	if !ok {
		return nil, fmt.Errorf("unsupported paper size: %v", paper)
	}
	pageWpx := mmToPx(pageWmm, dpi)
	pageHpx := mmToPx(pageHmm, dpi)
	targetGapPx := mmToPx(5, dpi)    // desired gap between plates
	targetMarginPx := mmToPx(5, dpi) // desired margin to page edges

	hasDesc := descPlates != nil && len(descPlates) == len(seedPlates)
	totalShares := len(seedPlates)
	sharesPerPage := 3 // matches PDF layout logic

	var pages []*image.Paletted
	for page := 0; page*sharesPerPage < totalShares; page++ {
		start := page * sharesPerPage
		end := minInt(start+sharesPerPage, totalShares)
		var slots []*image.Paletted

		// Build the page slot list in the same order as PDF layout
		if hasDesc {
			for i := start; i < end; i++ {
				slots = append(slots, seedPlates[i], descPlates[i])
			}
		} else {
			for i := start; i < end; i++ {
				slots = append(slots, seedPlates[i])
			}
		}

		pageImg := image.NewPaletted(image.Rect(0, 0, pageWpx, pageHpx), bwPalette)
		draw.Draw(pageImg, pageImg.Bounds(), &image.Uniform{bwPalette[0]}, image.Point{}, draw.Src)

		// Determine rows/cols for this page
		slotsThisPage := len(slots)
		cols := 2
		rows := (slotsThisPage + cols - 1) / cols

		// Baseline plate size (assume all plates same dims)
		var baseW, baseH int
		for _, pl := range slots {
			if pl != nil {
				b := pl.Bounds()
				baseW, baseH = b.Dx(), b.Dy()
				break
			}
		}
		if baseW == 0 || baseH == 0 {
			return nil, fmt.Errorf("invalid plate dimensions")
		}

		// Compute scaling to fit within margins/gaps
		availW := pageWpx - 2*targetMarginPx - targetGapPx*(cols-1)
		availH := pageHpx - 2*targetMarginPx - targetGapPx*(rows-1)
		scale := math.Min(1, math.Min(float64(availW)/(float64(baseW)*float64(cols)), float64(availH)/(float64(baseH)*float64(rows))))
		plateW := int(math.Round(float64(baseW) * scale))
		plateH := int(math.Round(float64(baseH) * scale))
		gapPx := targetGapPx
		marginX := (pageWpx - (cols*plateW + (cols-1)*gapPx)) / 2
		marginY := (pageHpx - (rows*plateH + (rows-1)*gapPx)) / 2

		// Place slots
		for slotIdx, plate := range slots {
			if plate == nil {
				continue
			}
			row := slotIdx / 2
			col := slotIdx % 2
			dst := image.NewPaletted(image.Rect(0, 0, plateW, plateH), plate.Palette)
			xdraw.NearestNeighbor.Scale(dst, dst.Bounds(), plate, plate.Bounds(), xdraw.Src, nil)

			offset := image.Point{
				X: marginX + col*(plateW+gapPx),
				Y: marginY + row*(plateH+gapPx),
			}
			r := image.Rectangle{Min: offset, Max: offset.Add(dst.Bounds().Size())}
			draw.Draw(pageImg, r, dst, image.Point{}, draw.Src)
		}
		pages = append(pages, pageImg)
	}
	return pages, nil
}

// WritePCL streams mono bitmaps as a PCL5e raster job (one page per image).
func WritePCL(w io.Writer, pages []*image.Paletted, dpi float64, paper PaperSize) error {
	if len(pages) == 0 {
		return fmt.Errorf("no pages to write")
	}
	paperCode, ok := paperCode(paper)
	if !ok {
		return fmt.Errorf("unsupported paper size: %v", paper)
	}

	uel := []byte{0x1b, '%', '-', '1', '2', '3', '4', '5', 'X'}
	if _, err := w.Write(uel); err != nil {
		return err
	}
	if _, err := w.Write([]byte("@PJL ENTER LANGUAGE = PCL\r\n")); err != nil {
		return err
	}

	for i, page := range pages {
		b := page.Bounds()
		width := b.Dx()
		height := b.Dy()
		if width <= 0 || height <= 0 {
			return fmt.Errorf("page %d has invalid dimensions", i)
		}

		if _, err := fmt.Fprintf(w, "\x1bE"); err != nil { // reset
			return err
		}
		if _, err := fmt.Fprintf(w, "\x1b&l%dA", paperCode); err != nil { // paper size
			return err
		}
		if _, err := fmt.Fprintf(w, "\x1b&l0E"); err != nil { // top margin = 0 lines
			return err
		}
		if _, err := fmt.Fprintf(w, "\x1b*t%dR", int(math.Round(dpi))); err != nil { // resolution
			return err
		}
		if _, err := fmt.Fprintf(w, "\x1b*r%dS", width); err != nil { // source width (pixels)
			return err
		}
		if _, err := fmt.Fprintf(w, "\x1b*r%dT", height); err != nil { // source height (rows)
			return err
		}
		if _, err := fmt.Fprintf(w, "\x1b*p0x0Y"); err != nil { // move to origin
			return err
		}
		if _, err := fmt.Fprintf(w, "\x1b*b0M"); err != nil { // compression: unencoded
			return err
		}
		if _, err := fmt.Fprintf(w, "\x1b*r0F"); err != nil { // start raster graphics
			return err
		}

		rowBytes := (width + 7) / 8
		buf := make([]byte, rowBytes)
		for y := 0; y < height; y++ {
			pix := page.Pix[y*page.Stride : y*page.Stride+width]
			packBits(buf, pix)
			if _, err := fmt.Fprintf(w, "\x1b*b%dW", rowBytes); err != nil {
				return err
			}
			if _, err := w.Write(buf); err != nil {
				return err
			}
		}

		if _, err := fmt.Fprintf(w, "\x1b*rC"); err != nil { // end raster graphics
			return err
		}
		if _, err := fmt.Fprintf(w, "\x0c"); err != nil { // form feed
			return err
		}
	}

	// Close job
	if _, err := w.Write(uel); err != nil {
		return err
	}
	return nil
}

func paperDimsMM(p PaperSize) (float64, float64, bool) {
	switch p {
	case PaperA4:
		return 210.0, 297.0, true
	case PaperLetter:
		return 215.9, 279.4, true
	default:
		return 0, 0, false
	}
}

func paperCode(p PaperSize) (int, bool) {
	switch p {
	case PaperA4:
		return 26, true
	case PaperLetter:
		return 2, true
	default:
		return 0, false
	}
}

func packBits(dst []byte, row []uint8) {
	for i := range dst {
		dst[i] = 0
	}
	for i, v := range row {
		if v != 0 { // palette index 1 = black
			dst[i/8] |= 1 << (7 - uint(i%8))
		}
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
