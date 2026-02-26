package printer

import (
	"fmt"
	"image"
	"image/draw"
)

type EtchPlateStat struct {
	PlateIndex int
	Side       string // "seed" or "descriptor"

	BlackPixels int64
	WhitePixels int64

	BlackMM2 float64 // Toner area within 90x90 mask
	WhiteMM2 float64 // Exposed area within 90x90 mask

	// Scenario A: user masks the 10mm outer margin (only 90x90 center etched).
	ExposedMaskedMM2 float64
	ExposedMaskedPct float64 // of full 100x100 steel plate

	// Scenario B: user does not mask the 10mm outer margin (outer ring also etched).
	ExposedUnmaskedMM2 float64
	ExposedUnmaskedPct float64 // of full 100x100 steel plate
}

type EtchStatsReport struct {
	DPI   float64
	Paper PaperSize

	MaskAreaMM2  float64 // 90x90 mask area
	SteelAreaMM2 float64 // 100x100 physical plate area

	Stats []EtchPlateStat
}

const (
	maskAreaMM2   = 90.0 * 90.0
	steelAreaMM2  = 100.0 * 100.0
	marginAreaMM2 = steelAreaMM2 - maskAreaMM2

	// Bench defaults for the operator section on the stats page.
	etchGapMM          = 15.0
	etchTempC          = 34.0
	etchSulfateGPerL   = 100.0
	etchVoltageLimitV  = 12.0
	etchCurrentDensity = 0.04 // A/cm^2 default for "set current"
)

func BuildEtchStatsReport(seedPlates, descPlates []*image.Paletted, dpi float64, paper PaperSize) (EtchStatsReport, error) {
	if len(seedPlates) == 0 {
		return EtchStatsReport{}, fmt.Errorf("no seed plates")
	}
	stats := make([]EtchPlateStat, 0, len(seedPlates)*2)
	for i, sp := range seedPlates {
		if sp != nil {
			stats = append(stats, plateStat(sp, i+1, "seed", dpi))
		}
		if len(descPlates) > 0 && i < len(descPlates) {
			p := descPlates[i]
			if p == nil {
				continue
			}
			stats = append(stats, plateStat(p, i+1, "descriptor", dpi))
		}
	}
	return BuildEtchStatsReportFromStats(stats, dpi, paper)
}

func BuildEtchStatsReportFromStats(stats []EtchPlateStat, dpi float64, paper PaperSize) (EtchStatsReport, error) {
	if len(stats) == 0 {
		return EtchStatsReport{}, fmt.Errorf("no plate stats")
	}
	r := EtchStatsReport{
		DPI:          dpi,
		Paper:        paper,
		MaskAreaMM2:  maskAreaMM2,
		SteelAreaMM2: steelAreaMM2,
		Stats:        make([]EtchPlateStat, len(stats)),
	}
	copy(r.Stats, stats)
	return r, nil
}

func ComputeEtchPlateStat(p *image.Paletted, plateIdx int, side string, dpi float64) EtchPlateStat {
	return plateStat(p, plateIdx, side, dpi)
}

func plateStat(p *image.Paletted, plateIdx int, side string, dpi float64) EtchPlateStat {
	var black, white int64
	b := p.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := p.Pix[y*p.Stride:]
		for x := b.Min.X; x < b.Max.X; x++ {
			if row[x] == 1 {
				black++
			} else {
				white++
			}
		}
	}
	pxArea := (25.4 / dpi) * (25.4 / dpi)
	blackMM2 := float64(black) * pxArea
	whiteMM2 := float64(white) * pxArea
	exposedMasked := whiteMM2
	exposedUnmasked := whiteMM2 + marginAreaMM2
	return EtchPlateStat{
		PlateIndex:         plateIdx,
		Side:               side,
		BlackPixels:        black,
		WhitePixels:        white,
		BlackMM2:           blackMM2,
		WhiteMM2:           whiteMM2,
		ExposedMaskedMM2:   exposedMasked,
		ExposedMaskedPct:   pct(exposedMasked, steelAreaMM2),
		ExposedUnmaskedMM2: exposedUnmasked,
		ExposedUnmaskedPct: pct(exposedUnmasked, steelAreaMM2),
	}
}

func pct(v, denom float64) float64 {
	if denom <= 0 {
		return 0
	}
	return 100.0 * v / denom
}

func RenderEtchStatsPage(report EtchStatsReport, paper PaperSize, dpi float64) (*image.Paletted, error) {
	pageWmm, pageHmm, ok := paperDimsMM(paper)
	if !ok {
		return nil, fmt.Errorf("unsupported paper size: %v", paper)
	}
	page := image.NewPaletted(image.Rect(0, 0, mmToPx(pageWmm, dpi), mmToPx(pageHmm, dpi)), bwPalette)
	draw.Draw(page, page.Bounds(), &image.Uniform{bwPalette[0]}, image.Point{}, draw.Src)
	black := uint8(1)

	faceTitle := loadFaceMedium(12, dpi)
	faceBody := loadFaceMedium(9, dpi)
	titleTrack := 0.04 * 12.0 * dpi / 72.0
	bodyTrack := 0.02 * 9.0 * dpi / 72.0

	x := 8.0
	y := 10.0 + capBaselineOffsetMM(faceTitle, dpi)
	drawTrackedText(page, faceTitle, dpi, x, y, "ETCH STATS", titleTrack)

	y += 6.0
	meta := fmt.Sprintf("DPI: %.0f  PAPER: %s  PHYSICAL PLATE: 100x100 mm", report.DPI, report.Paper)
	drawTrackedText(page, faceBody, dpi, x, y, meta, bodyTrack)

	y += 5.5
	defaults := fmt.Sprintf("DEFAULTS: Na2SO4 %.0fg/L  TEMP %.0fC  GAP %.0fmm  V-LIMIT %.0fV  J %.2fA/cm2",
		etchSulfateGPerL, etchTempC, etchGapMM, etchVoltageLimitV, etchCurrentDensity)
	drawTrackedText(page, faceBody, dpi, x, y, defaults, bodyTrack)

	y += 5.0
	drawTrackedText(page, faceBody, dpi, x, y, "AREA TABLE", bodyTrack)
	y += 4.5
	drawTrackedText(page, faceBody, dpi, x, y, "PLATE          TONER(mm2)  EXPOSED90(mm2)  EXPOSED+MARGIN(mm2)  %MASKED  %UNMASKED", bodyTrack)
	y += 4.0

	for _, s := range report.Stats {
		side := "SEED"
		if s.Side == "descriptor" {
			side = "DESC"
		}
		plateID := fmt.Sprintf("%02d %s", s.PlateIndex, side)
		line := fmt.Sprintf("%-13s %10.1f    %11.1f      %14.1f   %6.1f%%   %8.1f%%",
			plateID, s.BlackMM2, s.ExposedMaskedMM2, s.ExposedUnmaskedMM2, s.ExposedMaskedPct, s.ExposedUnmaskedPct)
		drawTrackedText(page, faceBody, dpi, x, y, line, bodyTrack)
		y += 4.0
		if y > pageHmm-20.0 {
			break
		}
	}

	y += 1.0
	drawTrackedText(page, faceBody, dpi, x, y, "PSU GUIDE TABLE", bodyTrack)
	y += 4.5
	drawTrackedText(page, faceBody, dpi, x, y, "PLATE          SET A MASKED  SET A UNMASKED", bodyTrack)
	y += 4.0
	for _, s := range report.Stats {
		side := "SEED"
		if s.Side == "descriptor" {
			side = "DESC"
		}
		plateID := fmt.Sprintf("%02d %s", s.PlateIndex, side)
		iMasked := setCurrentA(s.ExposedMaskedMM2, etchCurrentDensity)
		iUnmasked := setCurrentA(s.ExposedUnmaskedMM2, etchCurrentDensity)
		line := fmt.Sprintf("%-13s    %6.2f A       %6.2f A", plateID, iMasked, iUnmasked)
		drawTrackedText(page, faceBody, dpi, x, y, line, bodyTrack)
		y += 4.0
		if y > pageHmm-8.0 {
			break
		}
	}

	legendY := pageHmm - 8.0
	drawTrackedText(page, faceBody, dpi, x, legendY, "Masked=only 90x90 center exposed. Unmasked=90x90 + 10mm outer margin exposed.", bodyTrack)
	_ = black
	return page, nil
}

func setCurrentA(exposedMM2, currentDensityAperCM2 float64) float64 {
	if exposedMM2 <= 0 || currentDensityAperCM2 <= 0 {
		return 0
	}
	exposedCM2 := exposedMM2 / 100.0
	return exposedCM2 * currentDensityAperCM2
}
