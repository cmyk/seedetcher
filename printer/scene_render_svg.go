package printer

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

type SceneSVGRenderer struct{}

func (SceneSVGRenderer) Render(doc *PlateDocument, outDir string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}
	for i, s := range doc.Scenes {
		name := s.Name
		if name == "" {
			name = fmt.Sprintf("scene_%02d", i+1)
		}
		path := filepath.Join(outDir, sanitizeSceneFilename(name)+".svg")
		if err := os.WriteFile(path, []byte(renderSceneSVG(s)), 0644); err != nil {
			return err
		}
	}
	return nil
}

func renderSceneSVG(scene PlateScene) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%.4fmm\" height=\"%.4fmm\" viewBox=\"0 0 %.4f %.4f\">\n", scene.WidthMM, scene.HeightMM, scene.WidthMM, scene.HeightMM)
	for _, layer := range scene.Layers {
		if !layer.Visible {
			continue
		}
		fmt.Fprintf(&b, "  <g data-layer=\"%s\">\n", escapeXML(layer.Tag))
		for _, p := range layer.Primitives {
			renderPrimitiveSVG(&b, p, "    ")
		}
		b.WriteString("  </g>\n")
	}
	b.WriteString("</svg>\n")
	return b.String()
}

func renderPrimitiveSVG(b *strings.Builder, p ScenePrimitive, indent string) {
	fill := p.FillColor
	if fill == "" {
		fill = "none"
	}
	stroke := p.StrokeColor
	if stroke == "" {
		stroke = "none"
	}
	switch p.Kind {
	case PrimitiveRect:
		fmt.Fprintf(b, "%s<rect x=\"%.4f\" y=\"%.4f\" width=\"%.4f\" height=\"%.4f\" fill=\"%s\" stroke=\"%s\" stroke-width=\"%.4f\" />\n",
			indent, p.XMM, p.YMM, p.WidthMM, p.HeightMM, fill, stroke, p.StrokeMM)
	case PrimitiveRound:
		fmt.Fprintf(b, "%s<rect x=\"%.4f\" y=\"%.4f\" width=\"%.4f\" height=\"%.4f\" rx=\"%.4f\" ry=\"%.4f\" fill=\"%s\" stroke=\"%s\" stroke-width=\"%.4f\" />\n",
			indent, p.XMM, p.YMM, p.WidthMM, p.HeightMM, p.RadiusMM, p.RadiusMM, fill, stroke, p.StrokeMM)
	case PrimitiveCircle:
		fmt.Fprintf(b, "%s<circle cx=\"%.4f\" cy=\"%.4f\" r=\"%.4f\" fill=\"%s\" stroke=\"%s\" stroke-width=\"%.4f\" />\n",
			indent, p.CXMM, p.CYMM, p.RadiusMM, fill, stroke, p.StrokeMM)
	case PrimitivePath:
		fmt.Fprintf(b, "%s<path d=\"%s\" fill=\"%s\" stroke=\"%s\" stroke-width=\"%.4f\" />\n",
			indent, escapeXML(p.PathData), fill, stroke, p.StrokeMM)
	case PrimitiveText:
		pathData, ok := svgTextPath(p)
		if ok {
			transform := ""
			if p.RotateDeg != 0 {
				transform = fmt.Sprintf(" transform=\"rotate(%.2f %.4f %.4f)\"", p.RotateDeg, p.XMM, p.YMM)
			}
			fmt.Fprintf(b, "%s<path d=\"%s\" fill=\"%s\" stroke=\"none\"%s />\n", indent, pathData, fill, transform)
			return
		}
		// Fallback if outline conversion fails.
		transform := ""
		if p.RotateDeg != 0 {
			transform = fmt.Sprintf(" transform=\"rotate(%.2f %.4f %.4f)\"", p.RotateDeg, p.XMM, p.YMM)
		}
		letterSpacing := ""
		if p.TrackingEM != 0 {
			letterSpacing = fmt.Sprintf(" letter-spacing=\"%.4fem\"", p.TrackingEM)
		}
		fontSizeMM := p.FontSizePt * 25.4 / 72.0
		fmt.Fprintf(b, "%s<text x=\"%.4f\" y=\"%.4f\" fill=\"%s\" font-family=\"%s\" font-size=\"%.4fmm\"%s%s>%s</text>\n",
			indent, p.XMM, p.YMM, fill, escapeXML(svgFontFamily(p.FontFamily)), fontSizeMM, letterSpacing, transform, escapeXML(p.Text))
	case PrimitiveGroup:
		fmt.Fprintf(b, "%s<g>\n", indent)
		for _, c := range p.Children {
			renderPrimitiveSVG(b, c, indent+"  ")
		}
		fmt.Fprintf(b, "%s</g>\n", indent)
	}
}

const svgTextDPI = 600.0

var (
	svgFontOnce sync.Once
	svgFont     *sfnt.Font
	svgFontErr  error
)

func svgTextPath(p ScenePrimitive) (string, bool) {
	if strings.TrimSpace(p.Text) == "" || p.FontSizePt <= 0 {
		return "", false
	}
	fontFace, err := svgSceneFont()
	if err != nil || fontFace == nil {
		return "", false
	}
	ppem := fixed.Int26_6(math.Round(p.FontSizePt * svgTextDPI / 72.0 * 64.0))
	if ppem <= 0 {
		return "", false
	}
	cmds, ok := buildGlyphRunPath(fontFace, p.Text, ppem, p.TrackingEM, p.FontSizePt)
	if !ok {
		return "", false
	}
	if len(cmds) == 0 {
		return "", false
	}
	dir := p.Direction
	if dir == "" {
		dir = TextDirHorizontal
	}
	anchor := p.Anchor
	if anchor == "" {
		anchor = TextAnchorBaselineLeft
	}

	transformed := transformPathCommands(cmds, dir)
	minX, minY, maxX, maxY := pathCommandBounds(transformed)
	tx, ty := anchoredTextOffset(p.XMM, p.YMM, anchor, minX, minY, maxX, maxY)
	return serializePathCommands(transformed, tx, ty), true
}

type svgPoint struct {
	x float64
	y float64
}

type svgPathCmd struct {
	op byte
	p  [3]svgPoint
	n  int
}

func transformTextPoint(in svgPoint, dir TextDirection) svgPoint {
	switch dir {
	case TextDirVerticalUp:
		return svgPoint{x: in.y, y: -in.x}
	case TextDirVerticalDown:
		return svgPoint{x: -in.y, y: in.x}
	default:
		return in
	}
}

func buildGlyphRunPath(fontFace *sfnt.Font, text string, ppem fixed.Int26_6, trackingEM, sizePt float64) ([]svgPathCmd, bool) {
	trackMM := trackingEM * sizePt * 25.4 / 72.0
	var cmds []svgPathCmd
	var buf sfnt.Buffer
	penX := 0.0
	runes := []rune(text)
	var prev sfnt.GlyphIndex
	hasPrev := false
	for i, r := range runes {
		gi, err := fontFace.GlyphIndex(&buf, r)
		if err != nil {
			return nil, false
		}
		if hasPrev {
			if kern, err := fontFace.Kern(&buf, prev, gi, ppem, font.HintingFull); err == nil {
				penX += pxToMM(float64(kern) / 64.0)
			}
		}
		segs, err := fontFace.LoadGlyph(&buf, gi, ppem, nil)
		if err != nil {
			return nil, false
		}
		cmds = appendGlyphSegments(cmds, segs, penX)
		adv, err := fontFace.GlyphAdvance(&buf, gi, ppem, font.HintingFull)
		if err != nil {
			return nil, false
		}
		penX += pxToMM(float64(adv) / 64.0)
		if i < len(runes)-1 && trackMM != 0 {
			penX += trackMM
		}
		prev = gi
		hasPrev = true
	}
	return cmds, true
}

func appendGlyphSegments(dst []svgPathCmd, segs []sfnt.Segment, penX float64) []svgPathCmd {
	for _, s := range segs {
		switch s.Op {
		case sfnt.SegmentOpMoveTo:
			dst = append(dst, svgPathCmd{
				op: 'M',
				p:  [3]svgPoint{{x: penX + fxToMM(s.Args[0].X), y: fxToMM(s.Args[0].Y)}},
				n:  1,
			})
		case sfnt.SegmentOpLineTo:
			dst = append(dst, svgPathCmd{
				op: 'L',
				p:  [3]svgPoint{{x: penX + fxToMM(s.Args[0].X), y: fxToMM(s.Args[0].Y)}},
				n:  1,
			})
		case sfnt.SegmentOpQuadTo:
			dst = append(dst, svgPathCmd{
				op: 'Q',
				p: [3]svgPoint{
					{x: penX + fxToMM(s.Args[0].X), y: fxToMM(s.Args[0].Y)},
					{x: penX + fxToMM(s.Args[1].X), y: fxToMM(s.Args[1].Y)},
				},
				n: 2,
			})
		case sfnt.SegmentOpCubeTo:
			dst = append(dst, svgPathCmd{
				op: 'C',
				p: [3]svgPoint{
					{x: penX + fxToMM(s.Args[0].X), y: fxToMM(s.Args[0].Y)},
					{x: penX + fxToMM(s.Args[1].X), y: fxToMM(s.Args[1].Y)},
					{x: penX + fxToMM(s.Args[2].X), y: fxToMM(s.Args[2].Y)},
				},
				n: 3,
			})
		}
	}
	return dst
}

func transformPathCommands(cmds []svgPathCmd, dir TextDirection) []svgPathCmd {
	transformed := make([]svgPathCmd, len(cmds))
	for i := range cmds {
		transformed[i].op = cmds[i].op
		transformed[i].n = cmds[i].n
		for j := 0; j < cmds[i].n; j++ {
			transformed[i].p[j] = transformTextPoint(cmds[i].p[j], dir)
		}
	}
	return transformed
}

func pathCommandBounds(cmds []svgPathCmd) (minX, minY, maxX, maxY float64) {
	minX, minY = math.MaxFloat64, math.MaxFloat64
	maxX, maxY = -math.MaxFloat64, -math.MaxFloat64
	for i := range cmds {
		for j := 0; j < cmds[i].n; j++ {
			pt := cmds[i].p[j]
			if pt.x < minX {
				minX = pt.x
			}
			if pt.y < minY {
				minY = pt.y
			}
			if pt.x > maxX {
				maxX = pt.x
			}
			if pt.y > maxY {
				maxY = pt.y
			}
		}
	}
	return minX, minY, maxX, maxY
}

func anchoredTextOffset(xMM, yMM float64, anchor TextAnchor, minX, minY, maxX, maxY float64) (tx, ty float64) {
	tx, ty = xMM, yMM
	switch anchor {
	case TextAnchorTopLeft:
		tx -= minX
		ty -= minY
	case TextAnchorCenter:
		tx -= (minX + maxX) / 2
		ty -= (minY + maxY) / 2
	}
	return tx, ty
}

func serializePathCommands(cmds []svgPathCmd, tx, ty float64) string {
	var out strings.Builder
	for _, c := range cmds {
		switch c.op {
		case 'M', 'L':
			fmt.Fprintf(&out, "%c%.4f %.4f", c.op, c.p[0].x+tx, c.p[0].y+ty)
		case 'Q':
			fmt.Fprintf(&out, "Q%.4f %.4f %.4f %.4f",
				c.p[0].x+tx, c.p[0].y+ty,
				c.p[1].x+tx, c.p[1].y+ty)
		case 'C':
			fmt.Fprintf(&out, "C%.4f %.4f %.4f %.4f %.4f %.4f",
				c.p[0].x+tx, c.p[0].y+ty,
				c.p[1].x+tx, c.p[1].y+ty,
				c.p[2].x+tx, c.p[2].y+ty)
		}
	}
	return strings.TrimSpace(out.String())
}

func svgSceneFont() (*sfnt.Font, error) {
	svgFontOnce.Do(func() {
		data := loadFirstFontData(plateFontPrimary, martianMono)
		if data == nil {
			svgFontErr = fmt.Errorf("svg scene font data not found")
			return
		}
		svgFont, svgFontErr = sfnt.Parse(data)
	})
	return svgFont, svgFontErr
}

func fxToMM(v fixed.Int26_6) float64 {
	return pxToMM(float64(v) / 64.0)
}

func pxToMM(px float64) float64 {
	return px * 25.4 / svgTextDPI
}

func svgFontFamily(in string) string {
	base := strings.TrimSpace(in)
	if strings.EqualFold(base, "SeedEtcher-Regular") || base == "" {
		base = "SeedEtcher"
	}
	// Keep a stable fallback stack for environments where the custom face is missing.
	return base + ", monospace"
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

var sceneFileSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeSceneFilename(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return "scene"
	}
	n = sceneFileSanitizer.ReplaceAllString(n, "_")
	n = strings.Trim(n, "._-")
	if n == "" {
		return "scene"
	}
	return n
}
