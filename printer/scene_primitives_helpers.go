package printer

import "github.com/kortschak/qr"

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
			moduleX := xMM + offset + float64(x)*step
			moduleY := yMM + offset + float64(y)*step
			if opts.KeepIslandsSquare && islands[y*code.Size+x] {
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
	return out
}
