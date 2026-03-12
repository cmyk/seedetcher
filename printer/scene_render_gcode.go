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
	LaserOnCmd     string
	LaserOffCmd    string
	LaserMaxS      int
	CutFeedMMMin   float64
	RapidFeedMMMin float64
	CurveSteps     int
	FillStepMM     float64
	SpiralStepMM   float64
	PlateMM        float64
	LaserFlipX     bool
	LaserFlipY     bool
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
	if r.SpiralStepMM <= 0 {
		r.SpiralStepMM = r.FillStepMM
	}
	if r.PlateMM <= 0 {
		r.PlateMM = 100
	}
	// Scene space uses top-left origin with +Y downward.
	// GRBL/LB commonly uses bottom-left with +Y upward.
	// Default to Y-flip for sane laser orientation.
	if !r.LaserFlipX && !r.LaserFlipY {
		r.LaserFlipY = true
	}
	return r
}

type gcodePt struct {
	x float64
	y float64
}

type gcodeEmitter struct {
	b       strings.Builder
	cfg     SceneGCodeRenderer
	x       float64
	y       float64
	laserOn bool
	maxXY   float64
	sceneW  float64
	sceneH  float64
	err     error
}

func renderSceneGCode(scene PlateScene, cfg SceneGCodeRenderer) (string, error) {
	e := &gcodeEmitter{cfg: cfg, maxXY: cfg.PlateMM, sceneW: scene.WidthMM, sceneH: scene.HeightMM}
	if scene.WidthMM > cfg.PlateMM || scene.HeightMM > cfg.PlateMM {
		return "", fmt.Errorf("scene '%s' exceeds plate bounds: %.3fx%.3fmm > %.3fmm", scene.Name, scene.WidthMM, scene.HeightMM, cfg.PlateMM)
	}
	fmt.Fprintf(&e.b, "; SeedEtcher scene: %s\n", scene.Name)
	fmt.Fprintf(&e.b, "; Size: %.3fmm x %.3fmm\n", scene.WidthMM, scene.HeightMM)
	fmt.Fprintf(&e.b, "; Workspace: %.3fmm\n", cfg.PlateMM)
	e.b.WriteString("G21\n")
	e.b.WriteString("G90\n")
	fmt.Fprintf(&e.b, "G0 F%.1f\n", cfg.RapidFeedMMMin)
	fmt.Fprintf(&e.b, "G1 F%.1f\n", cfg.CutFeedMMMin)
	for _, layer := range scene.Layers {
		if !layer.Visible {
			continue
		}
		fmt.Fprintf(&e.b, "; Layer: %s\n", layer.Tag)
		for _, p := range layer.Primitives {
			e.renderPrimitive(p)
			if e.err != nil {
				return "", e.err
			}
		}
	}
	e.laserOffSafe()
	if e.err != nil {
		return "", e.err
	}
	e.b.WriteString("G0 X0.000 Y0.000\n")
	e.b.WriteString("M2\n")
	return e.b.String(), nil
}

func (e *gcodeEmitter) renderPrimitive(p ScenePrimitive) {
	if e.err != nil {
		return
	}
	if e.skipGuidePrimitive(p) {
		return
	}
	fillMode := effectiveFillMode(p)
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
		e.fillOrTrace(fillMode, [][]gcodePt{loop})
	case PrimitiveRound:
		e.fillOrTrace(fillMode, [][]gcodePt{roundRectPolyline(p.XMM, p.YMM, p.WidthMM, p.HeightMM, p.RadiusMM, 6)})
	case PrimitiveCircle:
		if fillMode == FillModeSpiral {
			e.tracePolyline(circleSpiralPolyline(p.CXMM, p.CYMM, p.RadiusMM, e.cfg.SpiralStepMM))
			e.tracePolyline(circlePolyline(p.CXMM, p.CYMM, p.RadiusMM, 40))
		} else {
			e.fillOrTrace(fillMode, [][]gcodePt{circlePolyline(p.CXMM, p.CYMM, p.RadiusMM, 40)})
		}
	case PrimitiveRing:
		outer, inner := ringOutlines(p.XMM, p.YMM, p.WidthMM, p.HeightMM, p.ThicknessMM, p.RadiusMM)
		if fillMode == FillModeOffset {
			for _, loop := range ringOffsetLoops(p.XMM, p.YMM, p.WidthMM, p.HeightMM, p.ThicknessMM, p.RadiusMM, e.cfg.FillStepMM) {
				e.tracePolyline(loop)
			}
			e.tracePolyline(outer)
			e.tracePolyline(inner)
		} else {
			e.fillOrTrace(fillMode, [][]gcodePt{outer, inner})
		}
	case PrimitivePath:
		loops := parseGCodePath(p.PathData, e.cfg.CurveSteps)
		if fillMode == FillModeOffset && strings.EqualFold(p.FillRule, "evenodd") {
			e.fillOrTrace(FillModeHatch, loops)
		} else {
			e.fillOrTrace(fillMode, loops)
		}
	case PrimitiveText:
		pathData, ok := svgTextPath(p)
		if !ok {
			return
		}
		e.fillOrTrace(fillMode, parseGCodePath(pathData, e.cfg.CurveSteps))
	}
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

func (e *gcodeEmitter) fillOrTrace(fillMode FillMode, loops [][]gcodePt) {
	if fillMode == FillModeHatch {
		for _, seg := range hatchSegments(loops, e.cfg.FillStepMM) {
			e.tracePolyline(seg)
		}
	}
	if fillMode == FillModeOffset {
		for _, poly := range loops {
			for _, loop := range offsetInwardLoops(poly, e.cfg.FillStepMM) {
				e.tracePolyline(loop)
			}
		}
	}
	for _, poly := range loops {
		e.tracePolyline(poly)
	}
}

func (e *gcodeEmitter) tracePolyline(poly []gcodePt) {
	if e.err != nil {
		return
	}
	if len(poly) < 2 {
		return
	}
	e.laserOffSafe()
	e.rapidTo(poly[0].x, poly[0].y)
	e.laserOnStart()
	for i := 1; i < len(poly); i++ {
		e.cutTo(poly[i].x, poly[i].y)
	}
	e.laserOffSafe()
}

func (e *gcodeEmitter) rapidTo(x, y float64) {
	x, y = e.mapXY(x, y)
	if !e.validXY(x, y) {
		return
	}
	fmt.Fprintf(&e.b, "G0 X%.3f Y%.3f\n", x, y)
	e.x, e.y = x, y
}

func (e *gcodeEmitter) cutTo(x, y float64) {
	x, y = e.mapXY(x, y)
	if !e.validXY(x, y) {
		return
	}
	fmt.Fprintf(&e.b, "G1 X%.3f Y%.3f\n", x, y)
	e.x, e.y = x, y
}

func (e *gcodeEmitter) laserOnStart() {
	if e.laserOn {
		return
	}
	fmt.Fprintf(&e.b, "%s S%d\n", e.cfg.LaserOnCmd, e.cfg.LaserMaxS)
	e.laserOn = true
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
}

func (e *gcodeEmitter) validXY(x, y float64) bool {
	if e.err != nil {
		return false
	}
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
		e.err = fmt.Errorf("invalid coordinate (NaN/Inf): x=%v y=%v", x, y)
		return false
	}
	if x < 0 || y < 0 || x > e.maxXY || y > e.maxXY {
		e.err = fmt.Errorf("gcode coordinate out of bounds: x=%.3f y=%.3f workspace=0..%.3f", x, y, e.maxXY)
		return false
	}
	return true
}

func (e *gcodeEmitter) mapXY(x, y float64) (float64, float64) {
	if e.cfg.LaserFlipX {
		x = e.maxXY - x
	}
	if e.cfg.LaserFlipY {
		y = e.maxXY - y
	}
	return x, y
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
			return nil
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
	minY, maxY := loops[0][0].y, loops[0][0].y
	for _, loop := range loops {
		for _, p := range loop {
			if p.y < minY {
				minY = p.y
			}
			if p.y > maxY {
				maxY = p.y
			}
		}
	}
	const eps = 1e-9
	out := make([][]gcodePt, 0, int((maxY-minY)/step)+1)
	line := 0
	for y := minY + eps; y <= maxY-eps; y += step {
		xs := scanlineIntersections(loops, y)
		if len(xs) < 2 {
			line++
			continue
		}
		for i := 0; i+1 < len(xs); i += 2 {
			x0, x1 := xs[i], xs[i+1]
			if x1-x0 <= eps {
				continue
			}
			seg := []gcodePt{{x: x0, y: y}, {x: x1, y: y}}
			if line%2 == 1 {
				seg[0], seg[1] = seg[1], seg[0]
			}
			out = append(out, seg)
		}
		line++
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

func isAlphaToken(s string) bool {
	return len(s) == 1 && ((s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= 'a' && s[0] <= 'z'))
}
