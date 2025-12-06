package gui

import (
	"image"
	"image/color"

	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/layout"
	"seedetcher.com/gui/op"
	"seedetcher.com/gui/widget"
)

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
				if e.Pressed && s.choice > 0 {
					s.choice--
				}
			case Down, CW:
				if e.Pressed && s.choice < len(s.Choices)-1 {
					s.choice++
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
