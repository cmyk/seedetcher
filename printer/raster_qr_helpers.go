package printer

import (
	"image"
	"math"

	"github.com/kortschak/qr"
)

func drawPlateQR(img *image.Paletted, code *qr.Code, dpi, xMm, yMm, sizeMm float64, idx uint8, opts plateQROptions) {
	if code == nil {
		return
	}
	if opts.QuietModules < 0 {
		opts.QuietModules = 0
	}
	x0 := mmToPx(xMm, dpi)
	y0 := mmToPx(yMm, dpi)
	sizePx := mmToPx(sizeMm, dpi)
	quiet := opts.QuietModules
	step := float64(sizePx) / float64(code.Size+2*quiet)
	offset := int(math.Round(float64(quiet) * step))
	var islandMask []bool
	if opts.KeepIslandsSquare {
		islandMask = buildQRIslandMask(code)
	}
	patternRadiusRatio := opts.PatternCornerRadiusRatio
	if patternRadiusRatio < 0 {
		patternRadiusRatio = 0
	}
	if patternRadiusRatio == 0 {
		patternRadiusRatio = plateQRPatternCornerRadiusRatio
	}
	if patternRadiusRatio > 0.5 {
		patternRadiusRatio = 0.5
	}

	for y := 0; y < code.Size; y++ {
		yStart := y0 + offset + int(math.Round(float64(y)*step))
		yEnd := y0 + offset + int(math.Round(float64(y+1)*step))
		for x := 0; x < code.Size; x++ {
			if !code.Black(x, y) {
				continue
			}
			xStart := x0 + offset + int(math.Round(float64(x)*step))
			xEnd := x0 + offset + int(math.Round(float64(x+1)*step))

			useSquare := opts.Shape == plateQRSquare
			isPatternModule := opts.KeepIslandsSquare && islandMask[y*code.Size+x]
			if isPatternModule {
				useSquare = true
			}
			if useSquare {
				fillRect(img, xStart, yStart, xEnd-xStart, yEnd-yStart, idx)
			} else {
				fillModuleCircleAtStep(img, xStart, yStart, xEnd, yEnd, idx, plateQRDotScale, step)
			}
		}
	}
	if opts.KeepIslandsSquare && patternRadiusRatio > 0 {
		applyFinderCornerRounding(img, code, x0+offset, y0+offset, step, idx, oppositeBWIndex(idx), patternRadiusRatio)
	}
}

func minIntQR(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func oppositeBWIndex(idx uint8) uint8 {
	if idx == 0 {
		return 1
	}
	return 0
}

func moduleRect(origin int, step float64, startModule, endModule int) (int, int) {
	start := origin + int(math.Round(float64(startModule)*step))
	end := origin + int(math.Round(float64(endModule)*step))
	return start, end - start
}

func finderModuleCornerRadius(step, ratio float64) int {
	modulePx := int(math.Round(step))
	if modulePx < 1 {
		modulePx = 1
	}
	// Map 0..0.5 => 0..1 module corner radius.
	r := int(math.Round(float64(modulePx) * (ratio * 2.0)))
	if r < 1 {
		r = 1
	}
	if r > modulePx {
		r = modulePx
	}
	return r
}

func roundPatternModule(img *image.Paletted, code *qr.Code, originX, originY int, step float64, x, y int, corners uint8, blackIdx, whiteIdx uint8, radius int) {
	if x < 0 || y < 0 || x >= code.Size || y >= code.Size || corners == 0 {
		return
	}
	xStart, w := moduleRect(originX, step, x, x+1)
	yStart, h := moduleRect(originY, step, y, y+1)
	fg, bg := whiteIdx, blackIdx
	if code.Black(x, y) {
		fg, bg = blackIdx, whiteIdx
	}
	fillRectWithRoundedCorners(img, xStart, yStart, w, h, radius, fg, bg, corners)
}

func applyFinderCornerRoundingAt(img *image.Paletted, code *qr.Code, originX, originY int, step float64, fx, fy int, blackIdx, whiteIdx uint8, radius int) {
	// Outer black ring corners.
	roundPatternModule(img, code, originX, originY, step, fx+0, fy+0, cornerTL, blackIdx, whiteIdx, radius)
	roundPatternModule(img, code, originX, originY, step, fx+6, fy+0, cornerTR, blackIdx, whiteIdx, radius)
	roundPatternModule(img, code, originX, originY, step, fx+0, fy+6, cornerBL, blackIdx, whiteIdx, radius)
	roundPatternModule(img, code, originX, originY, step, fx+6, fy+6, cornerBR, blackIdx, whiteIdx, radius)
	// White ring corners.
	roundPatternModule(img, code, originX, originY, step, fx+1, fy+1, cornerTL, blackIdx, whiteIdx, radius)
	roundPatternModule(img, code, originX, originY, step, fx+5, fy+1, cornerTR, blackIdx, whiteIdx, radius)
	roundPatternModule(img, code, originX, originY, step, fx+1, fy+5, cornerBL, blackIdx, whiteIdx, radius)
	roundPatternModule(img, code, originX, originY, step, fx+5, fy+5, cornerBR, blackIdx, whiteIdx, radius)
	// Inner black (3x3) corners.
	roundPatternModule(img, code, originX, originY, step, fx+2, fy+2, cornerTL, blackIdx, whiteIdx, radius)
	roundPatternModule(img, code, originX, originY, step, fx+4, fy+2, cornerTR, blackIdx, whiteIdx, radius)
	roundPatternModule(img, code, originX, originY, step, fx+2, fy+4, cornerBL, blackIdx, whiteIdx, radius)
	roundPatternModule(img, code, originX, originY, step, fx+4, fy+4, cornerBR, blackIdx, whiteIdx, radius)
}

func inFinderArea(x, y, size int) bool {
	inTL := x >= 0 && x <= 6 && y >= 0 && y <= 6
	inTR := x >= size-7 && x <= size-1 && y >= 0 && y <= 6
	inBL := x >= 0 && x <= 6 && y >= size-7 && y <= size-1
	return inTL || inTR || inBL
}

func applyAlignmentPatternStyling(img *image.Paletted, code *qr.Code, originX, originY int, step float64, blackIdx, whiteIdx uint8, radius int) {
	size := code.Size
	for cy := 2; cy <= size-3; cy++ {
		for cx := 2; cx <= size-3; cx++ {
			if inFinderArea(cx, cy, size) || !isAlignmentCenter(code, cx, cy) {
				continue
			}
			// 5x5 outer black corners.
			roundPatternModule(img, code, originX, originY, step, cx-2, cy-2, cornerTL, blackIdx, whiteIdx, radius)
			roundPatternModule(img, code, originX, originY, step, cx+2, cy-2, cornerTR, blackIdx, whiteIdx, radius)
			roundPatternModule(img, code, originX, originY, step, cx-2, cy+2, cornerBL, blackIdx, whiteIdx, radius)
			roundPatternModule(img, code, originX, originY, step, cx+2, cy+2, cornerBR, blackIdx, whiteIdx, radius)
			// 3x3 white ring corners.
			roundPatternModule(img, code, originX, originY, step, cx-1, cy-1, cornerTL, blackIdx, whiteIdx, radius)
			roundPatternModule(img, code, originX, originY, step, cx+1, cy-1, cornerTR, blackIdx, whiteIdx, radius)
			roundPatternModule(img, code, originX, originY, step, cx-1, cy+1, cornerBL, blackIdx, whiteIdx, radius)
			roundPatternModule(img, code, originX, originY, step, cx+1, cy+1, cornerBR, blackIdx, whiteIdx, radius)

			// Center module as an explicit circle at plateQRDotScale.
			xStart, w := moduleRect(originX, step, cx, cx+1)
			yStart, h := moduleRect(originY, step, cy, cy+1)
			fillRect(img, xStart, yStart, w, h, whiteIdx)
			fillModuleCircleAtStep(img, xStart, yStart, xStart+w, yStart+h, blackIdx, plateQRDotScale, step)
		}
	}
}

func applyFinderCornerRounding(img *image.Paletted, code *qr.Code, originX, originY int, step float64, blackIdx, whiteIdx uint8, radiusRatio float64) {
	size := code.Size
	r := finderModuleCornerRadius(step, radiusRatio)
	applyFinderCornerRoundingAt(img, code, originX, originY, step, 0, 0, blackIdx, whiteIdx, r)
	applyFinderCornerRoundingAt(img, code, originX, originY, step, size-7, 0, blackIdx, whiteIdx, r)
	applyFinderCornerRoundingAt(img, code, originX, originY, step, 0, size-7, blackIdx, whiteIdx, r)
	applyAlignmentPatternStyling(img, code, originX, originY, step, blackIdx, whiteIdx, r)
}

const (
	cornerTL uint8 = 1 << iota
	cornerTR
	cornerBL
	cornerBR
)

func fillModuleCircle(img *image.Paletted, xStart, yStart, xEnd, yEnd int, idx uint8) {
	fillModuleCircleScaled(img, xStart, yStart, xEnd, yEnd, idx, plateQRDotScale)
}

func fillModuleCircleAtStep(img *image.Paletted, xStart, yStart, xEnd, yEnd int, idx uint8, scale, step float64) {
	w := xEnd - xStart
	h := yEnd - yStart
	if w <= 0 || h <= 0 {
		return
	}
	if scale <= 0 {
		scale = 1.0
	}
	if scale > 1.0 {
		scale = 1.0
	}
	d := int(math.Round(step * scale))
	if d < 1 {
		d = 1
	}
	cellMin := minIntQR(w, h)
	if d > cellMin {
		d = cellMin
	}
	// Center a fixed-size circle in the cell, then render via scaled helper.
	cx := xStart + w/2
	cy := yStart + h/2
	left := cx - d/2
	top := cy - d/2
	fillModuleCircleScaled(img, left, top, left+d, top+d, idx, 1.0)
}

func fillModuleCircleScaled(img *image.Paletted, xStart, yStart, xEnd, yEnd int, idx uint8, scale float64) {
	w := xEnd - xStart
	h := yEnd - yStart
	if w <= 0 || h <= 0 {
		return
	}
	b := img.Bounds()
	if xEnd <= b.Min.X || xStart >= b.Max.X || yEnd <= b.Min.Y || yStart >= b.Max.Y {
		return
	}
	d := w
	if h < d {
		d = h
	}
	if scale <= 0 {
		scale = 1.0
	}
	if scale > 1.0 {
		scale = 1.0
	}
	scaled := int(math.Round(float64(d) * scale))
	if scaled < 1 {
		scaled = 1
	}
	d = scaled
	if d <= 1 {
		fillRect(img, xStart, yStart, w, h, idx)
		return
	}
	cx2 := 2*xStart + w
	cy2 := 2*yStart + h
	r := d - 1
	r2 := r * r
	y0 := clampInt(yStart, b.Min.Y, b.Max.Y)
	y1 := clampInt(yEnd, b.Min.Y, b.Max.Y)
	x0 := clampInt(xStart, b.Min.X, b.Max.X)
	x1 := clampInt(xEnd, b.Min.X, b.Max.X)
	for y := y0; y < y1; y++ {
		dy2 := 2*y + 1 - cy2
		row := img.Pix[y*img.Stride:]
		for x := x0; x < x1; x++ {
			dx2 := 2*x + 1 - cx2
			if dx2*dx2+dy2*dy2 <= r2 {
				row[x] = idx
			}
		}
	}
}

func buildQRIslandMask(code *qr.Code) []bool {
	size := code.Size
	mask := make([]bool, size*size)
	markRect := func(x0, y0, w, h int) {
		for y := y0; y < y0+h; y++ {
			if y < 0 || y >= size {
				continue
			}
			row := y * size
			for x := x0; x < x0+w; x++ {
				if x < 0 || x >= size {
					continue
				}
				mask[row+x] = true
			}
		}
	}

	// Finder patterns are always 7x7 in 3 corners.
	markRect(0, 0, 7, 7)
	markRect(size-7, 0, 7, 7)
	markRect(0, size-7, 7, 7)

	// Detect alignment patterns directly from the encoded module matrix.
	// This avoids version-center math drift and keeps islands stable across sizes.
	for cy := 2; cy <= size-3; cy++ {
		for cx := 2; cx <= size-3; cx++ {
			if !isAlignmentCenter(code, cx, cy) {
				continue
			}
			markRect(cx-2, cy-2, 5, 5)
		}
	}
	return mask
}

func isAlignmentCenter(code *qr.Code, cx, cy int) bool {
	size := code.Size
	if cx-2 < 0 || cy-2 < 0 || cx+2 >= size || cy+2 >= size {
		return false
	}
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			ax := absInt(dx)
			ay := absInt(dy)
			wantBlack := ax == 2 || ay == 2 || (ax == 0 && ay == 0)
			if code.Black(cx+dx, cy+dy) != wantBlack {
				return false
			}
		}
	}
	return true
}
