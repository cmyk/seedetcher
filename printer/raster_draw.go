package printer

import "image"

func strokeRect(img *image.Paletted, x, y, w, h, thickness int, idx uint8) {
	fillRect(img, x, y, w, thickness, idx)             // top
	fillRect(img, x, y+h-thickness, w, thickness, idx) // bottom
	fillRect(img, x, y, thickness, h, idx)             // left
	fillRect(img, x+w-thickness, y, thickness, h, idx) // right
}

func fillRect(img *image.Paletted, x, y, w, h int, idx uint8) {
	b := img.Bounds()
	x0, y0 := clampInt(x, b.Min.X, b.Max.X), clampInt(y, b.Min.Y, b.Max.Y)
	x1, y1 := clampInt(x+w, b.Min.X, b.Max.X), clampInt(y+h, b.Min.Y, b.Max.Y)
	if x1 <= x0 || y1 <= y0 {
		return
	}
	for yy := y0; yy < y1; yy++ {
		row := img.Pix[yy*img.Stride:]
		for xx := x0; xx < x1; xx++ {
			row[xx] = idx
		}
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
