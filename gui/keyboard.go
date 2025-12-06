package gui

import (
	"image"
	"math"
	"strings"
	"unicode/utf8"

	"seedetcher.com/bip39"
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/layout"
	"seedetcher.com/gui/op"
	"seedetcher.com/gui/widget"
)

const longestWord = "TOMORROW"

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
