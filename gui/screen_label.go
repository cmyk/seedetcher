package gui

import (
	"fmt"
	"image"
	"math"
	"strings"
	"unicode/utf8"

	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/layout"
	"seedetcher.com/gui/op"
	"seedetcher.com/gui/widget"
	"seedetcher.com/printer"
)

const labelMaxLen = 15

// LabelInputScreen collects an optional wallet label to print on plates.
type LabelInputScreen struct {
	Theme    *Colors
	Default  string
	Value    string
	OnDone   func(label string) Screen
	OnCancel func() Screen
}

func (s *LabelInputScreen) Update(ctx *Context, ops op.Ctx) Screen {
	th := s.Theme
	if th == nil {
		th = &descriptorTheme
	}
	fallback := s.Default
	if fallback == "" {
		fallback = printer.DefaultWalletLabel
	}
	start := s.Value
	kbd := NewLabelKeyboard(ctx, start)
	inp := new(InputTracker)

	for {
		for {
			kbd.Update(ctx)
			e, ok := inp.Next(ctx, Button1, Button3)
			if !ok {
				break
			}
			switch e.Button {
			case Button1:
				if inp.Clicked(e.Button) {
					if s.OnCancel != nil {
						return s.OnCancel()
					}
					return &MainMenuScreen{}
				}
			case Button3:
				if !inp.Clicked(e.Button) {
					break
				}
				label := sanitizeLabel(kbd.Value)
				if s.OnDone != nil {
					return s.OnDone(label)
				}
				return &MainMenuScreen{}
			}
		}

		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, th.Background)
		layoutTitle(ctx, ops, dims.X, th.Text, "Wallet Label")

		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		navh := assets.NavBtnPrimary.Bounds().Dy() + 6
		content, _ = content.CutBottom(navh)
		kbSize := kbd.Size()
		padding := kbd.KeyHeight() * 2
		top, bottom := content.CutBottom(kbSize.Y + padding)

		kbPos := bottom.Center(kbSize)
		kbPos.Y = bottom.Min.Y + padding
		kbSize = kbd.Layout(ctx, ops.Begin(), th)
		op.Position(ops, ops.End(), kbPos)

		current := sanitizeLabel(kbd.Value)
		display := current
		boxWidth := kbSize.X - 8
		if boxWidth < 0 {
			boxWidth = kbSize.X
		}
		boxHeight := ctx.Styles.lead.Measure(boxWidth, "%s", display).Y + 2
		if boxHeight < 8 {
			boxHeight = 8
		}
		box := image.Rect(0, 0, boxWidth, boxHeight)
		box = box.Add(image.Pt(kbPos.X+4, top.Min.Y))
		assets.ButtonFocused.Add(ops, box, true)
		op.ColorOp(ops, th.Text)
		preview := widget.Labelwf(ops.Begin(), ctx.Styles.lead, box.Dx()-12, th.Background, "%s", display)
		op.Position(ops, ops.End(), box.Min.Add(image.Pt(6, (box.Dy()-preview.Y)/2)))

		remaining := labelMaxLen - len(current)
		if remaining < 0 {
			remaining = 0
		}
		lead := fmt.Sprintf("%d characters left.", remaining)
		widget.Labelwf(ops.Begin(), ctx.Styles.body, kbSize.X, th.Text, "%s", lead)
		op.Position(ops, ops.End(), box.Min.Add(image.Pt(0, box.Dy()+2)))

		layoutNavigation(ctx, inp, ops, th, dims, []NavButton{
			{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
			{Button: Button3, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		ctx.Frame()
	}
}

// sanitizeLabel enforces uppercase, allowed runes, and max length.
func sanitizeLabel(val string) string {
	var out []rune
	for _, r := range strings.ToUpper(val) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			out = append(out, r)
		}
		if len(out) >= labelMaxLen {
			break
		}
	}
	return string(out)
}

var labelKeys = [...][]rune{
	[]rune("1234567890"),
	[]rune("QWERTYUIO⌫"),
	[]rune("PASDFGHJK"),
	[]rune("LZXCVBNM-"),
}

type LabelKeyboard struct {
	Value string

	positions [len(labelKeys)][]image.Point
	widest    image.Point
	backspace image.Point
	size      image.Point
	keyHeight int

	row, col int
	inp      InputTracker
}

func NewLabelKeyboard(ctx *Context, val string) *LabelKeyboard {
	k := &LabelKeyboard{}
	k.Value = sanitizeLabel(val)
	k.widest = ctx.Styles.keyboard.Measure(math.MaxInt, "W")
	bsb := assets.KeyBackspace.Bounds()
	bsWidth := bsb.Min.X*2 + bsb.Dx()
	k.backspace = image.Pt(bsWidth, k.widest.Y)
	bgbnds := assets.Key.Bounds(image.Rectangle{Max: k.widest})
	const margin = 2
	bgsz := bgbnds.Size().Add(image.Pt(margin, margin))
	k.keyHeight = bgsz.Y
	maxw := len(labelKeys[0])*bgsz.X - margin
	for i, row := range labelKeys {
		for j := range row {
			pos := image.Pt(j*bgsz.X, i*bgsz.Y)
			pos = pos.Sub(bgbnds.Min)
			k.positions[i] = append(k.positions[i], pos)
		}
	}
	k.size = image.Point{
		X: maxw,
		Y: len(labelKeys)*bgsz.Y - margin,
	}
	k.row = len(labelKeys) / 2
	k.col = len(labelKeys[k.row]) / 2
	k.adjust(false)
	return k
}

func (k *LabelKeyboard) Size() image.Point {
	return k.size
}

func (k *LabelKeyboard) KeyHeight() int {
	return k.keyHeight
}

func (k *LabelKeyboard) Valid(r rune) bool {
	if r == '⌫' {
		return len(k.Value) > 0
	}
	if len(k.Value) >= labelMaxLen {
		return false
	}
	if r == '-' {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}
	return r >= 'A' && r <= 'Z'
}

func (k *LabelKeyboard) Update(ctx *Context) {
	for {
		e, ok := k.inp.Next(ctx, Left, Right, Up, Down, CCW, CW, Center, Rune)
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
						nrows := len(labelKeys)
						k.row = (k.row - 1 + nrows) % nrows
					}
					next = len(labelKeys[k.row]) - 1
				}
				if !k.Valid(labelKeys[k.row][next]) {
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
				if next == len(labelKeys[k.row]) {
					if e.Button == CW {
						nrows := len(labelKeys)
						k.row = (k.row + 1 + nrows) % nrows
					}
					next = 0
				}
				if !k.Valid(labelKeys[k.row][next]) {
					continue
				}
				k.col = next
				k.adjust(true)
				break
			}
		case Up:
			n := len(labelKeys)
			next := k.row
			for {
				next = (next - 1 + n) % n
				if k.adjustCol(next) {
					k.adjust(true)
					break
				}
			}
		case Down:
			n := len(labelKeys)
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
		case Center:
			r := labelKeys[k.row][k.col]
			k.rune(r)
		}
	}
}

func (k *LabelKeyboard) rune(r rune) {
	if !k.Valid(r) {
		return
	}
	if r == '⌫' {
		_, n := utf8.DecodeLastRuneInString(k.Value)
		k.Value = k.Value[:len(k.Value)-n]
	} else {
		k.Value = sanitizeLabel(k.Value + string(r))
	}
	k.adjust(r == '⌫')
}

// adjust resets the row and column to the nearest valid key, if any.
func (k *LabelKeyboard) adjust(allowBackspace bool) {
	dist := int(1e6)
	current := k.positions[k.row][k.col]
	found := false
	for i, row := range labelKeys {
		for j, key := range row {
			if !k.Valid(key) || key == '⌫' && !allowBackspace {
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
		}
	}
	// Only if no other key was found, select backspace.
	if !found {
		k.row = len(k.positions) - 1
		k.col = len(k.positions[k.row]) - 1
	}
}

// adjustCol sets the column to the one nearest the x position.
func (k *LabelKeyboard) adjustCol(row int) bool {
	dist := int(1e6)
	found := false
	x := k.positions[k.row][k.col].X
	for i, r := range labelKeys[row] {
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

func (k *LabelKeyboard) Clear() {
	k.Value = ""
	k.adjust(false)
}

func (k *LabelKeyboard) Layout(ctx *Context, ops op.Ctx, th *Colors) image.Point {
	for i, row := range labelKeys {
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
			keyOps := ops.End()
			bg.Add(ops.Begin(), image.Rectangle{Max: bgsz}, true)
			op.ColorOp(ops, bgcol)
			op.Position(ops, keyOps, bgsz.Sub(sz).Div(2))
			op.Position(ops, ops.End(), k.positions[i][j])
		}
	}
	return k.size
}
