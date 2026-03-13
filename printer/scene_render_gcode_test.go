package printer

import (
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
