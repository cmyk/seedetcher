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
var bgColor = rgb(0x000000)
var logoColor = rgb(0xff6600)
var dropColor = logoColor
var prng = rand.New(rand.NewPCG(1, 2))
var logoRGB = color.RGBA{R: 0xff, G: 0x66, B: 0x00, A: 0xff}

// State drives a falling-bit saver that gradually reveals the SeedEtcher logo.
type State struct {
	mask      []bool
	maskAlpha []uint8
	reveal    []bool
	maskCount int
	maskRect  image.Rectangle
	dims      image.Point
	colors    [256]color.RGBA

	maskBuf      [maxMask]bool
	maskAlphaBuf [maxMask]uint8
	revealBuf    [maxMask]bool

	particles []particle
	pbuf      [maxParticles]particle

	bg   image.Image
	drop image.Image
	logo image.Image

	phase        phase
	revealN      int
	hold         int
	decayFrames  int
	revealFrames int
}

type phase int

const (
	phaseReveal phase = iota
	phaseDecay
)

type particle struct {
	x, y     int
	speed    int
	width    int
	height   int
	revealed bool
}

const (
	maxMask         = 240 * 240
	maxParticles    = 70
	minSpeed        = 1
	maxSpeed        = 3
	dropWidth       = 2
	dropHeight      = 2
	holdFrames      = 30
	maxDecayFrames  = 180
	maxRevealFrames = 600
)

func (s *State) init(dims image.Point) {
	s.dims = dims
	s.bg = bgColor
	s.drop = dropColor
	s.logo = logoColor
	s.initColors()

	s.buildMask(dims)
	s.initParticles(dims)
	s.phase = phaseReveal
	s.revealN = 0
	s.hold = 0
	s.decayFrames = 0
	s.revealFrames = 0
}

func (s *State) buildMask(dims image.Point) {
	pimg := assets.SeedetcherLogoScreensaver
	logo := pimg.Bounds() // image.Rectangle (has Dx/Dy)
	offset := image.Pt((dims.X-logo.Dx())/2, (dims.Y-logo.Dy())/2)
	s.maskRect = logo.Add(offset)
	size := s.maskRect.Dx() * s.maskRect.Dy()
	if size <= 0 || size > len(s.maskBuf) {
		s.mask = nil
		s.maskCount = 0
		return
	}
	s.mask = s.maskBuf[:size]
	s.maskAlpha = s.maskAlphaBuf[:size]
	s.reveal = s.revealBuf[:size]
	for i := 0; i < size; i++ {
		s.mask[i] = false
		s.maskAlpha[i] = 0
		s.reveal[i] = false
	}
	s.maskCount = 0
	width := int(pimg.Rect.MaxX - pimg.Rect.MinX)
	height := int(pimg.Rect.MaxY - pimg.Rect.MinY)
	stride := width // your generated paletted.Image is packed; no Stride field exists
	pal := pimg.Palette
	for y := 0; y < height; y++ {
		rowStart := y * stride
		row := pimg.Pix[rowStart : rowStart+width]
		for x := 0; x < width; x++ {
			c := pal.At(row[x])
			_, _, _, a := c.RGBA()
			if a == 0 {
				continue
			}
			dx := x + offset.X
			dy := y + offset.Y
			idx := (dy-s.maskRect.Min.Y)*s.maskRect.Dx() + (dx - s.maskRect.Min.X)
			if idx >= 0 && idx < len(s.mask) {
				s.mask[idx] = true
				s.maskAlpha[idx] = uint8(a >> 8)
				s.maskCount++
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
			p.revealed = false
		}
	}
}

func (s *State) Draw(screen Screen) {
	dims := screen.DisplaySize()
	if s.dims != dims || len(s.particles) == 0 || len(s.mask) == 0 {
		s.init(dims)
	}
	s.step()
	s.revealFrames++

	dr := image.Rectangle{Max: dims}
	chunks := newDraw(screen, dr)
	for {
		chunk, ok := chunks.Next()
		if !ok {
			break
		}
		imageDraw(chunk, chunk.Bounds(), s.bg, image.Point{}, draw.Src)
		for i := range s.particles {
			p := &s.particles[i]
			rect := image.Rect(p.x, p.y, p.x+p.width, p.y+p.height)
			if !rect.Overlaps(chunk.Bounds()) {
				continue
			}
			switch s.phase {
			case phaseReveal:
				if s.markReveal(rect) {
					p.revealed = true
				}
			case phaseDecay:
				if s.hold == 0 && s.markHide(rect) {
					p.revealed = true
				}
			}
			if p.revealed {
				// Stop drawing this drop after reveal.
				continue
			}
			draw.Draw(chunk, rect.Intersect(chunk.Bounds()), s.drop, image.Point{}, draw.Over)
		}
		s.drawReveal(chunk)
	}
	if s.phase == phaseReveal && s.maskCount > 0 && s.revealN >= s.maskCount {
		s.phase = phaseDecay
		s.hold = holdFrames
		s.decayFrames = 0
		s.revealFrames = 0
		for i := range s.particles {
			s.particles[i].revealed = false
		}
	}
	if s.phase == phaseDecay {
		if s.hold > 0 {
			s.hold--
		} else {
			if s.revealN == 0 {
				s.phase = phaseReveal
				s.clearReveal()
				for i := range s.particles {
					s.particles[i].revealed = false
				}
				s.revealFrames = 0
			}
		}
	}
}

func (s *State) markReveal(r image.Rectangle) bool {
	inter := r.Intersect(s.maskRect)
	if inter.Empty() || len(s.mask) == 0 {
		return false
	}
	var hit bool
	for y := inter.Min.Y; y < inter.Max.Y; y++ {
		for x := inter.Min.X; x < inter.Max.X; x++ {
			idx := (y-s.maskRect.Min.Y)*s.maskRect.Dx() + (x - s.maskRect.Min.X)
			if idx >= 0 && idx < len(s.mask) && s.mask[idx] && !s.reveal[idx] {
				s.reveal[idx] = true
				s.revealN++
				hit = true
			}
		}
	}
	return hit
}

func (s *State) markHide(r image.Rectangle) bool {
	inter := r.Intersect(s.maskRect)
	if inter.Empty() || len(s.mask) == 0 {
		return false
	}
	var hit bool
	for y := inter.Min.Y; y < inter.Max.Y; y++ {
		for x := inter.Min.X; x < inter.Max.X; x++ {
			idx := (y-s.maskRect.Min.Y)*s.maskRect.Dx() + (x - s.maskRect.Min.X)
			if idx >= 0 && idx < len(s.reveal) && s.reveal[idx] {
				s.reveal[idx] = false
				s.revealN--
				hit = true
			}
		}
	}
	return hit
}

func (s *State) decay() {
	if s.revealN == 0 {
		return
	}
	clear := 8
	for i := 0; i < clear && s.revealN > 0; i++ {
		idx := prng.IntN(len(s.reveal))
		if s.reveal[idx] {
			s.reveal[idx] = false
			s.revealN--
		}
	}
}

func (s *State) drawReveal(chunk draw.RGBA64Image) {
	b := s.maskRect.Intersect(chunk.Bounds())
	if b.Empty() {
		return
	}
	if img, ok := chunk.(*rgb565.Image); ok {
		s.drawRevealRGB565(img, b)
		return
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		rowOff := (y - s.maskRect.Min.Y) * s.maskRect.Dx()
		for x := b.Min.X; x < b.Max.X; x++ {
			idx := rowOff + (x - s.maskRect.Min.X)
			if idx >= 0 && idx < len(s.reveal) && s.reveal[idx] {
				a := s.maskAlpha[idx]
				if a == 0 {
					continue
				}
				chunk.Set(x, y, s.colors[a])
			}
		}
	}
}

func (s *State) drawRevealRGB565(img *rgb565.Image, b image.Rectangle) {
	for y := b.Min.Y; y < b.Max.Y; y++ {
		rowOff := (y - s.maskRect.Min.Y) * s.maskRect.Dx()
		for x := b.Min.X; x < b.Max.X; x++ {
			idx := rowOff + (x - s.maskRect.Min.X)
			if idx >= 0 && idx < len(s.reveal) && s.reveal[idx] {
				a := s.maskAlpha[idx]
				if a == 0 {
					continue
				}
				r := uint8((uint16(logoRGB.R) * uint16(a)) / 255)
				g := uint8((uint16(logoRGB.G) * uint16(a)) / 255)
				bb := uint8((uint16(logoRGB.B) * uint16(a)) / 255)
				img.Pix[img.PixOffset(x, y)] = rgb565.RGB888ToRGB565(r, g, bb)
			}
		}
	}
}

func (s *State) clearReveal() {
	for i := range s.reveal {
		s.reveal[i] = false
	}
	s.revealN = 0
}

func (s *State) initColors() {
	for i := range s.colors {
		s.colors[i] = color.RGBA{
			R: logoRGB.R,
			G: logoRGB.G,
			B: logoRGB.B,
			A: uint8(i),
		}
	}
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
