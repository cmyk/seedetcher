package printer

import (
	"fmt"
	"strconv"
	"strings"
)

type LaserCalibrationKind string

const (
	LaserCalibrationPowerGrid LaserCalibrationKind = "power-grid"
)

type LaserCalibrationBuilder struct {
	Kind          LaserCalibrationKind
	PlateMM       float64
	CalibrationMM float64
	OffsetXMM     float64
	OffsetYMM     float64
	Powers        []int
	Feeds         []float64
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
	default:
		return nil, fmt.Errorf("unsupported laser calibration kind: %s", b.Kind)
	}
}

func (b LaserCalibrationBuilder) buildPowerGridScene() (PlateScene, error) {
	if len(b.Powers) == 0 || len(b.Feeds) == 0 {
		return PlateScene{}, fmt.Errorf("power-grid calibration requires at least one power and one feed")
	}
	labelFontMM := maxFloat(6.0, b.CalibrationMM*0.12)
	sampleTextMM := 11.0
	marginMM := maxFloat(0.8, b.CalibrationMM*0.02)
	labelColWMM := maxFloat(4.5, b.CalibrationMM*0.09)
	labelRowHMM := maxFloat(6.0, b.CalibrationMM*0.12)
	cellGapMM := maxFloat(0.6, b.CalibrationMM*0.015)

	gridW := b.CalibrationMM - 2*marginMM - labelColWMM
	gridH := b.CalibrationMM - 2*marginMM - labelRowHMM
	if gridW <= 0 || gridH <= 0 {
		return PlateScene{}, fmt.Errorf("plate too small for calibration layout")
	}
	cellW := (gridW - cellGapMM*float64(len(b.Feeds)-1)) / float64(len(b.Feeds))
	cellH := (gridH - cellGapMM*float64(len(b.Powers)-1)) / float64(len(b.Powers))
	if cellW <= 0 || cellH <= 0 {
		return PlateScene{}, fmt.Errorf("invalid calibration grid sizing")
	}
	mask := SceneLayer{Tag: "mask", Visible: true}

	gridX := marginMM + labelColWMM
	gridY := marginMM + labelRowHMM
	for c, feed := range b.Feeds {
		x := gridX + float64(c)*(cellW+cellGapMM)
		mask.Primitives = append(mask.Primitives,
			newSceneText(x+cellW/2, marginMM+labelRowHMM*0.65, fmt.Sprintf("%.0f", feed), labelFontMM, 0.01, TextDirHorizontal, TextAnchorCenter),
		)
	}
	topLabelBaselineY := marginMM + labelRowHMM*0.65
	topLabelGap := gridY - topLabelBaselineY
	for r, power := range b.Powers {
		y := gridY + float64(r)*(cellH+cellGapMM)
		rowLabel := fmt.Sprintf("%d", power)
		rowRotW, rowRotH := rotatedInkSizeMMTracked(loadFace(labelFontMM, 600.0), 600.0, rowLabel, 0)
		rowX := gridX - topLabelGap - rowRotW
		rowY := y + (cellH-rowRotH)/2
		if rowY < marginMM {
			rowY = marginMM
		}
		mask.Primitives = append(mask.Primitives,
			newSceneText(rowX, rowY, rowLabel, labelFontMM, 0.01, TextDirVerticalUp, TextAnchorTopLeft),
		)
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
			mask.Primitives = append(mask.Primitives,
				ScenePrimitive{
					Kind:      PrimitiveRound,
					XMM:       rectX,
					YMM:       rectY,
					WidthMM:   rectW,
					HeightMM:  rectH,
					RadiusMM:  minFloat(rectW, rectH) * 0.14,
					FillColor: sceneBlack,
					FillMode:  FillModeHatch,
					PowerS:    power,
					FeedMMMin: feed,
				},
				ScenePrimitive{
					Kind:      PrimitiveCircle,
					CXMM:      circleCX,
					CYMM:      circleCY,
					RadiusMM:  circleR,
					FillColor: sceneBlack,
					FillMode:  FillModeSpiral,
					PowerS:    power,
					FeedMMMin: feed,
				},
				newSceneText(textX, textY, "SEED", sampleTextMM, 0.04, TextDirHorizontal, TextAnchorBaselineLeft),
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

func ParseLaserCalibrationKind(v string) (LaserCalibrationKind, error) {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "", string(LaserCalibrationPowerGrid):
		return LaserCalibrationPowerGrid, nil
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
