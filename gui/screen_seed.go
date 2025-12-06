package gui

import (
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/btcsuite/btcd/chaincfg"
	"seedetcher.com/bip39"
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/layout"
	"seedetcher.com/gui/op"
	"seedetcher.com/gui/widget"
	"seedetcher.com/nonstandard"
)

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
