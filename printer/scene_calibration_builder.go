package printer

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"golang.org/x/image/font"
)

type LaserCalibrationKind string

const (
	LaserCalibrationPowerGrid LaserCalibrationKind = "power-grid"
	LaserCalibrationTestTile  LaserCalibrationKind = "test-tile"
	LaserCalibrationLineWidth LaserCalibrationKind = "line-width-tile"
	LaserCalibrationFillStep  LaserCalibrationKind = "fill-step-test"
	LaserCalibrationPowerFeed LaserCalibrationKind = "power-feed-test"
	LaserCalibrationRepeat    LaserCalibrationKind = "repeatability-test"
	LaserCalibrationSectorRep LaserCalibrationKind = "sector-repeatability-test"
)

type LaserCalibrationBuilder struct {
	Kind                  LaserCalibrationKind
	PlateMM               float64
	CalibrationMM         float64
	OffsetXMM             float64
	OffsetYMM             float64
	Powers                []int
	Feeds                 []float64
	RowLaserModes         []string
	TilePowerS            int
	TileFeedMMMin         float64
	TileLaserMode         string
	TileFillStepMM        float64
	TileOutlinePowerScale float64
	TileOutlineFeedScale  float64
	FillStepFeeds         []float64
	FillStepSteps         []float64
	PowerFeedPowers       []int
	PowerFeedFeeds        []float64
	Title                 string
}

func (b LaserCalibrationBuilder) Build() (*PlateDocument, error) {
	if b.PlateMM <= 0 {
		b.PlateMM = 100
	}
	if b.CalibrationMM <= 0 {
		switch b.Kind {
		case LaserCalibrationRepeat, LaserCalibrationSectorRep:
			b.CalibrationMM = 150
		default:
			b.CalibrationMM = 50
		}
	}
	if b.Kind == LaserCalibrationSectorRep {
		if b.CalibrationMM <= b.PlateMM {
			return nil, fmt.Errorf(
				"sector-repeatability test needs calibration area > plate (got %.3fmm <= %.3fmm)",
				b.CalibrationMM, b.PlateMM,
			)
		}
		if b.OffsetXMM < 0 || b.OffsetYMM < 0 {
			return nil, fmt.Errorf("calibration offset must be non-negative (got %.3f,%.3f)", b.OffsetXMM, b.OffsetYMM)
		}
	} else {
		if b.CalibrationMM > b.PlateMM {
			return nil, fmt.Errorf("calibration area %.3fmm exceeds plate %.3fmm", b.CalibrationMM, b.PlateMM)
		}
		if b.OffsetXMM < 0 || b.OffsetYMM < 0 || b.OffsetXMM+b.CalibrationMM > b.PlateMM || b.OffsetYMM+b.CalibrationMM > b.PlateMM {
			return nil, fmt.Errorf(
				"calibration area %.3fmm with offset (%.3f,%.3f) exceeds plate %.3fmm",
				b.CalibrationMM, b.OffsetXMM, b.OffsetYMM, b.PlateMM,
			)
		}
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
	case LaserCalibrationLineWidth:
		scene, err := b.buildLineWidthTileScene()
		if err != nil {
			return nil, err
		}
		return &PlateDocument{
			Version: sceneVersion,
			Scenes:  []PlateScene{scene},
		}, nil
	case LaserCalibrationFillStep:
		scene, err := b.buildFillStepTileScene()
		if err != nil {
			return nil, err
		}
		return &PlateDocument{
			Version: sceneVersion,
			Scenes:  []PlateScene{scene},
		}, nil
	case LaserCalibrationPowerFeed:
		scene, err := b.buildPowerFeedTileScene()
		if err != nil {
			return nil, err
		}
		return &PlateDocument{
			Version: sceneVersion,
			Scenes:  []PlateScene{scene},
		}, nil
	case LaserCalibrationRepeat:
		scene, err := b.buildRepeatabilityScene()
		if err != nil {
			return nil, err
		}
		return &PlateDocument{
			Version: sceneVersion,
			Scenes:  []PlateScene{scene},
		}, nil
	case LaserCalibrationSectorRep:
		scene, err := b.buildSectorRepeatabilityScene()
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
	annotationPower := maxInt(1, maxIntSlice(b.Powers))
	annotationFeed := maxFloat(1, minPositiveFloat(b.Feeds, maxFloatSlice(b.Feeds)))
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
	annotationPower := maxInt(1, b.TilePowerS)
	annotationFeed := maxFloat(1, b.TileFeedMMMin)
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
	infoFillStep := b.TileFillStepMM
	if infoFillStep <= 0 {
		infoFillStep = 0.12
	}
	infoOutlinePowerScale := b.TileOutlinePowerScale
	if infoOutlinePowerScale <= 0 {
		infoOutlinePowerScale = 1
	}
	infoOutlineFeedScale := b.TileOutlineFeedScale
	if infoOutlineFeedScale <= 0 {
		infoOutlineFeedScale = 1
	}
	infoText := fmt.Sprintf("%s/%d/%s/%s/%s",
		formatCompactFloat(b.TileFeedMMMin),
		b.TilePowerS,
		formatCompactFloat(infoFillStep),
		formatCompactFloat(infoOutlinePowerScale),
		formatCompactFloat(infoOutlineFeedScale),
	)

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

	// Keep test metadata exactly where requested: directly below the small finder-with-dot.
	const (
		infoHeightMM = 1.5
		infoHSpaceEM = 0.05 // LightBurn hSpace 5 equivalent.
	)
	infoPt := infoHeightMM * 72.0 / 25.4
	infoX := smallFinderX
	infoY := smallFinderY + 5*step + maxFloat(0.80, step*0.72)
	info := newSceneText(infoX, infoY, infoText, infoPt, infoHSpaceEM, TextDirHorizontal, TextAnchorTopLeft)
	info.FillMode = FillModeHatch
	info.NoOutline = true
	info.FillStepMM = infoFillStep
	info.PowerS = annotationPower
	info.FeedMMMin = annotationFeed
	mask.Primitives = append(mask.Primitives, info)

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

func (b LaserCalibrationBuilder) buildLineWidthTileScene() (PlateScene, error) {
	if b.CalibrationMM <= 0 {
		b.CalibrationMM = 25
	}
	if b.TilePowerS <= 0 {
		b.TilePowerS = 1000
	}
	if b.TileFeedMMMin <= 0 {
		b.TileFeedMMMin = 900
	}
	sizeMM := b.CalibrationMM
	marginMM := maxFloat(1.0, sizeMM*0.04)
	annotationPower := maxInt(1, b.TilePowerS)
	annotationFeed := maxFloat(1, b.TileFeedMMMin)
	mask := SceneLayer{Tag: "mask", Visible: true}

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

	mode := strings.ToUpper(strings.TrimSpace(b.TileLaserMode))
	if mode != "M3" && mode != "M4" {
		mode = "M4"
	}
	title := strings.TrimSpace(b.Title)
	if title == "" {
		title = fmt.Sprintf("Line-Width Test S%d (%s)", b.TilePowerS, mode)
	}
	titlePt := maxFloat(4.8, sizeMM*0.16)
	titleY := marginMM + titlePt*25.4/72.0
	titleText := newSceneText(marginMM, titleY, title, titlePt, 0.0, TextDirHorizontal, TextAnchorBaselineLeft)
	titleText.FillMode = FillModeHatch
	titleText.NoOutline = true
	titleText.FillStepMM = 0.10
	titleText.PowerS = annotationPower
	titleText.FeedMMMin = annotationFeed
	mask.Primitives = append(mask.Primitives, titleText)

	feeds := append([]float64(nil), b.Feeds...)
	if len(feeds) == 0 {
		base := b.TileFeedMMMin
		feeds = []float64{
			base * 0.6,
			base * 0.7,
			base * 0.8,
			base * 0.9,
			base,
			base * 1.1,
			base * 1.2,
			base * 1.3,
			base * 1.4,
		}
	}
	if len(feeds) > 9 {
		feeds = feeds[:9]
	}
	if len(feeds) == 0 {
		return PlateScene{}, fmt.Errorf("line-width tile requires at least one feed")
	}
	powers := make([]int, len(feeds))
	for i := range powers {
		powers[i] = b.TilePowerS
		if len(b.Powers) == 1 {
			powers[i] = b.Powers[0]
		} else if i < len(b.Powers) && b.Powers[i] > 0 {
			powers[i] = b.Powers[i]
		}
		if powers[i] <= 0 {
			powers[i] = b.TilePowerS
		}
	}

	lineStartX := maxFloat(6.5, sizeMM*0.26)
	labelX := (marginMM + lineStartX) / 2
	lineEndX := sizeMM - marginMM
	topY := maxFloat(3.2, sizeMM*0.13)
	topY = maxFloat(topY, titleY+titlePt*25.4/72.0+0.8)
	bottomY := sizeMM - maxFloat(2.8, sizeMM*0.11)
	stepY := 0.0
	if len(feeds) > 1 {
		stepY = (bottomY - topY) / float64(len(feeds)-1)
	}
	labelPt := maxFloat(4.6, sizeMM*0.18)

	for i := range feeds {
		y := topY + float64(i)*stepY
		line := ScenePrimitive{
			Kind:        PrimitivePath,
			PathData:    fmt.Sprintf("M%.4f %.4fL%.4f %.4f", lineStartX, y, lineEndX, y),
			FillMode:    FillModeNone,
			StrokeColor: sceneBlack,
			StrokeMM:    0.1,
			PowerS:      powers[i],
			FeedMMMin:   feeds[i],
		}
		mask.Primitives = append(mask.Primitives, line)

		label := newSceneText(labelX, y, fmt.Sprintf("%.0f", feeds[i]), labelPt, 0.01, TextDirHorizontal, TextAnchorCenter)
		label.FillMode = FillModeHatch
		label.NoOutline = true
		label.FillStepMM = 0.10
		label.PowerS = annotationPower
		label.FeedMMMin = annotationFeed
		mask.Primitives = append(mask.Primitives, label)
	}

	return PlateScene{
		Name:             "laser_calibration_line_width_tile",
		WidthMM:          sizeMM,
		HeightMM:         sizeMM,
		AnchorInPlate:    "origin",
		OffsetInPlateXMM: b.OffsetXMM,
		OffsetInPlateYMM: b.OffsetYMM,
		Layers:           []SceneLayer{mask},
	}, nil
}

func (b LaserCalibrationBuilder) buildFillStepTileScene() (PlateScene, error) {
	if b.CalibrationMM <= 0 {
		b.CalibrationMM = 25
	}
	if b.TilePowerS <= 0 {
		b.TilePowerS = 850
	}
	if b.TileFeedMMMin <= 0 {
		b.TileFeedMMMin = 2000
	}
	sizeMM := b.CalibrationMM
	marginMM := maxFloat(1.0, sizeMM*0.04)
	annotationPower := maxInt(1, b.TilePowerS)
	annotationFeed := maxFloat(1, b.TileFeedMMMin)
	mask := SceneLayer{Tag: "mask", Visible: true}

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

	feeds := append([]float64(nil), b.FillStepFeeds...)
	if len(feeds) == 0 {
		feeds = []float64{1400, 1700, 2000, 2300, 2600}
	}
	if len(feeds) != 5 {
		return PlateScene{}, fmt.Errorf("fill-step test needs exactly 5 feed values (got %d)", len(feeds))
	}
	fillSteps := append([]float64(nil), b.FillStepSteps...)
	if len(fillSteps) == 0 {
		fillSteps = []float64{0.03, 0.035, 0.04, 0.05, 0.06}
	}
	if len(fillSteps) != 5 {
		return PlateScene{}, fmt.Errorf("fill-step test needs exactly 5 step values (got %d)", len(fillSteps))
	}

	title := strings.TrimSpace(b.Title)
	if title == "" {
		mode := strings.ToUpper(strings.TrimSpace(b.TileLaserMode))
		if mode != "M3" && mode != "M4" {
			mode = "M4"
		}
		title = fmt.Sprintf("Fill-Step Test S%d (%s)", b.TilePowerS, mode)
	}

	titlePt := maxFloat(3.9, sizeMM*0.15)
	topLabelPt := maxFloat(3.2, sizeMM*0.125)
	rowLabelPt := maxFloat(3.2, sizeMM*0.125)
	topTextY := marginMM + titlePt*25.4/72.0
	titleText := newSceneText(marginMM, topTextY, title, titlePt, 0.01, TextDirHorizontal, TextAnchorBaselineLeft)
	titleText.FillMode = FillModeNone
	titleText.PowerS = annotationPower
	titleText.FeedMMMin = annotationFeed
	mask.Primitives = append(mask.Primitives, titleText)

	gridTop := topTextY + maxFloat(0.9, sizeMM*0.036)
	topLabelH := topLabelPt * 25.4 / 72.0
	rowLabelW := maxFloat(4.8, sizeMM*0.19)
	cellGap := maxFloat(0.2, sizeMM*0.008)
	gridX := marginMM + rowLabelW
	rawGridY := gridTop + topLabelH + maxFloat(0.55, sizeMM*0.022)
	availW := sizeMM - marginMM - gridX
	availH := sizeMM - marginMM - rawGridY
	if availW <= 0 || availH <= 0 {
		return PlateScene{}, fmt.Errorf("plate too small for fill-step layout")
	}
	cellWAvail := (availW - 4*cellGap) / 5
	cellHAvail := (availH - 4*cellGap) / 5
	cellSize := minFloat(cellWAvail, cellHAvail)
	if cellSize <= 1.0 {
		return PlateScene{}, fmt.Errorf("invalid fill-step grid sizing")
	}
	gridH := 5*cellSize + 4*cellGap
	extraH := availH - gridH
	if extraH < 0 {
		extraH = 0
	}
	// Spend available slack above the grid so column labels are not cramped.
	gridY := rawGridY + extraH
	cellW := cellSize
	cellH := cellSize

	for c := 0; c < 5; c++ {
		x := gridX + float64(c)*(cellW+cellGap)
		label := newSceneText(x+cellW/2, gridTop+topLabelH*0.88, formatCompactFloat(fillSteps[c]), topLabelPt, 0.0, TextDirHorizontal, TextAnchorCenter)
		label.FillMode = FillModeNone
		label.PowerS = annotationPower
		label.FeedMMMin = annotationFeed
		mask.Primitives = append(mask.Primitives, label)
	}

	for r := 0; r < 5; r++ {
		y := gridY + float64(r)*(cellH+cellGap)
		rowLabel := newSceneText(marginMM, y+cellH*0.76, fmt.Sprintf("F%.0f", feeds[r]), rowLabelPt, 0.01, TextDirHorizontal, TextAnchorBaselineLeft)
		rowLabel.FillMode = FillModeNone
		rowLabel.PowerS = annotationPower
		rowLabel.FeedMMMin = annotationFeed
		mask.Primitives = append(mask.Primitives, rowLabel)
		for c := 0; c < 5; c++ {
			x := gridX + float64(c)*(cellW+cellGap)
			mask.Primitives = append(mask.Primitives, ScenePrimitive{
				Kind:       PrimitiveRect,
				XMM:        x,
				YMM:        y,
				WidthMM:    cellW,
				HeightMM:   cellH,
				FillColor:  sceneBlack,
				FillMode:   FillModeHatch,
				FillStepMM: fillSteps[c],
				NoOutline:  true,
				PowerS:     b.TilePowerS,
				FeedMMMin:  feeds[r],
			})
		}
	}

	return PlateScene{
		Name:             "laser_calibration_fill_step_test",
		WidthMM:          sizeMM,
		HeightMM:         sizeMM,
		AnchorInPlate:    "origin",
		OffsetInPlateXMM: b.OffsetXMM,
		OffsetInPlateYMM: b.OffsetYMM,
		Layers:           []SceneLayer{mask},
	}, nil
}

func (b LaserCalibrationBuilder) buildPowerFeedTileScene() (PlateScene, error) {
	if b.CalibrationMM <= 0 {
		b.CalibrationMM = 25
	}
	if b.TilePowerS <= 0 {
		b.TilePowerS = 850
	}
	if b.TileFeedMMMin <= 0 {
		b.TileFeedMMMin = 2000
	}
	sizeMM := b.CalibrationMM
	annotationPower := maxInt(1, b.TilePowerS)
	annotationFeed := maxFloat(1, b.TileFeedMMMin)
	fillStep := b.TileFillStepMM
	if fillStep <= 0 {
		fillStep = 0.04
	}
	mask := SceneLayer{Tag: "mask", Visible: true}

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

	powers := append([]int(nil), b.PowerFeedPowers...)
	if len(powers) == 0 {
		factors := []float64{0.60, 0.70, 0.80, 0.90, 1.00}
		powers = make([]int, len(factors))
		for i, f := range factors {
			p := int(math.Round(float64(b.TilePowerS) * f))
			if p < 1 {
				p = 1
			}
			powers[i] = p
		}
	}
	if len(powers) != 5 {
		return PlateScene{}, fmt.Errorf("power-feed test needs exactly 5 power values (got %d)", len(powers))
	}
	feeds := append([]float64(nil), b.PowerFeedFeeds...)
	if len(feeds) == 0 {
		feeds = []float64{1400, 1700, 2000, 2300, 2600}
	}
	if len(feeds) != 5 {
		return PlateScene{}, fmt.Errorf("power-feed test needs exactly 5 feed values (got %d)", len(feeds))
	}

	title := strings.TrimSpace(b.Title)
	if title == "" {
		mode := strings.ToUpper(strings.TrimSpace(b.TileLaserMode))
		if mode != "M3" && mode != "M4" {
			mode = "M4"
		}
		title = fmt.Sprintf("Power-Feed Test (%s, %s)", mode, formatCompactFloat(fillStep))
	}

	labelDPI := 600.0
	labelCapMM := 1.0
	labelPt := fontSizeForCapHeightMM(labelCapMM, labelDPI)
	labelFace := loadFace(labelPt, labelDPI)
	labelCapActualMM := capBaselineOffsetMM(labelFace, labelDPI)
	if labelCapActualMM <= 0 {
		labelCapActualMM = labelCapMM
	}
	rowTrackEM := 0.0
	rowTrackPx := rowTrackEM * labelPt * labelDPI / 72.0
	maxRowLabelW := 0.0
	for _, f := range feeds {
		w := trackedTextWidthMM(labelFace, labelDPI, formatCompactFloat(f), rowTrackPx)
		if w > maxRowLabelW {
			maxRowLabelW = w
		}
	}
	const labelFillStep = 0.10
	applyRasterLabelStyle := func(p *ScenePrimitive) {
		p.FillMode = FillModeHatch
		p.NoOutline = true
		// Keep calibration labels raster-only but coarser than tile fills for speed/readability.
		p.FillStepMM = labelFillStep
		p.PowerS = annotationPower
		p.FeedMMMin = annotationFeed
	}

	cellSizeMM := 3.0
	cellGapMM := 1.0
	gridW := 5*cellSizeMM + 4*cellGapMM
	gridH := 5*cellSizeMM + 4*cellGapMM

	leftPadMM := snapMMToPixel(maxFloat(0.5, sizeMM*0.02), labelDPI)
	rightPadMM := snapMMToPixel(maxFloat(0.5, sizeMM*0.02), labelDPI)
	topPadMM := snapMMToPixel(maxFloat(0.5, sizeMM*0.02), labelDPI)
	rowLabelGapMM := snapMMToPixel(0.7, labelDPI)
	titleGapMM := snapMMToPixel(0.8, labelDPI)
	colLabelGapMM := snapMMToPixel(0.8, labelDPI)
	colLabelDropMM := snapMMToPixel(1.0, labelDPI)

	rowLabelX := leftPadMM
	gridX := snapMMToPixel(rowLabelX+maxRowLabelW+rowLabelGapMM, labelDPI)
	if gridX+gridW > sizeMM-rightPadMM {
		gridX = sizeMM - rightPadMM - gridW
		gridX = snapMMToPixel(gridX, labelDPI)
	}
	if gridX < rowLabelX+maxRowLabelW {
		return PlateScene{}, fmt.Errorf("power-feed layout too tight for labels in %.1fmm area", sizeMM)
	}

	titleX := leftPadMM
	titleY := snapMMToPixel(topPadMM+labelCapActualMM, labelDPI)
	colLabelYBase := snapMMToPixel(titleY+labelCapActualMM+titleGapMM, labelDPI)
	colLabelY := snapMMToPixel(colLabelYBase+colLabelDropMM, labelDPI)
	gridY := snapMMToPixel(colLabelYBase+labelCapActualMM+colLabelGapMM, labelDPI)

	if gridY+gridH > sizeMM-topPadMM {
		return PlateScene{}, fmt.Errorf("power-feed layout overflow: grid exceeds %.1fmm area", sizeMM)
	}

	titleText := newSceneText(titleX, titleY, title, labelPt, 0.0, TextDirHorizontal, TextAnchorBaselineLeft)
	applyRasterLabelStyle(&titleText)
	mask.Primitives = append(mask.Primitives, titleText)

	for c := 0; c < 5; c++ {
		x := snapMMToPixel(gridX+float64(c)*(cellSizeMM+cellGapMM), labelDPI)
		s := fmt.Sprintf("%d", powers[c])
		w := trackedTextWidthMM(labelFace, labelDPI, s, 0)
		label := newSceneText(snapMMToPixel(x+(cellSizeMM-w)/2, labelDPI), colLabelY, s, labelPt, 0.0, TextDirHorizontal, TextAnchorBaselineLeft)
		applyRasterLabelStyle(&label)
		mask.Primitives = append(mask.Primitives, label)
	}

	for r := 0; r < 5; r++ {
		y := snapMMToPixel(gridY+float64(r)*(cellSizeMM+cellGapMM), labelDPI)
		rowLabelY := snapMMToPixel(y+cellSizeMM*0.5+labelCapActualMM*0.5, labelDPI)
		rowLabel := newSceneText(rowLabelX, rowLabelY, formatCompactFloat(feeds[r]), labelPt, rowTrackEM, TextDirHorizontal, TextAnchorBaselineLeft)
		applyRasterLabelStyle(&rowLabel)
		mask.Primitives = append(mask.Primitives, rowLabel)
		for c := 0; c < 5; c++ {
			x := snapMMToPixel(gridX+float64(c)*(cellSizeMM+cellGapMM), labelDPI)
			mask.Primitives = append(mask.Primitives, ScenePrimitive{
				Kind:       PrimitiveRect,
				XMM:        x,
				YMM:        y,
				WidthMM:    cellSizeMM,
				HeightMM:   cellSizeMM,
				FillColor:  sceneBlack,
				FillMode:   FillModeHatch,
				FillStepMM: fillStep,
				NoOutline:  true,
				PowerS:     powers[c],
				FeedMMMin:  feeds[r],
			})
		}
	}

	return PlateScene{
		Name:             "laser_calibration_power_feed_test",
		WidthMM:          sizeMM,
		HeightMM:         sizeMM,
		AnchorInPlate:    "origin",
		OffsetInPlateXMM: b.OffsetXMM,
		OffsetInPlateYMM: b.OffsetYMM,
		Layers:           []SceneLayer{mask},
	}, nil
}

func (b LaserCalibrationBuilder) buildRepeatabilityScene() (PlateScene, error) {
	if b.CalibrationMM <= 0 {
		b.CalibrationMM = 150
	}
	if b.TilePowerS <= 0 {
		b.TilePowerS = 850
	}
	if b.TileFeedMMMin <= 0 {
		b.TileFeedMMMin = 2000
	}
	sizeMM := b.CalibrationMM
	if sizeMM < 60 {
		return PlateScene{}, fmt.Errorf("repeatability test needs at least 60mm area (got %.3f)", sizeMM)
	}
	marginMM := maxFloat(8.0, sizeMM*0.08)
	annotationPower := maxInt(1, b.TilePowerS)
	annotationFeed := maxFloat(1, b.TileFeedMMMin)
	mask := SceneLayer{Tag: "mask", Visible: true}

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

	mode := strings.ToUpper(strings.TrimSpace(b.TileLaserMode))
	if mode != "M3" && mode != "M4" {
		mode = "M4"
	}
	const laps = 12
	title := strings.TrimSpace(b.Title)
	if title == "" {
		title = fmt.Sprintf("Repeatability Test L%d (%s)", laps, mode)
	}
	titlePt := maxFloat(4.2, sizeMM*0.036)
	titleY := marginMM*0.75 + titlePt*25.4/72.0
	titleText := newSceneText(marginMM, titleY, title, titlePt, 0.01, TextDirHorizontal, TextAnchorBaselineLeft)
	titleText.FillMode = FillModeNone
	titleText.PowerS = annotationPower
	titleText.FeedMMMin = annotationFeed
	mask.Primitives = append(mask.Primitives, titleText)

	// Reserve a top band for title so upper target labels never collide with it.
	headerBandMM := maxFloat(10.0, sizeMM*0.07)
	yTop := marginMM + headerBandMM
	yBottom := sizeMM - marginMM
	if yTop >= yBottom-10 {
		yTop = marginMM + maxFloat(6.0, sizeMM*0.05)
	}

	type target struct {
		name string
		x    float64
		y    float64
	}
	targets := []target{
		{name: "LL", x: marginMM, y: yBottom},
		{name: "LR", x: sizeMM - marginMM, y: yBottom},
		{name: "UR", x: sizeMM - marginMM, y: yTop},
		{name: "UL", x: marginMM, y: yTop},
		{name: "C", x: sizeMM * 0.5, y: sizeMM * 0.5},
	}
	labelPt := maxFloat(3.6, sizeMM*0.028)
	labelFace := loadFace(labelPt, 600.0)
	labelOffset := maxFloat(1.6, sizeMM*0.012)
	labelH := labelPt * 25.4 / 72.0
	for _, t := range targets {
		labelW := trackedTextWidthMM(labelFace, 600.0, t.name, 0)
		lx := t.x + labelOffset
		if t.x > sizeMM*0.5 {
			lx = t.x - labelOffset - labelW
		}
		ly := t.y - labelOffset
		if t.y <= yTop+1e-6 || t.name == "C" {
			ly = t.y + labelOffset + labelH
		}
		if t.name == "C" {
			lx = t.x + labelOffset
		}
		label := newSceneText(lx, ly, t.name, labelPt, 0.01, TextDirHorizontal, TextAnchorBaselineLeft)
		label.FillMode = FillModeNone
		label.PowerS = annotationPower
		label.FeedMMMin = annotationFeed
		mask.Primitives = append(mask.Primitives, label)
	}

	// Long-hop visit order to stress return-to-position repeatability.
	forward := []int{0, 2, 1, 3, 4} // LL -> UR -> LR -> UL -> C
	reverse := []int{4, 3, 1, 2, 0} // C -> UL -> LR -> UR -> LL
	half := maxFloat(0.4, minFloat(1.2, sizeMM*0.006))
	for lap := 0; lap < laps; lap++ {
		order := forward
		if lap%2 == 1 {
			order = reverse
		}
		for _, idx := range order {
			t := targets[idx]
			square := ScenePrimitive{
				Kind:        PrimitivePath,
				PathData:    fmt.Sprintf("M%.4f %.4fL%.4f %.4fL%.4f %.4fL%.4f %.4fZ", t.x-half, t.y-half, t.x+half, t.y-half, t.x+half, t.y+half, t.x-half, t.y+half),
				FillMode:    FillModeNone,
				StrokeColor: sceneBlack,
				StrokeMM:    0.10,
				PowerS:      b.TilePowerS,
				FeedMMMin:   b.TileFeedMMMin,
				LaserOnCmd:  mode,
			}
			mask.Primitives = append(mask.Primitives, square)
		}
	}

	return PlateScene{
		Name:             "laser_calibration_repeatability_test",
		WidthMM:          sizeMM,
		HeightMM:         sizeMM,
		AnchorInPlate:    "origin",
		OffsetInPlateXMM: b.OffsetXMM,
		OffsetInPlateYMM: b.OffsetYMM,
		Layers:           []SceneLayer{mask},
	}, nil
}

func (b LaserCalibrationBuilder) buildSectorRepeatabilityScene() (PlateScene, error) {
	if b.TilePowerS <= 0 {
		b.TilePowerS = 850
	}
	if b.TileFeedMMMin <= 0 {
		b.TileFeedMMMin = 2000
	}
	workspaceMM := b.CalibrationMM
	plateMM := b.PlateMM
	shiftMM := workspaceMM - plateMM
	if shiftMM <= 0 {
		return PlateScene{}, fmt.Errorf("sector-repeatability test needs calibration area > plate")
	}
	annotationPower := maxInt(1, b.TilePowerS)
	annotationFeed := maxFloat(1, b.TileFeedMMMin)
	mode := strings.ToUpper(strings.TrimSpace(b.TileLaserMode))
	if mode != "M3" && mode != "M4" {
		mode = "M4"
	}
	mask := SceneLayer{Tag: "mask", Visible: true}
	appendPatternedEdge := func(sb *strings.Builder, ax, ay, bx, by float64, pattern []float64) {
		dx := bx - ax
		dy := by - ay
		length := math.Hypot(dx, dy)
		if length <= 0 {
			return
		}
		if len(pattern) == 0 {
			pattern = []float64{length, 0}
		}
		ux := dx / length
		uy := dy / length
		pos := 0.0
		draw := true
		pidx := 0
		for pos < length-1e-9 {
			seg := pattern[pidx%len(pattern)]
			if seg <= 0 {
				pidx++
				draw = !draw
				continue
			}
			next := pos + seg
			if next > length {
				next = length
			}
			if draw {
				sx := ax + ux*pos
				sy := ay + uy*pos
				ex := ax + ux*next
				ey := ay + uy*next
				sb.WriteString(fmt.Sprintf("M%.4f %.4fL%.4f %.4f", sx, sy, ex, ey))
			}
			pos = next
			pidx++
			draw = !draw
		}
	}
	rectFramePath := func(x0, y0, x1, y1 float64, style string) string {
		var sb strings.Builder
		edges := [][4]float64{
			{x0, y0, x1, y0},
			{x1, y0, x1, y1},
			{x1, y1, x0, y1},
			{x0, y1, x0, y0},
		}
		switch style {
		case "dotted":
			for _, e := range edges {
				appendPatternedEdge(&sb, e[0], e[1], e[2], e[3], []float64{1.0, 2.0})
			}
		case "dashed":
			for _, e := range edges {
				appendPatternedEdge(&sb, e[0], e[1], e[2], e[3], []float64{7.0, 3.0})
			}
		case "dot-dash":
			for _, e := range edges {
				appendPatternedEdge(&sb, e[0], e[1], e[2], e[3], []float64{1.0, 2.0, 7.0, 2.0})
			}
		default:
			// Solid frame edges, segmented to avoid very long single G1 lines.
			for _, e := range edges {
				appendPatternedEdge(&sb, e[0], e[1], e[2], e[3], []float64{20.0, 0})
			}
		}
		return sb.String()
	}

	type markTarget struct {
		x float64
		y float64
	}
	type sector struct {
		name    string
		x       float64
		y       float64
		targets []markTarget
	}
	targetInset := maxFloat(6.0, minFloat(10.0, plateMM*0.08))
	makeTargets := func(x, y float64) []markTarget {
		return []markTarget{
			{x: x + targetInset, y: y + targetInset},
			{x: x + plateMM - targetInset, y: y + targetInset},
			{x: x + plateMM - targetInset, y: y + plateMM - targetInset},
			{x: x + targetInset, y: y + plateMM - targetInset},
			{x: x + plateMM*0.5, y: y + plateMM*0.5},
		}
	}
	sectors := []sector{
		{name: "BL", x: 0, y: shiftMM},
		{name: "BR", x: shiftMM, y: shiftMM},
		{name: "TL", x: 0, y: 0},
		{name: "TR", x: shiftMM, y: 0},
		{name: "C", x: shiftMM / 2, y: shiftMM / 2},
	}
	for i := range sectors {
		sectors[i].targets = makeTargets(sectors[i].x, sectors[i].y)
	}

	labelPt := maxFloat(3.2, workspaceMM*0.022)
	labelPad := maxFloat(1.5, plateMM*0.015)
	labelH := labelPt * 25.4 / 72.0
	labelFace := loadFace(labelPt, 600.0)
	edgeInset := maxFloat(0.6, minFloat(1.2, workspaceMM*0.006))
	for _, s := range sectors {
		x0 := maxFloat(s.x, edgeInset)
		y0 := maxFloat(s.y, edgeInset)
		x1 := minFloat(s.x+plateMM, workspaceMM-edgeInset)
		y1 := minFloat(s.y+plateMM, workspaceMM-edgeInset)
		frameStyle := "solid"
		switch s.name {
		case "TL":
			frameStyle = "dotted"
		case "TR":
			frameStyle = "dashed"
		case "BL":
			frameStyle = "dot-dash"
		}
		mask.Primitives = append(mask.Primitives, ScenePrimitive{
			Kind:        PrimitivePath,
			PathData:    rectFramePath(x0, y0, x1, y1, frameStyle),
			FillMode:    FillModeNone,
			StrokeColor: sceneBlack,
			StrokeMM:    0.12,
			PowerS:      annotationPower,
			FeedMMMin:   annotationFeed,
			LaserOnCmd:  mode,
		})
		labelX := s.x + labelPad
		labelY := s.y + labelPad + labelH
		labelAnchor := TextAnchorBaselineLeft
		labelW := trackedTextWidthMM(labelFace, 600.0, s.name, 0)
		switch s.name {
		case "BL":
			labelX = s.x + labelPad
			labelY = s.y + plateMM - labelPad
		case "BR":
			labelX = s.x + plateMM - labelPad - labelW
			labelY = s.y + plateMM - labelPad
		case "TL":
			labelX = s.x + labelPad
			labelY = s.y + labelPad + labelH
		case "TR":
			labelX = s.x + plateMM - labelPad - labelW
			labelY = s.y + labelPad + labelH
		case "C":
			labelX = s.x + plateMM/2
			labelY = s.y + plateMM/2
			labelAnchor = TextAnchorCenter
		}
		label := newSceneText(labelX, labelY, s.name, labelPt, 0.01, TextDirHorizontal, labelAnchor)
		label.FillMode = FillModeNone
		label.PowerS = annotationPower
		label.FeedMMMin = annotationFeed
		mask.Primitives = append(mask.Primitives, label)
	}

	const laps = 2
	targetForward := []int{0, 2, 4, 1, 3}
	targetReverse := []int{3, 1, 4, 2, 0}
	sectorOrder := []int{0, 1, 2, 3, 4}
	half := maxFloat(0.9, minFloat(1.8, plateMM*0.012))
	for _, sectorIdx := range sectorOrder {
		for lap := 0; lap < laps; lap++ {
			order := targetForward
			if lap%2 == 1 {
				order = targetReverse
			}
			for _, targetIdx := range order {
				t := sectors[sectorIdx].targets[targetIdx]
				mask.Primitives = append(mask.Primitives, ScenePrimitive{
					Kind:        PrimitivePath,
					PathData:    fmt.Sprintf("M%.4f %.4fL%.4f %.4fL%.4f %.4fL%.4f %.4fZ", t.x-half, t.y-half, t.x+half, t.y-half, t.x+half, t.y+half, t.x-half, t.y+half),
					FillMode:    FillModeNone,
					StrokeColor: sceneBlack,
					StrokeMM:    0.10,
					PowerS:      b.TilePowerS,
					FeedMMMin:   b.TileFeedMMMin,
					LaserOnCmd:  mode,
				})
			}
		}
	}

	return PlateScene{
		Name:             "laser_calibration_sector_repeatability_test",
		WidthMM:          workspaceMM,
		HeightMM:         workspaceMM,
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
	case string(LaserCalibrationLineWidth), "line-width":
		return LaserCalibrationLineWidth, nil
	case string(LaserCalibrationFillStep), "fill-step":
		return LaserCalibrationFillStep, nil
	case string(LaserCalibrationPowerFeed), "power-feed":
		return LaserCalibrationPowerFeed, nil
	case string(LaserCalibrationRepeat), "repeatability", "bed-repeatability":
		return LaserCalibrationRepeat, nil
	case string(LaserCalibrationSectorRep), "sector-repeatability", "sector-repeat", "sector-test", "bed-sector-repeatability":
		return LaserCalibrationSectorRep, nil
	default:
		return "", fmt.Errorf("unknown laser calibration kind: %s", v)
	}
}

func formatCompactFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func ParseCalibrationPowers(v string) ([]int, error) {
	return ParseCalibrationIntSeries(v, 4)
}

func ParseCalibrationIntSeries(v string, defaultRangeCount int) ([]int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	if strings.Contains(v, ":") && !strings.Contains(v, ",") {
		parts := strings.Split(v, ":")
		if len(parts) != 2 && len(parts) != 3 {
			return nil, fmt.Errorf("invalid range %q: expected start:end[:count]", v)
		}
		start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("invalid range start %q: %w", parts[0], err)
		}
		end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid range end %q: %w", parts[1], err)
		}
		if start <= 0 || end <= 0 {
			return nil, fmt.Errorf("invalid range %q: values must be > 0", v)
		}
		count := defaultRangeCount
		if count <= 0 {
			count = 5
		}
		if len(parts) == 3 {
			n, err := strconv.Atoi(strings.TrimSpace(parts[2]))
			if err != nil {
				return nil, fmt.Errorf("invalid range count %q: %w", parts[2], err)
			}
			count = n
		}
		if count < 2 {
			return nil, fmt.Errorf("invalid range count %d: must be >= 2", count)
		}
		out := make([]int, count)
		step := float64(end-start) / float64(count-1)
		for i := range out {
			out[i] = int(math.Round(float64(start) + float64(i)*step))
		}
		return out, nil
	}
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
	return ParseCalibrationFloatSeries(v, 4)
}

func ParseCalibrationFloatSeries(v string, defaultRangeCount int) ([]float64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	if strings.Contains(v, ":") && !strings.Contains(v, ",") {
		parts := strings.Split(v, ":")
		if len(parts) != 2 && len(parts) != 3 {
			return nil, fmt.Errorf("invalid range %q: expected start:end[:count]", v)
		}
		start, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid range start %q: %w", parts[0], err)
		}
		end, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid range end %q: %w", parts[1], err)
		}
		if start <= 0 || end <= 0 {
			return nil, fmt.Errorf("invalid range %q: values must be > 0", v)
		}
		count := defaultRangeCount
		if count <= 0 {
			count = 5
		}
		if len(parts) == 3 {
			n, err := strconv.Atoi(strings.TrimSpace(parts[2]))
			if err != nil {
				return nil, fmt.Errorf("invalid range count %q: %w", parts[2], err)
			}
			count = n
		}
		if count < 2 {
			return nil, fmt.Errorf("invalid range count %d: must be >= 2", count)
		}
		out := make([]float64, count)
		step := (end - start) / float64(count-1)
		for i := range out {
			out[i] = start + float64(i)*step
		}
		return out, nil
	}
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

func snapMMToPixel(mm, dpi float64) float64 {
	if dpi <= 0 {
		return mm
	}
	step := 25.4 / dpi
	return math.Round(mm/step) * step
}

func fontSizeForCapHeightMM(targetCapMM, dpi float64) float64 {
	if targetCapMM <= 0 {
		return 1
	}
	pt := targetCapMM * 72.0 / 25.4
	if pt <= 0 {
		pt = 1
	}
	for i := 0; i < 6; i++ {
		face := loadFace(pt, dpi)
		got := capBaselineOffsetMM(face, dpi)
		if got <= 0 {
			break
		}
		pt *= targetCapMM / got
		if pt <= 0 {
			pt = 1
			break
		}
	}
	return pt
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
