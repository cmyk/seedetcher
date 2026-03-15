package printer

import "testing"

func TestHatchSegmentsHorizontalSerpentineOrder(t *testing.T) {
	loops := [][]gcodePt{
		{
			{x: 0, y: 0},
			{x: 1, y: 0},
			{x: 1, y: 2},
			{x: 0, y: 2},
			{x: 0, y: 0},
		},
		{
			{x: 3, y: 0},
			{x: 4, y: 0},
			{x: 4, y: 2},
			{x: 3, y: 2},
			{x: 3, y: 0},
		},
	}

	segs := hatchSegments(loops, 1.0)
	if len(segs) < 4 {
		t.Fatalf("expected at least 4 segments, got %d", len(segs))
	}

	// First row: left-to-right by span order.
	if !(segs[0][0].x < segs[0][1].x && segs[1][0].x < segs[1][1].x) {
		t.Fatalf("expected first row left-to-right segments, got %+v %+v", segs[0], segs[1])
	}
	if !(segs[0][0].x < segs[1][0].x) {
		t.Fatalf("expected first-row span order left-to-right, got starts %.3f then %.3f", segs[0][0].x, segs[1][0].x)
	}

	// Second row: right-to-left and reversed span order.
	if !(segs[2][0].x > segs[2][1].x && segs[3][0].x > segs[3][1].x) {
		t.Fatalf("expected second row right-to-left segments, got %+v %+v", segs[2], segs[3])
	}
	if !(segs[2][0].x > segs[3][0].x) {
		t.Fatalf("expected second-row span order right-to-left, got starts %.3f then %.3f", segs[2][0].x, segs[3][0].x)
	}
}

func TestHatchSegmentsAutoVerticalForTallShape(t *testing.T) {
	loops := [][]gcodePt{
		{
			{x: 0, y: 0},
			{x: 1, y: 0},
			{x: 1, y: 5},
			{x: 0, y: 5},
			{x: 0, y: 0},
		},
	}

	segs := hatchSegments(loops, 1.0)
	if len(segs) == 0 {
		t.Fatalf("expected segments for tall shape")
	}
	// Vertical hatch emits x-constant segments.
	if segs[0][0].x != segs[0][1].x {
		t.Fatalf("expected vertical hatch segment, got %+v", segs[0])
	}
	if segs[0][0].y == segs[0][1].y {
		t.Fatalf("expected non-zero vertical segment length, got %+v", segs[0])
	}
}

func TestEffectiveFillStepMM_UsesPrimitiveOverride(t *testing.T) {
	cfg := (SceneGCodeRenderer{FillStepMM: 0.04}).withDefaults()
	if got := effectiveFillStepMM(cfg, ScenePrimitive{}); got != 0.04 {
		t.Fatalf("expected renderer fill step 0.04, got %.3f", got)
	}
	p := ScenePrimitive{FillStepMM: 0.06}
	if got := effectiveFillStepMM(cfg, p); got != 0.06 {
		t.Fatalf("expected primitive fill step 0.06, got %.3f", got)
	}
	if got := effectiveFillStepMM(SceneGCodeRenderer{}, ScenePrimitive{}); got != 0.12 {
		t.Fatalf("expected fallback fill step 0.12, got %.3f", got)
	}
}

func TestHatchSegments_NearSquareUsesHorizontalWithFloatNoise(t *testing.T) {
	loop := [][]gcodePt{
		{
			{x: 5.8, y: 9.48},
			{x: 9.28, y: 9.48},
			{x: 9.28, y: 12.96},
			{x: 5.8, y: 12.96},
			{x: 5.8, y: 9.48},
		},
	}
	segs := hatchSegments(loop, 0.03)
	if len(segs) == 0 {
		t.Fatalf("expected hatch segments")
	}
	// Must stay horizontal for square-ish cells.
	if segs[0][0].y != segs[0][1].y {
		t.Fatalf("expected horizontal segment, got %+v", segs[0])
	}
}
