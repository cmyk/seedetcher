package printer

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

func drawTrackedText(img *image.Paletted, face font.Face, dpi, xMm, yMm float64, text string, trackingPx float64) {
	d := font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.Black),
		Face: face,
		Dot: fixed.Point26_6{
			X: fixed.I(int(math.Round(mmToPxFloat(xMm, dpi)))),
			Y: fixed.I(int(math.Round(mmToPxFloat(yMm, dpi)))),
		},
	}
	rs := []rune(text)
	trackFixed := fixed.I(int(math.Round(trackingPx)))
	for i, r := range rs {
		d.DrawString(string(r))
		if i < len(rs)-1 && trackingPx > 0 {
			d.Dot.X += trackFixed
		}
	}
}

func trackedTextWidthMM(face font.Face, dpi float64, text string, trackingPx float64) float64 {
	rs := []rune(text)
	if len(rs) == 0 {
		return 0
	}
	var width fixed.Int26_6
	trackFixed := fixed.I(int(math.Round(trackingPx)))
	for i, r := range rs {
		if adv, ok := face.GlyphAdvance(r); ok {
			width += adv
		}
		if i < len(rs)-1 && trackingPx > 0 {
			width += trackFixed
		}
	}
	return float64(width.Ceil()) * 25.4 / dpi
}

func rotatedTextSizeMM(face font.Face, dpi float64, text string) (wMm, hMm float64) {
	return rotatedTextSizeMMTracked(face, dpi, text, 0)
}

func rotatedTextSizeMMTracked(face font.Face, dpi float64, text string, trackingPx float64) (wMm, hMm float64) {
	if text == "" {
		return 0, 0
	}
	wPx := trackedTextWidthPx(face, text, trackingPx)
	if wPx <= 0 {
		return 0, 0
	}
	m := face.Metrics()
	hPx := (m.Ascent + m.Descent).Ceil()
	if hPx <= 0 {
		return 0, 0
	}
	// Rotated CW 90: width/height swap.
	return float64(hPx) * 25.4 / dpi, float64(wPx) * 25.4 / dpi
}

func rotatedInkSizeMMTracked(face font.Face, dpi float64, text string, trackingPx float64) (wMm, hMm float64) {
	src := rasterizeTextAlpha(face, text, trackingPx)
	if src == nil {
		return 0, 0
	}
	minX, minY, maxX, maxY, ok := alphaInkBounds(src)
	if !ok {
		return 0, 0
	}
	trimW := maxX - minX + 1
	trimH := maxY - minY + 1
	return float64(trimH) * 25.4 / dpi, float64(trimW) * 25.4 / dpi
}

func drawTextRotatedCCW90Tracked(img *image.Paletted, face font.Face, dpi, xMm, yMm float64, text string, idx uint8, trackingPx float64) {
	src := rasterizeTextAlpha(face, text, trackingPx)
	if src == nil {
		return
	}
	minX, minY, maxX, maxY, ok := alphaInkBounds(src)
	if !ok {
		return
	}
	trimW := maxX - minX + 1
	trimH := maxY - minY + 1

	x0 := mmToPx(xMm, dpi)
	y0 := mmToPx(yMm, dpi)
	b := img.Bounds()
	for sy := 0; sy < trimH; sy++ {
		for sx := 0; sx < trimW; sx++ {
			tx := minX + sx
			ty := minY + sy
			if src.AlphaAt(tx, ty).A == 0 {
				continue
			}
			dx := sy
			dy := trimW - 1 - sx
			x := x0 + dx
			y := y0 + dy
			if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
				continue
			}
			img.Pix[y*img.Stride+x] = idx
		}
	}
}

func alphaInkBounds(src *image.Alpha) (minX, minY, maxX, maxY int, ok bool) {
	b := src.Bounds()
	minX, minY = b.Max.X, b.Max.Y
	maxX, maxY = b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := src.Pix[y*src.Stride:]
		for x := b.Min.X; x < b.Max.X; x++ {
			if row[x] == 0 {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
			ok = true
		}
	}
	return
}

func trackedTextWidthPx(face font.Face, text string, trackingPx float64) int {
	rs := []rune(text)
	if len(rs) == 0 {
		return 0
	}
	var width fixed.Int26_6
	trackFixed := fixed.I(int(math.Round(trackingPx)))
	for i, r := range rs {
		if adv, ok := face.GlyphAdvance(r); ok {
			width += adv
		}
		if i < len(rs)-1 && trackingPx > 0 {
			width += trackFixed
		}
	}
	return width.Ceil()
}

func rasterizeTextAlpha(face font.Face, text string, trackingPx float64) *image.Alpha {
	if text == "" {
		return nil
	}
	srcW := trackedTextWidthPx(face, text, trackingPx)
	if srcW <= 0 {
		return nil
	}
	metrics := face.Metrics()
	srcH := (metrics.Ascent + metrics.Descent).Ceil()
	if srcH <= 0 {
		return nil
	}
	src := image.NewAlpha(image.Rect(0, 0, srcW, srcH))
	d := font.Drawer{
		Dst:  src,
		Src:  image.NewUniform(color.Alpha{A: 0xff}),
		Face: face,
		Dot: fixed.Point26_6{
			X: 0,
			Y: fixed.I(metrics.Ascent.Ceil()),
		},
	}
	rs := []rune(text)
	trackFixed := fixed.I(int(math.Round(trackingPx)))
	for i, r := range rs {
		d.DrawString(string(r))
		if i < len(rs)-1 && trackingPx > 0 {
			d.Dot.X += trackFixed
		}
	}
	return src
}

func capBaselineOffsetMM(face font.Face, dpi float64) float64 {
	// Anchor by uppercase cap height so the visible top of uppercase letters
	// sits on the requested margin.
	b, _ := font.BoundString(face, "H")
	minYpx := float64(b.Min.Y) / 64.0
	if minYpx >= 0 {
		return float64(face.Metrics().Ascent.Ceil()) * 25.4 / dpi
	}
	return (-minYpx) * 25.4 / dpi
}

func wrapTextTracked(face font.Face, dpi float64, text string, maxWidthMm float64, trackingPx float64) []string {
	var lines []string
	if maxWidthMm <= 0 {
		return []string{text}
	}
	maxPx := int(math.Round(mmToPxFloat(maxWidthMm, dpi)))
	if maxPx <= 0 {
		return []string{text}
	}

	var buf []rune
	for _, r := range text {
		buf = append(buf, r)
		if trackedTextWidthPx(face, string(buf), trackingPx) > maxPx {
			// Overflow: push previous run and start new line with current rune.
			if len(buf) > 1 {
				lines = append(lines, string(buf[:len(buf)-1]))
				buf = buf[len(buf)-1:]
			} else {
				// Single rune too wide; force as line.
				lines = append(lines, string(buf))
				buf = buf[:0]
			}
		}
	}
	if len(buf) > 0 {
		lines = append(lines, string(buf))
	}
	return lines
}
