// package gui implements the SeedEtcher controller user interface.
package gui

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"math"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"seedetcher.com/address"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip32"
	"seedetcher.com/bip39"
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/layout"
	"seedetcher.com/gui/op"
	"seedetcher.com/gui/saver"
	"seedetcher.com/gui/text"
	"seedetcher.com/gui/widget"
	"seedetcher.com/printer"
)

const nbuttons = 8

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

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

type program int

const (
	backupWallet program = iota
)

type richText struct {
	Y int
}

func (r *richText) Add(ops op.Ctx, style text.Style, width int, col color.NRGBA, format string, args ...any) {
	m := style.Face.Metrics()
	offy := r.Y + m.Ascent.Ceil()
	lheight := style.LineHeight()
	l := &text.Layout{
		MaxWidth: width,
		Style:    style,
	}
	for {
		g, ok := l.Next(format, args...)
		if !ok {
			break
		}
		if g.Rune == '\n' {
			offy += lheight
			continue
		}
		off := image.Pt(g.Dot.Round(), offy)
		op.Offset(ops, off)
		op.GlyphOp(ops, style.Face, g.Rune)
		op.ColorOp(ops, col)
	}
	r.Y = offy + m.Descent.Ceil()
}

func ShowAddressesScreen(ctx *Context, ops op.Ctx, th *Colors, desc urtypes.OutputDescriptor) {
	var s struct {
		addresses [2][]string
		page      int
		scroll    int
	}

	counter := 0
	for page := range len(s.addresses) {
		for len(s.addresses[page]) < 20 {
			var addr string
			var err error
			switch page {
			case 0:
				addr, err = address.Receive(desc, uint32(counter))
			case 1:
				addr, err = address.Change(desc, uint32(counter))
			}
			counter++
			if err != nil {
				// Very unlikely.
				continue
			}
			const addrLen = 12
			fmtAddr := fmt.Sprintf("%d: %s", len(s.addresses[page])+1, shortenAddress(addrLen, addr))
			s.addresses[page] = append(s.addresses[page], fmtAddr)
		}
	}

	const maxPage = len(s.addresses)
	inp := new(InputTracker)
	for {
		scrollDelta := 0
		for {
			e, ok := inp.Next(ctx, Button1, Left, Right, Up, Down)
			if !ok {
				break
			}
			switch e.Button {
			case Button1:
				if inp.Clicked(e.Button) {
					return
				}
			case Left:
				if e.Pressed {
					s.page = (s.page - 1 + maxPage) % maxPage
					s.scroll = 0
				}
			case Right:
				if e.Pressed {
					s.page = (s.page + 1) % maxPage
					s.scroll = 0
				}
			case Up:
				if e.Pressed {
					scrollDelta--
				}
			case Down:
				if e.Pressed {
					scrollDelta++
				}
			}
		}
		op.ColorOp(ops, th.Background)
		dims := ctx.Platform.DisplaySize()

		// Title.
		r := layout.Rectangle{Max: dims}
		title := "Receive"
		if s.page == 1 {
			title = "Change"
		}
		layoutTitle(ctx, ops, dims.X, th.Text, "%s", title)

		op.ImageOp(ops.Begin(), assets.ArrowLeft, true)
		op.ColorOp(ops, th.Text)
		left := ops.End()

		op.ImageOp(ops.Begin(), assets.ArrowRight, true)
		op.ColorOp(ops, th.Text)
		right := ops.End()

		leftsz := assets.ArrowLeft.Bounds().Size()
		rightsz := assets.ArrowRight.Bounds().Size()

		content := r.Shrink(0, 12, 0, 12)
		body := content.Shrink(leadingSize, rightsz.X+12, 0, leftsz.X+12)
		inner := body.Shrink(scrollFadeDist, 0, scrollFadeDist, 0)

		op.Position(ops, left, content.W(leftsz))
		op.Position(ops, right, content.E(rightsz))

		var bodytxt richText
		ops.Begin()
		addrs := s.addresses[s.page]
		for _, addr := range addrs {
			ops := ops
			bodytxt.Add(ops, ctx.Styles.body, inner.Dx(), th.Text, "%s", addr)
		}
		addresses := ops.End()

		s.scroll += scrollDelta * body.Dy() / 2
		maxScroll := bodytxt.Y - inner.Dy()
		s.scroll = min(max(0, s.scroll), maxScroll)
		pos := inner.Min.Sub(image.Pt(0, s.scroll))
		op.Position(ops.Begin(), addresses, pos)
		fadeClip(ops, ops.End(), image.Rectangle(body))

		layoutNavigation(inp, ops, th, dims, []NavButton{{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack}}...)
		ctx.Frame()
	}
}

func shortenAddress(n int, addr string) string {
	if len(addr) <= n {
		return addr
	}
	return addr[:n/2] + "......" + addr[len(addr)-n/2:]
}

func descriptorKeyIdx(desc urtypes.OutputDescriptor, m bip39.Mnemonic, pass string) (int, bool) {
	if len(desc.Keys) == 0 {
		return 0, false
	}
	network := desc.Keys[0].Network
	seed := bip39.MnemonicSeed(m, pass)
	mk, err := hdkeychain.NewMaster(seed, network)
	if err != nil {
		return 0, false
	}
	for i, k := range desc.Keys {
		_, xpub, err := bip32.Derive(mk, k.DerivationPath)
		if err != nil {
			// A derivation that generates an invalid key is by itself very unlikely,
			// but also means that the seed doesn't match this xpub.
			continue
		}
		if k.String() == xpub.String() {
			return i, true
		}
	}
	return 0, false
}

// scaleRot is a specialized function for fast scaling and rotation of
// the camera frames for display.
func scaleRot(dst, src *image.Gray, rot180 bool) {
	db := dst.Bounds()
	sb := src.Bounds()
	if db.Empty() {
		return
	}
	scale := sb.Dx() / db.Dx()
	for y := 0; y < db.Dy(); y++ {
		sx := sb.Max.X - 1 - y*scale
		dy := db.Max.Y - y
		if rot180 {
			dy = y + db.Min.Y
		}
		for x := 0; x < db.Dx(); x++ {
			sy := x*scale + sb.Min.Y
			c := src.GrayAt(sx, sy)
			dx := db.Max.X - 1 - x
			if rot180 {
				dx = x + db.Min.X
			}
			dst.SetGray(dx, dy, c)
		}
	}
}

type ErrorScreen struct {
	Title string
	Body  string
	w     Warning
	inp   InputTracker
}

func (s *ErrorScreen) Layout(ctx *Context, ops op.Ctx, th *Colors, dims image.Point) bool {
	for {
		e, ok := s.inp.Next(ctx, Button3)
		if !ok {
			break
		}
		switch e.Button {
		case Button3:
			if s.inp.Clicked(e.Button) {
				return true
			}
		}
	}
	s.w.Layout(ctx, ops, th, dims, s.Title, s.Body)
	layoutNavigation(&s.inp, ops, th, dims, []NavButton{{Button: Button3, Style: StylePrimary, Icon: assets.IconCheckmark}}...)
	return false
}

type ConfirmWarningScreen struct {
	Title string
	Body  string
	Icon  image.RGBA64Image

	warning Warning
	confirm ConfirmDelay
	inp     InputTracker
}

type Warning struct {
	scroll  int
	txtclip int
	inp     InputTracker
}

type ConfirmResult int

const (
	ConfirmNone ConfirmResult = iota
	ConfirmNo
	ConfirmYes
)

type ConfirmDelay struct {
	timeout time.Time
}

func (c *ConfirmDelay) Start(ctx *Context, delay time.Duration) {
	c.timeout = ctx.Platform.Now().Add(delay)
}

func (c *ConfirmDelay) Progress(ctx *Context) float32 {
	if c.timeout.IsZero() {
		return 0.
	}
	now := ctx.Platform.Now()
	d := c.timeout.Sub(now)
	if d <= 0 {
		return 1.
	}
	ctx.Platform.Wakeup()
	return 1. - float32(d.Seconds()/confirmDelay.Seconds())
}

const confirmDelay = 1 * time.Second

func (w *Warning) Layout(ctx *Context, ops op.Ctx, th *Colors, dims image.Point, title, txt string) image.Point {
	for {
		e, ok := w.inp.Next(ctx, Up, Down)
		if !ok {
			break
		}
		switch e.Button {
		case Up:
			if e.Pressed {
				w.scroll -= w.txtclip / 2
			}
		case Down:
			if e.Pressed {
				w.scroll += w.txtclip / 2
			}
		}
	}
	const btnMargin = 4
	const boxMargin = 6

	op.ColorOp(ops, color.NRGBA{A: theme.overlayMask})

	wbbg := assets.WarningBoxBg
	wbout := assets.WarningBoxBorder
	ptop, pend, pbottom, pstart := wbbg.Padding()
	r := image.Rectangle{
		Min: image.Pt(pstart+boxMargin, ptop+boxMargin),
		Max: image.Pt(dims.X-pend-boxMargin, dims.Y-pbottom-boxMargin),
	}
	box := wbbg.Add(ops, r, true)
	op.ColorOp(ops, th.Background)
	wbout.Add(ops, r, true)
	op.ColorOp(ops, th.Text)

	btnOff := assets.NavBtnPrimary.Bounds().Dx() + btnMargin
	titlesz := widget.Labelwf(ops.Begin(), ctx.Styles.warning, dims.X-btnOff*2, th.Text, "%s", strings.ToTitle(title))
	titlew := ops.End()
	op.Position(ops, titlew, image.Pt((dims.X-titlesz.X)/2, r.Min.Y))

	bodyClip := image.Rectangle{
		Min: image.Pt(pstart+boxMargin, ptop+titlesz.Y),
		Max: image.Pt(dims.X-btnOff, dims.Y-pbottom-boxMargin),
	}
	bodysz := widget.Labelwf(ops.Begin(), ctx.Styles.body, bodyClip.Dx(), th.Text, "%s", txt)
	body := ops.End()
	innerCtx := ops.Begin()
	w.txtclip = bodyClip.Dy()
	maxScroll := bodysz.Y - (bodyClip.Dy() - 2*scrollFadeDist)
	if w.scroll > maxScroll {
		w.scroll = maxScroll
	}
	if w.scroll < 0 {
		w.scroll = 0
	}
	op.Position(innerCtx, body, image.Pt(bodyClip.Min.X, bodyClip.Min.Y+scrollFadeDist-w.scroll))
	fadeClip(ops, ops.End(), image.Rectangle(bodyClip))

	return box.Bounds().Size()
}

func (s *ConfirmWarningScreen) Layout(ctx *Context, ops op.Ctx, th *Colors, dims image.Point) ConfirmResult {
	var progress float32
	for {
		progress = s.confirm.Progress(ctx)
		if progress == 1 {
			return ConfirmYes
		}
		e, ok := s.inp.Next(ctx, Button3, Button1)
		if !ok {
			break
		}
		switch e.Button {
		case Button1:
			if s.inp.Clicked(e.Button) {
				return ConfirmNo
			}
		case Button3:
			if e.Pressed {
				s.confirm.Start(ctx, confirmDelay)
			} else {
				s.confirm = ConfirmDelay{}
			}
		}
	}
	s.warning.Layout(ctx, ops, th, dims, s.Title, s.Body)
	layoutNavigation(&s.inp, ops, th, dims, []NavButton{
		{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
		{Button: Button3, Style: StylePrimary, Icon: s.Icon, Progress: progress},
	}...)
	return ConfirmNone
}

type ProgressImage struct {
	Progress float32
	Src      image.RGBA64Image
}

func (p *ProgressImage) Add(ctx op.Ctx) {
	op.ParamImageOp(ctx, ProgressImageGen, true, p.Src.Bounds(), []any{p.Src}, []uint32{math.Float32bits(p.Progress)})
}

var ProgressImageGen = op.RegisterParameterizedImage(func(args op.ImageArguments, x, y int) color.RGBA64 {
	src := args.Refs[0].(image.RGBA64Image)
	progress := math.Float32frombits(args.Args[0])
	b := src.Bounds()
	c := b.Max.Add(b.Min).Div(2)
	d := image.Pt(x, y).Sub(c)
	angle := float32(math.Atan2(float64(d.X), float64(d.Y)))
	angle = math.Pi - angle
	if angle > 2*math.Pi*progress {
		return color.RGBA64{}
	}
	return src.RGBA64At(x, y)
})

type errDuplicateKey struct {
	Fingerprint uint32
}

func (e *errDuplicateKey) Error() string {
	return fmt.Sprintf("descriptor contains a duplicate share: %.8x", e.Fingerprint)
}

func (e *errDuplicateKey) Is(target error) bool {
	_, ok := target.(*errDuplicateKey)
	return ok
}

func NewErrorScreen(err error) *ErrorScreen {
	var errDup *errDuplicateKey
	switch {
	case errors.As(err, &errDup):
		return &ErrorScreen{
			Title: "Duplicated Share",
			Body:  fmt.Sprintf("The share %.8x is listed more than once in the wallet.", errDup.Fingerprint),
		}
	default:
		return &ErrorScreen{
			Title: "Error",
			Body:  err.Error(),
		}
	}
}

type ChoiceScreen struct {
	Title   string
	Lead    string
	Choices []string
	choice  int
}

func (s *ChoiceScreen) Choose(ctx *Context, ops op.Ctx, th *Colors) (int, bool) {
	inp := new(InputTracker)
	for {
		for {
			e, ok := inp.Next(ctx, Button1, Button3, Center, Up, Down, CCW, CW)
			if !ok {
				break
			}
			switch e.Button {
			case Button1:
				if inp.Clicked(e.Button) {
					return 0, false
				}
			case Button3, Center:
				if inp.Clicked(e.Button) {
					return s.choice, true
				}
			case Up, CCW:
				if e.Pressed {
					if s.choice > 0 {
						s.choice--
					}
				}
			case Down, CW:
				if e.Pressed {
					if s.choice < len(s.Choices)-1 {
						s.choice++
					}
				}
			}
		}

		dims := ctx.Platform.DisplaySize()
		s.Draw(ctx, ops, th, dims)

		layoutNavigation(inp, ops, th, dims, []NavButton{
			{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
			{Button: Button3, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		ctx.Frame()
	}
}

func (s *ChoiceScreen) Draw(ctx *Context, ops op.Ctx, th *Colors, dims image.Point) {
	r := layout.Rectangle{Max: dims}
	op.ColorOp(ops, th.Background)

	layoutTitle(ctx, ops, dims.X, th.Text, "%s", s.Title)

	_, bottom := r.CutTop(leadingSize)
	sz := widget.Labelwf(ops.Begin(), ctx.Styles.lead, dims.X-2*8, th.Text, "%s", s.Lead)
	content, lead := bottom.CutBottom(leadingSize)
	op.Position(ops, ops.End(), lead.Center(sz))

	content = content.Shrink(16, 0, 16, 0)

	children := make([]struct {
		Size image.Point
		W    op.CallOp
	}, len(s.Choices))
	maxW := 0
	for i, c := range s.Choices {
		style := ctx.Styles.button
		col := th.Text
		if i == s.choice {
			col = th.Background
		}
		sz := widget.Labelf(ops.Begin(), style, col, "%s", c)
		ch := ops.End()
		children[i].Size = sz
		children[i].W = ch
		if sz.X > maxW {
			maxW = sz.X
		}
	}

	inner := ops.Begin()
	h := 0
	for i, c := range children {
		xoff := (maxW - c.Size.X) / 2
		pos := image.Pt(xoff, h)
		txt := c.W
		if i == s.choice {
			bg := image.Rectangle{Max: c.Size}
			bg.Min.X -= xoff
			bg.Max.X += xoff
			assets.ButtonFocused.Add(inner.Begin(), bg, true)
			op.ColorOp(inner, th.Text)
			txt.Add(inner)
			txt = inner.End()
		}
		op.Position(inner, txt, pos)
		h += c.Size.Y
	}
	op.Position(ops, ops.End(), content.Center(image.Pt(maxW, h)))
}

func mainFlow(ctx *Context, ops op.Ctx) {
	var page program
	inp := new(InputTracker)
	for {
		dims := ctx.Platform.DisplaySize()
	events:
		for {
			e, ok := inp.Next(ctx, Button3, Center, Left, Right)
			if !ok {
				break
			}
			switch e.Button {
			case Button3, Center:
				if !inp.Clicked(e.Button) {
					break
				}
				ws := &ConfirmWarningScreen{
					Title: "Remove SD card",
					Body:  "Remove SD card to continue.\n\nHold button to ignore this warning.",
					Icon:  assets.IconRight,
				}
				th := mainScreenTheme(page)
			loop:
				for !ctx.EmptySDSlot {
					res := ws.Layout(ctx, ops.Begin(), th, dims)
					dialog := ops.End()
					switch res {
					case ConfirmYes:
						break loop
					case ConfirmNo:
						continue events
					}
					drawMainScreen(ctx, ops, dims, page)
					dialog.Add(ops)
					ctx.Frame()
				}
				ctx.EmptySDSlot = true
				switch page {
				case backupWallet:
					backupWalletFlow(ctx, ops, th)
				}
			case Left:
				if !e.Pressed {
					break
				}
				page--
				if page < 0 {
					page = backupWallet
				}
			case Right:
				if !e.Pressed {
					break
				}
				page++
				if page > backupWallet {
					page = 0
				}
			}
		}
		drawMainScreen(ctx, ops, dims, page)
		layoutNavigation(inp, ops, mainScreenTheme(page), dims, []NavButton{
			{Button: Button3, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		ctx.Frame()
	}
}

func mainScreenTheme(page program) *Colors {
	switch page {
	case backupWallet:
		return &descriptorTheme
	default:
		panic("invalid page")
	}
}

func drawMainScreen(ctx *Context, ops op.Ctx, dims image.Point, page program) {
	var th *Colors
	var title string
	th = mainScreenTheme(page)
	switch page {
	case backupWallet:
		title = "SeedEtcher Backup"
	}
	op.ColorOp(ops, th.Background)

	layoutTitle(ctx, ops, dims.X, th.Text, "%s", title)

	r := layout.Rectangle{Max: dims}
	sz := layoutMainPage(ops.Begin(), th, dims.X, page)
	op.Position(ops, ops.End(), r.Center(sz))

	sz = layoutMainPager(ops.Begin(), th, page)
	_, footer := r.CutBottom(leadingSize)
	op.Position(ops, ops.End(), footer.Center(sz))

	versz := widget.Labelwf(ops.Begin(), ctx.Styles.debug, 100, th.Text, "%s", ctx.Version)
	op.Position(ops, ops.End(), r.SE(versz.Add(image.Pt(4, 0))))
	shsz := widget.Labelwf(ops.Begin(), ctx.Styles.debug, 100, th.Text, "SeedEtcher")
	op.Position(ops, ops.End(), r.SW(shsz).Add(image.Pt(3, 0)))
}

func layoutTitle(ctx *Context, ops op.Ctx, width int, col color.NRGBA, title string, args ...any) image.Rectangle {
	const margin = 8
	sz := widget.Labelwf(ops.Begin(), ctx.Styles.title, width-2*16, col, title, args...)
	pos := image.Pt((width-sz.X)/2, margin)
	op.Position(ops, ops.End(), pos)
	return image.Rectangle{
		Min: pos,
		Max: pos.Add(sz),
	}
}

type ButtonStyle int

const (
	StyleNone ButtonStyle = iota
	StyleSecondary
	StylePrimary
)

type NavButton struct {
	Button   Button
	Style    ButtonStyle
	Icon     image.Image
	Progress float32
}

func layoutNavigation(inp *InputTracker, ops op.Ctx, th *Colors, dims image.Point, btns ...NavButton) image.Rectangle {
	navsz := assets.NavBtnPrimary.Bounds().Size()
	button := func(ops op.Ctx, b NavButton, pressed bool) {
		if b.Style == StyleNone {
			return
		}
		switch b.Style {
		case StyleSecondary:
			op.ImageOp(ops, assets.NavBtnPrimary, true)
			op.ColorOp(ops, th.Background)
			op.ImageOp(ops, assets.NavBtnSecondary, true)
			op.ColorOp(ops, th.Text)
		case StylePrimary:
			op.ImageOp(ops, assets.NavBtnPrimary, true)
			op.ColorOp(ops, th.Primary)
		}
		if b.Progress > 0 {
			(&ProgressImage{
				Progress: b.Progress,
				Src:      assets.IconProgress,
			}).Add(ops)
		} else {
			op.ImageOp(ops, b.Icon, true)
		}
		switch b.Style {
		case StyleSecondary:
			op.ColorOp(ops, th.Text)
		case StylePrimary:
			op.ColorOp(ops, th.Text)
		}
		if b.Progress == 0 && pressed {
			op.ImageOp(ops, assets.NavBtnPrimary, true)
			op.ColorOp(ops, color.NRGBA{A: theme.activeMask})
		}
	}
	btnsz := assets.NavBtnPrimary.Bounds().Size()
	ys := [3]int{
		leadingSize,
		(dims.Y - btnsz.Y) / 2,
		dims.Y - leadingSize - btnsz.Y,
	}
	var r image.Rectangle
	for _, b := range btns {
		idx := int(b.Button - Button1)
		button(ops.Begin(), b, inp.Pressed[b.Button])
		y := ys[idx]
		pos := image.Pt(dims.X-btnsz.X, y)
		op.Position(ops, ops.End(), pos)
		r = r.Union(image.Rectangle{
			Min: pos,
			Max: pos.Add(navsz),
		})
	}
	return r
}

func layoutMainPage(ops op.Ctx, th *Colors, width int, page program) image.Point {
	var h layout.Align

	op.ImageOp(ops.Begin(), assets.ArrowLeft, true)
	op.ColorOp(ops, th.Text)
	left := ops.End()
	leftsz := h.Add(assets.ArrowLeft.Bounds().Size())

	op.ImageOp(ops.Begin(), assets.ArrowRight, true)
	op.ColorOp(ops, th.Text)
	right := ops.End()
	rightsz := h.Add(assets.ArrowRight.Bounds().Size())

	contentsz := h.Add(layoutMainPlates(ops.Begin(), page))
	content := ops.End()

	const margin = 16

	op.Position(ops, content, image.Pt((width-contentsz.X)/2, 8+h.Y(contentsz)))
	const npage = int(backupWallet) + 1
	if npage > 1 {
		op.Position(ops, left, image.Pt(margin, h.Y(leftsz)))
		op.Position(ops, right, image.Pt(width-margin-rightsz.X, h.Y(rightsz)))
	}

	return image.Pt(width, h.Size.Y)
}

func layoutMainPlates(ops op.Ctx, page program) image.Point {
	switch page {
	case backupWallet:
		img := assets.Hammer
		op.ImageOp(ops, img, false)
		return img.Bounds().Size()
	}
	panic("invalid page")
}

func layoutMainPager(ops op.Ctx, th *Colors, page program) image.Point {
	const npages = int(backupWallet) + 1
	const space = 4
	if npages <= 1 {
		return image.Point{}
	}
	sz := assets.CircleFilled.Bounds().Size()
	for i := 0; i < npages; i++ {
		op.Offset(ops, image.Pt((sz.X+space)*i, 0))
		mask := assets.Circle
		if i == int(page) {
			mask = assets.CircleFilled
		}
		op.ImageOp(ops, mask, true)
		op.ColorOp(ops, th.Text)
	}
	return image.Pt((sz.X+space)*npages-space, sz.Y)
}

func confirmWarning(ctx *Context, ops op.Ctx, th *Colors, w *ConfirmWarningScreen) bool {
	for {
		dims := ctx.Platform.DisplaySize()
		switch w.Layout(ctx, ops, th, dims) {
		case ConfirmYes:
			return true
		case ConfirmNo:
			return false
		}
		ctx.Frame()
	}
}

func showError(ctx *Context, ops op.Ctx, th *Colors, err error) {
	scr := NewErrorScreen(err)
	for {
		dims := ctx.Platform.DisplaySize()
		if scr.Layout(ctx, ops, th, dims) {
			break
		}
		ctx.Frame()
	}
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
