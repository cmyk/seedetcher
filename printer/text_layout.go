package printer

import (
	"image"
	"strings"

	"golang.org/x/image/font"
)

type TextAlign uint8

const (
	TextAlignStart TextAlign = iota
	TextAlignCenter
	TextAlignEnd
)

type TextBlock struct {
	Face      font.Face
	Tracking  float64
	LeadingMM float64
	WidthMM   float64
	Align     TextAlign
	OriginXMM float64
	OriginYMM float64 // baseline of first line
}

type TextBlockResult struct {
	Lines           []string
	BoundsMM        image.Rectangle
	NextBaselineYMM float64
}

func DrawMetaLine(img *image.Paletted, dpi float64, xMM, baselineYMM float64, face font.Face, tracking float64, text string) {
	drawTrackedText(img, face, dpi, xMM, baselineYMM, text, tracking)
}

func DrawRotatedLabel(img *image.Paletted, dpi float64, xMM, yMM float64, face font.Face, tracking float64, idx uint8, text string) {
	drawTextRotatedCCW90Tracked(img, face, dpi, xMM, yMM, text, idx, tracking)
}

func DrawTextBlock(img *image.Paletted, dpi float64, block TextBlock, text string) TextBlockResult {
	if block.Face == nil {
		return TextBlockResult{Lines: nil, NextBaselineYMM: block.OriginYMM}
	}
	leading := block.LeadingMM
	if leading <= 0 {
		m := block.Face.Metrics()
		leading = float64((m.Ascent + m.Descent).Ceil()) * 25.4 / dpi
	}
	lines := wrapTrackedParagraphs(block.Face, dpi, text, block.WidthMM, block.Tracking)
	y := block.OriginYMM
	for _, line := range lines {
		lineW := trackedTextWidthMM(block.Face, dpi, line, block.Tracking)
		x := block.OriginXMM
		switch block.Align {
		case TextAlignCenter:
			x += (block.WidthMM - lineW) / 2
		case TextAlignEnd:
			x += block.WidthMM - lineW
		}
		drawTrackedText(img, block.Face, dpi, x, y, line, block.Tracking)
		y += leading
	}
	return TextBlockResult{
		Lines:           lines,
		BoundsMM:        image.Rect(mmToPx(block.OriginXMM, dpi), mmToPx(block.OriginYMM, dpi), mmToPx(block.OriginXMM+block.WidthMM, dpi), mmToPx(y, dpi)),
		NextBaselineYMM: y,
	}
}

func wrapTrackedParagraphs(face font.Face, dpi float64, text string, maxWidthMm float64, trackingPx float64) []string {
	parts := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, wrapTextTracked(face, dpi, p, maxWidthMm, trackingPx)...)
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}
