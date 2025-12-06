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
	"unicode/utf8"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
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
	"seedetcher.com/logutil"
	"seedetcher.com/nonstandard"
	"seedetcher.com/printer"
	"seedetcher.com/seedqr"
)

const nbuttons = 8

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

const maxTitleLen = 18

func sanitizeTitle(title string) string {
	title = strings.ToUpper(title)
	var b strings.Builder
	for _, r := range title {
		if b.Len() >= maxTitleLen {
			break
		}
		if !utf8.ValidRune(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func mnemonicString(m bip39.Mnemonic) string {
	var sb strings.Builder
	for i, w := range m {
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(bip39.LabelFor(w))
	}
	return sb.String()
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

const longestWord = "TOMORROW"

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

func deriveMasterKey(m bip39.Mnemonic, net *chaincfg.Params) (*hdkeychain.ExtendedKey, bool) {
	seed := bip39.MnemonicSeed(m, "")
	mk, err := hdkeychain.NewMaster(seed, net)
	// Err is only non-nil if the seed generates an invalid key, or we made a mistake.
	// According to [0] the odds of encountering a seed that generates
	// an invalid key by chance is 1 in 2^127.
	//
	// [0] https://bitcoin.stackexchange.com/questions/53180/bip-32-seed-resulting-in-an-invalid-private-key
	return mk, err == nil
}

func masterFingerprintFor(m bip39.Mnemonic, network *chaincfg.Params) (uint32, error) {
	mk, ok := deriveMasterKey(m, network)
	if !ok {
		return 0, errors.New("failed to derive mnemonic master key")
	}
	mfp, _, err := bip32.Derive(mk, urtypes.Path{0})
	if err != nil {
		return 0, err
	}
	return mfp, nil
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

func validateDescriptor(desc urtypes.OutputDescriptor) error {
	keys := make(map[string]bool)
	for _, k := range desc.Keys {
		xpub := k.String()
		if keys[xpub] {
			return &errDuplicateKey{Fingerprint: k.MasterFingerprint}
		}
		keys[xpub] = true
	}
	return nil
}

func isEmptyMnemonic(m bip39.Mnemonic) bool {
	for _, w := range m {
		if w != -1 {
			return false
		}
	}
	return true
}

func emptyMnemonic(nwords int) bip39.Mnemonic {
	m := make(bip39.Mnemonic, nwords)
	for i := range m {
		m[i] = -1
	}
	return m
}

const scrollFadeDist = 16

func fadeClip(ops op.Ctx, w op.CallOp, r image.Rectangle) {
	op.ParamImageOp(ops, scrollMask, true, r, nil, nil)
	op.Position(ops, w, image.Pt(0, 0))
}

var scrollMask = op.RegisterParameterizedImage(func(args op.ImageArguments, x, y int) color.RGBA64 {
	alpha := 0xffff
	if d := y - args.Bounds.Min.Y; d < scrollFadeDist {
		alpha = 0xffff * d / scrollFadeDist
	} else if d := args.Bounds.Max.Y - y; d < scrollFadeDist {
		alpha = 0xffff * d / scrollFadeDist
	}
	a16 := uint16(alpha)
	return color.RGBA64{A: a16}
})

func inputWordsFlow(ctx *Context, ops op.Ctx, th *Colors, mnemonic bip39.Mnemonic, selected int) {
	kbd := NewKeyboard(ctx)
	inp := new(InputTracker)
	for {
		for {
			kbd.Update(ctx)
			e, ok := inp.Next(ctx, Button1, Button2)
			if !ok {
				break
			}
			switch e.Button {
			case Button1:
				if inp.Clicked(e.Button) {
					return
				}
			case Button2:
				if !inp.Clicked(e.Button) {
					break
				}
				w, complete := kbd.Complete()
				if !complete {
					break
				}
				kbd.Clear()
				mnemonic[selected] = w
				for {
					selected++
					if selected == len(mnemonic) {
						return
					}
					if mnemonic[selected] == -1 {
						break
					}
				}
			}
		}
		dims := ctx.Platform.DisplaySize()
		completedWord, complete := kbd.Complete()
		op.ColorOp(ops, th.Background)
		layoutTitle(ctx, ops, dims.X, th.Text, "Input Words")

		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		content, _ = content.CutBottom(8)

		kbdsz := kbd.Layout(ctx, ops.Begin(), th)
		op.Position(ops, ops.End(), content.S(kbdsz))

		layoutWord := func(ops op.Ctx, n int, word string) image.Point {
			style := ctx.Styles.word
			return widget.Labelf(ops, style, th.Background, "%2d: %s", n, word)
		}

		longest := layoutWord(op.Ctx{}, 24, longestWord)
		hint := kbd.Word
		if complete {
			hint = strings.ToUpper(bip39.LabelFor(completedWord))
		}
		layoutWord(ops.Begin(), selected+1, hint)
		word := ops.End()
		r := image.Rectangle{Max: longest}
		r.Min.Y -= 3
		assets.ButtonFocused.Add(ops.Begin(), r, true)
		op.ColorOp(ops, th.Text)
		word.Add(ops)
		top, _ := content.CutBottom(kbdsz.Y)
		op.Position(ops, ops.End(), top.Center(longest))

		layoutNavigation(inp, ops, th, dims, []NavButton{{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack}}...)
		if complete {
			layoutNavigation(inp, ops, th, dims, []NavButton{{Button: Button2, Style: StylePrimary, Icon: assets.IconCheckmark}}...)
		}
		ctx.Frame()
	}
}

var kbdKeys = [...][]rune{
	[]rune("QWERTYUIOP"),
	[]rune("ASDFGHJKL"),
	[]rune("ZXCVBNM⌫"),
}

type Keyboard struct {
	Word string

	nvalid    int
	positions [len(kbdKeys)][]image.Point
	widest    image.Point
	backspace image.Point
	size      image.Point

	mask     uint32
	row, col int
	inp      InputTracker
}

func NewKeyboard(ctx *Context) *Keyboard {
	k := new(Keyboard)
	k.widest = ctx.Styles.keyboard.Measure(math.MaxInt, "W")
	bsb := assets.KeyBackspace.Bounds()
	bsWidth := bsb.Min.X*2 + bsb.Dx()
	k.backspace = image.Pt(bsWidth, k.widest.Y)
	bgbnds := assets.Key.Bounds(image.Rectangle{Max: k.widest})
	const margin = 2
	bgsz := bgbnds.Size().Add(image.Pt(margin, margin))
	longest := 0
	for _, row := range kbdKeys {
		if n := len(row); n > longest {
			longest = n
		}
	}
	maxw := longest*bgsz.X - margin
	for i, row := range kbdKeys {
		n := len(row)
		if i == len(kbdKeys)-1 {
			// Center row without the backspace key.
			n--
		}
		w := bgsz.X*n - margin
		off := image.Pt((maxw-w)/2, 0)
		for j := range row {
			pos := image.Pt(j*bgsz.X, i*bgsz.Y)
			pos = pos.Add(off)
			pos = pos.Sub(bgbnds.Min)
			k.positions[i] = append(k.positions[i], pos)
		}
	}
	k.size = image.Point{
		X: maxw,
		Y: len(kbdKeys)*bgsz.Y - margin,
	}
	k.Clear()
	return k
}

func (k *Keyboard) Complete() (bip39.Word, bool) {
	word := strings.ToLower(k.Word)
	w, ok := bip39.ClosestWord(word)
	if !ok {
		return -1, false
	}
	// The word is complete if it's in the word list or is the only option.
	return w, k.nvalid == 1 || word == bip39.LabelFor(w)
}

func (k *Keyboard) Clear() {
	k.Word = ""
	k.updateMask()
	k.row = len(kbdKeys) / 2
	k.col = len(kbdKeys[k.row]) / 2
	k.adjust(false)
}

func (k *Keyboard) updateMask() {
	k.mask = ^uint32(0)
	word := strings.ToLower(k.Word)
	w, valid := bip39.ClosestWord(word)
	if !valid {
		return
	}
	k.nvalid = 0
	for ; w < bip39.NumWords; w++ {
		bip39w := bip39.LabelFor(w)
		if !strings.HasPrefix(bip39w, word) {
			break
		}
		k.nvalid++
		suffix := bip39w[len(word):]
		if len(suffix) > 0 {
			r := rune(strings.ToUpper(suffix)[0])
			idx, valid := k.idxForRune(r)
			if !valid {
				panic("valid by construction")
			}
			k.mask &^= 1 << idx
		}
	}
	if k.nvalid == 1 {
		k.mask = ^uint32(0)
	}
}

func (k *Keyboard) idxForRune(r rune) (int, bool) {
	idx := int(r - 'A')
	if idx < 0 || idx >= 32 {
		return 0, false
	}
	return idx, true
}

func (k *Keyboard) Valid(r rune) bool {
	if r == '⌫' {
		return len(k.Word) > 0
	}
	idx, valid := k.idxForRune(r)
	return valid && k.mask&(1<<idx) == 0
}

func (k *Keyboard) Update(ctx *Context) {
	for {
		e, ok := k.inp.Next(ctx, Left, Right, Up, Down, CCW, CW, Center, Rune, Button3)
		if !ok {
			break
		}
		if !e.Pressed {
			continue
		}
		switch e.Button {
		case Left, CCW:
			next := k.col
			for {
				next--
				if next == -1 {
					if e.Button == CCW {
						nrows := len(kbdKeys)
						k.row = (k.row - 1 + nrows) % nrows
					}
					next = len(kbdKeys[k.row]) - 1
				}
				if !k.Valid(kbdKeys[k.row][next]) {
					continue
				}
				k.col = next
				k.adjust(true)
				break
			}
		case Right, CW:
			next := k.col
			for {
				next++
				if next == len(kbdKeys[k.row]) {
					if e.Button == CW {
						nrows := len(kbdKeys)
						k.row = (k.row + 1 + nrows) % nrows
					}
					next = 0
				}
				if !k.Valid(kbdKeys[k.row][next]) {
					continue
				}
				k.col = next
				k.adjust(true)
				break
			}
		case Up:
			n := len(kbdKeys)
			next := k.row
			for {
				next = (next - 1 + n) % n
				if k.adjustCol(next) {
					k.adjust(true)
					break
				}
			}
		case Down:
			n := len(kbdKeys)
			next := k.row
			for {
				next = (next + 1) % n
				if k.adjustCol(next) {
					k.adjust(true)
					break
				}
			}
		case Rune:
			k.rune(e.Rune)
		case Center, Button3:
			r := kbdKeys[k.row][k.col]
			k.rune(r)
		}
	}
}

func (k *Keyboard) rune(r rune) {
	if !k.Valid(r) {
		return
	}
	if r == '⌫' {
		_, n := utf8.DecodeLastRuneInString(k.Word)
		k.Word = k.Word[:len(k.Word)-n]
	} else {
		k.Word = k.Word + string(r)
	}
	k.updateMask()
	k.adjust(r == '⌫')
}

// adjust resets the row and column to the nearest valid key, if any.
func (k *Keyboard) adjust(allowBackspace bool) {
	dist := int(1e6)
	current := k.positions[k.row][k.col]
	found := false
	for i, row := range kbdKeys {
		j := 0
		for _, key := range row {
			if !k.Valid(key) || key == '⌫' && !allowBackspace {
				j++
				continue
			}
			p := k.positions[i][j]
			d := p.Sub(current)
			d2 := d.X*d.X + d.Y*d.Y
			if d2 < dist {
				dist = d2
				k.row, k.col = i, j
				found = true
			}
			j++
		}
	}
	// Only if no other key was found, select backspace.
	if !found {
		k.row = len(k.positions) - 1
		k.col = len(k.positions[k.row]) - 1
	}
}

// adjustCol sets the column to the one nearest the x position.
func (k *Keyboard) adjustCol(row int) bool {
	dist := int(1e6)
	found := false
	x := k.positions[k.row][k.col].X
	for i, r := range kbdKeys[row] {
		if !k.Valid(r) {
			continue
		}
		p := k.positions[row][i]
		found = true
		k.row = row
		d := p.X - x
		if d < 0 {
			d = -d
		}
		if d < dist {
			dist = d
			k.col = i
		}
	}
	return found
}

func (k *Keyboard) Layout(ctx *Context, ops op.Ctx, th *Colors) image.Point {
	for i, row := range kbdKeys {
		for j, key := range row {
			valid := k.Valid(key)
			bg := assets.Key
			bgsz := k.widest
			if key == '⌫' {
				bgsz = k.backspace
			}
			bgcol := th.Text
			style := ctx.Styles.keyboard
			col := th.Text
			switch {
			case !valid:
				bgcol.A = theme.inactiveMask
				col = bgcol
			case i == k.row && j == k.col:
				bg = assets.KeyActive
				col = th.Background
			}
			var sz image.Point
			if key == '⌫' {
				icn := assets.KeyBackspace
				sz = image.Pt(k.backspace.X, icn.Bounds().Dy())
				op.ImageOp(ops.Begin(), icn, true)
				op.ColorOp(ops, col)
			} else {
				sz = widget.Labelf(ops.Begin(), style, col, "%s", string(key))
			}
			key := ops.End()
			bg.Add(ops.Begin(), image.Rectangle{Max: bgsz}, true)
			op.ColorOp(ops, bgcol)
			op.Position(ops, key, bgsz.Sub(sz).Div(2))
			op.Position(ops, ops.End(), k.positions[i][j])
		}
	}
	return k.size
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

func newMnemonicFlow(ctx *Context, ops op.Ctx, th *Colors, current, total int) (bip39.Mnemonic, bool) {
	cs := &ChoiceScreen{
		Title:   fmt.Sprintf("Input Seed %d/%d", current, total), // Display "Seed X/Y"
		Lead:    "Choose input method",
		Choices: []string{"CAMERA", "KEYBOARD"},
	}
	showErr := func(errScreen *ErrorScreen) {
		for {
			dims := ctx.Platform.DisplaySize()
			dismissed := errScreen.Layout(ctx, ops.Begin(), th, dims)
			d := ops.End()
			if dismissed {
				break
			}
			cs.Draw(ctx, ops, th, dims)
			d.Add(ops)
			ctx.Frame()
		}
	}
outer:
	for {
		choice, ok := cs.Choose(ctx, ops, th)
		if !ok {
			return nil, false
		}
		switch choice {
		case 0: // Camera.
			res, ok := (&ScanScreen{
				Title: fmt.Sprintf("Scan Seed %d/%d", current, total), // Update ScanScreen title
				Lead:  "SeedQR or Mnemonic",
			}).Scan(ctx, ops)
			if !ok {
				continue
			}
			if b, ok := res.([]byte); ok {
				if sqr, ok := seedqr.Parse(b); ok {
					res = sqr
				} else if sqr, err := bip39.ParseMnemonic(strings.ToLower(string(b))); err == nil || errors.Is(err, bip39.ErrInvalidChecksum) {
					res = sqr
				}
			}
			seed, ok := res.(bip39.Mnemonic)
			if !ok {
				showErr(&ErrorScreen{
					Title: "Invalid Seed",
					Body:  "The scanned data does not represent a seed.",
				})
				continue
			}
			return seed, true

		case 1: // Keyboard.
			cs := &ChoiceScreen{
				Title:   fmt.Sprintf("Input Seed %d/%d", current, total), // Update keyboard choice title
				Lead:    "Choose number of words",
				Choices: []string{"12 WORDS", "24 WORDS"},
			}
			for {
				choice, ok := cs.Choose(ctx, ops, th)
				if !ok {
					continue outer
				}
				mnemonic := emptyMnemonic([]int{12, 24}[choice])
				inputWordsFlow(ctx, ops, th, mnemonic, 0)
				if !isEmptyMnemonic(mnemonic) {
					return mnemonic, true
				}
			}
		}
	}
}

type SeedScreen struct {
	selected int
}

func (s *SeedScreen) Confirm(ctx *Context, ops op.Ctx, th *Colors, mnemonic bip39.Mnemonic) bool {
	inp := new(InputTracker)
	for {
	events:
		for {
			e, ok := inp.Next(ctx, Button1, Button2, Center, Button3, Up, Down)
			if !ok {
				break
			}
			switch e.Button {
			case Button1:
				if !inp.Clicked(e.Button) {
					break
				}
				if isEmptyMnemonic(mnemonic) {
					return false
				}
				confirm := &ConfirmWarningScreen{
					Title: "Discard Seed?",
					Body:  "Going back will discard the seed.\n\nHold button to confirm.",
					Icon:  assets.IconDiscard,
				}
				for {
					dims := ctx.Platform.DisplaySize()
					res := confirm.Layout(ctx, ops.Begin(), th, dims)
					d := ops.End()
					switch res {
					case ConfirmNo:
						continue events
					case ConfirmYes:
						return false
					}
					s.Draw(ctx, ops, th, dims, mnemonic)
					d.Add(ops)
					ctx.Frame()
				}
			case Button2, Center:
				if !inp.Clicked(e.Button) {
					break
				}
				inputWordsFlow(ctx, ops, th, mnemonic, s.selected)
				continue
			case Button3:
				if !inp.Clicked(e.Button) || !isMnemonicComplete(mnemonic) {
					break
				}
				showErr := func(scr *ErrorScreen) {
					for {
						dims := ctx.Platform.DisplaySize()
						dismissed := scr.Layout(ctx, ops.Begin(), th, dims)
						d := ops.End()
						if dismissed {
							break
						}
						s.Draw(ctx, ops, th, dims, mnemonic)
						d.Add(ops)
						ctx.Frame()
					}
				}
				if !mnemonic.Valid() {
					scr := &ErrorScreen{
						Title: "Invalid Seed",
					}
					var words []string
					for _, w := range mnemonic {
						words = append(words, bip39.LabelFor(w))
					}
					if nonstandard.ElectrumSeed(strings.Join(words, " ")) {
						scr.Body = "Electrum seeds are not supported."
					} else {
						scr.Body = "The seed phrase is invalid.\n\nCheck the words and try again."
					}
					showErr(scr)
					break
				}
				_, ok = deriveMasterKey(mnemonic, &chaincfg.MainNetParams)
				if !ok {
					showErr(&ErrorScreen{
						Title: "Invalid Seed",
						Body:  "The seed is invalid.",
					})
					break
				}
				return true
			case Down:
				if e.Pressed && s.selected < len(mnemonic)-1 {
					s.selected++
				}
			case Up:
				if e.Pressed && s.selected > 0 {
					s.selected--
				}
			}
		}

		dims := ctx.Platform.DisplaySize()
		s.Draw(ctx, ops, th, dims, mnemonic)

		layoutNavigation(inp, ops, th, dims, []NavButton{
			{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
			{Button: Button2, Style: StyleSecondary, Icon: assets.IconEdit},
		}...)
		if isMnemonicComplete(mnemonic) {
			layoutNavigation(inp, ops, th, dims, []NavButton{
				{Button: Button3, Style: StylePrimary, Icon: assets.IconCheckmark},
			}...)
		}
		ctx.Frame()
	}
}

func isMnemonicComplete(m bip39.Mnemonic) bool {
	for _, w := range m {
		if w == -1 {
			return false
		}
	}
	return len(m) > 0
}

func (s *SeedScreen) Draw(ctx *Context, ops op.Ctx, th *Colors, dims image.Point, mnemonic bip39.Mnemonic) {
	op.ColorOp(ops, th.Background)
	layoutTitle(ctx, ops, dims.X, th.Text, "Confirm Seed")

	style := ctx.Styles.word
	longestPrefix := style.Measure(math.MaxInt, "24: ")
	layoutWord := func(ops op.Ctx, col color.NRGBA, n int, word string) image.Point {
		prefix := widget.Labelf(ops.Begin(), style, col, "%d: ", n)
		op.Position(ops, ops.End(), image.Pt(longestPrefix.X-prefix.X, 0))
		txt := widget.Labelf(ops.Begin(), style, col, "%s", word)
		op.Position(ops, ops.End(), image.Pt(longestPrefix.X, 0))
		return image.Pt(longestPrefix.X+txt.X, txt.Y)
	}

	y := 0
	longest := layoutWord(op.Ctx{}, color.NRGBA{}, 24, longestWord)
	r := layout.Rectangle{Max: dims}
	navw := assets.NavBtnPrimary.Bounds().Dx()
	list := r.Shrink(leadingSize, 0, 0, 0)
	content := list.Shrink(scrollFadeDist, navw, scrollFadeDist, navw)
	lineHeight := longest.Y + 2
	linesPerPage := content.Dy() / lineHeight
	scroll := s.selected - linesPerPage/2
	maxScroll := len(mnemonic) - linesPerPage
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}
	off := content.Min.Add(image.Pt(0, -scroll*lineHeight))
	{
		ops := ops.Begin()
		for i, w := range mnemonic {
			ops.Begin()
			col := th.Text
			if i == s.selected {
				col = th.Background
				r := image.Rectangle{Max: longest}
				r.Min.Y -= 3
				assets.ButtonFocused.Add(ops, r, true)
				op.ColorOp(ops, th.Text)
			}
			word := strings.ToUpper(bip39.LabelFor(w))
			layoutWord(ops, col, i+1, word)
			pos := image.Pt(0, y).Add(off)
			op.Position(ops, ops.End(), pos)
			y += lineHeight
		}
	}
	fadeClip(ops, ops.End(), image.Rectangle(list))
}

func inputDescriptorFlow(ctx *Context, ops op.Ctx, th *Colors, mnemonic bip39.Mnemonic) (*urtypes.OutputDescriptor, bool) {
	originalDesc := ctx.LastDescriptor // Save original
	cs := &ChoiceScreen{
		Title:   "Descriptor",
		Lead:    "Choose input method",
		Choices: []string{"SCAN", "SKIP"},
	}
	if ctx.LastDescriptor != nil {
		cs.Choices = append(cs.Choices, "RE-USE")
	}
	for {
		choice, ok := cs.Choose(ctx, ops, th)
		if !ok {
			logutil.DebugLog("inputDescriptorFlow: Choose returned false")
			ctx.LastDescriptor = originalDesc // Restore on back
			return nil, false
		}
		switch choice {
		case 0: // Scan
			res, ok := (&ScanScreen{
				Title: "Scan",
				Lead:  "Wallet Output Descriptor",
			}).Scan(ctx, ops)
			if !ok {
				logutil.DebugLog("inputDescriptorFlow: Scan returned false")
				continue
			}
			desc, ok := res.(urtypes.OutputDescriptor)
			if !ok {
				if b, isbytes := res.([]byte); isbytes {
					d, err := nonstandard.OutputDescriptor(b)
					desc, ok = d, err == nil
				}
			}
			if !ok {
				showError(ctx, ops, th, errors.New("Invalid Descriptor: Not a wallet output descriptor or XPUB key"))
				continue
			}
			logutil.DebugLog("Scanned descriptor: Type=%v, Script=%s, Keys=%d, Threshold=%d", desc.Type, desc.Script.String(), len(desc.Keys), desc.Threshold)
			if !address.Supported(desc) {
				showError(ctx, ops, th, errors.New("Unsupported Descriptor"))
				continue
			}
			if len(desc.Keys) == 1 && desc.Keys[0].MasterFingerprint == 0 {
				mfp, _ := masterFingerprintFor(mnemonic, &chaincfg.MainNetParams)
				desc.Keys[0].MasterFingerprint = mfp
			}
			desc.Title = sanitizeTitle(desc.Title)
			ctx.LastDescriptor = &desc
			logutil.DebugLog("inputDescriptorFlow: Returning desc with %d keys", len(desc.Keys))
			return &desc, true
		case 1: // Skip
			logutil.DebugLog("inputDescriptorFlow: Skipping descriptor")
			return nil, true
		case 2: // Re-use
			logutil.DebugLog("inputDescriptorFlow: Re-using last descriptor")
			return ctx.LastDescriptor, true
		}
	}
}

func backupWalletFlow(ctx *Context, ops op.Ctx, th *Colors) {
	logutil.DebugLog("backupWalletFlow: Starting")
descLoop:
	for {
		desc, ok := inputDescriptorFlow(ctx, ops, th, nil)
		logutil.DebugLog("backupWalletFlow: After inputDescriptorFlow, desc=%v, ok=%v", desc != nil, ok)
		if !ok {
			logutil.DebugLog("backupWalletFlow: inputDescriptorFlow failed, exiting")
			return
		}

		if desc == nil && ctx.LastDescriptor == nil {
			mnemonic, ok := newMnemonicFlow(ctx, ops, th, 1, 1) // Singlesig: 1/1
			if !ok {
				logutil.DebugLog("backupWalletFlow: newMnemonicFlow failed")
				continue descLoop
			}
			logutil.DebugLog("backupWalletFlow: Seed flow done")
			if !new(SeedScreen).Confirm(ctx, ops, th, mnemonic) {
				logutil.DebugLog("backupWalletFlow: SeedScreen.Confirm failed")
				continue descLoop
			}
			logutil.DebugLog("backupWalletFlow: Seed confirmed")
			mfp, err := masterFingerprintFor(mnemonic, &chaincfg.MainNetParams)
			if err != nil {
				logutil.DebugLog("backupWalletFlow: Fingerprint error: %v", err)
				showError(ctx, ops, th, fmt.Errorf("Failed to compute fingerprint: %v", err))
				continue descLoop
			}
			ctx.Keystores[mfp] = mnemonic
			logutil.DebugLog("backupWalletFlow: Keystore updated, printing singlesig")
			printScreen := &PrintSeedScreen{}
			if printScreen.Print(ctx, ops, th, mnemonic, nil, 0, printer.PaperA4) {
				logutil.DebugLog("backupWalletFlow: Print succeeded")
				return
			}
			logutil.DebugLog("backupWalletFlow: Print failed")
			continue descLoop
		}

		if desc == nil && ctx.LastDescriptor != nil {
			desc = ctx.LastDescriptor
		}

		logutil.DebugLog("backupWalletFlow: Descriptor present with %d keys", len(desc.Keys))
		totalSeeds := len(desc.Keys)
		for i := 1; i <= totalSeeds; i++ {
		seedLoop:
			for {
				mnemonic, ok := newMnemonicFlow(ctx, ops, th, i, totalSeeds)
				if !ok {
					logutil.DebugLog("backupWalletFlow: newMnemonicFlow failed, retrying seed %d", i)
					confirm := &ConfirmWarningScreen{
						Title: "Restart Process?",
						Body:  "Do you want to restart and clear all scanned data?\n\nHold button to confirm.",
						Icon:  assets.IconDiscard,
					}
					if confirmWarning(ctx, ops, th, confirm) {
						logutil.DebugLog("backupWalletFlow: User confirmed restart, clearing data")
						ctx.LastDescriptor = nil
						ctx.Keystores = make(map[uint32]bip39.Mnemonic)
						continue descLoop
					}
					logutil.DebugLog("backupWalletFlow: User declined restart, continuing seed input")
					continue seedLoop
				}
				logutil.DebugLog("backupWalletFlow: Seed flow done for seed %d", i)
				if !new(SeedScreen).Confirm(ctx, ops, th, mnemonic) {
					logutil.DebugLog("backupWalletFlow: SeedScreen.Confirm failed for seed %d", i)
					confirm := &ConfirmWarningScreen{
						Title: "Restart Process?",
						Body:  "Do you want to restart and clear all scanned data?\n\nHold button to confirm.",
						Icon:  assets.IconDiscard,
					}
					if confirmWarning(ctx, ops, th, confirm) {
						logutil.DebugLog("backupWalletFlow: User confirmed restart, clearing data")
						ctx.LastDescriptor = nil
						ctx.Keystores = make(map[uint32]bip39.Mnemonic)
						continue descLoop
					}
					logutil.DebugLog("backupWalletFlow: User declined restart, continuing seed input")
					continue seedLoop
				}
				logutil.DebugLog("backupWalletFlow: Seed confirmed for seed %d", i)
				mfp, err := masterFingerprintFor(mnemonic, &chaincfg.MainNetParams)
				if err != nil {
					logutil.DebugLog("backupWalletFlow: Fingerprint error: %v", err)
					showError(ctx, ops, th, fmt.Errorf("Failed to compute fingerprint: %v", err))
					continue seedLoop
				}
				if _, exists := ctx.Keystores[mfp]; exists {
					logutil.DebugLog("backupWalletFlow: Duplicate seed %.8x detected", mfp)
					showError(ctx, ops, th, fmt.Errorf("Seed was entered already"))
					continue seedLoop
				}
				_, matched := descriptorKeyIdx(*desc, mnemonic, "")
				if !matched {
					logutil.DebugLog("backupWalletFlow: Seed fingerprint %.8x doesn’t match descriptor", mfp)
					showError(ctx, ops, th, fmt.Errorf("Seed doesn’t match wallet descriptor"))
					continue seedLoop
				}
				ctx.Keystores[mfp] = mnemonic
				logutil.DebugLog("backupWalletFlow: Keystore updated, seeds scanned: %d/%d", len(ctx.Keystores), len(desc.Keys))
				break seedLoop
			}
			if len(ctx.Keystores) >= len(desc.Keys) {
				break
			}
		}

	confirmLoop:
		for {
			ds := &DescriptorScreen{Descriptor: *desc, Mnemonic: ctx.Keystores[desc.Keys[0].MasterFingerprint]}
			confirmKeyIdx, ok := ds.Confirm(ctx, ops, th)
			logutil.DebugLog("backupWalletFlow: Confirm returned keyIdx=%d, ok=%v", confirmKeyIdx, ok)
			if !ok {
				logutil.DebugLog("backupWalletFlow: Descriptor not confirmed, prompting restart")
				confirm := &ConfirmWarningScreen{
					Title: "Restart Process?",
					Body:  "Do you want to restart and clear all scanned data?\n\nHold button to confirm.",
					Icon:  assets.IconDiscard,
				}
				if confirmWarning(ctx, ops, th, confirm) {
					logutil.DebugLog("backupWalletFlow: User confirmed restart, clearing data")
					ctx.LastDescriptor = nil
					ctx.Keystores = make(map[uint32]bip39.Mnemonic)
					continue descLoop
				}
				logutil.DebugLog("backupWalletFlow: User declined restart, returning to confirm")
				continue confirmLoop
			}
			logutil.DebugLog("backupWalletFlow: All %d seeds collected, printing with keyIdx=%d", len(desc.Keys), confirmKeyIdx)
			printScreen := &PrintSeedScreen{}
			if printScreen.Print(ctx, ops, th, ds.Mnemonic, desc, confirmKeyIdx, printer.PaperA4) {
				logutil.DebugLog("backupWalletFlow: Print succeeded")
				return
			}
			logutil.DebugLog("backupWalletFlow: Print failed")
			continue confirmLoop // Back to Confirm Wallet, not descLoop
		}
	}
}

type DescriptorScreen struct {
	Descriptor urtypes.OutputDescriptor
	Mnemonic   bip39.Mnemonic
}

func (s *DescriptorScreen) Confirm(ctx *Context, ops op.Ctx, th *Colors) (int, bool) {
	showErr := func(errScreen *ErrorScreen) {
		for {
			dims := ctx.Platform.DisplaySize()
			dismissed := errScreen.Layout(ctx, ops.Begin(), th, dims)
			d := ops.End()
			if dismissed {
				break
			}
			s.Draw(ctx, ops, th, dims)
			d.Add(ops)
			ctx.Frame()
		}
	}
	inp := new(InputTracker)
	for {
		for {
			e, ok := inp.Next(ctx, Button1, Button2, Button3)
			if !ok {
				break
			}
			switch e.Button {
			case Button1:
				if inp.Clicked(e.Button) {
					return 0, false
				}
			case Button2:
				if !inp.Clicked(e.Button) {
					break
				}
				ShowAddressesScreen(ctx, ops, th, s.Descriptor)
			case Button3:
				if !inp.Clicked(e.Button) {
					break
				}
				if err := validateDescriptor(s.Descriptor); err != nil {
					showErr(NewErrorScreen(err))
					continue
				}
				keyIdx, ok := descriptorKeyIdx(s.Descriptor, s.Mnemonic, "")
				if !ok {
					// Passphrase protected seeds don't match the descriptor, so
					// allow the user to ignore the mismatch. Don't allow this for
					// multisig descriptors where we can't know which key the seed
					// belongs to.
					if len(s.Descriptor.Keys) == 1 {
						confirm := &ConfirmWarningScreen{
							Title: "Unknown Wallet",
							Body:  "The wallet does not match the seed.\n\nIf it is passphrase protected, long press to confirm.",
							Icon:  assets.IconCheckmark,
						}
					loop:
						for {
							dims := ctx.Platform.DisplaySize()
							res := confirm.Layout(ctx, ops.Begin(), th, dims)
							d := ops.End()
							switch res {
							case ConfirmYes:
								return 0, true
							case ConfirmNo:
								break loop
							}
							s.Draw(ctx, ops, th, dims)
							d.Add(ops)
							ctx.Frame()
						}
					} else {
						showErr(&ErrorScreen{
							Title: "Unknown Wallet",
							Body:  "The wallet does not match the seed or is passphrase protected.",
						})
					}
					continue
				}
				return keyIdx, true
			}
		}

		dims := ctx.Platform.DisplaySize()
		s.Draw(ctx, ops, th, dims)
		layoutNavigation(inp, ops, th, dims, []NavButton{
			{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
			{Button: Button2, Style: StyleSecondary, Icon: assets.IconInfo},
			{Button: Button3, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		ctx.Frame()
	}
}

func (s *DescriptorScreen) Draw(ctx *Context, ops op.Ctx, th *Colors, dims image.Point) {
	const infoSpacing = 8

	desc := s.Descriptor
	op.ColorOp(ops, th.Background)

	// Title.
	r := layout.Rectangle{Max: dims}
	layoutTitle(ctx, ops, dims.X, th.Text, "Confirm Wallet")

	btnw := assets.NavBtnPrimary.Bounds().Dx()
	body := r.Shrink(leadingSize, btnw, 0, btnw)

	{
		ops := ops.Begin()
		var bodytxt richText

		bodyst := ctx.Styles.body
		subst := ctx.Styles.subtitle
		if desc.Title != "" {
			bodytxt.Add(ops, subst, body.Dx(), th.Text, "Title")
			bodytxt.Add(ops, bodyst, body.Dx(), th.Text, "%s", desc.Title)
			bodytxt.Y += infoSpacing
		}
		bodytxt.Add(ops, subst, body.Dx(), th.Text, "Type")
		testnet := any("") // TODO: TinyGo allocates without explicit interface conversion.
		if len(desc.Keys) > 0 && desc.Keys[0].Network != &chaincfg.MainNetParams {
			testnet = " (testnet)"
		}
		switch desc.Type {
		case urtypes.Singlesig:
			bodytxt.Add(ops, bodyst, body.Dx(), th.Text, "Singlesig%s", testnet)
		default:
			bodytxt.Add(ops, bodyst, body.Dx(), th.Text, "%d-of-%d multisig%s", desc.Threshold, len(desc.Keys), testnet)
		}
		bodytxt.Y += infoSpacing
		bodytxt.Add(ops, subst, body.Dx(), th.Text, "Script")
		bodytxt.Add(ops, bodyst, body.Dx(), th.Text, "%s", desc.Script.String())
	}

	op.Position(ops, ops.End(), body.Min.Add(image.Pt(0, scrollFadeDist)))
}

type PrintSeedScreen struct {
	inp InputTracker
}

func (s *PrintSeedScreen) Print(ctx *Context, ops op.Ctx, th *Colors, mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paperFormat printer.PaperSize) bool {
	inp := &s.inp
	paperChoice := &ChoiceScreen{
		Title:   "Select Paper Size",
		Lead:    "Choose your printer's paper size",
		Choices: []string{"A4", "Letter"},
	}
	choice, ok := paperChoice.Choose(ctx, ops, th)
	if !ok {
		return false
	}
	selectedPaper := printer.PaperA4
	if choice == 1 {
		selectedPaper = printer.PaperLetter
	}
	for {
		for {
			e, ok := inp.Next(ctx, Button1, Button3)
			if !ok {
				break
			}
			switch e.Button {
			case Button1:
				if inp.Clicked(e.Button) {
					return false
				}
			case Button3:
				if inp.Clicked(e.Button) {
					progress := &PrintProgressScreen{}
					success, err := progress.Show(ctx, ops, th, mnemonic, desc, keyIdx, selectedPaper)
					if err != nil && err.Error() != "print canceled" {
						s.showError(ctx, ops, th, err)
					}
					if err != nil && err.Error() == "print canceled" {
						continue
					}
					result := &PrintResultScreen{success: success}
					if result.Show(ctx, ops, th, mnemonic, desc, keyIdx, selectedPaper) {
						continue
					}
					return true
				}
			}
		}
		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, th.Background)
		title := "Print Seed"
		if desc != nil {
			title = "Print Wallet Share"
		}
		layoutTitle(ctx, ops, dims.X, th.Text, "%s", title)
		lead := fmt.Sprintf("Paper size: %s\n\nEnsure your printer is connected before printing.\nPress Print to continue.", selectedPaper)
		if desc != nil {
			lead = fmt.Sprintf("Paper size: %s\n\nEnsure your printer is connected before printing share %d/%d.\nPress Print to continue.", selectedPaper, keyIdx+1, len(desc.Keys))
		}
		sz := widget.Labelwf(ops.Begin(), ctx.Styles.lead, dims.X-16, th.Text, "%s", lead)
		op.Position(ops, ops.End(), dims.Div(2).Sub(sz.Div(2)))
		layoutNavigation(inp, ops, th, dims, []NavButton{
			{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
			{Button: Button3, Style: StylePrimary, Icon: assets.IconHammer},
		}...)
		ctx.Frame()
	}
}

func (s *PrintSeedScreen) showError(ctx *Context, ops op.Ctx, th *Colors, err error) {
	logutil.DebugLog("showError called with error: %v", err)
	errScr := NewErrorScreen(err)
	for {
		dims := ctx.Platform.DisplaySize()
		dismissed := errScr.Layout(ctx, ops, th, dims)
		if dismissed {
			logutil.DebugLog("Error screen dismissed")
			break
		}
		ctx.Frame()
	}
}

type PrintResultScreen struct {
	success bool
	inp     InputTracker
}

func (s *PrintResultScreen) Show(ctx *Context, ops op.Ctx, th *Colors, mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paperFormat printer.PaperSize) bool {
	for {
		for {
			e, ok := s.inp.Next(ctx, Button2, Button3)
			if !ok {
				break
			}
			if !s.inp.Clicked(e.Button) {
				continue
			}
			switch e.Button {
			case Button2: // Print Again
				return true
			case Button3: // Delete Seed and Go Home
				return false
			}
		}
		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, th.Background)
		layoutTitle(ctx, ops, dims.X, th.Text, "Print Result")
		msg := "Print completed successfully."
		if !s.success {
			msg = "Print failed. Check printer connection."
		}
		sz := widget.Labelwf(ops.Begin(), ctx.Styles.lead, dims.X-16, th.Text, "%s", msg)
		op.Position(ops, ops.End(), dims.Div(2).Sub(sz.Div(2)))
		layoutNavigation(&s.inp, ops, th, dims, []NavButton{
			{Button: Button2, Style: StyleSecondary, Icon: assets.IconHammer, Progress: 0}, // Print Again
			{Button: Button3, Style: StylePrimary, Icon: assets.IconDiscard, Progress: 0},  // Delete Seed
		}...)
		ctx.Frame()
	}
}

type PrintProgressScreen struct {
	inp InputTracker
}

func (s *PrintProgressScreen) Show(ctx *Context, ops op.Ctx, th *Colors, mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paperFormat printer.PaperSize) (bool, error) {
	start := ctx.Platform.Now()
	duration := 2 * time.Second
	var printErr error
	go func() {
		if err := ctx.Platform.CreatePlates(ctx, mnemonic, desc, keyIdx); err != nil {
			logutil.DebugLog("Print failed in progress: %v", err)
			printErr = err
		}
		logutil.DebugLog("PrintPDF completed in goroutine")
	}()

	for {
		for {
			e, ok := s.inp.Next(ctx, Button1)
			if !ok {
				break
			}
			if e.Button == Button1 && s.inp.Clicked(e.Button) {
				return false, fmt.Errorf("print canceled")
			}
		}
		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, th.Background)
		layoutTitle(ctx, ops, dims.X, th.Text, "Printing")

		elapsed := ctx.Platform.Now().Sub(start)
		progress := float32(elapsed.Seconds() / duration.Seconds())
		if progress >= 1.0 || printErr != nil {
			logutil.DebugLog("Progress complete or error: success=%v, err=%v", printErr == nil, printErr)
			return printErr == nil, printErr
		}

		ctx.WakeupAt(ctx.Platform.Now().Add(100 * time.Millisecond))

		content := layout.Rectangle{Max: dims}.Shrink(leadingSize, 0, leadingSize, 0)
		op.Offset(ops, content.Center(assets.ProgressCircle.Bounds().Size()))
		(&ProgressImage{
			Progress: progress,
			Src:      assets.ProgressCircle,
		}).Add(ops)
		op.ColorOp(ops, th.Text)
		sz := widget.Labelf(ops.Begin(), ctx.Styles.progress, th.Text, "%d%%", int(progress*100))
		op.Position(ops, ops.End(), content.Center(sz))

		labelSz := widget.Labelwf(ops.Begin(), ctx.Styles.lead, dims.X-16, th.Text, "Printing...")
		op.Position(ops, ops.End(), content.Center(labelSz).Add(image.Pt(0, assets.ProgressCircle.Bounds().Dy()/2+12)))

		layoutNavigation(&s.inp, ops, th, dims, []NavButton{
			{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
		}...)
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

// Screen defines the minimal contract for a UI screen.
type Screen interface {
	// Update processes events and may return a new Screen (transition).
	Update(ctx *Context, ops op.Ctx) Screen
}

func Run(pl Platform, version string) func(yield func() bool) {
	return func(yield func() bool) {
		ctx := NewContext(pl)
		ctx.Version = version
		ctx.Session = Session{
			Paper:     printer.PaperA4,
			Keystores: make(map[uint32]bip39.Mnemonic),
		}
		current := Screen(&MainMenuScreen{})
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

		var evts []Event
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
		for {
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
			// Run current screen; allow it to transition.
			current = current.Update(a.ctx, a.root.Context())
			a.root.Reset()
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
