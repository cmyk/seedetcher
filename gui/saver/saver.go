package saver

import (
	"image"
	"image/color"
	"image/draw"
	"math/rand/v2"

	"seedetcher.com/gui/assets"
	"seedetcher.com/image/rgb565"
)

var black = rgb(0x000000)
var bgColor = rgb(0x1a1200)          // Darkened background.
var dropColor = rgba(0xffa202, 0x99) // Soft orange.
var glowColor = rgb(0xfff3c4)
var prng = rand.New(rand.NewPCG(1, 2))

// State drives a simple falling-bit saver that highlights the SeedEtcher logo.
type State struct {
	mask     []bool
	maskRect image.Rectangle
	dims     image.Point

	maskBuf [maxMask]bool

	particles []particle
	pbuf      [maxParticles]particle

	bg   image.Image
	drop image.Image
	glow image.Image
}

type particle struct {
	x, y   int
	speed  int
	width  int
	height int
}

const (
	maxMask      = 240 * 240
	maxParticles = 70
	minSpeed     = 1
	maxSpeed     = 3
	dropWidth    = 2
	dropHeight   = 8
)

func (s *State) init(dims image.Point) {
	s.dims = dims
	s.bg = bgColor
	s.drop = dropColor
	s.glow = glowColor

	s.buildMask(dims)
	s.initParticles(dims)
}

func (s *State) buildMask(dims image.Point) {
	pimg := assets.SeedetcherLogo
	logo := pimg.Bounds()
	offset := image.Pt((dims.X-logo.Dx())/2, (dims.Y-logo.Dy())/2)
	s.maskRect = logo.Add(offset)
	size := s.maskRect.Dx() * s.maskRect.Dy()
	if size <= 0 || size > len(s.maskBuf) {
		s.mask = nil
		return
	}
	s.mask = s.maskBuf[:size]
	for i := range s.mask {
		s.mask[i] = false
	}
	stride := int(pimg.Rect.MaxX - pimg.Rect.MinX)
	height := int(pimg.Rect.MaxY - pimg.Rect.MinY)
	for y := 0; y < height; y++ {
		row := pimg.Pix[y*stride : (y+1)*stride]
		for x := 0; x < stride; x++ {
			if row[x] == 0 {
				continue
			}
			dx := x + offset.X
			dy := y + offset.Y
			idx := (dy-s.maskRect.Min.Y)*s.maskRect.Dx() + (dx - s.maskRect.Min.X)
			if idx >= 0 && idx < len(s.mask) {
				s.mask[idx] = true
			}
		}
	}
}

func (s *State) initParticles(dims image.Point) {
	s.particles = s.pbuf[:maxParticles]
	for i := range s.particles {
		s.particles[i] = particle{
			x:      prng.IntN(dims.X),
			y:      prng.IntN(dims.Y),
			speed:  prng.IntN(maxSpeed-minSpeed+1) + minSpeed,
			width:  dropWidth,
			height: dropHeight,
		}
	}
}

func (s *State) step() {
	for i := range s.particles {
		p := &s.particles[i]
		p.y += p.speed
		if p.y > s.dims.Y {
			p.y = -prng.IntN(20)
			p.x = prng.IntN(s.dims.X)
			p.speed = prng.IntN(maxSpeed-minSpeed+1) + minSpeed
		}
	}
}

func (s *State) Draw(screen Screen) {
	dims := screen.DisplaySize()
	if s.dims != dims || len(s.particles) == 0 || len(s.mask) == 0 {
		s.init(dims)
	}
	s.step()

	// Redraw full frame for simplicity.
	dr := image.Rectangle{Max: dims}
	chunks := newDraw(screen, dr)
	for {
		chunk, ok := chunks.Next()
		if !ok {
			break
		}
		draw.Draw(chunk, chunk.Bounds(), s.bg, image.Point{}, draw.Src)
		for _, p := range s.particles {
			rect := image.Rect(p.x, p.y, p.x+p.width, p.y+p.height)
			if !rect.Overlaps(chunk.Bounds()) {
				continue
			}
			// Draw base drop.
			draw.Draw(chunk, rect.Intersect(chunk.Bounds()), s.drop, image.Point{}, draw.Over)
			// If intersecting logo mask, brighten.
			if s.hitMask(rect) {
				draw.Draw(chunk, rect.Intersect(chunk.Bounds()), s.glow, image.Point{}, draw.Over)
			}
		}
	}
}

func (s *State) hitMask(r image.Rectangle) bool {
	inter := r.Intersect(s.maskRect)
	if inter.Empty() || len(s.mask) == 0 {
		return false
	}
	for y := inter.Min.Y; y < inter.Max.Y; y++ {
		for x := inter.Min.X; x < inter.Max.X; x++ {
			idx := (y-s.maskRect.Min.Y)*s.maskRect.Dx() + (x - s.maskRect.Min.X)
			if idx >= 0 && idx < len(s.mask) && s.mask[idx] {
				return true
			}
		}
	}
	return false
}

type Screen interface {
	DisplaySize() image.Point
	// Dirty begins a refresh of the content
	// specified by r.
	Dirty(r image.Rectangle) error
	// NextChunk returns the next chunk of the refresh.
	NextChunk() (draw.RGBA64Image, bool)
}

func imageDraw(dst draw.RGBA64Image, dr image.Rectangle, src image.Image, sp image.Point, op draw.Op) {
	switch dst := dst.(type) {
	case *rgb565.Image:
		dst.Draw(dr, src, sp, op)
		return
	}
	draw.Draw(dst, dr, src, sp, op)
}

type chunks struct {
	scr Screen
}

func (c chunks) Next() (draw.RGBA64Image, bool) {
	img, ok := c.scr.NextChunk()
	if !ok {
		return nil, false
	}
	imageDraw(img, img.Bounds(), black, image.Point{}, draw.Src)
	return img, true
}

func newDraw(screen Screen, dr image.Rectangle) chunks {
	screen.Dirty(dr)
	return chunks{screen}
}

func rgb(c uint32) image.Image {
	r := uint8(c >> 16)
	g := uint8(c >> 8)
	b := uint8(c)
	return image.NewUniform(color.RGBA{
		A: 0xff, R: r, G: g, B: b,
	})
}

func rgba(c uint32, a uint8) image.Image {
	r := uint8(c >> 16)
	g := uint8(c >> 8)
	b := uint8(c)
	return image.NewUniform(color.RGBA{
		A: a, R: r, G: g, B: b,
	})
}
