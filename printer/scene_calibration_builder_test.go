package printer

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

func TestLaserCalibrationBuilderPowerGridUsesFullAnnotationPower(t *testing.T) {
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
	wantPower := 1000.0
	wantFeed := 400.0
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
		Kind:                  LaserCalibrationTestTile,
		PlateMM:               100,
		CalibrationMM:         25,
		OffsetXMM:             50,
		OffsetYMM:             25,
		TilePowerS:            850,
		TileFeedMMMin:         2000,
		TileFillStepMM:        0.04,
		TileOutlinePowerScale: 1.0,
		TileOutlineFeedScale:  0.7,
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
	foundFinder7 := false
	foundFinder5 := false
	finder7Width := 0.0
	finder5Width := 0.0
	dotRadius := 0.0
	foundInfo := false
	var infoX, infoY, infoPt, infoTracking float64
	infoAnchor := TextAnchor("")
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
		case p.Kind == PrimitiveText && p.Text == "CHOIC":
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
		case p.Kind == PrimitiveText && p.Text == "2000/850/0.04/1/0.7":
			foundInfo = true
			if p.FillMode != FillModeHatch {
				t.Fatalf("info text should be raster-filled, got fill mode %q", p.FillMode)
			}
			if !p.NoOutline {
				t.Fatalf("info text should be fill-only (NoOutline=true)")
			}
			if p.FillStepMM != 0.04 {
				t.Fatalf("info text should inherit tile fill-step, got %.3f", p.FillStepMM)
			}
			infoX = p.XMM
			infoY = p.YMM
			infoPt = p.FontSizePt
			infoTracking = p.TrackingEM
			infoAnchor = p.Anchor
		case p.Kind == PrimitiveRing:
			if p.WidthMM > 3 {
				foundFinder = true
				if finder7Width == 0 || p.WidthMM > finder7Width {
					finder7Width = p.WidthMM
				}
				if finder5Width == 0 || p.WidthMM < finder5Width {
					finder5Width = p.WidthMM
				}
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
	if !foundInfo {
		t.Fatalf("expected parameter info text in tile")
	}
	wantStep := seedQRSizeMM / 29.0
	wantFinder7W := 7 * wantStep
	wantFinder5W := 5 * wantStep
	if got := finder7Width; got < wantFinder7W-1e-6 || got > wantFinder7W+1e-6 {
		t.Fatalf("unexpected 7x7 finder width %.6f want %.6f (regular 2/3 seed QR)", got, wantFinder7W)
	}
	if got := finder5Width; got < wantFinder5W-1e-6 || got > wantFinder5W+1e-6 {
		t.Fatalf("unexpected 5x5 finder width %.6f want %.6f (regular 2/3 seed QR)", got, wantFinder5W)
	}
	wantDotR := wantStep * plateQRDotScale / 2
	if got := dotRadius; got < wantDotR-1e-6 || got > wantDotR+1e-6 {
		t.Fatalf("unexpected module dot radius %.6f want %.6f (regular 2/3 seed QR)", got, wantDotR)
	}
	smallFinderX := 1.0 + 8.0*wantStep
	smallFinderY := 25.0 - 1.0 - 7.0*wantStep
	if infoX < smallFinderX-1e-6 || infoX > smallFinderX+1e-6 {
		t.Fatalf("unexpected info X %.6f want %.6f (below small finder left edge)", infoX, smallFinderX)
	}
	if !(infoY > smallFinderY+5.0*wantStep) {
		t.Fatalf("unexpected info Y %.6f: expected below small finder bottom %.6f", infoY, smallFinderY+5.0*wantStep)
	}
	const wantInfoPt = 1.5 * 72.0 / 25.4
	if infoPt < wantInfoPt-1e-6 || infoPt > wantInfoPt+1e-6 {
		t.Fatalf("unexpected info font size %.6f want %.6fpt", infoPt, wantInfoPt)
	}
	if infoTracking < 0.05-1e-9 || infoTracking > 0.05+1e-9 {
		t.Fatalf("unexpected info tracking %.6f want 0.05", infoTracking)
	}
	if infoAnchor != TextAnchorTopLeft {
		t.Fatalf("unexpected info anchor %q want %q", infoAnchor, TextAnchorTopLeft)
	}
	foundFinder7 = finder7Width > 0
	foundFinder5 = finder5Width > 0
	if !foundFinder7 || !foundFinder5 {
		t.Fatalf("expected both 7x7 and 5x5 finders in tile, got finder7=%v finder5=%v", foundFinder7, foundFinder5)
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

func TestLaserCalibrationBuilderBuildsLineWidthTile(t *testing.T) {
	doc, err := (LaserCalibrationBuilder{
		Kind:          LaserCalibrationLineWidth,
		PlateMM:       100,
		CalibrationMM: 25,
		OffsetXMM:     25,
		OffsetYMM:     50,
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
	if scene.Name != "laser_calibration_line_width_tile" {
		t.Fatalf("unexpected scene name %q", scene.Name)
	}
	if scene.WidthMM != 25 || scene.HeightMM != 25 {
		t.Fatalf("unexpected scene size %.1fx%.1f", scene.WidthMM, scene.HeightMM)
	}
	if scene.OffsetInPlateXMM != 25 || scene.OffsetInPlateYMM != 50 {
		t.Fatalf("unexpected offsets %.1f,%.1f", scene.OffsetInPlateXMM, scene.OffsetInPlateYMM)
	}
	lineCount := 0
	labelCount := 0
	centeredLabelCount := 0
	for _, p := range scene.Layers[0].Primitives {
		switch {
		case p.Kind == PrimitivePath && p.FillMode == FillModeNone && p.PowerS > 0 && p.FeedMMMin > 0:
			lineCount++
		case p.Kind == PrimitiveText:
			labelCount++
			if p.Anchor == TextAnchorCenter {
				centeredLabelCount++
			}
		}
	}
	if lineCount != 9 {
		t.Fatalf("expected 9 calibration lines, got %d", lineCount)
	}
	if labelCount != 9 {
		t.Fatalf("expected 9 labels, got %d", labelCount)
	}
	if centeredLabelCount != 9 {
		t.Fatalf("expected 9 center-anchored labels, got %d", centeredLabelCount)
	}
}

func TestLaserCalibrationLineWidthTileRendersGCodeWithinBounds(t *testing.T) {
	doc, err := (LaserCalibrationBuilder{
		Kind:          LaserCalibrationLineWidth,
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
	if _, err := os.Stat(filepath.Join(outDir, "laser_calibration_line_width_tile.gcode")); err != nil {
		t.Fatalf("expected gcode output: %v", err)
	}
}

func TestLaserCalibrationBuilderBuildsFillStepTile(t *testing.T) {
	doc, err := (LaserCalibrationBuilder{
		Kind:          LaserCalibrationFillStep,
		PlateMM:       100,
		CalibrationMM: 25,
		OffsetXMM:     25,
		OffsetYMM:     50,
		TilePowerS:    850,
		Feeds:         []float64{1400, 1700, 2000, 2300, 2600},
	}).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(doc.Scenes) != 1 {
		t.Fatalf("expected 1 scene, got %d", len(doc.Scenes))
	}
	scene := doc.Scenes[0]
	if scene.Name != "laser_calibration_fill_step_test" {
		t.Fatalf("unexpected scene name %q", scene.Name)
	}
	if scene.WidthMM != 25 || scene.HeightMM != 25 {
		t.Fatalf("unexpected scene size %.1fx%.1f", scene.WidthMM, scene.HeightMM)
	}
	if scene.OffsetInPlateXMM != 25 || scene.OffsetInPlateYMM != 50 {
		t.Fatalf("unexpected offsets %.1f,%.1f", scene.OffsetInPlateXMM, scene.OffsetInPlateYMM)
	}
	titleFound := false
	colLabels := map[string]bool{}
	rowLabels := map[string]bool{}
	fillCells := 0
	steps := map[float64]bool{}
	rowFeeds := map[float64]bool{}
	minCellY := 1e9
	maxTopLabelBaselineY := 0.0
	for _, p := range scene.Layers[0].Primitives {
		if p.Kind == PrimitiveText && p.Text == "Fill-Step Test S850 (M4)" {
			titleFound = true
		}
		if p.Kind == PrimitiveText {
			if p.Text == "0.03" || p.Text == "0.035" || p.Text == "0.04" || p.Text == "0.05" || p.Text == "0.06" {
				colLabels[p.Text] = true
				if p.YMM > maxTopLabelBaselineY {
					maxTopLabelBaselineY = p.YMM
				}
			}
			if p.Text == "F1400" || p.Text == "F1700" || p.Text == "F2000" || p.Text == "F2300" || p.Text == "F2600" {
				rowLabels[p.Text] = true
			}
		}
		if p.Kind == PrimitiveRect && p.FillMode == FillModeHatch && p.FillColor == sceneBlack {
			fillCells++
			steps[p.FillStepMM] = true
			rowFeeds[p.FeedMMMin] = true
			if p.PowerS != 850 {
				t.Fatalf("unexpected power in cell: %d", p.PowerS)
			}
			if !p.NoOutline {
				t.Fatalf("fill-step cells must be fill-only (NoOutline=true)")
			}
			if p.WidthMM != p.HeightMM {
				t.Fatalf("fill-step cells should be square, got %.4fx%.4f", p.WidthMM, p.HeightMM)
			}
			if p.YMM < minCellY {
				minCellY = p.YMM
			}
		}
	}
	if !titleFound {
		t.Fatalf("missing title text")
	}
	if len(colLabels) != 5 {
		t.Fatalf("expected 5 column labels, got %d", len(colLabels))
	}
	if len(rowLabels) != 5 {
		t.Fatalf("expected 5 row labels, got %d", len(rowLabels))
	}
	if fillCells != 25 {
		t.Fatalf("expected 25 fill cells, got %d", fillCells)
	}
	if len(steps) != 5 {
		t.Fatalf("expected 5 distinct fill steps, got %d", len(steps))
	}
	if len(rowFeeds) != 5 {
		t.Fatalf("expected 5 distinct row feeds, got %d", len(rowFeeds))
	}
	if minCellY <= maxTopLabelBaselineY+0.45 {
		t.Fatalf("top labels too close to first row: baselineY=%.3f cellY=%.3f", maxTopLabelBaselineY, minCellY)
	}
}

func TestLaserCalibrationFillStepTileRendersGCodeWithinBounds(t *testing.T) {
	doc, err := (LaserCalibrationBuilder{
		Kind:          LaserCalibrationFillStep,
		PlateMM:       100,
		CalibrationMM: 25,
		TilePowerS:    850,
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
	if _, err := os.Stat(filepath.Join(outDir, "laser_calibration_fill_step_test.gcode")); err != nil {
		t.Fatalf("expected gcode output: %v", err)
	}
}

func TestParseCalibrationFloatSeries_RangeDefaultsToCount(t *testing.T) {
	got, err := ParseCalibrationFloatSeries("1400:2600", 5)
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 values, got %d", len(got))
	}
	if got[0] != 1400 || got[len(got)-1] != 2600 {
		t.Fatalf("unexpected endpoints: %v", got)
	}
}

func TestParseCalibrationFloatSeries_RangeWithExplicitCount(t *testing.T) {
	got, err := ParseCalibrationFloatSeries("0.03:0.06:5", 0)
	if err != nil {
		t.Fatalf("parse range with count: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 values, got %d", len(got))
	}
	if got[0] != 0.03 || got[2] != 0.045 || got[4] != 0.06 {
		t.Fatalf("unexpected values: %v", got)
	}
}

func TestLaserCalibrationBuilderBuildsFillStepTileTitleUsesLaserMode(t *testing.T) {
	doc, err := (LaserCalibrationBuilder{
		Kind:          LaserCalibrationFillStep,
		PlateMM:       100,
		CalibrationMM: 25,
		TilePowerS:    850,
		TileLaserMode: "m3",
		FillStepFeeds: []float64{1400, 1700, 2000, 2300, 2600},
	}).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	scene := doc.Scenes[0]
	found := false
	for _, p := range scene.Layers[0].Primitives {
		if p.Kind == PrimitiveText && p.Text == "Fill-Step Test S850 (M3)" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected fill-step title to include selected laser mode")
	}
}

func TestLaserCalibrationBuilderBuildsPowerFeedTile(t *testing.T) {
	doc, err := (LaserCalibrationBuilder{
		Kind:            LaserCalibrationPowerFeed,
		PlateMM:         100,
		CalibrationMM:   25,
		OffsetXMM:       10,
		OffsetYMM:       20,
		TilePowerS:      850,
		TileLaserMode:   "m3",
		TileFillStepMM:  0.04,
		PowerFeedPowers: []int{680, 720, 760, 800, 850},
		PowerFeedFeeds:  []float64{1400, 1700, 2000, 2300, 2600},
	}).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(doc.Scenes) != 1 {
		t.Fatalf("expected 1 scene, got %d", len(doc.Scenes))
	}
	scene := doc.Scenes[0]
	if scene.Name != "laser_calibration_power_feed_test" {
		t.Fatalf("unexpected scene name %q", scene.Name)
	}
	if scene.OffsetInPlateXMM != 10 || scene.OffsetInPlateYMM != 20 {
		t.Fatalf("unexpected offsets %.1f,%.1f", scene.OffsetInPlateXMM, scene.OffsetInPlateYMM)
	}
	titleFound := false
	fillCells := 0
	powerSet := map[int]bool{}
	feedSet := map[float64]bool{}
	colLabels := map[string]bool{}
	rowLabels := map[string]bool{}
	for _, p := range scene.Layers[0].Primitives {
		if p.Kind == PrimitiveText && p.Text == "Power-Feed Test (M3, 0.04)" {
			titleFound = true
		}
		if p.Kind == PrimitiveText {
			switch p.Text {
			case "680", "720", "760", "800", "850":
				colLabels[p.Text] = true
			case "1400", "1700", "2000", "2300", "2600":
				rowLabels[p.Text] = true
			}
			if p.FillMode != FillModeHatch {
				t.Fatalf("power-feed labels should be raster-filled, got fill mode %q for %q", p.FillMode, p.Text)
			}
			if !p.NoOutline {
				t.Fatalf("power-feed labels should be no-outline raster, got NoOutline=%v for %q", p.NoOutline, p.Text)
			}
		}
		if p.Kind == PrimitiveRect && p.FillMode == FillModeHatch && p.FillColor == sceneBlack {
			fillCells++
			powerSet[p.PowerS] = true
			feedSet[p.FeedMMMin] = true
			if !p.NoOutline {
				t.Fatalf("power-feed cells must be fill-only (NoOutline=true)")
			}
			if p.FillStepMM != 0.04 {
				t.Fatalf("expected per-cell fill step 0.04, got %.3f", p.FillStepMM)
			}
		}
	}
	if !titleFound {
		t.Fatalf("missing power-feed title")
	}
	if len(colLabels) != 5 {
		t.Fatalf("expected 5 column labels, got %d", len(colLabels))
	}
	if len(rowLabels) != 5 {
		t.Fatalf("expected 5 row labels, got %d", len(rowLabels))
	}
	if fillCells != 25 {
		t.Fatalf("expected 25 fill cells, got %d", fillCells)
	}
	if len(powerSet) != 5 {
		t.Fatalf("expected 5 distinct powers, got %d", len(powerSet))
	}
	if len(feedSet) != 5 {
		t.Fatalf("expected 5 distinct feeds, got %d", len(feedSet))
	}
}

func TestLaserCalibrationPowerFeedTileRendersGCodeWithinBounds(t *testing.T) {
	doc, err := (LaserCalibrationBuilder{
		Kind:          LaserCalibrationPowerFeed,
		PlateMM:       100,
		CalibrationMM: 25,
		TilePowerS:    850,
	}).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	outDir := t.TempDir()
	if err := (SceneGCodeRenderer{
		LaserOnCmd:     "M4",
		LaserMaxS:      850,
		CutFeedMMMin:   2000,
		FillStepMM:     0.04,
		RapidFeedMMMin: 8000,
		BedMM:          150,
		PlateMM:        100,
	}).Render(doc, outDir); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "laser_calibration_power_feed_test.gcode")); err != nil {
		t.Fatalf("expected gcode output: %v", err)
	}
}

func TestParseCalibrationIntSeries_RangeWithExplicitCount(t *testing.T) {
	got, err := ParseCalibrationIntSeries("650:850:5", 0)
	if err != nil {
		t.Fatalf("parse int range with count: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 values, got %d", len(got))
	}
	if got[0] != 650 || got[2] != 750 || got[4] != 850 {
		t.Fatalf("unexpected values: %v", got)
	}
}

func TestLaserCalibrationBuilderBuildsRepeatabilityTile(t *testing.T) {
	doc, err := (LaserCalibrationBuilder{
		Kind:          LaserCalibrationRepeat,
		PlateMM:       150,
		CalibrationMM: 150,
		TilePowerS:    850,
		TileFeedMMMin: 2000,
		TileLaserMode: "m4",
	}).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(doc.Scenes) != 1 {
		t.Fatalf("expected 1 scene, got %d", len(doc.Scenes))
	}
	scene := doc.Scenes[0]
	if scene.Name != "laser_calibration_repeatability_test" {
		t.Fatalf("unexpected scene name %q", scene.Name)
	}
	titleFound := false
	labelSet := map[string]bool{}
	labelY := map[string]float64{}
	titleY := 0.0
	markCount := 0
	for _, p := range scene.Layers[0].Primitives {
		if p.Kind == PrimitiveText && p.Text == "Repeatability Test L12 (M4)" {
			titleFound = true
			titleY = p.YMM
		}
		if p.Kind == PrimitiveText && (p.Text == "LL" || p.Text == "LR" || p.Text == "UR" || p.Text == "UL" || p.Text == "C") {
			labelSet[p.Text] = true
			labelY[p.Text] = p.YMM
		}
		if p.Kind == PrimitivePath && p.FillMode == FillModeNone && p.StrokeMM == 0.10 && p.PowerS == 850 && p.FeedMMMin == 2000 {
			markCount++
		}
	}
	if !titleFound {
		t.Fatalf("missing repeatability title")
	}
	if len(labelSet) != 5 {
		t.Fatalf("expected 5 sector labels, got %d", len(labelSet))
	}
	if labelY["UL"] <= titleY+2.0 || labelY["UR"] <= titleY+2.0 {
		t.Fatalf("upper labels too close to title: titleY=%.3f UL=%.3f UR=%.3f", titleY, labelY["UL"], labelY["UR"])
	}
	if markCount != 60 {
		t.Fatalf("expected 60 repeated marks (12 laps * 5 targets), got %d", markCount)
	}
}

func TestLaserCalibrationRepeatabilityTileRendersGCodeWithinBounds(t *testing.T) {
	doc, err := (LaserCalibrationBuilder{
		Kind:          LaserCalibrationRepeat,
		PlateMM:       150,
		CalibrationMM: 150,
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
		FillStepMM:     0.04,
		RapidFeedMMMin: 8000,
		BedMM:          150,
		PlateMM:        150,
	}).Render(doc, outDir); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "laser_calibration_repeatability_test.gcode")); err != nil {
		t.Fatalf("expected gcode output: %v", err)
	}
}

func TestParseLaserCalibrationKind_RepeatabilityAliases(t *testing.T) {
	for _, in := range []string{"repeatability-test", "repeatability", "bed-repeatability"} {
		got, err := ParseLaserCalibrationKind(in)
		if err != nil {
			t.Fatalf("parse %q: %v", in, err)
		}
		if got != LaserCalibrationRepeat {
			t.Fatalf("parse %q got %q want %q", in, got, LaserCalibrationRepeat)
		}
	}
}

func TestLaserCalibrationBuilderBuildsSectorRepeatabilityTile(t *testing.T) {
	doc, err := (LaserCalibrationBuilder{
		Kind:          LaserCalibrationSectorRep,
		PlateMM:       100,
		CalibrationMM: 150,
		TilePowerS:    850,
		TileFeedMMMin: 2000,
		TileLaserMode: "m4",
	}).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(doc.Scenes) != 1 {
		t.Fatalf("expected 1 scene, got %d", len(doc.Scenes))
	}
	scene := doc.Scenes[0]
	if scene.Name != "laser_calibration_sector_repeatability_test" {
		t.Fatalf("unexpected scene name %q", scene.Name)
	}
	labelSet := map[string]bool{}
	labelPos := map[string]struct {
		x      float64
		y      float64
		anchor TextAnchor
	}{}
	frameCount := 0
	markCount := 0
	for _, p := range scene.Layers[0].Primitives {
		if p.Kind == PrimitivePath && p.FillMode == FillModeNone && p.StrokeMM == 0.12 {
			frameCount++
			if strings.Contains(p.PathData, " 0.0000") || strings.Contains(p.PathData, "150.0000") {
				t.Fatalf("guide frame must stay inside workspace perimeter, got %q", p.PathData)
			}
		}
		if p.Kind == PrimitiveText && (p.Text == "BL" || p.Text == "BR" || p.Text == "TL" || p.Text == "TR" || p.Text == "C") {
			labelSet[p.Text] = true
			labelPos[p.Text] = struct {
				x      float64
				y      float64
				anchor TextAnchor
			}{x: p.XMM, y: p.YMM, anchor: p.Anchor}
		}
		if p.Kind == PrimitivePath && p.FillMode == FillModeNone && p.StrokeMM == 0.10 && p.PowerS == 850 && p.FeedMMMin == 2000 {
			markCount++
		}
	}
	if frameCount != 5 {
		t.Fatalf("expected 5 sector guide frames, got %d", frameCount)
	}
	if len(labelSet) != 5 {
		t.Fatalf("expected 5 sector labels, got %d", len(labelSet))
	}
	// Verify label semantics: each corner label is placed in matching frame corner.
	if p, ok := labelPos["TL"]; !ok || !(p.x < 10 && p.y < 15 && p.anchor == TextAnchorBaselineLeft) {
		t.Fatalf("TL label not in top-left corner: %+v", p)
	}
	if p, ok := labelPos["TR"]; !ok || !(p.x > 140 && p.y < 15 && p.anchor == TextAnchorBaselineLeft) {
		t.Fatalf("TR label not in top-right corner: %+v", p)
	}
	if p, ok := labelPos["BL"]; !ok || !(p.x < 10 && p.y > 145 && p.anchor == TextAnchorBaselineLeft) {
		t.Fatalf("BL label not in bottom-left corner: %+v", p)
	}
	if p, ok := labelPos["BR"]; !ok || !(p.x > 140 && p.y > 145 && p.anchor == TextAnchorBaselineLeft) {
		t.Fatalf("BR label not in bottom-right corner: %+v", p)
	}
	if p, ok := labelPos["C"]; !ok || !(p.x > 70 && p.x < 80 && p.y > 70 && p.y < 80 && p.anchor == TextAnchorCenter) {
		t.Fatalf("C label not centered: %+v", p)
	}
	if markCount != 50 {
		t.Fatalf("expected 50 repeated marks (2 laps * 5 sectors * 5 anchors), got %d", markCount)
	}
	sectorLocalPrefix := 0
	for _, p := range scene.Layers[0].Primitives {
		if p.Kind != PrimitivePath || p.FillMode != FillModeNone || p.StrokeMM != 0.10 || p.PowerS != 850 || p.FeedMMMin != 2000 {
			continue
		}
		var x, y float64
		if _, err := fmt.Sscanf(p.PathData, "M%f %fL", &x, &y); err != nil {
			t.Fatalf("parse mark path %q: %v", p.PathData, err)
		}
		// First sector is BL: x in [0,100], y in [50,150] (using mark lower-left point).
		if x < 0 || x > 100 || y < 50 || y > 150 {
			break
		}
		sectorLocalPrefix++
	}
	if sectorLocalPrefix < 10 {
		t.Fatalf("expected first sector marks to stay local for at least 10 marks, got %d", sectorLocalPrefix)
	}
}

func TestLaserCalibrationSectorRepeatabilityRendersGCodeWithinBounds(t *testing.T) {
	doc, err := (LaserCalibrationBuilder{
		Kind:          LaserCalibrationSectorRep,
		PlateMM:       100,
		CalibrationMM: 150,
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
		FillStepMM:     0.04,
		RapidFeedMMMin: 8000,
		BedMM:          150,
		PlateMM:        100,
	}).Render(doc, outDir); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "laser_calibration_sector_repeatability_test.gcode")); err != nil {
		t.Fatalf("expected gcode output: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outDir, "laser_calibration_sector_repeatability_test.gcode"))
	if err != nil {
		t.Fatalf("read gcode output: %v", err)
	}
	if strings.Contains(string(data), "; Preview frame (laser off)") {
		t.Fatalf("sector repeatability gcode should not inject renderer preview frame")
	}
}

func TestParseLaserCalibrationKind_SectorRepeatabilityAliases(t *testing.T) {
	for _, in := range []string{"sector-repeatability-test", "sector-repeatability", "sector-repeat", "sector-test", "bed-sector-repeatability"} {
		got, err := ParseLaserCalibrationKind(in)
		if err != nil {
			t.Fatalf("parse %q: %v", in, err)
		}
		if got != LaserCalibrationSectorRep {
			t.Fatalf("parse %q got %q want %q", in, got, LaserCalibrationSectorRep)
		}
	}
}
