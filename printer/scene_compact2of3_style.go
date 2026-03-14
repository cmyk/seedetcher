package printer

import "golang.org/x/image/font"

type compact2of3WordStyle struct {
	dpi          float64
	sizePt       float64
	trackingEM   float64
	leadingMM    float64
	numWordGapMM float64
	face         font.Face
}

func newCompact2of3WordStyle() compact2of3WordStyle {
	const (
		dpi          = 600.0
		sizePt       = 11.0
		trackingEM   = 0.04
		leadingMM    = 9.8 * 25.4 / 72.0
		numWordGapMM = 0.1
	)
	return compact2of3WordStyle{
		dpi:          dpi,
		sizePt:       sizePt,
		trackingEM:   trackingEM,
		leadingMM:    leadingMM,
		numWordGapMM: numWordGapMM,
		face:         loadFace(sizePt, dpi),
	}
}

func (s compact2of3WordStyle) baselineOffsetMM() float64 {
	return capBaselineOffsetMM(s.face, s.dpi)
}

func (s compact2of3WordStyle) leading() float64 {
	return s.leadingMM
}

func (s compact2of3WordStyle) linePrimitives(xMM, baselineYMM float64, num, word string, power int, feed float64) []ScenePrimitive {
	trackPx := s.trackingEM * s.sizePt * s.dpi / 72.0
	numColW := trackedTextWidthMM(s.face, s.dpi, "24", trackPx)
	spaceW := trackedTextWidthMM(s.face, s.dpi, " ", trackPx) + s.numWordGapMM
	numW := trackedTextWidthMM(s.face, s.dpi, num, trackPx)
	numText := newSceneText(xMM+numColW-numW, baselineYMM, num, s.sizePt, s.trackingEM, TextDirHorizontal, TextAnchorBaselineLeft)
	numText.PowerS = power
	numText.FeedMMMin = feed
	wordText := newSceneText(xMM+numColW+spaceW, baselineYMM, word, s.sizePt, s.trackingEM, TextDirHorizontal, TextAnchorBaselineLeft)
	wordText.PowerS = power
	wordText.FeedMMMin = feed
	return []ScenePrimitive{numText, wordText}
}

type compact2of3SeedQRStyle struct {
	sizeMM       float64
	quietModules int
	opts         plateQROptions
}

func newCompact2of3SeedQRStyle() compact2of3SeedQRStyle {
	return compact2of3SeedQRStyle{
		sizeMM:       27.0,
		quietModules: 4,
		opts: plateQROptions{
			QuietModules:      4,
			Shape:             plateQRCircle,
			KeepIslandsSquare: true,
		},
	}
}

func (s compact2of3SeedQRStyle) moduleStepMM() float64 {
	codeSize := 25
	return s.sizeMM / float64(codeSize+2*s.quietModules)
}

func (s compact2of3SeedQRStyle) moduleRadiusMM() float64 {
	return s.moduleStepMM() * plateQRDotScale / 2
}

func (s compact2of3SeedQRStyle) smallFinderPrimitives(xMM, yMM float64, power int, feed float64) []ScenePrimitive {
	step := s.moduleStepMM()
	size := 5 * step
	return []ScenePrimitive{
		sceneRingPrimitive(xMM, yMM, size, size, step, step*plateQRPatternCornerRadiusRatio),
		{
			Kind:      PrimitiveCircle,
			CXMM:      xMM + size/2,
			CYMM:      yMM + size/2,
			RadiusMM:  s.moduleRadiusMM(),
			FillColor: sceneBlack,
			PowerS:    power,
			FeedMMMin: feed,
		},
	}
}

func (s compact2of3SeedQRStyle) dotPatchPrimitives(xMM, yMM float64, pattern []string, power int, feed float64) []ScenePrimitive {
	step := s.moduleStepMM()
	r := s.moduleRadiusMM()
	var out []ScenePrimitive
	for row, line := range pattern {
		for col, ch := range line {
			if ch != '1' {
				continue
			}
			out = append(out, ScenePrimitive{
				Kind:      PrimitiveCircle,
				CXMM:      xMM + float64(col)*step + step/2,
				CYMM:      yMM + float64(row)*step + step/2,
				RadiusMM:  r,
				FillColor: sceneBlack,
				PowerS:    power,
				FeedMMMin: feed,
			})
		}
	}
	return out
}
