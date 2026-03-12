package printer

import (
	"fmt"
	"strings"

	"github.com/kortschak/qr"
)

func newSceneText(xMM, yMM float64, text string, sizePt, trackingEM float64, dir TextDirection, anchor TextAnchor) ScenePrimitive {
	if dir == "" {
		dir = TextDirHorizontal
	}
	if anchor == "" {
		anchor = TextAnchorBaselineLeft
	}
	return ScenePrimitive{
		Kind:       PrimitiveText,
		XMM:        xMM,
		YMM:        yMM,
		Text:       text,
		FontFamily: "SeedEtcher-Regular",
		FontSizePt: sizePt,
		TrackingEM: trackingEM,
		Direction:  dir,
		Anchor:     anchor,
		FillColor:  sceneBlack,
	}
}

func sceneQRModules(code *qr.Code, xMM, yMM, sizeMM float64, opts plateQROptions) []ScenePrimitive {
	if code == nil || code.Size <= 0 {
		return nil
	}
	quiet := opts.QuietModules
	if quiet < 0 {
		quiet = 0
	}
	step := sizeMM / float64(code.Size+2*quiet)
	offset := float64(quiet) * step
	patternRadiusRatio := opts.PatternCornerRadiusRatio
	if patternRadiusRatio < 0 {
		patternRadiusRatio = 0
	}
	if patternRadiusRatio == 0 {
		patternRadiusRatio = plateQRPatternCornerRadiusRatio
	}
	if patternRadiusRatio > 0.5 {
		patternRadiusRatio = 0.5
	}
	// Keep structural corner radius below half-cell so we preserve square-module look.
	if patternRadiusRatio > 0.5 {
		patternRadiusRatio = 0.5
	}
	patternRadiusMM := step * patternRadiusRatio
	if patternRadiusMM > step/2 {
		patternRadiusMM = step / 2
	}
	islands := []bool(nil)
	if opts.KeepIslandsSquare {
		islands = buildQRIslandMask(code)
	}
	out := make([]ScenePrimitive, 0, code.Size*code.Size/2)
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if !code.Black(x, y) {
				continue
			}
			if opts.KeepIslandsSquare && islands[y*code.Size+x] {
				// Structural modules (finder/alignment) are emitted as unified
				// shape primitives below, not as per-module cells.
				continue
			}
			moduleX := xMM + offset + float64(x)*step
			moduleY := yMM + offset + float64(y)*step
			if opts.Shape == plateQRSquare {
				out = append(out, ScenePrimitive{
					Kind:      PrimitiveRect,
					XMM:       moduleX,
					YMM:       moduleY,
					WidthMM:   step,
					HeightMM:  step,
					FillColor: sceneBlack,
				})
				continue
			}
			out = append(out, ScenePrimitive{
				Kind:      PrimitiveCircle,
				CXMM:      moduleX + step/2,
				CYMM:      moduleY + step/2,
				RadiusMM:  step * plateQRDotScale / 2,
				FillColor: sceneBlack,
			})
		}
	}
	if opts.KeepIslandsSquare {
		out = append(out, sceneFinderAndAlignmentShapes(code, xMM+offset, yMM+offset, step, patternRadiusMM)...)
	}
	return out
}

func isAlignmentCenterModule(code *qr.Code, x, y int) bool {
	size := code.Size
	if inFinderArea(x, y, size) {
		return false
	}
	return isAlignmentCenter(code, x, y)
}

func sceneFinderAndAlignmentShapes(code *qr.Code, originX, originY, step, r float64) []ScenePrimitive {
	if code == nil || code.Size <= 0 {
		return nil
	}
	out := make([]ScenePrimitive, 0, 16)
	size := code.Size
	finders := [][2]int{
		{0, 0},
		{size - 7, 0},
		{0, size - 7},
	}
	for _, f := range finders {
		fx, fy := f[0], f[1]
		x := originX + float64(fx)*step
		y := originY + float64(fy)*step
		out = append(out, sceneRingPrimitive(x, y, 7*step, 7*step, 1*step, r))
		out = append(out, ScenePrimitive{
			Kind:      PrimitivePath,
			PathData:  roundedRectPath(x+2*step, y+2*step, 3*step, 3*step, r, true),
			FillColor: sceneBlack,
		})
	}

	for cy := 2; cy <= size-3; cy++ {
		for cx := 2; cx <= size-3; cx++ {
			if inFinderArea(cx, cy, size) || !isAlignmentCenter(code, cx, cy) {
				continue
			}
			x := originX + float64(cx-2)*step
			y := originY + float64(cy-2)*step
			out = append(out, sceneRingPrimitive(x, y, 5*step, 5*step, 1*step, r))
			out = append(out, ScenePrimitive{
				Kind:      PrimitiveCircle,
				CXMM:      originX + float64(cx)*step + step/2,
				CYMM:      originY + float64(cy)*step + step/2,
				RadiusMM:  step * plateQRDotScale / 2,
				FillColor: sceneBlack,
			})
		}
	}
	return out
}

func sceneRingPrimitive(x, y, w, h, t, r float64) ScenePrimitive {
	if t < 0 {
		t = 0
	}
	if 2*t > w {
		t = w / 2
	}
	if 2*t > h {
		t = h / 2
	}
	outer := roundedRectPath(x, y, w, h, r, true)
	inner := roundedRectPath(x+t, y+t, w-2*t, h-2*t, r, true)
	return ScenePrimitive{
		Kind:      PrimitivePath,
		PathData:  strings.TrimSpace(outer + " " + inner),
		FillColor: sceneBlack,
		FillRule:  "evenodd",
	}
}

func roundedRectPath(x, y, w, h, r float64, clockwise bool) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	maxR := w / 2
	if h/2 < maxR {
		maxR = h / 2
	}
	if r < 0 {
		r = 0
	}
	if r > maxR {
		r = maxR
	}

	tl, tr, br, bl := r > 0, r > 0, r > 0, r > 0

	s := func(format string, args ...any) string { return fmt.Sprintf(format, args...) }
	if clockwise {
		path := ""
		if tl {
			path += s("M %.4f %.4f ", x+r, y)
		} else {
			path += s("M %.4f %.4f ", x, y)
		}
		if tr {
			path += s("L %.4f %.4f Q %.4f %.4f %.4f %.4f ", x+w-r, y, x+w, y, x+w, y+r)
		} else {
			path += s("L %.4f %.4f ", x+w, y)
		}
		if br {
			path += s("L %.4f %.4f Q %.4f %.4f %.4f %.4f ", x+w, y+h-r, x+w, y+h, x+w-r, y+h)
		} else {
			path += s("L %.4f %.4f ", x+w, y+h)
		}
		if bl {
			path += s("L %.4f %.4f Q %.4f %.4f %.4f %.4f ", x+r, y+h, x, y+h, x, y+h-r)
		} else {
			path += s("L %.4f %.4f ", x, y+h)
		}
		if tl {
			path += s("L %.4f %.4f Q %.4f %.4f %.4f %.4f Z", x, y+r, x, y, x+r, y)
		} else {
			path += "Z"
		}
		return path
	}
	path := ""
	if tl {
		path += s("M %.4f %.4f ", x+r, y)
	} else {
		path += s("M %.4f %.4f ", x, y)
	}
	if bl {
		path += s("L %.4f %.4f Q %.4f %.4f %.4f %.4f ", x, y+h-r, x, y+h, x+r, y+h)
	} else {
		path += s("L %.4f %.4f ", x, y+h)
	}
	if br {
		path += s("L %.4f %.4f Q %.4f %.4f %.4f %.4f ", x+w-r, y+h, x+w, y+h, x+w, y+h-r)
	} else {
		path += s("L %.4f %.4f ", x+w, y+h)
	}
	if tr {
		path += s("L %.4f %.4f Q %.4f %.4f %.4f %.4f ", x+w, y+r, x+w, y, x+w-r, y)
	} else {
		path += s("L %.4f %.4f ", x+w, y)
	}
	if tl {
		path += s("L %.4f %.4f Q %.4f %.4f %.4f %.4f Z", x+r, y, x, y, x, y+r)
	} else {
		path += "Z"
	}
	return path
}
