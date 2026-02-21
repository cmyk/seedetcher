package printer

import "image"

func applyPostProcess(img *image.Paletted, opts RasterOptions) {
	if opts.Mirror {
		mirrorHorizontal(img)
	}
}

func invertAll(img *image.Paletted) {
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		row := img.Pix[y*img.Stride:]
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			if row[x] == 0 {
				row[x] = 1
			} else if row[x] == 1 {
				row[x] = 0
			}
		}
	}
}

// invertInterior flips black/white inside the plate while preserving the outer border.
func invertInterior(img *image.Paletted, borderPx int) {
	if borderPx <= 0 {
		invertAll(img)
		return
	}
	b := img.Bounds()
	x0 := b.Min.X + borderPx
	y0 := b.Min.Y + borderPx
	x1 := b.Max.X - borderPx
	y1 := b.Max.Y - borderPx
	if x0 >= x1 || y0 >= y1 {
		return
	}
	for y := y0; y < y1; y++ {
		row := img.Pix[y*img.Stride:]
		for x := x0; x < x1; x++ {
			if row[x] == 0 {
				row[x] = 1
			} else if row[x] == 1 {
				row[x] = 0
			}
		}
	}
}

func mirrorHorizontal(img *image.Paletted) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	for y := 0; y < h; y++ {
		row := img.Pix[y*img.Stride:]
		for x := 0; x < w/2; x++ {
			row[x], row[w-1-x] = row[w-1-x], row[x]
		}
	}
}
