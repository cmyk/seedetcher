// package gui implements the SeedEtcher controller user interface.
package gui

import (
	"image"
	"image/color"
	"image/draw"
	"io"
	"time"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/gui/op"
	"seedetcher.com/gui/saver"
	"seedetcher.com/printer"
)

const nbuttons = 8

type Context struct {
	Platform Platform
	Styles   Styles
	Wakeup   time.Time
	Frame    func()
	Session  Session

	// Global UI state.
	Version        string
	Calibrated     bool
	EmptySDSlot    bool
	RotateCamera   bool
	LastDescriptor *urtypes.OutputDescriptor
	Keystores      map[uint32]bip39.Mnemonic // Fingerprint -> Mnemonic
	events         []Event
}

// Session holds the mutable data for a user flow. It makes screen transitions
// explicit and keeps cross-screen state in one place.
type Session struct {
	Descriptor *urtypes.OutputDescriptor
	Keystores  map[uint32]bip39.Mnemonic // Fingerprint -> Mnemonic
	Paper      printer.PaperSize
}

func NewContext(pl Platform) *Context {
	c := &Context{
		Platform:  pl,
		Styles:    NewStyles(),
		Keystores: make(map[uint32]bip39.Mnemonic),
	}
	return c
}

func (c *Context) WakeupAt(t time.Time) {
	if c.Wakeup.IsZero() || t.Before(c.Wakeup) {
		c.Wakeup = t
	}
}

const repeatStartDelay = 400 * time.Millisecond
const repeatDelay = 100 * time.Millisecond

func isRepeatButton(b Button) bool {
	switch b {
	case Up, Down, Right, Left:
		return true
	}
	return false
}

func (c *Context) Reset() {
	c.events = c.events[:0]
	c.Wakeup = time.Time{}
}

func (c *Context) Events(evts ...Event) {
	c.events = append(c.events, evts...)
}

func (c *Context) FrameEvent() (FrameEvent, bool) {
	for i, e := range c.events {
		if e, ok := e.AsFrame(); ok {
			c.events = append(c.events[:i], c.events[i+1:]...)
			return e, true
		}
	}
	return FrameEvent{}, false
}

func (c *Context) Next(btns ...Button) (ButtonEvent, bool) {
	for i, e := range c.events {
		e, ok := e.AsButton()
		if !ok {
			continue
		}
		for _, btn := range btns {
			if e.Button == btn {
				c.events = append(c.events[:i], c.events[i+1:]...)
				return e, true
			}
		}
	}
	return ButtonEvent{}, false
}

type InputTracker struct {
	Pressed [nbuttons]bool
	clicked [nbuttons]bool
	repeats [nbuttons]time.Time
}

func (t *InputTracker) Next(c *Context, btns ...Button) (ButtonEvent, bool) {
	now := c.Platform.Now()
	for _, b := range btns {
		if !isRepeatButton(b) {
			continue
		}
		if !t.Pressed[b] {
			t.repeats[b] = time.Time{}
			continue
		}
		wakeup := t.repeats[b]
		if wakeup.IsZero() {
			wakeup = now.Add(repeatStartDelay)
		}
		repeat := !now.Before(wakeup)
		if repeat {
			wakeup = now.Add(repeatDelay)
		}
		t.repeats[b] = wakeup
		c.WakeupAt(wakeup)
		if repeat {
			return ButtonEvent{Button: b, Pressed: true}, true
		}
	}

	e, ok := c.Next(btns...)
	if !ok {
		return ButtonEvent{}, false
	}
	if int(e.Button) < len(t.clicked) {
		t.clicked[e.Button] = !e.Pressed && t.Pressed[e.Button]
		t.Pressed[e.Button] = e.Pressed
	}
	return e, true
}

func (t *InputTracker) Clicked(b Button) bool {
	c := t.clicked[b]
	t.clicked[b] = false
	return c
}

type Platform interface {
	AppendEvents(deadline time.Time, evts []Event) []Event
	Wakeup()
	CameraFrame(size image.Point)
	Now() time.Time
	DisplaySize() image.Point
	// Dirty begins a refresh of the content
	// specified by r.
	Dirty(r image.Rectangle) error
	// NextChunk returns the next chunk of the refresh.
	NextChunk() (draw.RGBA64Image, bool)
	ScanQR(qr *image.Gray) ([][]byte, error)
	Debug() bool
	Printer() io.Writer
	CreatePlates(ctx *Context, mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int) error // Updated
}

type FrameEvent struct {
	Error error
	Image image.Image
}

type Event struct {
	typ  int
	data [4]uint32
	refs [2]any
}

const (
	buttonEvent = 1 + iota
	sdcardEvent
	frameEvent
)

type ButtonEvent struct {
	Button  Button
	Pressed bool
	// Rune is only valid if Button is Rune.
	Rune rune
}

type SDCardEvent struct {
	Inserted bool
}

type Button int

const (
	Up Button = iota
	Down
	Left
	Right
	Center
	Button1
	Button2
	Button3
	CCW
	CW
	// Synthetic keys only generated in debug mode.
	Rune // Enter rune.
)

func (b Button) String() string {
	switch b {
	case Up:
		return "up"
	case Down:
		return "down"
	case Left:
		return "left"
	case Right:
		return "right"
	case Center:
		return "center"
	case Button1:
		return "b1"
	case Button2:
		return "b2"
	case Button3:
		return "b3"
	case CCW:
		return "ccw"
	case CW:
		return "cw"
	case Rune:
		return "rune"
	default:
		panic("invalid button")
	}
}

const idleTimeout = 3 * time.Minute

func Run(pl Platform, version string) func(yield func() bool) {
	return func(yield func() bool) {
		ctx := NewContext(pl)
		ctx.Version = version
		ctx.Session = Session{
			Paper:     printer.PaperA4,
			Keystores: make(map[uint32]bip39.Mnemonic),
		}
		a := struct {
			root op.Ops
			mask *image.Alpha
			ctx  *Context
			idle struct {
				start  time.Time
				active bool
				state  saver.State
			}
		}{
			ctx: ctx,
		}
		a.idle.start = pl.Now()

		for {
			it := func(yield func() bool) {
				stop := new(int)
				ctx.Frame = func() {
					if !yield() {
						panic(stop)
					}
				}
				defer func() {
					if err := recover(); err != stop {
						panic(err)
					}
				}()
				mainFlow(ctx, a.root.Context())
			}
			var evts []Event
			for range it {
				dirty := a.root.Clip(image.Rectangle{Max: a.ctx.Platform.DisplaySize()})
				if err := a.ctx.Platform.Dirty(dirty); err != nil {
					panic(err)
				}
				for {
					fb, ok := a.ctx.Platform.NextChunk()
					if !ok {
						break
					}
					fbdims := fb.Bounds().Size()
					if a.mask == nil || fbdims != a.mask.Bounds().Size() {
						a.mask = image.NewAlpha(image.Rectangle{Max: fbdims})
					}
					a.root.Draw(fb, a.mask)
				}
				for {
					if !yield() {
						return
					}
					wakeup := a.ctx.Wakeup
					a.ctx.Reset()
					for _, e := range a.ctx.Platform.AppendEvents(wakeup, evts[:0]) {
						a.idle.start = a.ctx.Platform.Now()
						if se, ok := e.AsSDCard(); ok {
							a.ctx.EmptySDSlot = !se.Inserted
						} else {
							a.ctx.Events(e)
						}
					}
					idleWakeup := a.idle.start.Add(idleTimeout)
					now := a.ctx.Platform.Now()
					idle := now.Sub(idleWakeup) >= 0
					if a.idle.active != idle {
						a.idle.active = idle
						if idle {
							a.idle.state = saver.State{}
						} else {
							// The screen saver has invalidated the cached
							// frame content.
							a.root = op.Ops{}
						}
					}
					if a.idle.active {
						a.idle.state.Draw(a.ctx.Platform)
						// Throttle screen saver speed.
						const minFrameTime = 40 * time.Millisecond
						a.ctx.WakeupAt(now.Add(minFrameTime))
						continue
					}
					a.ctx.WakeupAt(idleWakeup)
					break
				}
				a.root.Reset()
			}
		}
	}
}

func rgb(c uint32) color.NRGBA {
	return argb(0xff000000 | c)
}

func argb(c uint32) color.NRGBA {
	return color.NRGBA{A: uint8(c >> 24), R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c)}
}

func (f FrameEvent) Event() Event {
	e := Event{typ: frameEvent}
	e.refs[0] = f.Error
	e.refs[1] = f.Image
	return e
}

func (b ButtonEvent) Event() Event {
	pressed := uint32(0)
	if b.Pressed {
		pressed = 1
	}
	e := Event{typ: buttonEvent}
	e.data[0] = uint32(b.Button)
	e.data[1] = pressed
	e.data[2] = uint32(b.Rune)
	return e
}

func (s SDCardEvent) Event() Event {
	e := Event{typ: sdcardEvent}
	if s.Inserted {
		e.data[0] = 1
	}
	return e
}

func (e Event) AsFrame() (FrameEvent, bool) {
	if e.typ != frameEvent {
		return FrameEvent{}, false
	}
	f := FrameEvent{}
	if r := e.refs[0]; r != nil {
		f.Error = r.(error)
	}
	if r := e.refs[1]; r != nil {
		f.Image = r.(image.Image)
	}
	return f, true
}

func (e Event) AsButton() (ButtonEvent, bool) {
	if e.typ != buttonEvent {
		return ButtonEvent{}, false
	}
	return ButtonEvent{
		Button:  Button(e.data[0]),
		Pressed: e.data[1] != 0,
		Rune:    rune(e.data[2]),
	}, true
}

func (e Event) AsSDCard() (SDCardEvent, bool) {
	if e.typ != sdcardEvent {
		return SDCardEvent{}, false
	}
	return SDCardEvent{
		Inserted: e.data[0] != 0,
	}, true
}
