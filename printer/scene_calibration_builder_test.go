package printer

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

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

func TestLaserCalibrationBuilderPowerGridKeepsBottomClearance(t *testing.T) {
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
	scene := doc.Scenes[0]
	maxBottom := 0.0
	for _, p := range scene.Layers[0].Primitives {
		if p.Kind != PrimitiveRect || p.StrokeMM <= 0 || p.WidthMM <= 5 || p.HeightMM <= 5 {
			continue
		}
		if bottom := p.YMM + p.HeightMM; bottom > maxBottom {
			maxBottom = bottom
		}
	}
	if maxBottom == 0 {
		t.Fatalf("did not find matrix boxes")
	}
	if gotClearance := scene.HeightMM - maxBottom; gotClearance < 2.5 {
		t.Fatalf("bottom clearance too small: got %.3fmm, want at least 2.5mm", gotClearance)
	}
}

func TestLaserCalibrationBuilderPowerGridUsesReducedAnnotationPower(t *testing.T) {
	doc, err := (LaserCalibrationBuilder{
		Kind:          LaserCalibrationPowerGrid,
		PlateMM:       100,
		CalibrationMM: 50,
		Powers:        []int{1000, 950, 900, 850},
		Feeds:         []float64{1000, 800, 600, 400},
	}).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	scene := doc.Scenes[0]
	wantPower := 500.0
	wantFeed := 1000.0
	foundBorder := false
	foundLabel := false
	foundMark := false
	for _, p := range scene.Layers[0].Primitives {
		switch {
		case p.Kind == PrimitiveRect && p.StrokeMM > 0 && p.WidthMM > 5 && p.HeightMM > 5:
			foundBorder = true
			if p.PowerS != int(wantPower) || p.FeedMMMin != wantFeed {
				t.Fatalf("border style got power/feed %d/%.0f want %0.f/%.0f", p.PowerS, p.FeedMMMin, wantPower, wantFeed)
			}
		case p.Kind == PrimitiveText && p.Text == "SEED":
			foundLabel = true
			if p.PowerS < 850 || p.FeedMMMin < 400 {
				t.Fatalf("sample text should keep explicit calibration settings, got %d/%.0f", p.PowerS, p.FeedMMMin)
			}
		case p.Kind == PrimitiveText && p.Text != "SEED":
			if p.FillMode != FillModeNone {
				t.Fatalf("annotation text should be stroke-only, got fill mode %q", p.FillMode)
			}
		case p.Kind == PrimitiveRound && p.FillMode == FillModeHatch:
			foundMark = true
			if p.PowerS < 850 || p.FeedMMMin < 400 {
				t.Fatalf("expected test mark to keep explicit calibration settings, got %d/%.0f", p.PowerS, p.FeedMMMin)
			}
		}
	}
	if !foundBorder || !foundLabel || !foundMark {
		t.Fatalf("expected to find border=%v label=%v mark=%v", foundBorder, foundLabel, foundMark)
	}
}

func TestLaserCalibrationBuilderPowerGridAssignsRowLaserModes(t *testing.T) {
	doc, err := (LaserCalibrationBuilder{
		Kind:          LaserCalibrationPowerGrid,
		PlateMM:       100,
		CalibrationMM: 50,
		Powers:        []int{1000, 900, 800, 700},
		Feeds:         []float64{1000, 800, 600, 400},
		RowLaserModes: []string{"M4", "M3", "M4", "M3"},
	}).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	scene := doc.Scenes[0]
	rowModes := map[float64]string{}
	for _, p := range scene.Layers[0].Primitives {
		if p.Kind == PrimitiveRound && p.FillMode == FillModeHatch {
			if _, ok := rowModes[p.YMM]; !ok {
				rowModes[p.YMM] = p.LaserOnCmd
			}
		}
	}
	if len(rowModes) != 4 {
		t.Fatalf("expected 4 row modes, got %d", len(rowModes))
	}
	ys := make([]float64, 0, len(rowModes))
	for y := range rowModes {
		ys = append(ys, y)
	}
	slices.Sort(ys)
	got := make([]string, 0, len(ys))
	for _, y := range ys {
		got = append(got, rowModes[y])
	}
	want := []string{"M4", "M3", "M4", "M3"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d mode=%q want %q", i, got[i], want[i])
		}
	}
}

func TestLaserCalibrationBuilderBuildsTestTile(t *testing.T) {
	doc, err := (LaserCalibrationBuilder{
		Kind:          LaserCalibrationTestTile,
		PlateMM:       100,
		CalibrationMM: 25,
		OffsetXMM:     50,
		OffsetYMM:     25,
		TilePowerS:    850,
		TileFeedMMMin: 2000,
	}).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(doc.Scenes) != 1 {
		t.Fatalf("expected 1 scene, got %d", len(doc.Scenes))
	}
	scene := doc.Scenes[0]
	if scene.Name != "laser_calibration_test_tile" {
		t.Fatalf("unexpected scene name %q", scene.Name)
	}
	if scene.WidthMM != 25 || scene.HeightMM != 25 {
		t.Fatalf("unexpected scene size %.1fx%.1f", scene.WidthMM, scene.HeightMM)
	}
	if scene.OffsetInPlateXMM != 50 || scene.OffsetInPlateYMM != 25 {
		t.Fatalf("unexpected offsets %.1f,%.1f", scene.OffsetInPlateXMM, scene.OffsetInPlateYMM)
	}
	foundVague := false
	foundChoice := false
	foundCurve := false
	found20 := false
	found18 := false
	found13 := false
	foundFinder := false
	foundDots := 0
	finderWidth := 0.0
	dotRadius := 0.0
	for _, p := range scene.Layers[0].Primitives {
		switch {
		case p.Kind == PrimitiveText && p.Text == "20":
			found20 = true
			if p.FontSizePt != 14 || p.TrackingEM != 0.05 {
				t.Fatalf("unexpected number text style size=%.1f tracking=%.2f", p.FontSizePt, p.TrackingEM)
			}
		case p.Kind == PrimitiveText && p.Text == "18":
			found18 = true
			if p.FontSizePt != 14 || p.TrackingEM != 0.05 {
				t.Fatalf("unexpected number text style size=%.1f tracking=%.2f", p.FontSizePt, p.TrackingEM)
			}
		case p.Kind == PrimitiveText && p.Text == "13":
			found13 = true
			if p.FontSizePt != 14 || p.TrackingEM != 0.05 {
				t.Fatalf("unexpected number text style size=%.1f tracking=%.2f", p.FontSizePt, p.TrackingEM)
			}
		case p.Kind == PrimitiveText && p.Text == "VAGUE":
			foundVague = true
			if p.FontSizePt != 14 || p.TrackingEM != 0.12 {
				t.Fatalf("unexpected word text style size=%.1f tracking=%.2f", p.FontSizePt, p.TrackingEM)
			}
			if p.PowerS != 850 || p.FeedMMMin != 2000 {
				t.Fatalf("unexpected tile word settings %d/%.0f", p.PowerS, p.FeedMMMin)
			}
		case p.Kind == PrimitiveText && p.Text == "CHOICE":
			foundChoice = true
			if p.FontSizePt != 14 || p.TrackingEM != 0.12 {
				t.Fatalf("unexpected word text style size=%.1f tracking=%.2f", p.FontSizePt, p.TrackingEM)
			}
			if p.PowerS != 850 || p.FeedMMMin != 2000 {
				t.Fatalf("unexpected tile word settings %d/%.0f", p.PowerS, p.FeedMMMin)
			}
		case p.Kind == PrimitiveText && p.Text == "CURVE":
			foundCurve = true
			if p.FontSizePt != 14 || p.TrackingEM != 0.12 {
				t.Fatalf("unexpected word text style size=%.1f tracking=%.2f", p.FontSizePt, p.TrackingEM)
			}
			if p.PowerS != 850 || p.FeedMMMin != 2000 {
				t.Fatalf("unexpected tile word settings %d/%.0f", p.PowerS, p.FeedMMMin)
			}
		case p.Kind == PrimitiveRing:
			if p.WidthMM > 3 {
				foundFinder = true
				finderWidth = p.WidthMM
			}
		case p.Kind == PrimitiveCircle:
			foundDots++
			if dotRadius == 0 {
				dotRadius = p.RadiusMM
			}
		}
	}
	if !found20 || !found18 || !found13 || !foundVague || !foundChoice || !foundCurve || !foundFinder || foundDots < 7 {
		t.Fatalf("expected tile content 20=%v 18=%v 13=%v vague=%v choice=%v curve=%v finder=%v dots=%d", found20, found18, found13, foundVague, foundChoice, foundCurve, foundFinder, foundDots)
	}
	wantStep := seedQRSizeMM / 29.0
	wantFinderW := 7 * wantStep
	if got := finderWidth; got < wantFinderW-1e-6 || got > wantFinderW+1e-6 {
		t.Fatalf("unexpected finder width %.6f want %.6f (regular 2/3 seed QR)", got, wantFinderW)
	}
	wantDotR := wantStep * plateQRDotScale / 2
	if got := dotRadius; got < wantDotR-1e-6 || got > wantDotR+1e-6 {
		t.Fatalf("unexpected module dot radius %.6f want %.6f (regular 2/3 seed QR)", got, wantDotR)
	}
}

func TestLaserCalibrationTestTileRendersGCodeWithinBounds(t *testing.T) {
	doc, err := (LaserCalibrationBuilder{
		Kind:          LaserCalibrationTestTile,
		PlateMM:       100,
		CalibrationMM: 25,
		TilePowerS:    850,
		TileFeedMMMin: 2000,
	}).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	outDir := t.TempDir()
	if err := (SceneGCodeRenderer{
		LaserOnCmd:     "M4",
		LaserMaxS:      850,
		CutFeedMMMin:   2000,
		FillStepMM:     0.03,
		RapidFeedMMMin: 8000,
		BedMM:          150,
		PlateMM:        100,
	}).Render(doc, outDir); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "laser_calibration_test_tile.gcode")); err != nil {
		t.Fatalf("expected gcode output: %v", err)
	}
}
