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
		"Workspace: 100.000mm",
		"Offset: X=5.000mm Y=5.000mm",
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
