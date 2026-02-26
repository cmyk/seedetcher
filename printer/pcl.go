package printer

import (
	"fmt"
	"image"
	"image/draw"
	"io"
	"math"
)

type progressWriter struct {
	w        io.Writer
	written  int64
	total    int64
	stage    PrintStage
	progress ProgressFunc
}

type placedPlate struct {
	plate *image.Paletted
	x     int
	y     int
}

type pagePlacement struct {
	slots []placedPlate
}

type placementPlan struct {
	pageWpx int
	pageHpx int
	pages   []pagePlacement
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
	plan, err := buildPlacementPlan(seedPlates, descPlates, paper, dpi, progress)
	if err != nil {
		return nil, err
	}
	var pages []*image.Paletted
	for _, page := range plan.pages {
		pageImg := image.NewPaletted(image.Rect(0, 0, plan.pageWpx, plan.pageHpx), bwPalette)
		draw.Draw(pageImg, pageImg.Bounds(), &image.Uniform{bwPalette[0]}, image.Point{}, draw.Src)
		for _, slot := range page.slots {
			if slot.plate == nil {
				continue
			}
			b := slot.plate.Bounds()
			r := image.Rect(slot.x, slot.y, slot.x+b.Dx(), slot.y+b.Dy())
			draw.Draw(pageImg, r, slot.plate, b.Min, draw.Src)
		}
		pages = append(pages, pageImg)
	}
	return pages, nil
}

// WritePCLPlates composes seed/descriptor plates directly into a PCL raster job
// without creating full-page intermediate images.
func WritePCLPlates(w io.Writer, seedPlates, descPlates []*image.Paletted, dpi float64, paper PaperSize, progress ProgressFunc) error {
	plan, err := buildPlacementPlan(seedPlates, descPlates, paper, dpi, progress)
	if err != nil {
		return err
	}
	paperCode, ok := paperCode(paper)
	if !ok {
		return fmt.Errorf("unsupported paper size: %v", paper)
	}
	totalBytes, err := estimatePCLBytesForPlan(plan, dpi, paper)
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

	width := plan.pageWpx
	height := plan.pageHpx
	rowBytes := (width + 7) / 8
	rowPix := make([]uint8, width)
	rowPacked := make([]byte, rowBytes)

	for _, page := range plan.pages {
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
		if _, err := fmt.Fprintf(pw, "\x1b*b0M"); err != nil { // compression: unencoded
			return err
		}
		if _, err := fmt.Fprintf(pw, "\x1b*r0F"); err != nil { // start raster graphics
			return err
		}

		for y := 0; y < height; y++ {
			for i := range rowPix {
				rowPix[i] = 0
			}
			for _, slot := range page.slots {
				if slot.plate == nil {
					continue
				}
				pb := slot.plate.Bounds()
				ly := y - slot.y
				if ly < 0 || ly >= pb.Dy() {
					continue
				}
				srcRow := slot.plate.Pix[(pb.Min.Y+ly)*slot.plate.Stride+pb.Min.X : (pb.Min.Y+ly)*slot.plate.Stride+pb.Min.X+pb.Dx()]
				copy(rowPix[slot.x:slot.x+pb.Dx()], srcRow)
			}
			packBits(rowPacked, rowPix)
			if _, err := fmt.Fprintf(pw, "\x1b*b%dW", rowBytes); err != nil {
				return err
			}
			if _, err := pw.Write(rowPacked); err != nil {
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

	if _, err := pw.Write(uel); err != nil {
		return err
	}
	return nil
}

// EstimatePCLPlatesBytes estimates the raw PCL byte size for a plate-set job.
// Useful for aggregating multi-batch send progress.
func EstimatePCLPlatesBytes(seedPlates, descPlates []*image.Paletted, dpi float64, paper PaperSize) (int64, error) {
	plan, err := buildPlacementPlan(seedPlates, descPlates, paper, dpi, nil)
	if err != nil {
		return 0, err
	}
	return estimatePCLBytesForPlan(plan, dpi, paper)
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

// EstimatePCLBytes estimates raw PCL bytes for already-composed pages.
func EstimatePCLBytes(pages []*image.Paletted, dpi float64, paper PaperSize) (int64, error) {
	return estimatePCLBytes(pages, dpi, paper)
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

func buildPlacementPlan(seedPlates, descPlates []*image.Paletted, paper PaperSize, dpi float64, progress ProgressFunc) (placementPlan, error) {
	if len(seedPlates) == 0 {
		return placementPlan{}, fmt.Errorf("no seed plates to compose")
	}
	pageWmm, pageHmm, ok := paperDimsMM(paper)
	if !ok {
		return placementPlan{}, fmt.Errorf("unsupported paper size: %v", paper)
	}
	pageWpx := mmToPx(pageWmm, dpi)
	pageHpx := mmToPx(pageHmm, dpi)
	targetGapPx := mmToPx(4, dpi)    // desired gap between plates
	targetMarginPx := mmToPx(5, dpi) // desired margin to page edges

	hasDesc := descPlates != nil && len(descPlates) == len(seedPlates)
	totalShares := len(seedPlates)
	maxSlotsPerPage := 6 // A4 default: 2x3
	if paper == PaperLetter {
		maxSlotsPerPage = 4 // Letter: 2x2 to preserve true 90mm plates without scaling
	}
	slotsPerShare := 1
	if hasDesc {
		slotsPerShare = 2
	}
	sharesPerPage := maxSlotsPerPage / slotsPerShare
	if sharesPerPage < 1 {
		sharesPerPage = 1
	}

	totalSlots := totalShares
	if hasDesc {
		totalSlots *= 2
	}
	if progress != nil && totalSlots > 0 {
		progress(StageCompose, 0, int64(totalSlots))
	}
	placed := int64(0)

	var pages []pagePlacement
	for page := 0; page*sharesPerPage < totalShares; page++ {
		start := page * sharesPerPage
		end := minInt(start+sharesPerPage, totalShares)
		var slots []*image.Paletted
		if hasDesc {
			for i := start; i < end; i++ {
				slots = append(slots, seedPlates[i], descPlates[i])
			}
		} else {
			for i := start; i < end; i++ {
				slots = append(slots, seedPlates[i])
			}
		}
		slotsThisPage := len(slots)
		cols := 2
		rows := (slotsThisPage + cols - 1) / cols
		baseW, baseH, err := basePlateDims(slots)
		if err != nil {
			return placementPlan{}, err
		}
		plateW := baseW
		plateH := baseH
		gapPx := targetGapPx
		reqW := cols*plateW + (cols-1)*gapPx + 2*targetMarginPx
		reqH := rows*plateH + (rows-1)*gapPx + 2*targetMarginPx
		if reqW > pageWpx || reqH > pageHpx {
			return placementPlan{}, fmt.Errorf("plates do not fit page at fixed size (paper=%s req=%dx%d page=%dx%d)", paper, reqW, reqH, pageWpx, pageHpx)
		}
		marginX := (pageWpx - (cols*plateW + (cols-1)*gapPx)) / 2
		marginY := targetMarginPx

		pp := pagePlacement{slots: make([]placedPlate, 0, len(slots))}
		for slotIdx, plate := range slots {
			if plate == nil {
				continue
			}
			b := plate.Bounds()
			if b.Dx() != plateW || b.Dy() != plateH {
				return placementPlan{}, fmt.Errorf("mismatched plate dimensions: got %dx%d want %dx%d", b.Dx(), b.Dy(), plateW, plateH)
			}
			row := slotIdx / 2
			col := slotIdx % 2
			pp.slots = append(pp.slots, placedPlate{
				plate: plate,
				x:     marginX + col*(plateW+gapPx),
				y:     marginY + row*(plateH+gapPx),
			})
			placed++
			if progress != nil && totalSlots > 0 {
				progress(StageCompose, placed, int64(totalSlots))
			}
		}
		pages = append(pages, pp)
	}
	return placementPlan{pageWpx: pageWpx, pageHpx: pageHpx, pages: pages}, nil
}

func basePlateDims(slots []*image.Paletted) (int, int, error) {
	for _, pl := range slots {
		if pl != nil {
			b := pl.Bounds()
			if b.Dx() <= 0 || b.Dy() <= 0 {
				return 0, 0, fmt.Errorf("invalid plate dimensions")
			}
			return b.Dx(), b.Dy(), nil
		}
	}
	return 0, 0, fmt.Errorf("invalid plate dimensions")
}

func estimatePCLBytesForPlan(plan placementPlan, dpi float64, paper PaperSize) (int64, error) {
	if _, ok := paperCode(paper); !ok {
		return 0, fmt.Errorf("unsupported paper size: %v", paper)
	}
	if plan.pageWpx <= 0 || plan.pageHpx <= 0 {
		return 0, fmt.Errorf("invalid page dimensions")
	}
	total := int64(0)
	uel := []byte{0x1b, '%', '-', '1', '2', '3', '4', '5', 'X'}
	total += int64(len(uel))
	total += int64(len([]byte("@PJL ENTER LANGUAGE = PCL\r\n")))
	rowBytes := (plan.pageWpx + 7) / 8
	perRowPrefix := fmt.Sprintf("\x1b*b%dW", rowBytes)
	for range plan.pages {
		resetSeq := []string{"\x1bE", fmt.Sprintf("\x1b&l%dA", mustPaperCode(paper)), "\x1b&l0E"}
		for _, seq := range resetSeq {
			total += int64(len(seq))
		}
		pageSeq := []string{
			fmt.Sprintf("\x1b*t%dR", int(math.Round(dpi))),
			fmt.Sprintf("\x1b*r%dS", plan.pageWpx),
			fmt.Sprintf("\x1b*r%dT", plan.pageHpx),
			"\x1b*p0x0Y",
			"\x1b*b0M",
			"\x1b*r0F",
		}
		for _, seq := range pageSeq {
			total += int64(len(seq))
		}
		rowChunk := int64(len(perRowPrefix) + rowBytes)
		total += int64(plan.pageHpx) * rowChunk
		total += int64(len("\x1b*rC"))
		total += int64(len("\x0c"))
	}
	total += int64(len(uel))
	return total, nil
}

func mustPaperCode(p PaperSize) int {
	code, _ := paperCode(p)
	return code
}
