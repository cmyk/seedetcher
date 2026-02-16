package gui

import (
	"fmt"
	"image"
	"strings"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/op"
	"seedetcher.com/gui/text"
	"seedetcher.com/gui/widget"
)

const fingerprintsPerPage = 5

// FingerprintsScreen reviews all cosigner fingerprints with pagination.
type FingerprintsScreen struct {
	Theme      *Colors
	Descriptor *urtypes.OutputDescriptor
	Page       int
	OnBack     func() Screen
	OnContinue func() Screen
}

func (s *FingerprintsScreen) Update(ctx *Context, ops op.Ctx) Screen {
	th := s.Theme
	if th == nil {
		th = &singleTheme
	}
	if s.Descriptor == nil || len(s.Descriptor.Keys) == 0 {
		if s.OnContinue != nil {
			return s.OnContinue()
		}
		return &MainMenuScreen{}
	}

	totalPages := (len(s.Descriptor.Keys) + fingerprintsPerPage - 1) / fingerprintsPerPage
	if totalPages < 1 {
		totalPages = 1
	}
	if s.Page < 0 {
		s.Page = 0
	}
	if s.Page >= totalPages {
		s.Page = totalPages - 1
	}

	inp := new(InputTracker)
	for {
		for {
			e, ok := inp.Next(ctx, Button1, Button3, Left, Right, CCW, CW)
			if !ok {
				break
			}
			clicked := inp.Clicked(e.Button)
			if !clicked && !e.Pressed {
				continue
			}
			switch e.Button {
			case Button1:
				if clicked {
					if s.OnBack != nil {
						return s.OnBack()
					}
					return &MainMenuScreen{}
				}
			case Button3:
				if clicked {
					if s.OnContinue != nil {
						return s.OnContinue()
					}
					return &MainMenuScreen{}
				}
			case Left, CCW:
				if s.Page > 0 {
					s.Page--
				}
			case Right, CW:
				if s.Page+1 < totalPages {
					s.Page++
				}
			}
		}

		dims := ctx.Platform.DisplaySize()
		op.ColorOp(ops, th.Background)
		title := layoutTitle(ctx, ops, dims.X, th.Text, "Fingerprints")

		start := s.Page * fingerprintsPerPage
		end := start + fingerprintsPerPage
		if end > len(s.Descriptor.Keys) {
			end = len(s.Descriptor.Keys)
		}
		lines := make([]string, 0, end-start+2)
		for i := start; i < end; i++ {
			fp := strings.ToUpper(fmt.Sprintf("%08x", s.Descriptor.Keys[i].MasterFingerprint))
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, fp))
		}
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("Page %d/%d", s.Page+1, totalPages))
		body := strings.Join(lines, "\n")
		arrowW := assets.ArrowLeft.Bounds().Dx()
		navW := assets.NavBtnPrimary.Bounds().Dx()
		leftPad := 10
		rightPad := 10 + navW + 6
		if totalPages > 1 {
			leftPad += arrowW + 10
			rightPad += arrowW + 10
		}
		width := dims.X - leftPad - rightPad
		if width < 80 {
			width = 80
		}
		style := ctx.Styles.body
		style.Alignment = text.AlignStart
		bodySize := widget.Labelwf(ops.Begin(), style, width, th.Text, "%s", body)
		bodyPos := image.Pt(leftPad, title.Max.Y+10)
		op.Position(ops, ops.End(), bodyPos)

		if totalPages > 1 {
			arrowH := assets.ArrowLeft.Bounds().Dy()
			navTop := dims.Y - leadingSize - assets.NavBtnPrimary.Bounds().Dy()
			contentTop := title.Max.Y + 10
			arrowY := contentTop + (navTop-contentTop-arrowH)/2
			if arrowY < contentTop {
				arrowY = contentTop
			}
			if s.Page > 0 {
				l := ops.Begin()
				op.ImageOp(l, assets.ArrowLeft, true)
				op.ColorOp(l, th.Text)
				op.Position(ops, ops.End(), image.Pt(8, arrowY))
			}
			if s.Page+1 < totalPages {
				r := ops.Begin()
				op.ImageOp(r, assets.ArrowRight, true)
				op.ColorOp(r, th.Text)
				op.Position(ops, ops.End(), image.Pt(dims.X-assets.ArrowRight.Bounds().Dx()-8, arrowY))
			}
		}
		_ = bodySize

		layoutNavigation(ctx, inp, ops, th, dims,
			NavButton{Button: Button1, Style: StyleSecondary, Icon: assets.IconBack},
			NavButton{Button: Button3, Style: StylePrimary, Icon: assets.IconCheckmark},
		)
		ctx.Frame()
	}
}
