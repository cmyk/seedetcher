package printer

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSceneGCodeRenderer_Render(t *testing.T) {
	doc := &PlateDocument{
		Version: sceneVersion,
		Scenes: []PlateScene{
			{
				Name:     "seed 01",
				WidthMM:  90,
				HeightMM: 90,
				Layers: []SceneLayer{
					{
						Tag:     "mask",
						Visible: true,
						Primitives: []ScenePrimitive{
							{Kind: PrimitiveRect, XMM: 5, YMM: 5, WidthMM: 20, HeightMM: 10, StrokeColor: sceneBlack, StrokeMM: 0.2},
							{Kind: PrimitiveCircle, CXMM: 40, CYMM: 40, RadiusMM: 3, FillColor: sceneBlack},
							{Kind: PrimitivePath, PathData: "M50 50Q55 45 60 50C62 52 64 54 66 56"},
						},
					},
				},
			},
		},
	}

	outDir := t.TempDir()
	if err := (SceneGCodeRenderer{}).Render(doc, outDir); err != nil {
		t.Fatalf("Render: %v", err)
	}

	path := filepath.Join(outDir, "seed_01.gcode")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	s := string(data)
	for _, want := range []string{
		"G21",
		"G90",
		"Bed: 100.000mm",
		"Plate: 100.000mm at work origin X=0.000mm Y=0.000mm",
		"Layout offset in plate: X=5.000mm Y=5.000mm",
		"M4 S1000",
		"M5",
		"G0 X10.000 Y90.000",
		"G1 X30.000 Y90.000",
		"M2",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in output", want)
		}
	}
	if strings.Count(s, "G1 X") < 20 {
		t.Fatalf("expected many cut segments, got %d", strings.Count(s, "G1 X"))
	}
	if strings.Contains(s, "G0 X0.000 Y0.000") {
		t.Fatalf("unexpected forced return-to-origin in output")
	}
	if strings.Contains(s, "Machine offset:") {
		t.Fatalf("unexpected machine offset line for zero offset config")
	}
}

func TestSceneGCodeRenderer_Render_OutOfBoundsFails(t *testing.T) {
	doc := &PlateDocument{
		Version: sceneVersion,
		Scenes: []PlateScene{
			{
				Name:     "seed_01",
				WidthMM:  90,
				HeightMM: 90,
				Layers: []SceneLayer{
					{
						Tag:     "mask",
						Visible: true,
						Primitives: []ScenePrimitive{
							{Kind: PrimitiveRect, XMM: 85, YMM: 10, WidthMM: 10, HeightMM: 10},
						},
					},
				},
			},
		},
	}
	err := (SceneGCodeRenderer{PlateMM: 90}).Render(doc, t.TempDir())
	if err == nil {
		t.Fatalf("expected bounds error")
	}
	if !strings.Contains(err.Error(), "out of bounds") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSceneGCodeRenderer_Render_WithOffsets(t *testing.T) {
	doc := &PlateDocument{
		Version: sceneVersion,
		Scenes: []PlateScene{
			{
				Name:     "seed_01",
				WidthMM:  90,
				HeightMM: 90,
				Layers: []SceneLayer{
					{
						Tag:     "mask",
						Visible: true,
						Primitives: []ScenePrimitive{
							{Kind: PrimitiveRect, XMM: 5, YMM: 5, WidthMM: 20, HeightMM: 10, StrokeColor: sceneBlack, StrokeMM: 0.2},
						},
					},
				},
			},
		},
	}
	outDir := t.TempDir()
	if err := (SceneGCodeRenderer{
		BedMM:          150,
		PlateMM:        100,
		PlateOriginXMM: 2,
		PlateOriginYMM: 3,
	}).Render(doc, outDir); err != nil {
		t.Fatalf("Render with offsets: %v", err)
	}
	path := filepath.Join(outDir, "seed_01.gcode")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	s := string(data)
	for _, want := range []string{
		"Bed: 150.000mm",
		"Plate: 100.000mm at work origin X=2.000mm Y=3.000mm",
		"Layout offset in plate: X=5.000mm Y=5.000mm",
		"G0 X12.000 Y93.000",
		"G1 X32.000 Y93.000",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in output", want)
		}
	}
}

func TestSceneGCodeRenderer_Render_CalibrationAnchorUsesPlateOrigin(t *testing.T) {
	doc := &PlateDocument{
		Version: sceneVersion,
		Scenes: []PlateScene{
			{
				Name:          "laser_calibration_power_grid",
				WidthMM:       50,
				HeightMM:      50,
				AnchorInPlate: "origin",
				Layers: []SceneLayer{
					{
						Tag:     "mask",
						Visible: true,
						Primitives: []ScenePrimitive{
							{Kind: PrimitiveRect, XMM: 5, YMM: 5, WidthMM: 20, HeightMM: 10, StrokeColor: sceneBlack, StrokeMM: 0.2},
						},
					},
				},
			},
		},
	}
	outDir := t.TempDir()
	if err := (SceneGCodeRenderer{
		BedMM:          150,
		PlateMM:        100,
		PlateOriginXMM: 0,
		PlateOriginYMM: 0,
	}).Render(doc, outDir); err != nil {
		t.Fatalf("Render calibration anchor: %v", err)
	}
	path := filepath.Join(outDir, "laser_calibration_power_grid.gcode")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	s := string(data)
	for _, want := range []string{
		"Plate: 100.000mm at work origin X=0.000mm Y=0.000mm",
		"Layout offset in plate: X=0.000mm Y=0.000mm",
		"; Preview frame (laser off)",
		"G0 X0.000 Y50.000",
		"G0 X50.000 Y50.000",
		"G0 X50.000 Y0.000",
		"G0 X0.000 Y0.000",
		"G0 X5.000 Y45.000",
		"G1 X25.000 Y45.000",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in output", want)
		}
	}
}

func TestSceneGCodeRenderer_Render_CalibrationAnchorUsesPlateOffset(t *testing.T) {
	doc := &PlateDocument{
		Version: sceneVersion,
		Scenes: []PlateScene{
			{
				Name:             "laser_calibration_power_grid",
				WidthMM:          50,
				HeightMM:         50,
				AnchorInPlate:    "origin",
				OffsetInPlateXMM: 50,
				OffsetInPlateYMM: 50,
				Layers: []SceneLayer{
					{
						Tag:     "mask",
						Visible: true,
						Primitives: []ScenePrimitive{
							{Kind: PrimitiveRect, XMM: 5, YMM: 5, WidthMM: 20, HeightMM: 10, StrokeColor: sceneBlack, StrokeMM: 0.2},
						},
					},
				},
			},
		},
	}
	outDir := t.TempDir()
	if err := (SceneGCodeRenderer{
		BedMM:          150,
		PlateMM:        100,
		PlateOriginXMM: 0,
		PlateOriginYMM: 0,
	}).Render(doc, outDir); err != nil {
		t.Fatalf("Render calibration offset: %v", err)
	}
	path := filepath.Join(outDir, "laser_calibration_power_grid.gcode")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	s := string(data)
	for _, want := range []string{
		"Layout offset in plate: X=50.000mm Y=50.000mm",
		"G0 X55.000 Y95.000",
		"G1 X75.000 Y95.000",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in output", want)
		}
	}
}

func TestSceneGCodeRenderer_Render_WithMachineOffset(t *testing.T) {
	doc := &PlateDocument{
		Version: sceneVersion,
		Scenes: []PlateScene{
			{
				Name:     "seed_01",
				WidthMM:  90,
				HeightMM: 90,
				Layers: []SceneLayer{
					{
						Tag:     "mask",
						Visible: true,
						Primitives: []ScenePrimitive{
							{Kind: PrimitiveRect, XMM: 5, YMM: 5, WidthMM: 20, HeightMM: 10, StrokeColor: sceneBlack, StrokeMM: 0.2},
						},
					},
				},
			},
		},
	}
	outDir := t.TempDir()
	if err := (SceneGCodeRenderer{
		BedMM:            150,
		PlateMM:          100,
		PlateOriginXMM:   1,
		PlateOriginYMM:   0,
		MachineOffsetXMM: -1,
		MachineOffsetYMM: 4,
	}).Render(doc, outDir); err != nil {
		t.Fatalf("Render with machine offset: %v", err)
	}
	path := filepath.Join(outDir, "seed_01.gcode")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	s := string(data)
	for _, want := range []string{
		"Plate: 100.000mm at work origin X=1.000mm Y=0.000mm",
		"Machine offset: X=-1.000mm Y=4.000mm",
		"G0 X10.000 Y94.000",
		"G1 X30.000 Y94.000",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in output", want)
		}
	}
}

func TestSceneGCodeRenderer_Render_UsesPerPrimitiveLaserMode(t *testing.T) {
	doc := &PlateDocument{
		Version: sceneVersion,
		Scenes: []PlateScene{
			{
				Name:     "laser_mode_test",
				WidthMM:  20,
				HeightMM: 20,
				Layers: []SceneLayer{
					{
						Tag:     "mask",
						Visible: true,
						Primitives: []ScenePrimitive{
							{Kind: PrimitiveRect, XMM: 1, YMM: 1, WidthMM: 4, HeightMM: 4, FillColor: sceneBlack, LaserOnCmd: "M3"},
							{Kind: PrimitiveRect, XMM: 7, YMM: 1, WidthMM: 4, HeightMM: 4, FillColor: sceneBlack, LaserOnCmd: "M4"},
						},
					},
				},
			},
		},
	}
	outDir := t.TempDir()
	if err := (SceneGCodeRenderer{}).Render(doc, outDir); err != nil {
		t.Fatalf("Render per-primitive laser mode: %v", err)
	}
	path := filepath.Join(outDir, "laser_mode_test.gcode")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	s := string(data)
	if !strings.Contains(s, "M3 S1000") || !strings.Contains(s, "M4 S1000") {
		t.Fatalf("expected both M3 and M4 in output, got:\n%s", s)
	}
}

func TestSceneGCodeRenderer_Render_UsesOutlineScales(t *testing.T) {
	doc := &PlateDocument{
		Version: sceneVersion,
		Scenes: []PlateScene{
			{
				Name:     "outline_scale_test",
				WidthMM:  20,
				HeightMM: 20,
				Layers: []SceneLayer{
					{
						Tag:     "mask",
						Visible: true,
						Primitives: []ScenePrimitive{
							{Kind: PrimitiveRect, XMM: 1, YMM: 1, WidthMM: 6, HeightMM: 4, FillColor: sceneBlack, FillMode: FillModeHatch},
						},
					},
				},
			},
		},
	}
	outDir := t.TempDir()
	if err := (SceneGCodeRenderer{
		LaserMaxS:         1000,
		CutFeedMMMin:      1000,
		FillStepMM:        1.0,
		OutlinePowerScale: 0.9,
		OutlineFeedScale:  1.2,
	}).Render(doc, outDir); err != nil {
		t.Fatalf("Render outline scale test: %v", err)
	}
	path := filepath.Join(outDir, "outline_scale_test.gcode")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	s := string(data)
	if !strings.Contains(s, "G1 F1000.0\nM4 S1000") {
		t.Fatalf("missing base fill feed/power in output:\n%s", s)
	}
	if !strings.Contains(s, "G1 F1200.0\nM4 S900") {
		t.Fatalf("missing scaled outline feed/power in output:\n%s", s)
	}
}

func TestSceneGCodeRenderer_Render_UsesFillInsetForFillOnly(t *testing.T) {
	doc := &PlateDocument{
		Version: sceneVersion,
		Scenes: []PlateScene{
			{
				Name:     "fill_inset_test",
				WidthMM:  20,
				HeightMM: 20,
				Layers: []SceneLayer{
					{
						Tag:     "mask",
						Visible: true,
						Primitives: []ScenePrimitive{
							{Kind: PrimitiveRect, XMM: 2, YMM: 2, WidthMM: 6, HeightMM: 4, FillColor: sceneBlack, FillMode: FillModeHatch},
						},
					},
				},
			},
		},
	}
	outDir := t.TempDir()
	if err := (SceneGCodeRenderer{
		LaserMaxS:         1000,
		CutFeedMMMin:      1000,
		FillStepMM:        1.0,
		FillInsetMM:       0.5,
		OutlineFeedScale:  1.0,
		OutlinePowerScale: 1.0,
	}).Render(doc, outDir); err != nil {
		t.Fatalf("Render fill inset test: %v", err)
	}
	path := filepath.Join(outDir, "fill_inset_test.gcode")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	s := string(data)
	if !strings.Contains(s, "X42.500 Y") {
		t.Fatalf("missing inset fill coordinates in output:\n%s", s)
	}
	if !strings.Contains(s, "X42.000 Y58.000") {
		t.Fatalf("missing original-geometry outline coordinates in output:\n%s", s)
	}
}

func TestSceneGCodeRenderer_Render_UsesOutlineInsetForPath(t *testing.T) {
	doc := &PlateDocument{
		Version: sceneVersion,
		Scenes: []PlateScene{
			{
				Name:     "outline_inset_test",
				WidthMM:  20,
				HeightMM: 20,
				Layers: []SceneLayer{
					{
						Tag:     "mask",
						Visible: true,
						Primitives: []ScenePrimitive{
							{
								Kind:      PrimitivePath,
								PathData:  "M2 2 L8 2 L8 6 L2 6 Z",
								FillColor: sceneBlack,
								FillMode:  FillModeHatch,
							},
						},
					},
				},
			},
		},
	}
	outDir := t.TempDir()
	if err := (SceneGCodeRenderer{
		LaserMaxS:      1000,
		CutFeedMMMin:   1000,
		FillStepMM:     1.0,
		FillInsetMM:    0.0,
		OutlineInsetMM: 0.5,
	}).Render(doc, outDir); err != nil {
		t.Fatalf("Render outline inset test: %v", err)
	}
	path := filepath.Join(outDir, "outline_inset_test.gcode")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	s := string(data)
	if !strings.Contains(s, "X42.500 Y57.500") {
		t.Fatalf("missing inset outline coordinates in output:\n%s", s)
	}
	if strings.Contains(s, "X42.000 Y58.000\nG1 X48.000 Y58.000") {
		t.Fatalf("unexpected non-inset outline in output:\n%s", s)
	}
}

func TestSceneGCodeRenderer_Render_UsesDualOutlinePassForPath(t *testing.T) {
	doc := &PlateDocument{
		Version: sceneVersion,
		Scenes: []PlateScene{
			{
				Name:     "dual_outline_test",
				WidthMM:  20,
				HeightMM: 20,
				Layers: []SceneLayer{
					{
						Tag:     "mask",
						Visible: true,
						Primitives: []ScenePrimitive{
							{
								Kind:      PrimitivePath,
								PathData:  "M2 2 L8 2 L8 6 L2 6 Z",
								FillColor: sceneBlack,
								FillMode:  FillModeHatch,
							},
						},
					},
				},
			},
		},
	}
	outDir := t.TempDir()
	if err := (SceneGCodeRenderer{
		LaserMaxS:       1000,
		CutFeedMMMin:    1000,
		FillStepMM:      1.0,
		FillInsetMM:     0.0,
		OutlineInsetMM:  0.5,
		DualOutlinePass: true,
	}).Render(doc, outDir); err != nil {
		t.Fatalf("Render dual outline test: %v", err)
	}
	path := filepath.Join(outDir, "dual_outline_test.gcode")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	s := string(data)
	if !strings.Contains(s, "X42.500 Y57.500") {
		t.Fatalf("missing inset outline coordinates in output:\n%s", s)
	}
	if !strings.Contains(s, "G0 X42.000 Y58.000") || !strings.Contains(s, "G1 X48.000 Y58.000") {
		t.Fatalf("missing second outer outline coordinates in output:\n%s", s)
	}
}

func TestSceneGCodeRenderer_Render_UsesDualOutlinePassForRect(t *testing.T) {
	doc := &PlateDocument{
		Version: sceneVersion,
		Scenes: []PlateScene{
			{
				Name:     "dual_outline_rect_test",
				WidthMM:  20,
				HeightMM: 20,
				Layers: []SceneLayer{
					{
						Tag:     "mask",
						Visible: true,
						Primitives: []ScenePrimitive{
							{Kind: PrimitiveRect, XMM: 2, YMM: 2, WidthMM: 6, HeightMM: 4, FillColor: sceneBlack, FillMode: FillModeHatch},
						},
					},
				},
			},
		},
	}
	outDir := t.TempDir()
	if err := (SceneGCodeRenderer{
		LaserMaxS:       1000,
		CutFeedMMMin:    1000,
		FillStepMM:      1.0,
		FillInsetMM:     0.0,
		OutlineInsetMM:  0.5,
		DualOutlinePass: true,
	}).Render(doc, outDir); err != nil {
		t.Fatalf("Render dual outline rect test: %v", err)
	}
	path := filepath.Join(outDir, "dual_outline_rect_test.gcode")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	s := string(data)
	if !strings.Contains(s, "G0 X42.500 Y57.500") || !strings.Contains(s, "G1 X47.500 Y57.500") {
		t.Fatalf("missing inset outline for rect in output:\n%s", s)
	}
	if !strings.Contains(s, "G0 X42.000 Y58.000") || !strings.Contains(s, "G1 X48.000 Y58.000") {
		t.Fatalf("missing second outer outline for rect in output:\n%s", s)
	}
}

func TestSceneGCodeRenderer_Render_FeatureShrinkSkipsCalibrationStrokeText(t *testing.T) {
	scene := PlateScene{
		Name:     "laser_calibration_text_label",
		WidthMM:  25,
		HeightMM: 25,
		Layers: []SceneLayer{
			{
				Tag:     "mask",
				Visible: true,
				Primitives: []ScenePrimitive{
					func() ScenePrimitive {
						p := newSceneText(2, 8, "TEST", 14, 0.01, TextDirHorizontal, TextAnchorBaselineLeft)
						p.FillMode = FillModeNone
						return p
					}(),
				},
			},
		},
	}
	cfgBase := (SceneGCodeRenderer{
		PlateMM:        25,
		BedMM:          150,
		LaserMaxS:      1000,
		CutFeedMMMin:   1000,
		RapidFeedMMMin: 3000,
	}).withDefaults()

	noShrink := cfgBase
	noShrink.FeatureShrinkMM = 0
	g0, err := renderSceneGCode(scene, noShrink)
	if err != nil {
		t.Fatalf("render no-shrink calibration text: %v", err)
	}

	withShrink := cfgBase
	withShrink.FeatureShrinkMM = 0.2
	g1, err := renderSceneGCode(scene, withShrink)
	if err != nil {
		t.Fatalf("render shrink calibration text: %v", err)
	}

	if g0 != g1 {
		t.Fatalf("expected calibration stroke text to ignore feature shrink")
	}
}

func TestShouldApplyFeatureShrink_RespectsCalibrationStrokeTextExemption(t *testing.T) {
	e := &gcodeEmitter{
		cfg: SceneGCodeRenderer{
			FeatureShrinkMM: 0.2,
		},
	}
	text := newSceneText(2, 8, "TEST", 14, 0.01, TextDirHorizontal, TextAnchorBaselineLeft)
	text.FillMode = FillModeNone
	if !e.shouldApplyFeatureShrink(text, FillModeNone) {
		t.Fatalf("expected non-calibration stroke text to allow feature shrink")
	}
	e.isCalibration = true
	if e.shouldApplyFeatureShrink(text, FillModeNone) {
		t.Fatalf("expected calibration stroke text to skip feature shrink")
	}
	if !e.shouldApplyFeatureShrink(text, FillModeHatch) {
		t.Fatalf("expected calibration filled text to allow feature shrink")
	}
	if e.shouldApplyFeatureShrink(ScenePrimitive{Kind: PrimitiveRect}, FillModeHatch) {
		t.Fatalf("expected non-text/non-path primitive to skip feature shrink")
	}
}

func TestPlanHorizontalTextHatchBatches_GroupsSameRow(t *testing.T) {
	e := &gcodeEmitter{cfg: (SceneGCodeRenderer{}).withDefaults()}
	y := 10.0
	prims := []ScenePrimitive{
		newSceneText(5, y, "1", 14, 0.05, TextDirHorizontal, TextAnchorBaselineLeft),
		newSceneText(12, y, "CHOIC", 14, 0.12, TextDirHorizontal, TextAnchorBaselineLeft),
		newSceneText(55, y, "17", 14, 0.05, TextDirHorizontal, TextAnchorBaselineLeft),
		newSceneText(63, y, "CURVE", 14, 0.12, TextDirHorizontal, TextAnchorBaselineLeft),
		newSceneText(5, y+3.88, "2", 14, 0.05, TextDirHorizontal, TextAnchorBaselineLeft),
	}
	batches, batched := e.planHorizontalTextHatchBatches(prims)
	row, ok := batches[0]
	if !ok {
		t.Fatalf("expected leader batch at index 0")
	}
	if len(row) != 4 {
		t.Fatalf("expected 4 primitives in first-row batch, got %d (%v)", len(row), row)
	}
	for _, idx := range []int{0, 1, 2, 3} {
		if !batched[idx] {
			t.Fatalf("expected primitive %d to be batched", idx)
		}
	}
	if batched[4] {
		t.Fatalf("unexpected batching for different-row primitive")
	}
}

func TestShrinkFeatureLoops_ShrinksOuterAndExpandsHole(t *testing.T) {
	outer := []gcodePt{
		{x: 0, y: 0},
		{x: 10, y: 0},
		{x: 10, y: 10},
		{x: 0, y: 10},
		{x: 0, y: 0},
	}
	hole := []gcodePt{
		{x: 3, y: 3},
		{x: 3, y: 7},
		{x: 7, y: 7},
		{x: 7, y: 3},
		{x: 3, y: 3},
	}
	orig := [][]gcodePt{outer, hole}
	out := shrinkFeatureLoops(orig, 0.5)
	if len(out) != 2 {
		t.Fatalf("expected 2 loops, got %d", len(out))
	}
	origOuterArea := math.Abs(polygonArea(orig[0]))
	origHoleArea := math.Abs(polygonArea(orig[1]))
	outOuterArea := math.Abs(polygonArea(out[0]))
	outHoleArea := math.Abs(polygonArea(out[1]))
	if !(outOuterArea < origOuterArea) {
		t.Fatalf("outer area not reduced: orig=%f out=%f", origOuterArea, outOuterArea)
	}
	if !(outHoleArea > origHoleArea) {
		t.Fatalf("hole area not expanded: orig=%f out=%f", origHoleArea, outHoleArea)
	}
}

func TestShrinkFeatureLoops_DisjointOppositeWindingBothShrink(t *testing.T) {
	loopA := []gcodePt{
		{x: 0, y: 0},
		{x: 10, y: 0},
		{x: 10, y: 10},
		{x: 0, y: 10},
		{x: 0, y: 0},
	}
	loopB := []gcodePt{
		{x: 20, y: 0},
		{x: 20, y: 10},
		{x: 30, y: 10},
		{x: 30, y: 0},
		{x: 20, y: 0},
	}
	orig := [][]gcodePt{loopA, loopB}
	out := shrinkFeatureLoops(orig, 0.5)
	if len(out) != 2 {
		t.Fatalf("expected 2 loops, got %d", len(out))
	}
	for i := range orig {
		origArea := math.Abs(polygonArea(orig[i]))
		outArea := math.Abs(polygonArea(out[i]))
		if !(outArea < origArea) {
			t.Fatalf("loop %d was not shrunk: orig=%f out=%f", i, origArea, outArea)
		}
	}
}

func TestInsetLoop_AcuteCornerMiterIsLimited(t *testing.T) {
	loop := []gcodePt{
		{x: 0, y: 0},
		{x: 4.9, y: 0},
		{x: 5.0, y: 10.0},
		{x: 5.1, y: 0},
		{x: 10, y: 0},
		{x: 10, y: 12},
		{x: 0, y: 12},
		{x: 0, y: 0},
	}
	d := 0.2
	out := insetLoop(loop, d)
	if len(out) < 4 {
		t.Fatalf("expected inset loop, got %d points", len(out))
	}
	const eps = 1e-9
	limit := 4.0*d + eps
	for i := 0; i < len(out)-1; i++ {
		p := out[i]
		minDist := math.Inf(1)
		for j := 0; j < len(loop)-1; j++ {
			q := loop[j]
			if dist := math.Hypot(p.x-q.x, p.y-q.y); dist < minDist {
				minDist = dist
			}
		}
		if minDist > limit {
			t.Fatalf("inset point escaped miter limit: point=(%.6f,%.6f) minDist=%.6f limit=%.6f", p.x, p.y, minDist, limit)
		}
	}
}

func TestSceneGCodeRenderer_Render_OffsetOutOfBoundsFails(t *testing.T) {
	doc := &PlateDocument{
		Version: sceneVersion,
		Scenes: []PlateScene{
			{
				Name:     "seed_01",
				WidthMM:  90,
				HeightMM: 90,
				Layers: []SceneLayer{
					{
						Tag:     "mask",
						Visible: true,
						Primitives: []ScenePrimitive{
							{Kind: PrimitiveRect, XMM: 5, YMM: 5, WidthMM: 20, HeightMM: 10, StrokeColor: sceneBlack, StrokeMM: 0.2},
						},
					},
				},
			},
		},
	}
	err := (SceneGCodeRenderer{
		BedMM:          100,
		PlateMM:        100,
		PlateOriginXMM: 20,
	}).Render(doc, t.TempDir())
	if err == nil {
		t.Fatalf("expected out-of-bounds error for offset placement")
	}
	if !strings.Contains(err.Error(), "plate origin out of workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
}
