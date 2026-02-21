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
			if opts.KeepIslandsSquare && islandMask[y*code.Size+x] {
				useSquare = true
			}
			if useSquare {
				fillRect(img, xStart, yStart, xEnd-xStart, yEnd-yStart, idx)
			} else {
				fillModuleCircle(img, xStart, yStart, xEnd, yEnd, idx)
			}
		}
	}
}

func fillModuleCircle(img *image.Paletted, xStart, yStart, xEnd, yEnd int, idx uint8) {
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
	scaled := int(math.Round(float64(d) * plateQRDotScale))
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
