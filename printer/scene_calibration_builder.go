package printer

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/image/font"
)

type LaserCalibrationKind string

const (
	LaserCalibrationPowerGrid LaserCalibrationKind = "power-grid"
	LaserCalibrationTestTile  LaserCalibrationKind = "test-tile"
)

type LaserCalibrationBuilder struct {
	Kind          LaserCalibrationKind
	PlateMM       float64
	CalibrationMM float64
	OffsetXMM     float64
	OffsetYMM     float64
	Powers        []int
	Feeds         []float64
	RowLaserModes []string
	TilePowerS    int
	TileFeedMMMin float64
	Title         string
}

func (b LaserCalibrationBuilder) Build() (*PlateDocument, error) {
	if b.PlateMM <= 0 {
		b.PlateMM = 100
	}
	if b.CalibrationMM <= 0 {
		b.CalibrationMM = 50
	}
	if b.CalibrationMM > b.PlateMM {
		return nil, fmt.Errorf("calibration area %.3fmm exceeds plate %.3fmm", b.CalibrationMM, b.PlateMM)
	}
	if b.OffsetXMM < 0 || b.OffsetYMM < 0 || b.OffsetXMM+b.CalibrationMM > b.PlateMM || b.OffsetYMM+b.CalibrationMM > b.PlateMM {
		return nil, fmt.Errorf(
			"calibration area %.3fmm with offset (%.3f,%.3f) exceeds plate %.3fmm",
			b.CalibrationMM, b.OffsetXMM, b.OffsetYMM, b.PlateMM,
		)
	}
	switch b.Kind {
	case LaserCalibrationPowerGrid:
		scene, err := b.buildPowerGridScene()
		if err != nil {
			return nil, err
		}
		return &PlateDocument{
			Version: sceneVersion,
			Scenes:  []PlateScene{scene},
		}, nil
	case LaserCalibrationTestTile:
		scene, err := b.buildTestTileScene()
		if err != nil {
			return nil, err
		}
		return &PlateDocument{
			Version: sceneVersion,
			Scenes:  []PlateScene{scene},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported laser calibration kind: %s", b.Kind)
	}
}

func (b LaserCalibrationBuilder) buildPowerGridScene() (PlateScene, error) {
	if len(b.Powers) == 0 || len(b.Feeds) == 0 {
		return PlateScene{}, fmt.Errorf("power-grid calibration requires at least one power and one feed")
	}
	annotationPower := maxInt(1, maxIntSlice(b.Powers)/2)
	annotationFeed := maxFloat(900, maxFloatSlice(b.Feeds))
	labelFontMM := maxFloat(6.0, b.CalibrationMM*0.12)
	sampleTextMM := 11.0
	sideMarginMM := maxFloat(0.8, b.CalibrationMM*0.02)
	topMarginMM := maxFloat(0.8, b.CalibrationMM*0.016)
	bottomMarginMM := maxFloat(3.0, b.CalibrationMM*0.06)
	labelColWMM := maxFloat(4.5, b.CalibrationMM*0.09)
	labelRowHMM := maxFloat(6.0, b.CalibrationMM*0.12)
	cellGapMM := maxFloat(0.6, b.CalibrationMM*0.015)

	gridW := b.CalibrationMM - 2*sideMarginMM - labelColWMM
	gridH := b.CalibrationMM - topMarginMM - bottomMarginMM - labelRowHMM
	if gridW <= 0 || gridH <= 0 {
		return PlateScene{}, fmt.Errorf("plate too small for calibration layout")
	}
	cellW := (gridW - cellGapMM*float64(len(b.Feeds)-1)) / float64(len(b.Feeds))
	cellH := (gridH - cellGapMM*float64(len(b.Powers)-1)) / float64(len(b.Powers))
	if cellW <= 0 || cellH <= 0 {
		return PlateScene{}, fmt.Errorf("invalid calibration grid sizing")
	}
	mask := SceneLayer{Tag: "mask", Visible: true}

	gridX := sideMarginMM + labelColWMM
	gridY := topMarginMM + labelRowHMM
	for c, feed := range b.Feeds {
		x := gridX + float64(c)*(cellW+cellGapMM)
		label := newSceneText(x+cellW/2, topMarginMM+labelRowHMM*0.65, fmt.Sprintf("%.0f", feed), labelFontMM, 0.01, TextDirHorizontal, TextAnchorCenter)
		label.FillMode = FillModeNone
		label.PowerS = annotationPower
		label.FeedMMMin = annotationFeed
		mask.Primitives = append(mask.Primitives,
			label,
		)
	}
	topLabelBaselineY := topMarginMM + labelRowHMM*0.65
	topLabelGap := gridY - topLabelBaselineY
	for r, power := range b.Powers {
		rowLaserOnCmd := ""
		if len(b.RowLaserModes) > 0 {
			rowLaserOnCmd = b.RowLaserModes[r]
		}
		y := gridY + float64(r)*(cellH+cellGapMM)
		rowLabel := fmt.Sprintf("%d", power)
		rowRotW, rowRotH := rotatedInkSizeMMTracked(loadFace(labelFontMM, 600.0), 600.0, rowLabel, 0)
		rowX := gridX - topLabelGap - rowRotW
		rowY := y + (cellH-rowRotH)/2
		if rowY < topMarginMM {
			rowY = topMarginMM
		}
		rowText := newSceneText(rowX, rowY, rowLabel, labelFontMM, 0.01, TextDirVerticalUp, TextAnchorTopLeft)
		rowText.FillMode = FillModeNone
		rowText.PowerS = annotationPower
		rowText.FeedMMMin = annotationFeed
		mask.Primitives = append(mask.Primitives, rowText)
		for c, feed := range b.Feeds {
			x := gridX + float64(c)*(cellW+cellGapMM)
			box := ScenePrimitive{
				Kind:        PrimitiveRect,
				XMM:         x,
				YMM:         y,
				WidthMM:     cellW,
				HeightMM:    cellH,
				StrokeColor: sceneBlack,
				StrokeMM:    0.15,
				PowerS:      annotationPower,
				FeedMMMin:   annotationFeed,
			}
			mask.Primitives = append(mask.Primitives, box)
			contentInset := maxFloat(0.6, minFloat(cellW, cellH)*0.08)
			contentY := y + contentInset
			contentH := cellH - 2*contentInset
			rectW := (cellW - 2.2*contentInset) * 0.42
			if rectW < 1.2 {
				rectW = 1.2
			}
			circleR := 0.45
			rectH := contentH * 0.56
			if rectH < 1.6 {
				rectH = 1.6
			}
			rectX := x + contentInset
			rectY := contentY + 0.02*contentH
			circleCX := rectX + rectW + contentInset*0.55 + circleR + 1.5
			maxCircleCX := x + cellW - contentInset - circleR
			if circleCX > maxCircleCX {
				circleCX = maxCircleCX
			}
			circleCY := contentY + contentH*0.28
			textX := x + contentInset - 0.2
			textY := y + cellH - contentInset
			sampleText := newSceneText(textX, textY, "SEED", sampleTextMM, 0.04, TextDirHorizontal, TextAnchorBaselineLeft)
			sampleText.PowerS = power
			sampleText.FeedMMMin = feed
			sampleText.LaserOnCmd = rowLaserOnCmd
			mask.Primitives = append(mask.Primitives,
				ScenePrimitive{
					Kind:       PrimitiveRound,
					XMM:        rectX,
					YMM:        rectY,
					WidthMM:    rectW,
					HeightMM:   rectH,
					RadiusMM:   minFloat(rectW, rectH) * 0.14,
					FillColor:  sceneBlack,
					FillMode:   FillModeHatch,
					PowerS:     power,
					FeedMMMin:  feed,
					LaserOnCmd: rowLaserOnCmd,
				},
				ScenePrimitive{
					Kind:       PrimitiveCircle,
					CXMM:       circleCX,
					CYMM:       circleCY,
					RadiusMM:   circleR,
					FillColor:  sceneBlack,
					FillMode:   FillModeSpiral,
					PowerS:     power,
					FeedMMMin:  feed,
					LaserOnCmd: rowLaserOnCmd,
				},
				sampleText,
			)
		}
	}

	return PlateScene{
		Name:             "laser_calibration_power_grid",
		WidthMM:          b.CalibrationMM,
		HeightMM:         b.CalibrationMM,
		AnchorInPlate:    "origin",
		OffsetInPlateXMM: b.OffsetXMM,
		OffsetInPlateYMM: b.OffsetYMM,
		Layers:           []SceneLayer{mask},
	}, nil
}

func (b LaserCalibrationBuilder) buildTestTileScene() (PlateScene, error) {
	if b.CalibrationMM <= 0 {
		b.CalibrationMM = 25
	}
	if b.TilePowerS <= 0 {
		b.TilePowerS = 1000
	}
	if b.TileFeedMMMin <= 0 {
		b.TileFeedMMMin = 900
	}
	annotationPower := maxInt(1, b.TilePowerS/2)
	annotationFeed := maxFloat(900, b.TileFeedMMMin)
	sizeMM := b.CalibrationMM
	marginMM := maxFloat(1.0, sizeMM*0.04)
	mask := SceneLayer{Tag: "mask", Visible: true}
	wordStyle := newTestTileWordStyle()
	qrStyle := newTestTileRegularSeedQRStyle()

	// Tile outline for placement/debug.
	mask.Primitives = append(mask.Primitives, ScenePrimitive{
		Kind:        PrimitiveRect,
		XMM:         0,
		YMM:         0,
		WidthMM:     sizeMM,
		HeightMM:    sizeMM,
		StrokeColor: sceneBlack,
		StrokeMM:    0.12,
		PowerS:      annotationPower,
		FeedMMMin:   annotationFeed,
	})

	baselineY := marginMM + wordStyle.baselineOffsetMM()
	mask.Primitives = append(mask.Primitives, wordStyle.linePrimitives(marginMM, baselineY, "20", "VAGUE", b.TilePowerS, b.TileFeedMMMin)...)
	mask.Primitives = append(mask.Primitives, wordStyle.linePrimitives(marginMM, baselineY+wordStyle.leading(), "18", "CHOIC", b.TilePowerS, b.TileFeedMMMin)...)
	mask.Primitives = append(mask.Primitives, wordStyle.linePrimitives(marginMM, baselineY+2*wordStyle.leading(), "13", "CURVE", b.TilePowerS, b.TileFeedMMMin)...)

	step := qrStyle.moduleStepMM()
	finderX := marginMM
	finderY := sizeMM - marginMM - 7*step
	mask.Primitives = append(mask.Primitives, qrStyle.finder7Primitives(finderX, finderY, b.TilePowerS, b.TileFeedMMMin)...)

	smallFinderX := finderX + 7*step + step
	smallFinderY := finderY
	mask.Primitives = append(mask.Primitives, qrStyle.finder5WithDotPrimitives(smallFinderX, smallFinderY, b.TilePowerS, b.TileFeedMMMin)...)

	patchX := smallFinderX + 5*step + step
	patchY := finderY
	mask.Primitives = append(mask.Primitives, qrStyle.dotPatchPrimitives(patchX, patchY, []string{
		"10110",
		"01101",
		"11011",
		"00110",
		"10101",
	}, b.TilePowerS, b.TileFeedMMMin)...)

	return PlateScene{
		Name:             "laser_calibration_test_tile",
		WidthMM:          sizeMM,
		HeightMM:         sizeMM,
		AnchorInPlate:    "origin",
		OffsetInPlateXMM: b.OffsetXMM,
		OffsetInPlateYMM: b.OffsetYMM,
		Layers:           []SceneLayer{mask},
	}, nil
}

type testTileWordStyle struct {
	dpi          float64
	sizePt       float64
	wordTrackEM  float64
	numTrackEM   float64
	leadingMM    float64
	numWordGapMM float64
	face         font.Face
}

type testTileRegularSeedQRStyle struct {
	sizeMM       float64
	quietModules int
	codeSize     int
}

func newTestTileRegularSeedQRStyle() testTileRegularSeedQRStyle {
	return testTileRegularSeedQRStyle{
		sizeMM:       seedQRSizeMM,
		quietModules: 0,
		// Regular multisig 2/3 seed-plate seed QR is 24-word payload -> 29 modules at ECC M.
		codeSize: 29,
	}
}

func newTestTileWordStyle() testTileWordStyle {
	const (
		dpi          = 600.0
		sizePt       = 14.0
		wordTrackEM  = 0.12
		numTrackEM   = 0.05
		leadingMM    = 15.2 * 25.4 / 72.0
		numWordGapMM = 0.5
	)
	return testTileWordStyle{
		dpi:          dpi,
		sizePt:       sizePt,
		wordTrackEM:  wordTrackEM,
		numTrackEM:   numTrackEM,
		leadingMM:    leadingMM,
		numWordGapMM: numWordGapMM,
		face:         loadFace(sizePt, dpi),
	}
}

func (s testTileWordStyle) baselineOffsetMM() float64 {
	return capBaselineOffsetMM(s.face, s.dpi)
}

func (s testTileWordStyle) leading() float64 {
	return s.leadingMM
}

func (s testTileWordStyle) linePrimitives(xMM, baselineYMM float64, num, word string, power int, feed float64) []ScenePrimitive {
	wordTrackPx := s.wordTrackEM * s.sizePt * s.dpi / 72.0
	numTrackPx := s.numTrackEM * s.sizePt * s.dpi / 72.0
	numColW := trackedTextWidthMM(s.face, s.dpi, "24", numTrackPx)
	spaceW := trackedTextWidthMM(s.face, s.dpi, " ", wordTrackPx) + s.numWordGapMM
	numW := trackedTextWidthMM(s.face, s.dpi, num, numTrackPx)
	numText := newSceneText(xMM+numColW-numW, baselineYMM, num, s.sizePt, s.numTrackEM, TextDirHorizontal, TextAnchorBaselineLeft)
	numText.PowerS = power
	numText.FeedMMMin = feed
	wordText := newSceneText(xMM+numColW+spaceW, baselineYMM, word, s.sizePt, s.wordTrackEM, TextDirHorizontal, TextAnchorBaselineLeft)
	wordText.PowerS = power
	wordText.FeedMMMin = feed
	return []ScenePrimitive{numText, wordText}
}

func (s testTileRegularSeedQRStyle) moduleStepMM() float64 {
	return s.sizeMM / float64(s.codeSize+2*s.quietModules)
}

func (s testTileRegularSeedQRStyle) moduleRadiusMM() float64 {
	return s.moduleStepMM() * plateQRDotScale / 2
}

func (s testTileRegularSeedQRStyle) finder7Primitives(xMM, yMM float64, power int, feed float64) []ScenePrimitive {
	step := s.moduleStepMM()
	size := 7 * step
	return []ScenePrimitive{
		sceneRingPrimitive(xMM, yMM, size, size, step, step*plateQRPatternCornerRadiusRatio),
		{
			Kind:      PrimitiveRound,
			XMM:       xMM + 2*step,
			YMM:       yMM + 2*step,
			WidthMM:   3 * step,
			HeightMM:  3 * step,
			RadiusMM:  step * plateQRPatternCornerRadiusRatio,
			FillColor: sceneBlack,
			PowerS:    power,
			FeedMMMin: feed,
		},
	}
}

func (s testTileRegularSeedQRStyle) finder5WithDotPrimitives(xMM, yMM float64, power int, feed float64) []ScenePrimitive {
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

func (s testTileRegularSeedQRStyle) dotPatchPrimitives(xMM, yMM float64, pattern []string, power int, feed float64) []ScenePrimitive {
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

func ParseLaserCalibrationKind(v string) (LaserCalibrationKind, error) {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "", string(LaserCalibrationPowerGrid):
		return LaserCalibrationPowerGrid, nil
	case string(LaserCalibrationTestTile):
		return LaserCalibrationTestTile, nil
	default:
		return "", fmt.Errorf("unknown laser calibration kind: %s", v)
	}
}

func ParseCalibrationPowers(v string) ([]int, error) {
	parts := strings.Split(v, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid calibration power %q: %w", p, err)
		}
		if n <= 0 {
			return nil, fmt.Errorf("invalid calibration power %q: must be > 0", p)
		}
		out = append(out, n)
	}
	return out, nil
}

func ParseCalibrationFeeds(v string) ([]float64, error) {
	parts := strings.Split(v, ",")
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid calibration feed %q: %w", p, err)
		}
		if n <= 0 {
			return nil, fmt.Errorf("invalid calibration feed %q: must be > 0", p)
		}
		out = append(out, n)
	}
	return out, nil
}

func ParseCalibrationLaserModes(v string, rows int) ([]string, error) {
	if strings.TrimSpace(v) == "" {
		return nil, nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		mode := strings.ToUpper(strings.TrimSpace(p))
		switch mode {
		case "M3", "M4":
			out = append(out, mode)
		case "":
			continue
		default:
			return nil, fmt.Errorf("invalid calibration laser mode %q: expected m3 or m4", p)
		}
	}
	if len(out) == 1 && rows > 1 {
		mode := out[0]
		out = make([]string, rows)
		for i := range out {
			out[i] = mode
		}
	}
	if rows > 0 && len(out) != rows {
		return nil, fmt.Errorf("invalid calibration laser modes: got %d mode(s), want 1 or %d", len(out), rows)
	}
	return out, nil
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxIntSlice(vs []int) int {
	if len(vs) == 0 {
		return 0
	}
	m := vs[0]
	for _, v := range vs[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func maxFloatSlice(vs []float64) float64 {
	if len(vs) == 0 {
		return 0
	}
	m := vs[0]
	for _, v := range vs[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func minPositiveFloat(vs []float64, fallback float64) float64 {
	m := 0.0
	for _, v := range vs {
		if v <= 0 {
			continue
		}
		if m == 0 || v < m {
			m = v
		}
	}
	if m == 0 {
		return fallback
	}
	return m
}
