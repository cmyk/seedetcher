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

	ExposedMM2 float64 // Exposed area on the plate
	ExposedPct float64 // of full 100x100 steel plate
}

type EtchStatsReport struct {
	DPI   float64
	Paper PaperSize

	SteelAreaMM2 float64 // 100x100 physical plate area

	Stats []EtchPlateStat
}

const (
	steelAreaMM2 = 100.0 * 100.0
	plateEdgeMM  = 2.0 * (100.0 + 100.0) // Perimeter of 100x100mm plate.

	// Bench defaults for the operator section on the stats page.
	etchGapMM          = 15.0
	etchTempC          = 34.0
	etchSulfateGPerL   = 100.0
	etchVoltageLimitV  = 12.0
	etchCurrentDensity = 0.04 // A/cm^2 default for "set current"
)

var etchThicknessMM = []float64{1.0, 1.5, 2.0, 3.0}

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
	exposed := whiteMM2
	return EtchPlateStat{
		PlateIndex:  plateIdx,
		Side:        side,
		BlackPixels: black,
		WhitePixels: white,
		BlackMM2:    blackMM2,
		WhiteMM2:    whiteMM2,
		ExposedMM2:  exposed,
		ExposedPct:  pct(exposed, steelAreaMM2),
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
	y += 4.0
	drawTrackedText(page, faceBody, dpi, x, y, "If you only etch one side, please mask the other side completely with tape.", bodyTrack)
	y += 4.0
	psuExplain := "The MASKED column in PSU GUIDE TABLE means you did mask the side walls of the plate. If you leave the walls unmasked refer to t (plate thickness)."
	noteLines := wrapTextTracked(faceBody, dpi, psuExplain, pageWmm-2*x, bodyTrack)
	for _, ln := range noteLines {
		drawTrackedText(page, faceBody, dpi, x, y, ln, bodyTrack)
		y += 4.0
	}
	drawTrackedText(page, faceBody, dpi, x, y, "If you etch both sides of the plate (SEED and DESC) together using 2 cathodes,", bodyTrack)
	y += 4.0
	drawTrackedText(page, faceBody, dpi, x, y, "sum both sides' A.", bodyTrack)
	y += 4.0

	y += 4.0
	drawTrackedText(page, faceBody, dpi, x, y, "AREA TABLE (t=plate thickness)", bodyTrack)
	y += 4.5
	const (
		plateColW = 8
		areaColW  = 11
		pctColW   = 9
		curColW   = 10
	)
	areaHeader := fmt.Sprintf("%-*s %*s %*s %*s %*s %*s %*s %*s",
		plateColW, "PLATE",
		areaColW, "TONER(cm2)",
		areaColW, "EXPOSED(cm2)",
		pctColW, "MASKED",
		pctColW, "t=1 +%",
		pctColW, "t=1.5 +%",
		pctColW, "t=2 +%",
		pctColW, "t=3 +%",
	)
	drawTrackedText(page, faceBody, dpi, x, y, areaHeader, bodyTrack)
	y += 4.0

	for _, s := range report.Stats {
		side := "SEED"
		if s.Side == "descriptor" {
			side = "DESC"
		}
		plateID := fmt.Sprintf("%02d %s", s.PlateIndex, side)
		line := fmt.Sprintf("%-*s %*.2f %*.2f %*s %*s %*s %*s %*s",
			plateColW, plateID,
			areaColW, s.BlackMM2/100.0,
			areaColW, s.ExposedMM2/100.0,
			pctColW, fmtPct(s.ExposedPct),
			pctColW, fmtPct(edgePctForThickness(etchThicknessMM[0])),
			pctColW, fmtPct(edgePctForThickness(etchThicknessMM[1])),
			pctColW, fmtPct(edgePctForThickness(etchThicknessMM[2])),
			pctColW, fmtPct(edgePctForThickness(etchThicknessMM[3])),
		)
		drawTrackedText(page, faceBody, dpi, x, y, line, bodyTrack)
		y += 4.0
		if y > pageHmm-20.0 {
			break
		}
	}

	y += 5.0
	drawTrackedText(page, faceBody, dpi, x, y, "PSU GUIDE TABLE (set A to)", bodyTrack)
	y += 4.5
	psuHeader := fmt.Sprintf("%-*s %*s %*s %*s %*s %*s",
		plateColW, "PLATE",
		curColW, "MASKED",
		curColW, "t=1mm",
		curColW, "t=1.5mm",
		curColW, "t=2mm",
		curColW, "t=3mm",
	)
	drawTrackedText(page, faceBody, dpi, x, y, psuHeader, bodyTrack)
	y += 4.0
	for _, s := range report.Stats {
		side := "SEED"
		if s.Side == "descriptor" {
			side = "DESC"
		}
		plateID := fmt.Sprintf("%02d %s", s.PlateIndex, side)
		line := fmt.Sprintf("%-*s %*s %*s %*s %*s %*s",
			plateColW, plateID,
			curColW, fmtCurrentA(setCurrentA(s.ExposedMM2, etchCurrentDensity)),
			curColW, fmtCurrentA(setCurrentA(exposedMM2ForThickness(s.ExposedMM2, etchThicknessMM[0]), etchCurrentDensity)),
			curColW, fmtCurrentA(setCurrentA(exposedMM2ForThickness(s.ExposedMM2, etchThicknessMM[1]), etchCurrentDensity)),
			curColW, fmtCurrentA(setCurrentA(exposedMM2ForThickness(s.ExposedMM2, etchThicknessMM[2]), etchCurrentDensity)),
			curColW, fmtCurrentA(setCurrentA(exposedMM2ForThickness(s.ExposedMM2, etchThicknessMM[3]), etchCurrentDensity)),
		)
		drawTrackedText(page, faceBody, dpi, x, y, line, bodyTrack)
		y += 4.0
		if y > pageHmm-8.0 {
			break
		}
	}

	return page, nil
}

func setCurrentA(exposedMM2, currentDensityAperCM2 float64) float64 {
	if exposedMM2 <= 0 || currentDensityAperCM2 <= 0 {
		return 0
	}
	exposedCM2 := exposedMM2 / 100.0
	return exposedCM2 * currentDensityAperCM2
}

func edgeAreaMM2ForThickness(thicknessMM float64) float64 {
	if thicknessMM <= 0 {
		return 0
	}
	return plateEdgeMM * thicknessMM
}

func edgePctForThickness(thicknessMM float64) float64 {
	return pct(edgeAreaMM2ForThickness(thicknessMM), steelAreaMM2)
}

func exposedMM2ForThickness(topExposedMM2, thicknessMM float64) float64 {
	return topExposedMM2 + edgeAreaMM2ForThickness(thicknessMM)
}

func exposedPctForThickness(topExposedMM2, thicknessMM float64) float64 {
	return pct(exposedMM2ForThickness(topExposedMM2, thicknessMM), steelAreaMM2)
}

func fmtPct(v float64) string {
	return fmt.Sprintf("%.1f%%", v)
}

func fmtCurrentA(v float64) string {
	return fmt.Sprintf("%.2fA", v)
}
