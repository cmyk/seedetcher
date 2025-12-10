package gui

import (
	"fmt"
	"image"
	"image/color"

	"seedetcher.com/address"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/layout"
	"seedetcher.com/gui/op"
	"seedetcher.com/gui/text"
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
				// Reset counter for change addresses; start at index 0.
				idx := len(s.addresses[page])
				addr, err = address.Change(desc, uint32(idx))
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
			e, ok := inp.Next(ctx, Left, Right, Up, Down, CCW, CW, Button1)
			if !ok {
				break
			}
			switch e.Button {
			case Button1:
				if inp.Clicked(e.Button) {
					return
				}
			case Left:
				if e.Pressed && s.page > 0 {
					s.page--
					s.scroll = 0
				}
			case Right:
				if e.Pressed && s.page+1 < maxPage {
					s.page++
					s.scroll = 0
				}
			case CCW:
				if !e.Pressed {
					break
				}
				s.page = (s.page - 1 + maxPage) % maxPage
			case CW:
				if !e.Pressed {
					break
				}
				s.page = (s.page + 1 + maxPage) % maxPage
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

		layoutNavigation(ctx, inp, ops, th, dims, []NavButton{{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack}}...)
		ctx.Frame()
	}
}

func shortenAddress(n int, addr string) string {
	if len(addr) <= n {
		return addr
	}
	return addr[:n/2] + "......" + addr[len(addr)-n/2:]
}
