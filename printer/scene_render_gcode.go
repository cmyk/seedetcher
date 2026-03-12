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
		if err := os.WriteFile(path, []byte(renderSceneGCode(s, cfg)), 0644); err != nil {
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
}

func renderSceneGCode(scene PlateScene, cfg SceneGCodeRenderer) string {
	e := &gcodeEmitter{cfg: cfg}
	fmt.Fprintf(&e.b, "; SeedEtcher scene: %s\n", scene.Name)
	fmt.Fprintf(&e.b, "; Size: %.3fmm x %.3fmm\n", scene.WidthMM, scene.HeightMM)
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
		}
	}
	e.laserOffSafe()
	e.b.WriteString("G0 X0.000 Y0.000\n")
	e.b.WriteString("M2\n")
	return e.b.String()
}

func (e *gcodeEmitter) renderPrimitive(p ScenePrimitive) {
	switch p.Kind {
	case PrimitiveGroup:
		for _, c := range p.Children {
			e.renderPrimitive(c)
		}
	case PrimitiveRect:
		e.tracePolyline([]gcodePt{
			{x: p.XMM, y: p.YMM},
			{x: p.XMM + p.WidthMM, y: p.YMM},
			{x: p.XMM + p.WidthMM, y: p.YMM + p.HeightMM},
			{x: p.XMM, y: p.YMM + p.HeightMM},
			{x: p.XMM, y: p.YMM},
		})
	case PrimitiveRound:
		e.tracePolyline(roundRectPolyline(p.XMM, p.YMM, p.WidthMM, p.HeightMM, p.RadiusMM, 6))
	case PrimitiveCircle:
		e.tracePolyline(circlePolyline(p.CXMM, p.CYMM, p.RadiusMM, 40))
	case PrimitivePath:
		for _, poly := range parseGCodePath(p.PathData, e.cfg.CurveSteps) {
			e.tracePolyline(poly)
		}
	case PrimitiveText:
		pathData, ok := svgTextPath(p)
		if !ok {
			return
		}
		for _, poly := range parseGCodePath(pathData, e.cfg.CurveSteps) {
			e.tracePolyline(poly)
		}
	}
}

func (e *gcodeEmitter) tracePolyline(poly []gcodePt) {
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
	fmt.Fprintf(&e.b, "G0 X%.3f Y%.3f\n", x, y)
	e.x, e.y = x, y
}

func (e *gcodeEmitter) cutTo(x, y float64) {
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
	if !e.laserOn {
		return
	}
	e.b.WriteString(e.cfg.LaserOffCmd)
	e.b.WriteByte('\n')
	e.laserOn = false
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
