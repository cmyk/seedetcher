package printer

import "image"

const (
	transferOuterMarginMM       = 5.0
	transferPlateInsetLeftMM    = 5.0
	transferPlateInsetTopMM     = 5.0
	transferPlateInsetBottomMM  = 5.0
	transferRowGapMM            = 0.0
	transferCutMarkLenMM        = 1.8
	transferInstructionGapMM    = 6.0
	transferInstructionMarginMM = 8.0
)

func renderPlannedRow(rowPix []uint8, y int, page pagePlacement, invert bool) {
	for i := range rowPix {
		rowPix[i] = 0
	}

	for _, box := range page.cutBoxes {
		if y < box.Min.Y || y >= box.Max.Y {
			continue
		}
		if invert {
			setBlackRange(rowPix, box.Min.X, box.Max.X-1)
			continue
		}
		if y == box.Min.Y || y == box.Max.Y-1 {
			setBlackRange(rowPix, box.Min.X, box.Max.X-1)
			continue
		}
		if box.Min.X >= 0 && box.Min.X < len(rowPix) {
			rowPix[box.Min.X] = 1
		}
		xr := box.Max.X - 1
		if xr >= 0 && xr < len(rowPix) {
			rowPix[xr] = 1
		}
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
		src := slot.plate.Pix[(pb.Min.Y+ly)*slot.plate.Stride+pb.Min.X : (pb.Min.Y+ly)*slot.plate.Stride+pb.Min.X+pb.Dx()]
		dstStart := slot.x
		srcStart := 0
		n := len(src)
		if dstStart < 0 {
			srcStart = -dstStart
			dstStart = 0
		}
		if dstStart >= len(rowPix) || srcStart >= n {
			continue
		}
		n -= srcStart
		if dstStart+n > len(rowPix) {
			n = len(rowPix) - dstStart
		}
		if n > 0 {
			copy(rowPix[dstStart:dstStart+n], src[srcStart:srcStart+n])
		}
	}

	for _, m := range page.marks {
		drawCutMarkRow(rowPix, y, m)
	}
	for _, ov := range page.overlays {
		blendOverlayRow(rowPix, y, ov)
	}
}

func setBlackRange(rowPix []uint8, x0, x1 int) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if x1 < 0 || x0 >= len(rowPix) {
		return
	}
	if x0 < 0 {
		x0 = 0
	}
	if x1 >= len(rowPix) {
		x1 = len(rowPix) - 1
	}
	for x := x0; x <= x1; x++ {
		rowPix[x] = 1
	}
}

func drawCutMarkRow(rowPix []uint8, y int, m cutMark) {
	x0, x1 := m.x0, m.x1
	y0, y1 := m.y0, m.y1
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}

	if y < y0 || y > y1 {
		return
	}
	if y0 == y1 {
		setBlackRange(rowPix, x0, x1)
		return
	}
	if x0 == x1 && x0 >= 0 && x0 < len(rowPix) {
		rowPix[x0] = 1
	}
}

func blendOverlayRow(rowPix []uint8, y int, ov placedPlate) {
	if ov.plate == nil {
		return
	}
	b := ov.plate.Bounds()
	ly := y - ov.y
	if ly < 0 || ly >= b.Dy() {
		return
	}
	src := ov.plate.Pix[(b.Min.Y+ly)*ov.plate.Stride+b.Min.X : (b.Min.Y+ly)*ov.plate.Stride+b.Min.X+b.Dx()]
	for i, v := range src {
		if v == 0 {
			continue
		}
		x := ov.x + i
		if x < 0 || x >= len(rowPix) {
			continue
		}
		rowPix[x] = 1
	}
}

func buildTransferCutMarks(grid image.Rectangle, vCuts, hCuts []int, dpi float64) []cutMark {
	markLen := mmToPx(transferCutMarkLenMM, dpi)
	if markLen < 1 {
		markLen = 1
	}
	left := grid.Min.X
	top := grid.Min.Y
	right := grid.Max.X - 1
	bottom := grid.Max.Y - 1
	marks := make([]cutMark, 0, 2*(len(vCuts)+len(hCuts)))

	for _, x := range vCuts {
		xx := x
		if xx > left {
			xx--
		}
		marks = append(marks, cutMark{x0: xx, y0: top - markLen, x1: xx, y1: top - 1})
		marks = append(marks, cutMark{x0: xx, y0: bottom + 1, x1: xx, y1: bottom + markLen})
	}
	for _, y := range hCuts {
		yy := y
		if yy > top {
			yy--
		}
		marks = append(marks, cutMark{x0: left - markLen, y0: yy, x1: left - 1, y1: yy})
		marks = append(marks, cutMark{x0: right + 1, y0: yy, x1: right + markLen, y1: yy})
	}
	return marks
}

func buildTransferInstructionOverlay(pageWpx, pageHpx int, dpi float64, gridBottom int, tapeXs []int, tapeLabelCenterAbsX, leftTextAbsX int) (placedPlate, bool) {
	marginPx := mmToPx(transferInstructionMarginMM, dpi)
	topPx := gridBottom + mmToPx(transferInstructionGapMM, dpi)
	width := pageWpx - 2*marginPx
	height := pageHpx - topPx - marginPx
	if width <= 0 || height <= 0 {
		return placedPlate{}, false
	}

	img := image.NewPaletted(image.Rect(0, 0, width, height), bwPalette)
	face := loadFaceMedium(8, dpi)
	track := 0.02 * 8.0 * dpi / 72.0
	stepMM := 3.8
	pageWMM := float64(width) * 25.4 / dpi
	pxToMM := func(px int) float64 {
		return float64(px) * 25.4 / dpi
	}

	yMM := 3.0 + capBaselineOffsetMM(face, dpi)
	caret := "^"
	caretWMM := trackedTextWidthMM(face, dpi, caret, track)
	for _, xAbs := range tapeXs {
		xRel := xAbs - marginPx
		if xRel < 0 || xRel >= width {
			continue
		}
		xMM := float64(xRel)*25.4/dpi - (caretWMM / 2)
		drawTrackedText(img, face, dpi, xMM, yMM, caret, track)
	}

	tapeLine := "Add tape along these sides"
	tapeLine2 := "(left side when placed face down on plate)"
	tapeWMM := trackedTextWidthMM(face, dpi, tapeLine, track)
	tapeWMM2 := trackedTextWidthMM(face, dpi, tapeLine2, track)
	tapeCenterRel := tapeLabelCenterAbsX - marginPx
	if tapeCenterRel < 0 {
		tapeCenterRel = 0
	}
	if tapeCenterRel >= width {
		tapeCenterRel = width - 1
	}
	tapeXMM := pxToMM(tapeCenterRel) - tapeWMM/2
	if tapeXMM < 0 {
		tapeXMM = 0
	}
	maxTapeXMM := pageWMM - tapeWMM
	if tapeXMM > maxTapeXMM {
		tapeXMM = maxTapeXMM
	}
	tapeXMM2 := pxToMM(tapeCenterRel) - tapeWMM2/2
	if tapeXMM2 < 0 {
		tapeXMM2 = 0
	}
	maxTapeXMM2 := pageWMM - tapeWMM2
	if tapeXMM2 > maxTapeXMM2 {
		tapeXMM2 = maxTapeXMM2
	}
	drawTrackedText(img, face, dpi, tapeXMM, yMM, tapeLine, track)
	yMM += stepMM
	drawTrackedText(img, face, dpi, tapeXMM2, yMM, tapeLine2, track)

	yMM += stepMM + 1.5
	leftRel := leftTextAbsX - marginPx
	if leftRel < 0 {
		leftRel = 0
	}
	if leftRel >= width {
		leftRel = width - 1
	}
	leftXMM := pxToMM(leftRel)
	maxTextWidthMM := pageWMM * 2.0 / 3.0
	if leftXMM+maxTextWidthMM > pageWMM {
		maxTextWidthMM = pageWMM - leftXMM
	}
	if maxTextWidthMM < 5.0 {
		maxTextWidthMM = pageWMM - leftXMM
	}

	messages := []string{
		"Cut layouts at cut marks.",
		"To position on plate, flip the mask left-right (mirror), place it face down, and align the top and right edges to the plate.",
		"Add a small thin strip of masking tape along the left side",
		"to secure the mask before transfer.",
		"DO NOT TOUCH THE TONER! Use a clean surface and ruler to cut.",
		"Do not scratch the toner mask.",
	}
	for _, msg := range messages {
		lines := wrapTextTracked(face, dpi, msg, maxTextWidthMM, track)
		for _, ln := range lines {
			if mmToPx(yMM, dpi) >= height-2 {
				break
			}
			drawTrackedText(img, face, dpi, leftXMM, yMM, ln, track)
			yMM += stepMM
		}
	}

	return placedPlate{plate: img, x: marginPx, y: topPx}, true
}
