package gui

import (
	"image"
	"image/color"
	"math"

	"seedetcher.com/bip39"
	"seedetcher.com/gui/op"
)

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
