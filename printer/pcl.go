package printer

import (
	"fmt"
	"image"
	"image/draw"
	"io"
	"math"

	xdraw "golang.org/x/image/draw"
)

type progressWriter struct {
	w        io.Writer
	written  int64
	total    int64
	stage    PrintStage
	progress ProgressFunc
}

func newProgressWriter(stage PrintStage, w io.Writer, total int64, progress ProgressFunc) *progressWriter {
	return &progressWriter{
		w:        w,
		total:    total,
		stage:    stage,
		progress: progress,
	}
}

func (pw *progressWriter) Write(b []byte) (int, error) {
	n, err := pw.w.Write(b)
	pw.written += int64(n)
	if pw.progress != nil && pw.total > 0 {
		pw.progress(pw.stage, pw.written, pw.total)
	}
	return n, err
}

// ComposePages assembles plate bitmaps into A4/Letter pages (2x3 grid), matching the PDF layout.
// Mirrors/inversion should be handled at the plate level via RasterOptions.
// progress, if set, receives StageCompose updates as slots are placed.
func ComposePages(seedPlates, descPlates []*image.Paletted, paper PaperSize, dpi float64, progress ProgressFunc) ([]*image.Paletted, error) {
	if len(seedPlates) == 0 {
		return nil, fmt.Errorf("no seed plates to compose")
	}
	pageWmm, pageHmm, ok := paperDimsMM(paper)
	if !ok {
		return nil, fmt.Errorf("unsupported paper size: %v", paper)
	}
	pageWpx := mmToPx(pageWmm, dpi)
	pageHpx := mmToPx(pageHmm, dpi)
	targetGapPx := mmToPx(4, dpi)    // desired gap between plates
	targetMarginPx := mmToPx(5, dpi) // desired margin to page edges

	hasDesc := descPlates != nil && len(descPlates) == len(seedPlates)
	totalShares := len(seedPlates)
	sharesPerPage := 3 // matches PDF layout logic

	// Total slots we expect to place (for progress).
	totalSlots := totalShares
	if hasDesc {
		totalSlots *= 2
	}
	if progress != nil && totalSlots > 0 {
		progress(StageCompose, 0, int64(totalSlots))
	}
	placed := int64(0)

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
		// Keep pages top-anchored so partial pages (e.g. 1/1 or 2/2) start at the top.
		marginY := targetMarginPx

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

			placed++
			if progress != nil && totalSlots > 0 {
				progress(StageCompose, placed, int64(totalSlots))
			}
		}
		pages = append(pages, pageImg)
	}
	return pages, nil
}

func estimatePCLBytes(pages []*image.Paletted, dpi float64, paper PaperSize) (int64, error) {
	paperCode, ok := paperCode(paper)
	if !ok {
		return 0, fmt.Errorf("unsupported paper size: %v", paper)
	}
	total := int64(0)
	uel := []byte{0x1b, '%', '-', '1', '2', '3', '4', '5', 'X'}
	total += int64(len(uel))                                     // enter
	total += int64(len([]byte("@PJL ENTER LANGUAGE = PCL\r\n"))) // PJL header
	// per page accounting done inside loop
	for _, page := range pages {
		b := page.Bounds()
		width, height := b.Dx(), b.Dy()
		if width <= 0 || height <= 0 {
			return 0, fmt.Errorf("page has invalid dimensions")
		}
		rowBytes := (width + 7) / 8
		perRowPrefix := fmt.Sprintf("\x1b*b%dW", rowBytes)
		resetSeq := []string{"\x1bE", fmt.Sprintf("\x1b&l%dA", paperCode), "\x1b&l0E"}
		for _, seq := range resetSeq {
			total += int64(len(seq))
		}
		pageSeq := []string{
			fmt.Sprintf("\x1b*t%dR", int(math.Round(dpi))),
			fmt.Sprintf("\x1b*r%dS", width),
			fmt.Sprintf("\x1b*r%dT", height),
			"\x1b*p0x0Y",
			"\x1b*b0M",
			"\x1b*r0F",
		}
		for _, seq := range pageSeq {
			total += int64(len(seq))
		}
		rowChunk := int64(len(perRowPrefix) + rowBytes)
		total += int64(height) * rowChunk
		total += int64(len("\x1b*rC"))
		total += int64(len("\x0c"))
	}
	total += int64(len(uel)) // closing UEL
	return total, nil
}

// WritePCL streams mono bitmaps as a PCL5e raster job (one page per image).
// progress, if non-nil, is called with bytes written out of the estimated total.
func WritePCL(w io.Writer, pages []*image.Paletted, dpi float64, paper PaperSize, progress ProgressFunc) error {
	if len(pages) == 0 {
		return fmt.Errorf("no pages to write")
	}
	paperCode, ok := paperCode(paper)
	if !ok {
		return fmt.Errorf("unsupported paper size: %v", paper)
	}

	totalBytes, err := estimatePCLBytes(pages, dpi, paper)
	if err != nil {
		return err
	}
	pw := newProgressWriter(StageSend, w, totalBytes, progress)
	if progress != nil && totalBytes > 0 {
		progress(StageSend, 0, totalBytes)
	}

	uel := []byte{0x1b, '%', '-', '1', '2', '3', '4', '5', 'X'}
	if _, err := pw.Write(uel); err != nil {
		return err
	}
	if _, err := pw.Write([]byte("@PJL ENTER LANGUAGE = PCL\r\n")); err != nil {
		return err
	}

	for i, page := range pages {
		b := page.Bounds()
		width := b.Dx()
		height := b.Dy()
		if width <= 0 || height <= 0 {
			return fmt.Errorf("page %d has invalid dimensions", i)
		}

		if _, err := fmt.Fprintf(pw, "\x1bE"); err != nil { // reset
			return err
		}
		if _, err := fmt.Fprintf(pw, "\x1b&l%dA", paperCode); err != nil { // paper size
			return err
		}
		if _, err := fmt.Fprintf(pw, "\x1b&l0E"); err != nil { // top margin = 0 lines
			return err
		}
		if _, err := fmt.Fprintf(pw, "\x1b*t%dR", int(math.Round(dpi))); err != nil { // resolution
			return err
		}
		if _, err := fmt.Fprintf(pw, "\x1b*r%dS", width); err != nil { // source width (pixels)
			return err
		}
		if _, err := fmt.Fprintf(pw, "\x1b*r%dT", height); err != nil { // source height (rows)
			return err
		}
		if _, err := fmt.Fprintf(pw, "\x1b*p0x0Y"); err != nil { // move to origin
			return err
		}
		if _, err := fmt.Fprintf(pw, "\x1b*b0M"); err != nil { // compression: unencoded (avoid streaking on some devices)
			return err
		}
		if _, err := fmt.Fprintf(pw, "\x1b*r0F"); err != nil { // start raster graphics
			return err
		}

		rowBytes := (width + 7) / 8
		buf := make([]byte, rowBytes)
		for y := 0; y < height; y++ {
			pix := page.Pix[y*page.Stride : y*page.Stride+width]
			packBits(buf, pix)
			if _, err := fmt.Fprintf(pw, "\x1b*b%dW", rowBytes); err != nil {
				return err
			}
			if _, err := pw.Write(buf); err != nil {
				return err
			}
		}

		if _, err := fmt.Fprintf(pw, "\x1b*rC"); err != nil { // end raster graphics
			return err
		}
		if _, err := fmt.Fprintf(pw, "\x0c"); err != nil { // form feed
			return err
		}
	}

	// Close job
	if _, err := pw.Write(uel); err != nil {
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
