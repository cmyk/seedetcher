package printer

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type SceneGCodeRenderer struct {
	LaserOnCmd        string
	LaserOffCmd       string
	LaserMaxS         int
	CutFeedMMMin      float64
	RapidFeedMMMin    float64
	CurveSteps        int
	FillStepMM        float64
	FillInsetMM       float64
	OutlineInsetMM    float64
	SpiralStepMM      float64
	FeatureShrinkMM   float64
	OutlinePowerScale float64
	OutlineFeedScale  float64
	DualOutlinePass   bool
	BedMM             float64
	PlateMM           float64
	PlateOriginXMM    float64
	PlateOriginYMM    float64
	MachineOffsetXMM  float64
	MachineOffsetYMM  float64
	LaserFlipX        bool
	LaserFlipY        bool
}

func (r SceneGCodeRenderer) Render(doc *PlateDocument, outDir string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}
	cfg := r.withDefaults()
	for i, s := range doc.Scenes {
		name := s.Name
		if name == "" {
			name = fmt.Sprintf("scene_%02d", i+1)
		}
		path := filepath.Join(outDir, sanitizeSceneFilename(name)+".gcode")
		gcode, err := renderSceneGCode(s, cfg)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(gcode), 0644); err != nil {
			return err
		}
	}
	return nil
}

func (r SceneGCodeRenderer) withDefaults() SceneGCodeRenderer {
	if strings.TrimSpace(r.LaserOnCmd) == "" {
		r.LaserOnCmd = "M4"
	}
	if strings.TrimSpace(r.LaserOffCmd) == "" {
		r.LaserOffCmd = "M5"
	}
	if r.LaserMaxS <= 0 {
		r.LaserMaxS = 1000
	}
	if r.CutFeedMMMin <= 0 {
		r.CutFeedMMMin = 900
	}
	if r.RapidFeedMMMin <= 0 {
		r.RapidFeedMMMin = 3000
	}
	if r.CurveSteps <= 0 {
		r.CurveSteps = 10
	}
	if r.FillStepMM <= 0 {
		r.FillStepMM = 0.12
	}
	if r.FillInsetMM < 0 {
		r.FillInsetMM = 0
	}
	if r.OutlineInsetMM < 0 {
		r.OutlineInsetMM = 0
	}
	if r.SpiralStepMM <= 0 {
		r.SpiralStepMM = r.FillStepMM
	}
	if r.FeatureShrinkMM < 0 {
		r.FeatureShrinkMM = 0
	}
	if r.OutlinePowerScale <= 0 {
		r.OutlinePowerScale = 1
	}
	if r.OutlineFeedScale <= 0 {
		r.OutlineFeedScale = 1
	}
	if r.PlateMM <= 0 {
		r.PlateMM = 100
	}
	if r.BedMM <= 0 {
		r.BedMM = r.PlateMM
	}
	return r
}

type gcodePt struct {
	x float64
	y float64
}

type gcodeEmitter struct {
	b                strings.Builder
	cfg              SceneGCodeRenderer
	x                float64
	y                float64
	laserOn          bool
	activeFeed       float64
	activePowerS     int
	activeLaserOnCmd string
	bedMax           float64
	sceneW           float64
	sceneH           float64
	isCalibration    bool
	layoutOffsetX    float64
	layoutOffsetY    float64
	err              error
}

type primitiveLaserParams struct {
	fillMode      FillMode
	fillStepMM    float64
	cutFeed       float64
	powerS        int
	outlineFeed   float64
	outlinePowerS int
	laserOnCmd    string
}

func renderSceneGCode(scene PlateScene, cfg SceneGCodeRenderer) (string, error) {
	e := &gcodeEmitter{
		cfg:           cfg,
		bedMax:        cfg.BedMM,
		sceneW:        scene.WidthMM,
		sceneH:        scene.HeightMM,
		isCalibration: strings.HasPrefix(scene.Name, "laser_calibration_"),
	}
	if scene.WidthMM > cfg.PlateMM || scene.HeightMM > cfg.PlateMM {
		return "", fmt.Errorf("scene '%s' exceeds plate bounds: %.3fx%.3fmm > plate %.3fmm", scene.Name, scene.WidthMM, scene.HeightMM, cfg.PlateMM)
	}
	plateMinX := cfg.PlateOriginXMM + cfg.MachineOffsetXMM
	plateMinY := cfg.PlateOriginYMM + cfg.MachineOffsetYMM
	if plateMinX < 0 || plateMinY < 0 || plateMinX+cfg.PlateMM > cfg.BedMM || plateMinY+cfg.PlateMM > cfg.BedMM {
		return "", fmt.Errorf(
			"scene '%s' plate origin out of workspace: plate=%.3f work-origin=(%.3f,%.3f) machine-offset=(%.3f,%.3f) workspace=0..%.3f",
			scene.Name, cfg.PlateMM, cfg.PlateOriginXMM, cfg.PlateOriginYMM, cfg.MachineOffsetXMM, cfg.MachineOffsetYMM, cfg.BedMM,
		)
	}
	// Default wallet behavior centers the scene in the physical plate.
	// Calibration scenes can opt into plate-origin anchoring.
	if !strings.EqualFold(scene.AnchorInPlate, "origin") {
		if cfg.PlateMM > scene.WidthMM {
			e.layoutOffsetX = (cfg.PlateMM - scene.WidthMM) / 2
		}
		if cfg.PlateMM > scene.HeightMM {
			e.layoutOffsetY = (cfg.PlateMM - scene.HeightMM) / 2
		}
	}
	e.layoutOffsetX += scene.OffsetInPlateXMM
	e.layoutOffsetY += scene.OffsetInPlateYMM
	if e.layoutOffsetX < 0 || e.layoutOffsetY < 0 || e.layoutOffsetX+scene.WidthMM > cfg.PlateMM || e.layoutOffsetY+scene.HeightMM > cfg.PlateMM {
		return "", fmt.Errorf(
			"scene '%s' offset in plate out of bounds: scene=%.3fx%.3f offset=(%.3f,%.3f) plate=%.3f",
			scene.Name, scene.WidthMM, scene.HeightMM, e.layoutOffsetX, e.layoutOffsetY, cfg.PlateMM,
		)
	}
	fmt.Fprintf(&e.b, "; SeedEtcher scene: %s\n", scene.Name)
	fmt.Fprintf(&e.b, "; Size: %.3fmm x %.3fmm\n", scene.WidthMM, scene.HeightMM)
	fmt.Fprintf(&e.b, "; Bed: %.3fmm\n", cfg.BedMM)
	fmt.Fprintf(&e.b, "; Plate: %.3fmm at work origin X=%.3fmm Y=%.3fmm\n", cfg.PlateMM, cfg.PlateOriginXMM, cfg.PlateOriginYMM)
	if cfg.MachineOffsetXMM != 0 || cfg.MachineOffsetYMM != 0 {
		fmt.Fprintf(&e.b, "; Machine offset: X=%.3fmm Y=%.3fmm\n", cfg.MachineOffsetXMM, cfg.MachineOffsetYMM)
	}
	fmt.Fprintf(&e.b, "; Layout offset in plate: X=%.3fmm Y=%.3fmm\n", e.layoutOffsetX, e.layoutOffsetY)
	e.b.WriteString("G21\n")
	e.b.WriteString("G90\n")
	fmt.Fprintf(&e.b, "G0 F%.1f\n", cfg.RapidFeedMMMin)
	fmt.Fprintf(&e.b, "G1 F%.1f\n", cfg.CutFeedMMMin)
	if strings.HasPrefix(scene.Name, "laser_calibration_") {
		e.emitPreviewFrame()
		if e.err != nil {
			return "", e.err
		}
	}
	for _, layer := range scene.Layers {
		if !layer.Visible {
			continue
		}
		fmt.Fprintf(&e.b, "; Layer: %s\n", layer.Tag)
		e.renderLayer(layer)
		if e.err != nil {
			return "", e.err
		}
	}
	e.laserOffSafe()
	if e.err != nil {
		return "", e.err
	}
	e.b.WriteString("M2\n")
	return e.b.String(), nil
}

func (e *gcodeEmitter) emitPreviewFrame() {
	e.laserOffSafe()
	e.b.WriteString("; Preview frame (laser off)\n")
	frame := []gcodePt{
		{x: 0, y: 0},
		{x: e.sceneW, y: 0},
		{x: e.sceneW, y: e.sceneH},
		{x: 0, y: e.sceneH},
		{x: 0, y: 0},
	}
	for _, pt := range frame {
		e.rapidTo(pt.x, pt.y)
		if e.err != nil {
			return
		}
	}
}

func (e *gcodeEmitter) renderLayer(layer SceneLayer) {
	batches, batched := e.planHorizontalTextHatchBatches(layer.Primitives)
	for i, p := range layer.Primitives {
		if idxs, ok := batches[i]; ok {
			e.renderHorizontalTextHatchBatch(layer.Primitives, idxs)
			if e.err != nil {
				return
			}
			continue
		}
		if batched[i] {
			continue
		}
		e.renderPrimitive(p)
		if e.err != nil {
			return
		}
	}
}

func (e *gcodeEmitter) planHorizontalTextHatchBatches(primitives []ScenePrimitive) (map[int][]int, []bool) {
	keyToIdxs := make(map[string][]int)
	for i, p := range primitives {
		if p.Kind != PrimitiveText {
			continue
		}
		params := e.primitiveLaserParams(p)
		if params.fillMode != FillModeHatch || p.Direction != TextDirHorizontal {
			continue
		}
		// Group by baseline Y and burn profile so each mnemonic row is filled as one serpentine unit.
		key := fmt.Sprintf(
			"y=%d|a=%s|f=%d|p=%d|lf=%s|of=%d|op=%d|fs=%d",
			quantizedMM(p.YMM, 0.001),
			p.Anchor,
			quantizedMM(params.cutFeed, 0.1),
			params.powerS,
			strings.ToUpper(strings.TrimSpace(params.laserOnCmd)),
			quantizedMM(params.outlineFeed, 0.1),
			params.outlinePowerS,
			quantizedMM(params.fillStepMM, 0.001),
		)
		keyToIdxs[key] = append(keyToIdxs[key], i)
	}
	leaders := make(map[int][]int)
	batched := make([]bool, len(primitives))
	for _, idxs := range keyToIdxs {
		if len(idxs) < 2 {
			continue
		}
		leader := idxs[0]
		leaders[leader] = idxs
		for _, idx := range idxs {
			batched[idx] = true
		}
	}
	return leaders, batched
}

func (e *gcodeEmitter) renderHorizontalTextHatchBatch(primitives []ScenePrimitive, idxs []int) {
	if e.err != nil || len(idxs) == 0 {
		return
	}
	first := primitives[idxs[0]]
	params := e.primitiveLaserParams(first)
	fillLoops := make([][]gcodePt, 0, len(idxs))
	type outlineItem struct {
		original [][]gcodePt
		outline  [][]gcodePt
	}
	outlines := make([]outlineItem, 0, len(idxs))
	for _, idx := range idxs {
		p := primitives[idx]
		orig, outline, fill, ok := e.textLoopsForRender(p, params.fillMode)
		if !ok {
			continue
		}
		fillLoops = append(fillLoops, fill...)
		outlines = append(outlines, outlineItem{original: orig, outline: outline})
	}
	for _, seg := range hatchSegments(fillLoops, params.fillStepMM) {
		e.tracePolyline(seg, params.cutFeed, params.powerS, params.laserOnCmd)
		if e.err != nil {
			return
		}
	}
	for _, it := range outlines {
		for _, poly := range it.outline {
			e.tracePolyline(poly, params.outlineFeed, params.outlinePowerS, params.laserOnCmd)
			if e.err != nil {
				return
			}
		}
		if e.cfg.DualOutlinePass && e.cfg.OutlineInsetMM > 0 && !loopsApproxEqual(it.original, it.outline) {
			for _, poly := range it.original {
				e.tracePolyline(poly, params.outlineFeed, params.outlinePowerS, params.laserOnCmd)
				if e.err != nil {
					return
				}
			}
		}
	}
}

func quantizedMM(v, step float64) int64 {
	if step <= 0 {
		step = 0.001
	}
	return int64(math.Round(v / step))
}

func (e *gcodeEmitter) renderPrimitive(p ScenePrimitive) {
	if e.err != nil {
		return
	}
	if e.skipGuidePrimitive(p) {
		return
	}
	params := e.primitiveLaserParams(p)
	fillMode := params.fillMode
	fillStepMM := params.fillStepMM
	cutFeed := params.cutFeed
	powerS := params.powerS
	outlineFeed := params.outlineFeed
	outlinePowerS := params.outlinePowerS
	laserOnCmd := params.laserOnCmd
	switch p.Kind {
	case PrimitiveGroup:
		for _, c := range p.Children {
			e.renderPrimitive(c)
		}
	case PrimitiveRect:
		loop := []gcodePt{
			{x: p.XMM, y: p.YMM},
			{x: p.XMM + p.WidthMM, y: p.YMM},
			{x: p.XMM + p.WidthMM, y: p.YMM + p.HeightMM},
			{x: p.XMM, y: p.YMM + p.HeightMM},
			{x: p.XMM, y: p.YMM},
		}
		e.fillOrTrace(fillMode, [][]gcodePt{loop}, nil, e.cfg.FillInsetMM, fillStepMM, cutFeed, powerS, outlineFeed, outlinePowerS, laserOnCmd, !p.NoOutline)
	case PrimitiveRound:
		e.fillOrTrace(fillMode, [][]gcodePt{roundRectPolyline(p.XMM, p.YMM, p.WidthMM, p.HeightMM, p.RadiusMM, 6)}, nil, e.cfg.FillInsetMM, fillStepMM, cutFeed, powerS, outlineFeed, outlinePowerS, laserOnCmd, !p.NoOutline)
	case PrimitiveCircle:
		if fillMode == FillModeSpiral {
			spiralStep := e.cfg.SpiralStepMM
			if p.FillStepMM > 0 {
				spiralStep = p.FillStepMM
			}
			e.tracePolyline(circleSpiralPolyline(p.CXMM, p.CYMM, p.RadiusMM, spiralStep), cutFeed, powerS, laserOnCmd)
			if !p.NoOutline {
				baseOutlines := [][]gcodePt{circlePolyline(p.CXMM, p.CYMM, p.RadiusMM, 40)}
				outlineLoops := baseOutlines
				if e.cfg.OutlineInsetMM > 0 {
					if in := shrinkFeatureLoops(baseOutlines, e.cfg.OutlineInsetMM); len(in) > 0 {
						outlineLoops = in
					}
				}
				for _, poly := range outlineLoops {
					e.tracePolyline(poly, outlineFeed, outlinePowerS, laserOnCmd)
				}
				if e.cfg.DualOutlinePass && e.cfg.OutlineInsetMM > 0 && !loopsApproxEqual(outlineLoops, baseOutlines) {
					for _, poly := range baseOutlines {
						e.tracePolyline(poly, outlineFeed, outlinePowerS, laserOnCmd)
					}
				}
			}
		} else {
			e.fillOrTrace(fillMode, [][]gcodePt{circlePolyline(p.CXMM, p.CYMM, p.RadiusMM, 40)}, nil, e.cfg.FillInsetMM, fillStepMM, cutFeed, powerS, outlineFeed, outlinePowerS, laserOnCmd, !p.NoOutline)
		}
	case PrimitiveRing:
		outer, inner := ringOutlines(p.XMM, p.YMM, p.WidthMM, p.HeightMM, p.ThicknessMM, p.RadiusMM)
		if fillMode == FillModeOffset {
			for _, loop := range ringOffsetLoops(p.XMM, p.YMM, p.WidthMM, p.HeightMM, p.ThicknessMM, p.RadiusMM, fillStepMM) {
				e.tracePolyline(loop, cutFeed, powerS, laserOnCmd)
			}
			if !p.NoOutline {
				baseOutlines := [][]gcodePt{outer, inner}
				outlineLoops := baseOutlines
				if e.cfg.OutlineInsetMM > 0 {
					if in := shrinkFeatureLoops(baseOutlines, e.cfg.OutlineInsetMM); len(in) > 0 {
						outlineLoops = in
					}
				}
				for _, poly := range outlineLoops {
					e.tracePolyline(poly, outlineFeed, outlinePowerS, laserOnCmd)
				}
				if e.cfg.DualOutlinePass && e.cfg.OutlineInsetMM > 0 && !loopsApproxEqual(outlineLoops, baseOutlines) {
					for _, poly := range baseOutlines {
						e.tracePolyline(poly, outlineFeed, outlinePowerS, laserOnCmd)
					}
				}
			}
		} else {
			e.fillOrTrace(fillMode, [][]gcodePt{outer, inner}, nil, e.cfg.FillInsetMM, fillStepMM, cutFeed, powerS, outlineFeed, outlinePowerS, laserOnCmd, !p.NoOutline)
		}
	case PrimitivePath:
		loops := parseGCodePath(p.PathData, e.cfg.CurveSteps)
		if e.shouldApplyFeatureShrink(p, fillMode) {
			loops = shrinkFeatureLoops(loops, e.cfg.FeatureShrinkMM)
		}
		outlineLoops := loops
		if fillMode != FillModeNone && e.cfg.OutlineInsetMM > 0 {
			if in := shrinkFeatureLoops(loops, e.cfg.OutlineInsetMM); len(in) > 0 {
				outlineLoops = in
			}
		}
		fillInsetMM := e.cfg.FillInsetMM
		if fillMode != FillModeNone && e.cfg.OutlineInsetMM > fillInsetMM {
			fillInsetMM = e.cfg.OutlineInsetMM
		}
		if fillMode == FillModeOffset && strings.EqualFold(p.FillRule, "evenodd") {
			e.fillOrTrace(FillModeHatch, loops, outlineLoops, fillInsetMM, fillStepMM, cutFeed, powerS, outlineFeed, outlinePowerS, laserOnCmd, !p.NoOutline)
		} else {
			e.fillOrTrace(fillMode, loops, outlineLoops, fillInsetMM, fillStepMM, cutFeed, powerS, outlineFeed, outlinePowerS, laserOnCmd, !p.NoOutline)
		}
	case PrimitiveText:
		loops, outlineLoops, _, ok := e.textLoopsForRender(p, fillMode)
		if !ok {
			return
		}
		fillInsetMM := e.cfg.FillInsetMM
		if fillMode != FillModeNone && e.cfg.OutlineInsetMM > fillInsetMM {
			fillInsetMM = e.cfg.OutlineInsetMM
		}
		e.fillOrTrace(fillMode, loops, outlineLoops, fillInsetMM, fillStepMM, cutFeed, powerS, outlineFeed, outlinePowerS, laserOnCmd, !p.NoOutline)
	}
}

func (e *gcodeEmitter) primitiveLaserParams(p ScenePrimitive) primitiveLaserParams {
	fillMode := effectiveFillMode(p)
	fillStepMM := effectiveFillStepMM(e.cfg, p)
	cutFeed := effectiveCutFeed(e.cfg, p)
	powerS := effectivePowerS(e.cfg, p)
	outlineFeed := cutFeed * e.cfg.OutlineFeedScale
	outlinePowerS := int(math.Round(float64(powerS) * e.cfg.OutlinePowerScale))
	if outlineFeed <= 0 {
		outlineFeed = cutFeed
	}
	if outlinePowerS <= 0 {
		outlinePowerS = powerS
	}
	return primitiveLaserParams{
		fillMode:      fillMode,
		fillStepMM:    fillStepMM,
		cutFeed:       cutFeed,
		powerS:        powerS,
		outlineFeed:   outlineFeed,
		outlinePowerS: outlinePowerS,
		laserOnCmd:    effectiveLaserOnCmd(e.cfg, p),
	}
}

func (e *gcodeEmitter) textLoopsForRender(p ScenePrimitive, fillMode FillMode) (originalLoops [][]gcodePt, outlineLoops [][]gcodePt, fillLoops [][]gcodePt, ok bool) {
	pathData, ok := svgTextPath(p)
	if !ok {
		return nil, nil, nil, false
	}
	loops := parseGCodePath(pathData, e.cfg.CurveSteps)
	if e.shouldApplyFeatureShrink(p, fillMode) {
		loops = shrinkFeatureLoops(loops, e.cfg.FeatureShrinkMM)
	}
	if len(loops) == 0 {
		return nil, nil, nil, false
	}
	outlineLoops = loops
	if fillMode != FillModeNone && e.cfg.OutlineInsetMM > 0 {
		if in := shrinkFeatureLoops(loops, e.cfg.OutlineInsetMM); len(in) > 0 {
			outlineLoops = in
		}
	}
	fillLoops = loops
	fillInsetMM := e.cfg.FillInsetMM
	if fillMode != FillModeNone && e.cfg.OutlineInsetMM > fillInsetMM {
		fillInsetMM = e.cfg.OutlineInsetMM
	}
	if fillInsetMM > 0 && fillMode != FillModeNone {
		if shrunk := shrinkFeatureLoops(loops, fillInsetMM); len(shrunk) > 0 {
			fillLoops = shrunk
		}
	}
	return loops, outlineLoops, fillLoops, true
}

func ringOutlines(x, y, w, h, t, r float64) ([]gcodePt, []gcodePt) {
	if t < 0 {
		t = 0
	}
	if 2*t > w {
		t = w / 2
	}
	if 2*t > h {
		t = h / 2
	}
	outer := roundRectPolyline(x, y, w, h, r, 8)
	inner := roundRectPolyline(x+t, y+t, w-2*t, h-2*t, r, 8)
	return outer, inner
}

func ringOffsetLoops(x, y, w, h, t, r, step float64) [][]gcodePt {
	if step <= 0 {
		step = 0.12
	}
	if t <= 0 || w <= 0 || h <= 0 {
		return nil
	}
	maxInset := t
	loops := make([][]gcodePt, 0, int(maxInset/step)+1)
	for d := step; d < maxInset; d += step {
		ow := w - 2*d
		oh := h - 2*d
		if ow <= 0 || oh <= 0 {
			break
		}
		loops = append(loops, roundRectPolyline(x+d, y+d, ow, oh, r, 8))
	}
	return loops
}

func (e *gcodeEmitter) skipGuidePrimitive(p ScenePrimitive) bool {
	if p.Kind != PrimitiveRect {
		return false
	}
	// Builders add a full-plate stroke rectangle as visual border guide.
	// Keep it in scene/SVG, but do not burn it by default in laser G-code.
	if p.FillColor != "" && p.FillColor != "none" {
		return false
	}
	if p.StrokeColor == "" || p.StrokeColor == "none" || p.StrokeMM <= 0 {
		return false
	}
	return almostEq(p.XMM, 0) &&
		almostEq(p.YMM, 0) &&
		almostEq(p.WidthMM, e.sceneW) &&
		almostEq(p.HeightMM, e.sceneH)
}

func almostEq(a, b float64) bool {
	return math.Abs(a-b) <= 1e-6
}

func (e *gcodeEmitter) fillOrTrace(fillMode FillMode, loops [][]gcodePt, outlineLoops [][]gcodePt, fillInsetMM float64, fillStepMM float64, cutFeed float64, powerS int, outlineFeed float64, outlinePowerS int, laserOnCmd string, emitOutline bool) {
	hadExplicitOutline := len(outlineLoops) > 0
	fillLoops := loops
	if len(outlineLoops) == 0 {
		outlineLoops = loops
	}
	if fillMode != FillModeNone && e.cfg.OutlineInsetMM > 0 && !hadExplicitOutline {
		if in := shrinkFeatureLoops(outlineLoops, e.cfg.OutlineInsetMM); len(in) > 0 {
			outlineLoops = in
		}
	}
	if fillInsetMM > 0 && fillMode != FillModeNone {
		if shrunk := shrinkFeatureLoops(loops, fillInsetMM); len(shrunk) > 0 {
			fillLoops = shrunk
		}
	}
	if fillMode == FillModeHatch {
		for _, seg := range hatchSegments(fillLoops, fillStepMM) {
			e.tracePolyline(seg, cutFeed, powerS, laserOnCmd)
		}
	}
	if fillMode == FillModeOffset {
		for _, poly := range fillLoops {
			for _, loop := range offsetInwardLoops(poly, fillStepMM) {
				e.tracePolyline(loop, cutFeed, powerS, laserOnCmd)
			}
		}
	}
	if emitOutline {
		for _, poly := range outlineLoops {
			e.tracePolyline(poly, outlineFeed, outlinePowerS, laserOnCmd)
		}
		if e.cfg.DualOutlinePass && fillMode != FillModeNone && e.cfg.OutlineInsetMM > 0 && !loopsApproxEqual(outlineLoops, loops) {
			for _, poly := range loops {
				e.tracePolyline(poly, outlineFeed, outlinePowerS, laserOnCmd)
			}
		}
	}
}

func (e *gcodeEmitter) shouldApplyFeatureShrink(p ScenePrimitive, fillMode FillMode) bool {
	if e.cfg.FeatureShrinkMM <= 0 {
		return false
	}
	if p.Kind != PrimitiveText && p.Kind != PrimitivePath {
		return false
	}
	// Keep calibration labels/readouts exact and legible even when global feature shrink is set.
	if e.isCalibration && p.Kind == PrimitiveText && fillMode == FillModeNone {
		return false
	}
	return true
}

func (e *gcodeEmitter) tracePolyline(poly []gcodePt, cutFeed float64, powerS int, laserOnCmd string) {
	if e.err != nil {
		return
	}
	if len(poly) < 2 {
		return
	}
	e.laserOffSafe()
	e.rapidTo(poly[0].x, poly[0].y)
	e.laserOnStart(cutFeed, powerS, laserOnCmd)
	for i := 1; i < len(poly); i++ {
		e.cutTo(poly[i].x, poly[i].y)
	}
	e.laserOffSafe()
}

func (e *gcodeEmitter) rapidTo(x, y float64) {
	x += e.layoutOffsetX
	y += e.layoutOffsetY
	x, y = e.mapXY(x, y)
	if !e.validXY(x, y) {
		return
	}
	fmt.Fprintf(&e.b, "G0 X%.3f Y%.3f\n", x, y)
	e.x, e.y = x, y
}

func (e *gcodeEmitter) cutTo(x, y float64) {
	x += e.layoutOffsetX
	y += e.layoutOffsetY
	x, y = e.mapXY(x, y)
	if !e.validXY(x, y) {
		return
	}
	fmt.Fprintf(&e.b, "G1 X%.3f Y%.3f\n", x, y)
	e.x, e.y = x, y
}

func (e *gcodeEmitter) laserOnStart(feed float64, powerS int, laserOnCmd string) {
	if strings.TrimSpace(laserOnCmd) == "" {
		laserOnCmd = e.cfg.LaserOnCmd
	}
	if e.laserOn && almostEq(e.activeFeed, feed) && e.activePowerS == powerS && e.activeLaserOnCmd == laserOnCmd {
		return
	}
	if e.laserOn {
		e.laserOffSafe()
	}
	fmt.Fprintf(&e.b, "G1 F%.1f\n", feed)
	fmt.Fprintf(&e.b, "%s S%d\n", laserOnCmd, powerS)
	e.laserOn = true
	e.activeFeed = feed
	e.activePowerS = powerS
	e.activeLaserOnCmd = laserOnCmd
}

func (e *gcodeEmitter) laserOffSafe() {
	if e.err != nil {
		return
	}
	if !e.laserOn {
		return
	}
	e.b.WriteString(e.cfg.LaserOffCmd)
	e.b.WriteByte('\n')
	e.laserOn = false
	e.activeLaserOnCmd = ""
}

func (e *gcodeEmitter) validXY(x, y float64) bool {
	if e.err != nil {
		return false
	}
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
		e.err = fmt.Errorf("invalid coordinate (NaN/Inf): x=%v y=%v", x, y)
		return false
	}
	if x < 0 || y < 0 || x > e.bedMax || y > e.bedMax {
		e.err = fmt.Errorf("gcode coordinate out of bounds: x=%.3f y=%.3f workspace=0..%.3f", x, y, e.bedMax)
		return false
	}
	return true
}

func (e *gcodeEmitter) mapXY(x, y float64) (float64, float64) {
	// Scene space uses graphic-design coordinates: top-left origin, +Y downward.
	// Machine space is Cartesian: bottom-left origin, +Y upward.
	ax := e.cfg.PlateOriginXMM + e.cfg.MachineOffsetXMM + x
	ay := e.cfg.PlateOriginYMM + e.cfg.MachineOffsetYMM + (e.layoutOffsetY + e.sceneH - (y - e.layoutOffsetY))
	if e.cfg.LaserFlipX {
		ax = e.cfg.PlateOriginXMM + e.cfg.MachineOffsetXMM + e.cfg.PlateMM - x
	}
	if e.cfg.LaserFlipY {
		ay = e.cfg.PlateOriginYMM + e.cfg.MachineOffsetYMM + y
	}
	return ax, ay
}

func circlePolyline(cx, cy, r float64, segments int) []gcodePt {
	if r <= 0 {
		return nil
	}
	if segments < 8 {
		segments = 8
	}
	out := make([]gcodePt, 0, segments+1)
	for i := 0; i <= segments; i++ {
		t := 2 * math.Pi * float64(i) / float64(segments)
		out = append(out, gcodePt{
			x: cx + r*math.Cos(t),
			y: cy + r*math.Sin(t),
		})
	}
	return out
}

func circleSpiralPolyline(cx, cy, r, pitch float64) []gcodePt {
	if r <= 0 {
		return nil
	}
	if pitch <= 0 {
		pitch = 0.12
	}
	turns := r / pitch
	if turns < 1 {
		turns = 1
	}
	maxTheta := 2 * math.Pi * turns
	stepTheta := 0.25
	points := int(math.Ceil(maxTheta/stepTheta)) + 1
	if points < 24 {
		points = 24
	}
	out := make([]gcodePt, 0, points+1)
	for i := 0; i <= points; i++ {
		t := maxTheta * float64(i) / float64(points)
		cr := r - pitch*t/(2*math.Pi)
		if cr < 0 {
			cr = 0
		}
		out = append(out, gcodePt{
			x: cx + cr*math.Cos(t),
			y: cy + cr*math.Sin(t),
		})
	}
	return out
}

func roundRectPolyline(x, y, w, h, r float64, arcSegments int) []gcodePt {
	if w <= 0 || h <= 0 {
		return nil
	}
	if r <= 0 {
		return []gcodePt{
			{x: x, y: y},
			{x: x + w, y: y},
			{x: x + w, y: y + h},
			{x: x, y: y + h},
			{x: x, y: y},
		}
	}
	maxR := math.Min(w, h) / 2
	if r > maxR {
		r = maxR
	}
	if arcSegments < 2 {
		arcSegments = 2
	}
	out := make([]gcodePt, 0, 8*arcSegments+5)
	out = append(out, gcodePt{x: x + r, y: y})
	out = append(out, gcodePt{x: x + w - r, y: y})
	out = append(out, arcPoints(x+w-r, y+r, r, -math.Pi/2, 0, arcSegments)...)
	out = append(out, gcodePt{x: x + w, y: y + h - r})
	out = append(out, arcPoints(x+w-r, y+h-r, r, 0, math.Pi/2, arcSegments)...)
	out = append(out, gcodePt{x: x + r, y: y + h})
	out = append(out, arcPoints(x+r, y+h-r, r, math.Pi/2, math.Pi, arcSegments)...)
	out = append(out, gcodePt{x: x, y: y + r})
	out = append(out, arcPoints(x+r, y+r, r, math.Pi, 3*math.Pi/2, arcSegments)...)
	out = append(out, gcodePt{x: x + r, y: y})
	return out
}

func arcPoints(cx, cy, r, a0, a1 float64, segments int) []gcodePt {
	out := make([]gcodePt, 0, segments)
	for i := 1; i <= segments; i++ {
		t := a0 + (a1-a0)*float64(i)/float64(segments)
		out = append(out, gcodePt{
			x: cx + r*math.Cos(t),
			y: cy + r*math.Sin(t),
		})
	}
	return out
}

func effectiveFillMode(p ScenePrimitive) FillMode {
	if p.FillMode != "" {
		return p.FillMode
	}
	if p.FillColor == "" || p.FillColor == "none" {
		return FillModeNone
	}
	if p.Kind == PrimitiveCircle {
		return FillModeSpiral
	}
	return FillModeHatch
}

func effectiveFillStepMM(cfg SceneGCodeRenderer, p ScenePrimitive) float64 {
	if p.FillStepMM > 0 {
		return p.FillStepMM
	}
	if cfg.FillStepMM > 0 {
		return cfg.FillStepMM
	}
	return 0.12
}

func effectiveCutFeed(cfg SceneGCodeRenderer, p ScenePrimitive) float64 {
	if p.FeedMMMin > 0 {
		return p.FeedMMMin
	}
	return cfg.CutFeedMMMin
}

func effectivePowerS(cfg SceneGCodeRenderer, p ScenePrimitive) int {
	if p.PowerS > 0 {
		return p.PowerS
	}
	return cfg.LaserMaxS
}

func effectiveLaserOnCmd(cfg SceneGCodeRenderer, p ScenePrimitive) string {
	if strings.TrimSpace(p.LaserOnCmd) != "" {
		return p.LaserOnCmd
	}
	return cfg.LaserOnCmd
}

func offsetInwardLoops(loop []gcodePt, step float64) [][]gcodePt {
	if step <= 0 {
		step = 0.12
	}
	closed := normalizeClosedLoops([][]gcodePt{loop})
	if len(closed) == 0 {
		return nil
	}
	base := closed[0]
	if len(base) < 4 {
		return nil
	}
	var loops [][]gcodePt
	for d := step; d <= maxInsetDistance(base); d += step {
		in := insetLoop(base, d)
		if len(in) < 4 || polygonArea(in) <= 1e-6 {
			break
		}
		loops = append(loops, in)
	}
	return loops
}

func maxInsetDistance(loop []gcodePt) float64 {
	minX, minY := loop[0].x, loop[0].y
	maxX, maxY := minX, minY
	for _, p := range loop {
		if p.x < minX {
			minX = p.x
		}
		if p.x > maxX {
			maxX = p.x
		}
		if p.y < minY {
			minY = p.y
		}
		if p.y > maxY {
			maxY = p.y
		}
	}
	w := maxX - minX
	h := maxY - minY
	if w <= 0 || h <= 0 {
		return 0
	}
	return math.Min(w, h) / 2
}

func insetLoop(loop []gcodePt, d float64) []gcodePt {
	n := len(loop) - 1 // normalized closed loop has repeated endpoint
	if n < 3 {
		return nil
	}
	ccw := polygonArea(loop) > 0
	out := make([]gcodePt, 0, n+1)
	miterLimit := math.Abs(d) * 4.0
	for i := 0; i < n; i++ {
		a := loop[(i-1+n)%n]
		b := loop[i]
		c := loop[(i+1)%n]

		p1, p2, ok1 := offsetEdge(a, b, d, ccw)
		q1, q2, ok2 := offsetEdge(b, c, d, ccw)
		if !ok1 || !ok2 {
			return nil
		}
		x, y, ok := lineIntersection(p1, p2, q1, q2)
		if !ok {
			x = (p2.x + q1.x) * 0.5
			y = (p2.y + q1.y) * 0.5
		}
		if miterLimit > 0 {
			if math.Hypot(x-b.x, y-b.y) > miterLimit {
				// Clamp acute-corner spikes ("wings") to a bevel midpoint.
				x = (p2.x + q1.x) * 0.5
				y = (p2.y + q1.y) * 0.5
			}
		}
		out = append(out, gcodePt{x: x, y: y})
	}
	if len(out) < 3 {
		return nil
	}
	out = append(out, out[0])
	return out
}

func offsetEdge(a, b gcodePt, d float64, ccw bool) (gcodePt, gcodePt, bool) {
	dx := b.x - a.x
	dy := b.y - a.y
	l := math.Hypot(dx, dy)
	if l <= 1e-9 {
		return gcodePt{}, gcodePt{}, false
	}
	ux := dx / l
	uy := dy / l
	nx, ny := -uy, ux
	// For CCW polygons, interior is left side of edges.
	if !ccw {
		nx, ny = -nx, -ny
	}
	return gcodePt{x: a.x + nx*d, y: a.y + ny*d}, gcodePt{x: b.x + nx*d, y: b.y + ny*d}, true
}

func lineIntersection(a1, a2, b1, b2 gcodePt) (float64, float64, bool) {
	ax := a2.x - a1.x
	ay := a2.y - a1.y
	bx := b2.x - b1.x
	by := b2.y - b1.y
	den := ax*by - ay*bx
	if math.Abs(den) <= 1e-9 {
		return 0, 0, false
	}
	cx := b1.x - a1.x
	cy := b1.y - a1.y
	t := (cx*by - cy*bx) / den
	return a1.x + t*ax, a1.y + t*ay, true
}

func polygonArea(loop []gcodePt) float64 {
	if len(loop) < 4 {
		return 0
	}
	a := 0.0
	n := len(loop) - 1
	for i := 0; i < n; i++ {
		p := loop[i]
		q := loop[(i+1)%n]
		a += p.x*q.y - q.x*p.y
	}
	return a / 2
}

func hatchSegments(loops [][]gcodePt, step float64) [][]gcodePt {
	if step <= 0 {
		step = 0.12
	}
	loops = normalizeClosedLoops(loops)
	if len(loops) == 0 {
		return nil
	}
	minX, maxX := loops[0][0].x, loops[0][0].x
	minY, maxY := loops[0][0].y, loops[0][0].y
	for _, loop := range loops {
		for _, p := range loop {
			if p.x < minX {
				minX = p.x
			}
			if p.x > maxX {
				maxX = p.x
			}
			if p.y < minY {
				minY = p.y
			}
			if p.y > maxY {
				maxY = p.y
			}
		}
	}

	// Auto-orient hatch: wide shapes scan by rows, tall shapes scan by columns.
	// Use a tolerance so near-square features don't flip orientation due to float jitter.
	if (maxY - minY) > (maxX-minX)+1e-6 {
		return hatchSegmentsVertical(loops, minX, maxX, step)
	}

	return hatchSegmentsHorizontal(loops, minY, maxY, step)
}

func hatchSegmentsHorizontal(loops [][]gcodePt, minY, maxY, step float64) [][]gcodePt {
	const eps = 1e-9
	out := make([][]gcodePt, 0, int((maxY-minY)/step)+1)
	line := 0
	for y := minY + eps; y <= maxY-eps; y += step {
		xs := scanlineIntersections(loops, y)
		if len(xs) < 2 {
			line++
			continue
		}
		if line%2 == 0 {
			for i := 0; i+1 < len(xs); i += 2 {
				x0, x1 := xs[i], xs[i+1]
				if x1-x0 <= eps {
					continue
				}
				out = append(out, []gcodePt{{x: x0, y: y}, {x: x1, y: y}})
			}
		} else {
			for i := len(xs) - 2; i >= 0; i -= 2 {
				x0, x1 := xs[i], xs[i+1]
				if x1-x0 <= eps {
					continue
				}
				out = append(out, []gcodePt{{x: x1, y: y}, {x: x0, y: y}})
			}
		}
		line++
	}
	return out
}

func hatchSegmentsVertical(loops [][]gcodePt, minX, maxX, step float64) [][]gcodePt {
	const eps = 1e-9
	out := make([][]gcodePt, 0, int((maxX-minX)/step)+1)
	col := 0
	for x := minX + eps; x <= maxX-eps; x += step {
		ys := scanlineIntersectionsVertical(loops, x)
		if len(ys) < 2 {
			col++
			continue
		}
		if col%2 == 0 {
			for i := 0; i+1 < len(ys); i += 2 {
				y0, y1 := ys[i], ys[i+1]
				if y1-y0 <= eps {
					continue
				}
				out = append(out, []gcodePt{{x: x, y: y0}, {x: x, y: y1}})
			}
		} else {
			for i := len(ys) - 2; i >= 0; i -= 2 {
				y0, y1 := ys[i], ys[i+1]
				if y1-y0 <= eps {
					continue
				}
				out = append(out, []gcodePt{{x: x, y: y1}, {x: x, y: y0}})
			}
		}
		col++
	}
	return out
}

func normalizeClosedLoops(loops [][]gcodePt) [][]gcodePt {
	out := make([][]gcodePt, 0, len(loops))
	for _, loop := range loops {
		if len(loop) < 3 {
			continue
		}
		if !pointsEqual(loop[0], loop[len(loop)-1]) {
			cp := make([]gcodePt, 0, len(loop)+1)
			cp = append(cp, loop...)
			cp = append(cp, loop[0])
			out = append(out, cp)
			continue
		}
		out = append(out, loop)
	}
	return out
}

func pointsEqual(a, b gcodePt) bool {
	const eps = 1e-9
	return math.Abs(a.x-b.x) <= eps && math.Abs(a.y-b.y) <= eps
}

func loopsApproxEqual(a, b [][]gcodePt) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if !pointsEqual(a[i][j], b[i][j]) {
				return false
			}
		}
	}
	return true
}

func scanlineIntersections(loops [][]gcodePt, y float64) []float64 {
	const eps = 1e-9
	xs := make([]float64, 0, 32)
	for _, loop := range loops {
		for i := 0; i+1 < len(loop); i++ {
			a, b := loop[i], loop[i+1]
			if math.Abs(a.y-b.y) <= eps {
				continue
			}
			yMin, yMax := a.y, b.y
			xAtYMin, xAtYMax := a.x, b.x
			if yMin > yMax {
				yMin, yMax = yMax, yMin
				xAtYMin, xAtYMax = xAtYMax, xAtYMin
			}
			if y < yMin || y >= yMax {
				continue
			}
			t := (y - yMin) / (yMax - yMin)
			xs = append(xs, xAtYMin+t*(xAtYMax-xAtYMin))
		}
	}
	if len(xs) < 2 {
		return nil
	}
	sortFloat64s(xs)
	filtered := xs[:0]
	for _, x := range xs {
		if len(filtered) == 0 || math.Abs(x-filtered[len(filtered)-1]) > eps {
			filtered = append(filtered, x)
		}
	}
	return filtered
}

func scanlineIntersectionsVertical(loops [][]gcodePt, x float64) []float64 {
	const eps = 1e-9
	ys := make([]float64, 0, 32)
	for _, loop := range loops {
		for i := 0; i+1 < len(loop); i++ {
			a, b := loop[i], loop[i+1]
			if math.Abs(a.x-b.x) <= eps {
				continue
			}
			xMin, xMax := a.x, b.x
			yAtXMin, yAtXMax := a.y, b.y
			if xMin > xMax {
				xMin, xMax = xMax, xMin
				yAtXMin, yAtXMax = yAtXMax, yAtXMin
			}
			if x < xMin || x >= xMax {
				continue
			}
			t := (x - xMin) / (xMax - xMin)
			ys = append(ys, yAtXMin+t*(yAtXMax-yAtXMin))
		}
	}
	if len(ys) < 2 {
		return nil
	}
	sortFloat64s(ys)
	filtered := ys[:0]
	for _, y := range ys {
		if len(filtered) == 0 || math.Abs(y-filtered[len(filtered)-1]) > eps {
			filtered = append(filtered, y)
		}
	}
	return filtered
}

func sortFloat64s(xs []float64) {
	for i := 1; i < len(xs); i++ {
		x := xs[i]
		j := i - 1
		for ; j >= 0 && xs[j] > x; j-- {
			xs[j+1] = xs[j]
		}
		xs[j+1] = x
	}
}

var gcodePathTokenRe = regexp.MustCompile(`[A-Za-z]|[-+]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][-+]?\d+)?`)

func parseGCodePath(d string, curveSteps int) [][]gcodePt {
	toks := gcodePathTokenRe.FindAllString(d, -1)
	if len(toks) == 0 {
		return nil
	}
	readNum := func(i *int) (float64, bool) {
		if *i >= len(toks) || isAlphaToken(toks[*i]) {
			return 0, false
		}
		v, err := strconv.ParseFloat(toks[*i], 64)
		if err != nil {
			return 0, false
		}
		*i++
		return v, true
	}

	var polys [][]gcodePt
	var cur []gcodePt
	var curr gcodePt
	var start gcodePt
	cmd := byte(0)
	i := 0
	for i < len(toks) {
		if isAlphaToken(toks[i]) {
			cmd = toks[i][0]
			i++
			if cmd == 'Z' || cmd == 'z' {
				if len(cur) > 0 {
					cur = append(cur, start)
				}
				continue
			}
		}
		switch cmd {
		case 'M', 'm':
			x, ok1 := readNum(&i)
			y, ok2 := readNum(&i)
			if !ok1 || !ok2 {
				i++
				continue
			}
			if cmd == 'm' {
				x += curr.x
				y += curr.y
			}
			if len(cur) > 1 {
				polys = append(polys, cur)
			}
			cur = []gcodePt{{x: x, y: y}}
			curr = gcodePt{x: x, y: y}
			start = curr
			cmd = 'L'
		case 'L', 'l':
			x, ok1 := readNum(&i)
			y, ok2 := readNum(&i)
			if !ok1 || !ok2 {
				i++
				continue
			}
			if cmd == 'l' {
				x += curr.x
				y += curr.y
			}
			curr = gcodePt{x: x, y: y}
			cur = append(cur, curr)
		case 'Q', 'q':
			cx, ok1 := readNum(&i)
			cy, ok2 := readNum(&i)
			ex, ok3 := readNum(&i)
			ey, ok4 := readNum(&i)
			if !ok1 || !ok2 || !ok3 || !ok4 {
				i++
				continue
			}
			if cmd == 'q' {
				cx += curr.x
				cy += curr.y
				ex += curr.x
				ey += curr.y
			}
			p0 := curr
			p1 := gcodePt{x: cx, y: cy}
			p2 := gcodePt{x: ex, y: ey}
			for t := 1; t <= curveSteps; t++ {
				u := float64(t) / float64(curveSteps)
				x := (1-u)*(1-u)*p0.x + 2*(1-u)*u*p1.x + u*u*p2.x
				y := (1-u)*(1-u)*p0.y + 2*(1-u)*u*p1.y + u*u*p2.y
				cur = append(cur, gcodePt{x: x, y: y})
			}
			curr = p2
		case 'C', 'c':
			c1x, ok1 := readNum(&i)
			c1y, ok2 := readNum(&i)
			c2x, ok3 := readNum(&i)
			c2y, ok4 := readNum(&i)
			ex, ok5 := readNum(&i)
			ey, ok6 := readNum(&i)
			if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 {
				i++
				continue
			}
			if cmd == 'c' {
				c1x += curr.x
				c1y += curr.y
				c2x += curr.x
				c2y += curr.y
				ex += curr.x
				ey += curr.y
			}
			p0 := curr
			p1 := gcodePt{x: c1x, y: c1y}
			p2 := gcodePt{x: c2x, y: c2y}
			p3 := gcodePt{x: ex, y: ey}
			for t := 1; t <= curveSteps; t++ {
				u := float64(t) / float64(curveSteps)
				x := math.Pow(1-u, 3)*p0.x + 3*math.Pow(1-u, 2)*u*p1.x + 3*(1-u)*u*u*p2.x + u*u*u*p3.x
				y := math.Pow(1-u, 3)*p0.y + 3*math.Pow(1-u, 2)*u*p1.y + 3*(1-u)*u*u*p2.y + u*u*u*p3.y
				cur = append(cur, gcodePt{x: x, y: y})
			}
			curr = p3
		default:
			i++
		}
	}
	if len(cur) > 1 {
		polys = append(polys, cur)
	}
	return polys
}

func shrinkFeatureLoops(loops [][]gcodePt, d float64) [][]gcodePt {
	if d <= 0 {
		return loops
	}
	closed := normalizeClosedLoops(loops)
	if len(closed) == 0 {
		return closed
	}
	out := make([][]gcodePt, 0, len(closed))
	for i, loop := range closed {
		a := polygonArea(loop)
		if math.Abs(a) <= 1e-9 {
			continue
		}
		sample, ok := loopInteriorSample(loop)
		if !ok {
			out = append(out, loop)
			continue
		}
		depth := 0
		for j, other := range closed {
			if i == j {
				continue
			}
			if pointInLoop(sample, other) {
				depth++
			}
		}
		offset := d
		if depth%2 == 1 {
			// Odd containment depth means this is a hole; expand it to thin surrounding strokes.
			offset = -d
		}
		adj := insetLoop(loop, offset)
		if len(adj) < 4 || math.Abs(polygonArea(adj)) <= 1e-9 {
			out = append(out, loop)
			continue
		}
		out = append(out, adj)
	}
	return out
}

func loopInteriorSample(loop []gcodePt) (gcodePt, bool) {
	n := len(loop)
	if n < 4 {
		return gcodePt{}, false
	}
	ccw := polygonArea(loop) > 0
	const eps = 1e-4
	for i := 0; i+1 < n; i++ {
		a := loop[i]
		b := loop[i+1]
		dx := b.x - a.x
		dy := b.y - a.y
		l := math.Hypot(dx, dy)
		if l <= 1e-12 {
			continue
		}
		mx := (a.x + b.x) * 0.5
		my := (a.y + b.y) * 0.5
		nx := -dy / l
		ny := dx / l
		if !ccw {
			nx = -nx
			ny = -ny
		}
		return gcodePt{x: mx + nx*eps, y: my + ny*eps}, true
	}
	return gcodePt{}, false
}

func pointInLoop(p gcodePt, loop []gcodePt) bool {
	n := len(loop)
	if n < 4 {
		return false
	}
	inside := false
	last := n - 2
	for i := 0; i < n-1; i++ {
		a := loop[last]
		b := loop[i]
		last = i
		if (a.y > p.y) == (b.y > p.y) {
			continue
		}
		x := (b.x-a.x)*(p.y-a.y)/(b.y-a.y) + a.x
		if p.x < x {
			inside = !inside
		}
	}
	return inside
}

func isAlphaToken(s string) bool {
	return len(s) == 1 && ((s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= 'a' && s[0] <= 'z'))
}
