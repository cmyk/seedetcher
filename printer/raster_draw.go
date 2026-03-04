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

func fillRectWithRoundedCorners(img *image.Paletted, x, y, w, h, radius int, fg, bg uint8, cornerMask uint8) {
	if w <= 0 || h <= 0 {
		return
	}
	fillRect(img, x, y, w, h, fg)
	if radius <= 0 || cornerMask == 0 {
		return
	}
	if radius > w/2 {
		radius = w / 2
	}
	if radius > h/2 {
		radius = h / 2
	}
	if radius <= 0 {
		return
	}
	r2 := radius * radius
	for dy := 0; dy < radius; dy++ {
		for dx := 0; dx < radius; dx++ {
			// Keep pixels inside quarter-circle, clear outside.
			inQuarter := dx*dx+dy*dy <= r2
			if inQuarter {
				continue
			}
			if cornerMask&cornerTL != 0 {
				px, py := x+radius-1-dx, y+radius-1-dy
				if px >= img.Rect.Min.X && px < img.Rect.Max.X && py >= img.Rect.Min.Y && py < img.Rect.Max.Y {
					img.Pix[py*img.Stride+px] = bg
				}
			}
			if cornerMask&cornerTR != 0 {
				px, py := x+w-radius+dx, y+radius-1-dy
				if px >= img.Rect.Min.X && px < img.Rect.Max.X && py >= img.Rect.Min.Y && py < img.Rect.Max.Y {
					img.Pix[py*img.Stride+px] = bg
				}
			}
			if cornerMask&cornerBL != 0 {
				px, py := x+radius-1-dx, y+h-radius+dy
				if px >= img.Rect.Min.X && px < img.Rect.Max.X && py >= img.Rect.Min.Y && py < img.Rect.Max.Y {
					img.Pix[py*img.Stride+px] = bg
				}
			}
			if cornerMask&cornerBR != 0 {
				px, py := x+w-radius+dx, y+h-radius+dy
				if px >= img.Rect.Min.X && px < img.Rect.Max.X && py >= img.Rect.Min.Y && py < img.Rect.Max.Y {
					img.Pix[py*img.Stride+px] = bg
				}
			}
		}
	}
}
