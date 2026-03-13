package printer

import "testing"

func TestLaserCalibrationBuilderPowerGridRowsAscendInSceneY(t *testing.T) {
	doc, err := (LaserCalibrationBuilder{
		Kind:          LaserCalibrationPowerGrid,
		PlateMM:       100,
		CalibrationMM: 50,
		Powers:        []int{30, 50, 70, 90},
		Feeds:         []float64{400, 700, 1000, 1300},
	}).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(doc.Scenes) != 1 {
		t.Fatalf("expected 1 scene, got %d", len(doc.Scenes))
	}
	scene := doc.Scenes[0]
	var ys []float64
	seen := map[float64]bool{}
	for _, p := range scene.Layers[0].Primitives {
		if p.Kind == PrimitiveRect && p.StrokeMM > 0 && p.WidthMM > 5 && p.HeightMM > 5 {
			if seen[p.YMM] {
				continue
			}
			seen[p.YMM] = true
			ys = append(ys, p.YMM)
			if len(ys) == 4 {
				break
			}
		}
	}
	if len(ys) < 4 {
		t.Fatalf("expected at least 4 matrix boxes, got %d", len(ys))
	}
	if !(ys[0] < ys[1] && ys[1] < ys[2] && ys[2] < ys[3]) {
		t.Fatalf("expected matrix rows to ascend in scene Y (top-down scene order), got %v", ys)
	}
}
