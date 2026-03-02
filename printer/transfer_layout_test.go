package printer

import (
	"image"
	"testing"
)

func TestBuildPlacementPlanFixedSlotsAcrossPapers(t *testing.T) {
	const dpi = 300.0
	seed := make([]*image.Paletted, 7)
	for i := range seed {
		seed[i] = newTestPlate(100, 100, 1)
	}

	for _, paper := range []PaperSize{PaperA4, PaperLetter} {
		plan, err := buildPlacementPlan(seed, nil, paper, dpi, false, nil)
		if err != nil {
			t.Fatalf("buildPlacementPlan(%s): %v", paper, err)
		}
		if len(plan.pages) != 2 {
			t.Fatalf("%s: expected 2 pages for 7 shares in 2x2 layout, got %d", paper, len(plan.pages))
		}
		for pi, page := range plan.pages {
			if got := len(page.slots); got > 4 {
				t.Fatalf("%s: page %d has %d slots, want <=4", paper, pi, got)
			}
		}
	}
}

func TestBuildPlacementPlanUses95x100CutBoxAndPlateInset(t *testing.T) {
	const dpi = 300.0
	plateW, plateH := 120, 90
	seed := []*image.Paletted{newTestPlate(plateW, plateH, 1)}

	plan, err := buildPlacementPlan(seed, nil, PaperA4, dpi, false, nil)
	if err != nil {
		t.Fatalf("buildPlacementPlan: %v", err)
	}
	if len(plan.pages) != 1 || len(plan.pages[0].cutBoxes) != 1 {
		t.Fatalf("unexpected plan shape: pages=%d cutBoxes=%d", len(plan.pages), len(plan.pages[0].cutBoxes))
	}

	box := plan.pages[0].cutBoxes[0]
	wantW := plateW + mmToPx(transferPlateInsetLeftMM, dpi)
	if box.Dx() != wantW {
		t.Fatalf("cut-box width=%d, want=%d", box.Dx(), wantW)
	}
	wantH := plateH + mmToPx(transferPlateInsetTopMM, dpi) + mmToPx(transferPlateInsetBottomMM, dpi)
	if box.Dy() != wantH {
		t.Fatalf("cut-box height=%d, want=%d", box.Dy(), wantH)
	}
	slot := plan.pages[0].slots[0]
	wantX := box.Min.X + mmToPx(transferPlateInsetLeftMM, dpi)
	wantY := box.Min.Y + mmToPx(transferPlateInsetTopMM, dpi)
	if slot.x != wantX || slot.y != wantY {
		t.Fatalf("slot origin=(%d,%d), want=(%d,%d)", slot.x, slot.y, wantX, wantY)
	}
	if slot.x+plateW != box.Max.X {
		t.Fatalf("plate right edge=%d, want cut-box right edge=%d", slot.x+plateW, box.Max.X)
	}
}

func TestRenderPlannedRowInvertFillsTransferBox(t *testing.T) {
	const dpi = 300.0
	plateW, plateH := 120, 90
	seed := []*image.Paletted{newTestPlate(plateW, plateH, 1)}

	plan, err := buildPlacementPlan(seed, nil, PaperA4, dpi, true, nil)
	if err != nil {
		t.Fatalf("buildPlacementPlan: %v", err)
	}
	page := plan.pages[0]
	box := page.cutBoxes[0]
	insideInsetX := box.Min.X + mmToPx(transferPlateInsetLeftMM, dpi)/2
	insideInsetY := box.Min.Y + mmToPx(transferPlateInsetTopMM, dpi)/2
	if insideInsetX <= box.Min.X {
		insideInsetX = box.Min.X + 1
	}
	if insideInsetY <= box.Min.Y {
		insideInsetY = box.Min.Y + 1
	}
	if insideInsetX >= box.Max.X-1 {
		insideInsetX = box.Max.X - 2
	}
	if insideInsetY >= box.Max.Y-1 {
		insideInsetY = box.Max.Y - 2
	}

	row := make([]uint8, plan.pageWpx)
	renderPlannedRow(row, insideInsetY, page, true)
	for x := box.Min.X; x < box.Max.X; x++ {
		if row[x] != 1 {
			t.Fatalf("invert row x=%d inside transfer box is %d, want black(1)", x, row[x])
		}
	}

	renderPlannedRow(row, insideInsetY, page, false)
	if row[insideInsetX] != 0 {
		t.Fatalf("non-invert inset interior x=%d is %d, want white(0)", insideInsetX, row[insideInsetX])
	}
}

func newTestPlate(w, h int, fill uint8) *image.Paletted {
	img := image.NewPaletted(image.Rect(0, 0, w, h), bwPalette)
	if fill != 0 {
		for i := range img.Pix {
			img.Pix[i] = fill
		}
	}
	return img
}
