package printer

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
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

func TestSceneGCodeRenderer_Render_CalibrationCanExceedPlateSize(t *testing.T) {
	doc := &PlateDocument{
		Version: sceneVersion,
		Scenes: []PlateScene{
			{
				Name:          "laser_calibration_sector_repeatability_test",
				WidthMM:       150,
				HeightMM:      150,
				AnchorInPlate: "origin",
				Layers: []SceneLayer{
					{
						Tag:     "mask",
						Visible: true,
						Primitives: []ScenePrimitive{
							{Kind: PrimitiveRect, XMM: 0, YMM: 50, WidthMM: 100, HeightMM: 100, FillMode: FillModeNone, StrokeColor: sceneBlack, StrokeMM: 0.2},
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
		t.Fatalf("Render oversized calibration scene: %v", err)
	}
	path := filepath.Join(outDir, "laser_calibration_sector_repeatability_test.gcode")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	s := string(data)
	for _, want := range []string{
		"Size: 150.000mm x 150.000mm",
		"Plate: 100.000mm at work origin X=0.000mm Y=0.000mm",
		"G0 X0.000 Y100.000",
		"G1 X100.000 Y100.000",
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

func TestPlanHorizontalTextHatchBatches_DoesNotGroupContourText(t *testing.T) {
	e := &gcodeEmitter{cfg: (SceneGCodeRenderer{TextFillMode: "contour"}).withDefaults()}
	y := 10.0
	prims := []ScenePrimitive{
		newSceneText(5, y, "20", 14, 0.05, TextDirHorizontal, TextAnchorBaselineLeft),
		newSceneText(18, y, "VAGUE", 14, 0.12, TextDirHorizontal, TextAnchorBaselineLeft),
	}
	batches, batched := e.planHorizontalTextHatchBatches(prims)
	if len(batches) != 0 {
		t.Fatalf("expected no grouped contour text batches, got %v", batches)
	}
	if batched[0] || batched[1] {
		t.Fatalf("contour text should not be marked as batched: %v", batched)
	}
}

func TestSceneGCodeRenderer_withDefaults_NormalizesLaserPassOrder(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "default empty", in: "", want: laserPassOrderGrouped},
		{name: "grouped", in: "grouped", want: laserPassOrderGrouped},
		{name: "local", in: "local", want: laserPassOrderLocal},
		{name: "sweep", in: "sweep", want: laserPassOrderSweep},
		{name: "global alias", in: "global", want: laserPassOrderSweep},
		{name: "case-insensitive", in: "LoCaL", want: laserPassOrderLocal},
		{name: "invalid fallback", in: "anything", want: laserPassOrderGrouped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (SceneGCodeRenderer{LaserPassOrder: tt.in}).withDefaults().LaserPassOrder
			if got != tt.want {
				t.Fatalf("withDefaults pass order = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSceneGCodeRenderer_withDefaults_NormalizesTextFillMode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "default empty", in: "", want: laserTextFillRaster},
		{name: "raster", in: "raster", want: laserTextFillRaster},
		{name: "contour", in: "contour", want: laserTextFillContour},
		{name: "case-insensitive", in: "ConTour", want: laserTextFillContour},
		{name: "invalid fallback", in: "anything", want: laserTextFillRaster},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (SceneGCodeRenderer{TextFillMode: tt.in}).withDefaults().TextFillMode
			if got != tt.want {
				t.Fatalf("withDefaults text fill mode = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrimitiveLaserParams_TextFillModeContourUsesOffset(t *testing.T) {
	p := newSceneText(5, 10, "TEST", 14, 0.05, TextDirHorizontal, TextAnchorBaselineLeft)
	contour := &gcodeEmitter{cfg: (SceneGCodeRenderer{TextFillMode: "contour"}).withDefaults()}
	if got := contour.primitiveLaserParams(p); got.fillMode != FillModeOffset {
		t.Fatalf("contour text fill mode = %q, want %q", got.fillMode, FillModeOffset)
	}
	raster := &gcodeEmitter{cfg: (SceneGCodeRenderer{TextFillMode: "raster"}).withDefaults()}
	if got := raster.primitiveLaserParams(p); got.fillMode != FillModeHatch {
		t.Fatalf("raster text fill mode = %q, want %q", got.fillMode, FillModeHatch)
	}
}

func TestLayerPassPlan_GroupedBatchesInterleavedHorizontalText(t *testing.T) {
	e := &gcodeEmitter{cfg: (SceneGCodeRenderer{LaserPassOrder: "grouped"}).withDefaults()}
	prims := []ScenePrimitive{
		newSceneText(5, 10, "1", 14, 0.05, TextDirHorizontal, TextAnchorBaselineLeft),
		{Kind: PrimitivePath, PathData: "M40 12 L44 12"},
		newSceneText(18, 10, "A", 14, 0.05, TextDirHorizontal, TextAnchorBaselineLeft),
	}
	got := e.layerPassPlan(prims)
	want := [][]int{{0, 2}, {1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("grouped pass plan = %v, want %v", got, want)
	}
}

func TestLayerPassPlan_LocalKeepsPrimitiveOrder(t *testing.T) {
	e := &gcodeEmitter{cfg: (SceneGCodeRenderer{LaserPassOrder: "local"}).withDefaults()}
	prims := []ScenePrimitive{
		newSceneText(5, 10, "1", 14, 0.05, TextDirHorizontal, TextAnchorBaselineLeft),
		{Kind: PrimitivePath, PathData: "M40 12 L44 12"},
		newSceneText(18, 10, "A", 14, 0.05, TextDirHorizontal, TextAnchorBaselineLeft),
	}
	got := e.layerPassPlan(prims)
	want := [][]int{{0}, {1}, {2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("local pass plan = %v, want %v", got, want)
	}
}

func TestHatchOverscanEndpoints_ClampsToBounds(t *testing.T) {
	a, b, ok := hatchOverscanEndpoints([]gcodePt{{x: 5, y: 10}, {x: 15, y: 10}}, 1.0, 0, 20, 0, 20)
	if !ok {
		t.Fatalf("expected overscan endpoints")
	}
	if !pointsEqual(a, gcodePt{x: 4, y: 10}) || !pointsEqual(b, gcodePt{x: 16, y: 10}) {
		t.Fatalf("unexpected endpoints: a=%+v b=%+v", a, b)
	}

	a, b, ok = hatchOverscanEndpoints([]gcodePt{{x: 0.2, y: 10}, {x: 5, y: 10}}, 1.0, 0, 20, 0, 20)
	if !ok {
		t.Fatalf("expected overscan endpoints near bound")
	}
	if math.Abs(a.x-0.0) > 1e-9 || math.Abs(b.x-6.0) > 1e-9 {
		t.Fatalf("expected left clamp at X=0 and right overscan to X=6, got a=%+v b=%+v", a, b)
	}
}

func TestHatchSegmentsCenteredByComponent_CentersScanPhase(t *testing.T) {
	loop := []gcodePt{
		{x: 0, y: 0},
		{x: 2, y: 0},
		{x: 2, y: 0.7},
		{x: 0, y: 0.7},
		{x: 0, y: 0},
	}
	segs := hatchSegmentsCenteredByComponent([][]gcodePt{loop}, 0.2)
	if len(segs) == 0 {
		t.Fatalf("expected centered hatch segments")
	}
	yVals := make([]float64, 0, len(segs))
	for _, seg := range segs {
		if len(seg) != 2 {
			continue
		}
		y := seg[0].y
		if len(yVals) == 0 || math.Abs(y-yVals[len(yVals)-1]) > 1e-6 {
			yVals = append(yVals, y)
		}
	}
	if len(yVals) != 4 {
		t.Fatalf("expected 4 centered scanlines, got %d (%v)", len(yVals), yVals)
	}
	if math.Abs(yVals[0]-0.05) > 1e-3 {
		t.Fatalf("expected centered first scanline near y=0.05, got %.6f", yVals[0])
	}

	defaultSegs := hatchSegments([][]gcodePt{loop}, 0.2)
	if len(defaultSegs) == 0 {
		t.Fatalf("expected default hatch segments")
	}
	if defaultSegs[0][0].y >= 0.01 {
		t.Fatalf("expected default first scanline near lower bound, got %.6f", defaultSegs[0][0].y)
	}
}

func TestHatchSegments_NoAutoRotationForTallShapes(t *testing.T) {
	loop := []gcodePt{
		{x: 0, y: 0},
		{x: 0.7, y: 0},
		{x: 0.7, y: 2},
		{x: 0, y: 2},
		{x: 0, y: 0},
	}
	segs := hatchSegments([][]gcodePt{loop}, 0.2)
	if len(segs) == 0 {
		t.Fatalf("expected hatch segments")
	}
	first := segs[0]
	if len(first) != 2 {
		t.Fatalf("unexpected segment: %+v", first)
	}
	if math.Abs(first[0].y-first[1].y) > 1e-9 {
		t.Fatalf("expected horizontal hatch segment for tall shape, got %+v", first)
	}
}

func TestTraceHatchSweepSegments_UsesInlinePowerModulation(t *testing.T) {
	e := &gcodeEmitter{
		cfg: (SceneGCodeRenderer{
			LaserOnCmd:     "M4",
			CutFeedMMMin:   1000,
			RapidFeedMMMin: 3000,
			PlateMM:        20,
			BedMM:          20,
		}).withDefaults(),
		bedMax: 20,
		sceneW: 20,
		sceneH: 20,
	}
	segs := [][]gcodePt{
		{{x: 2, y: 10}, {x: 5, y: 10}},
		{{x: 7, y: 10}, {x: 9, y: 10}},
		{{x: 9, y: 9.5}, {x: 6, y: 9.5}},
	}
	e.traceHatchSweepSegments(segs, 1000, 800, "M4")
	if e.err != nil {
		t.Fatalf("traceHatchSweepSegments: %v", e.err)
	}
	g := e.b.String()
	if !strings.Contains(g, "M4 S0") {
		t.Fatalf("expected sweep pre-arm with M4 S0 in output:\n%s", g)
	}
	if !strings.Contains(g, "G1 X5.000 Y10.000 S800") {
		t.Fatalf("expected inline burn segment with S800 in output:\n%s", g)
	}
	if !strings.Contains(g, "G1 X7.000 Y10.000 S0") {
		t.Fatalf("expected inline non-burn travel with S0 in output:\n%s", g)
	}
	if got := strings.Count(g, "M4 S0"); got != 1 {
		t.Fatalf("expected one sweep arm command, got %d in output:\n%s", got, g)
	}
	if got := strings.Count(g, "M5"); got != 1 {
		t.Fatalf("expected one sweep stop command, got %d in output:\n%s", got, g)
	}
}

func TestTraceHatchSweepSegments_AppliesRowOverscan(t *testing.T) {
	e := &gcodeEmitter{
		cfg: (SceneGCodeRenderer{
			LaserOnCmd:      "M4",
			CutFeedMMMin:    1000,
			RapidFeedMMMin:  3000,
			HatchOverscanMM: 1.0,
			PlateMM:         20,
			BedMM:           20,
		}).withDefaults(),
		bedMax: 20,
		sceneW: 20,
		sceneH: 20,
	}
	segs := [][]gcodePt{
		{{x: 2, y: 10}, {x: 5, y: 10}},
		{{x: 7, y: 10}, {x: 9, y: 10}},
	}
	e.traceHatchSweepSegments(segs, 1000, 800, "M4")
	if e.err != nil {
		t.Fatalf("traceHatchSweepSegments: %v", e.err)
	}
	g := e.b.String()
	if !strings.Contains(g, "G0 X1.000 Y10.000") {
		t.Fatalf("expected sweep row lead-in overscan move:\n%s", g)
	}
	if !strings.Contains(g, "G1 X10.000 Y10.000 S0") {
		t.Fatalf("expected sweep row lead-out overscan move with laser off:\n%s", g)
	}
}

func TestSceneGCodeRenderer_Render_SweepAppliesToNonTextHatch(t *testing.T) {
	doc := &PlateDocument{
		Version: sceneVersion,
		Scenes: []PlateScene{
			{
				Name:     "sweep_non_text_hatch",
				WidthMM:  20,
				HeightMM: 20,
				Layers: []SceneLayer{
					{
						Tag:     "mask",
						Visible: true,
						Primitives: []ScenePrimitive{
							{Kind: PrimitiveCircle, CXMM: 6, CYMM: 6, RadiusMM: 2, FillColor: sceneBlack, NoOutline: true},
							{Kind: PrimitiveRing, XMM: 10, YMM: 4, WidthMM: 6, HeightMM: 6, ThicknessMM: 1, RadiusMM: 0.2, FillColor: sceneBlack, FillMode: FillModeOffset, NoOutline: true},
						},
					},
				},
			},
		},
	}
	outDir := t.TempDir()
	if err := (SceneGCodeRenderer{
		BedMM:          20,
		PlateMM:        20,
		LaserPassOrder: "sweep",
		LaserOnCmd:     "M4",
		LaserMaxS:      800,
		NoOutlinePass:  true,
	}).Render(doc, outDir); err != nil {
		t.Fatalf("Render sweep_non_text_hatch: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outDir, "sweep_non_text_hatch.gcode"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "M4 S0") {
		t.Fatalf("expected sweep pre-arm M4 S0 for non-text hatch primitives:\n%s", s)
	}
	if !strings.Contains(s, " S800") {
		t.Fatalf("expected inline S800 burn segments in sweep output:\n%s", s)
	}
}

func TestSceneGCodeRenderer_Render_UsesHatchOverscan(t *testing.T) {
	doc := &PlateDocument{
		Version: sceneVersion,
		Scenes: []PlateScene{
			{
				Name:     "overscan_test",
				WidthMM:  30,
				HeightMM: 30,
				Layers: []SceneLayer{
					{
						Tag:     "mask",
						Visible: true,
						Primitives: []ScenePrimitive{
							{Kind: PrimitiveRect, XMM: 5, YMM: 5, WidthMM: 10, HeightMM: 10, FillColor: sceneBlack, NoOutline: true},
						},
					},
				},
			},
		},
	}
	outDir := t.TempDir()
	if err := (SceneGCodeRenderer{
		BedMM:           30,
		PlateMM:         30,
		FillStepMM:      100,
		HatchOverscanMM: 0.5,
	}).Render(doc, outDir); err != nil {
		t.Fatalf("Render with hatch overscan: %v", err)
	}
	path := filepath.Join(outDir, "overscan_test.gcode")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	s := string(data)
	if !strings.Contains(s, "G0 X4.500 Y25.000") {
		t.Fatalf("expected overscan lead-in move in output")
	}
	if !strings.Contains(s, "G1 X15.500 Y25.000") {
		t.Fatalf("expected overscan lead-out move in output")
	}
}

func TestSceneGCodeRenderer_Render_NoOutlinePassSkipsFilledOutlines(t *testing.T) {
	scene := PlateScene{
		Name:     "no_outline_pass_test",
		WidthMM:  20,
		HeightMM: 20,
		Layers: []SceneLayer{
			{
				Tag:     "mask",
				Visible: true,
				Primitives: []ScenePrimitive{
					{Kind: PrimitiveRect, XMM: 5, YMM: 5, WidthMM: 10, HeightMM: 10, FillColor: sceneBlack},
				},
			},
		},
	}
	baseCfg := (SceneGCodeRenderer{
		LaserOnCmd:   "M4",
		LaserMaxS:    1000,
		CutFeedMMMin: 1000,
		FillStepMM:   100,
		PlateMM:      20,
		BedMM:        20,
	}).withDefaults()

	gWithOutline, err := renderSceneGCode(scene, baseCfg)
	if err != nil {
		t.Fatalf("render with outline: %v", err)
	}
	cfgNoOutline := baseCfg
	cfgNoOutline.NoOutlinePass = true
	gNoOutline, err := renderSceneGCode(scene, cfgNoOutline)
	if err != nil {
		t.Fatalf("render no-outline-pass: %v", err)
	}
	withCount := strings.Count(gWithOutline, "M4 S1000")
	noCount := strings.Count(gNoOutline, "M4 S1000")
	if withCount <= noCount {
		t.Fatalf("expected fewer laser-on segments without outline pass: with=%d no=%d", withCount, noCount)
	}
	if noCount == 0 {
		t.Fatalf("expected fill segment to remain when no-outline-pass is enabled")
	}
}

func TestRotateClosedLoopStart(t *testing.T) {
	loop := []gcodePt{
		{x: 0, y: 0},
		{x: 10, y: 0},
		{x: 10, y: 10},
		{x: 0, y: 10},
		{x: 0, y: 0},
	}
	rot := rotateClosedLoopStart(loop, 1)
	if len(rot) != len(loop) {
		t.Fatalf("rotated loop len=%d want %d", len(rot), len(loop))
	}
	if !pointsEqual(rot[0], gcodePt{x: 10, y: 0}) {
		t.Fatalf("unexpected rotated start: %+v", rot[0])
	}
	if !pointsEqual(rot[len(rot)-1], rot[0]) {
		t.Fatalf("rotated loop not closed: start=%+v end=%+v", rot[0], rot[len(rot)-1])
	}
}

func TestRenderTextContourByComponent_OutlineFirstAndFinalM3(t *testing.T) {
	e := &gcodeEmitter{
		cfg: (SceneGCodeRenderer{
			LaserOnCmd:      "M4",
			CutFeedMMMin:    1000,
			RapidFeedMMMin:  3000,
			OutlineInsetMM:  0.0,
			DualOutlinePass: false,
			BedMM:           50,
			PlateMM:         50,
		}).withDefaults(),
		bedMax: 50,
		sceneW: 50,
		sceneH: 50,
	}
	outer := []gcodePt{
		{x: 5, y: 5},
		{x: 15, y: 5},
		{x: 15, y: 15},
		{x: 5, y: 15},
		{x: 5, y: 5},
	}
	e.renderTextContourByComponent(
		[][]gcodePt{outer},
		[][]gcodePt{outer},
		[][]gcodePt{outer},
		2.0,
		1000,
		800,
		1000,
		800,
		"M4",
		true,
	)
	if e.err != nil {
		t.Fatalf("renderTextContourByComponent error: %v", e.err)
	}
	s := e.b.String()
	firstM4 := strings.Index(s, "M4 S800")
	firstM3 := strings.Index(s, "M3 S800")
	if firstM4 < 0 {
		t.Fatalf("missing outline/fill M4 command in output:\n%s", s)
	}
	if firstM3 < 0 {
		t.Fatalf("missing final contour M3 command in output:\n%s", s)
	}
	if firstM3 <= firstM4 {
		t.Fatalf("expected M3 after initial M4 (outline-first then inner-core M3), got M4@%d M3@%d", firstM4, firstM3)
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
