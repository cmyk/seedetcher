package gui

import (
	"image"
	"image/color"
	"io"
	"strings"

	"seedetcher.com/bc/ur"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/layout"
	"seedetcher.com/gui/op"
	"seedetcher.com/gui/widget"
	"seedetcher.com/logutil"
	"seedetcher.com/nonstandard"
)

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

type QRDecoder struct {
	decoder   ur.Decoder
	nsdecoder nonstandard.Decoder
}

func (d *QRDecoder) Progress() int {
	progress := int(100 * d.decoder.Progress())
	if progress == 0 {
		progress = int(100 * d.nsdecoder.Progress())
	}
	return progress
}

func (d *QRDecoder) parseNonStandard(qr []byte) (any, bool) {
	if err := d.nsdecoder.Add(string(qr)); err != nil {
		d.nsdecoder = nonstandard.Decoder{}
		return qr, true
	}
	enc := d.nsdecoder.Result()
	if enc == nil {
		return nil, false
	}
	return enc, true
}

func (d *QRDecoder) parseQR(qr []byte) (any, bool) {
	uqr := strings.ToUpper(string(qr))
	if !strings.HasPrefix(uqr, "UR:") {
		// Try parsing as non-standard, including Sparrow base64
		d.decoder = ur.Decoder{}
		return d.parseNonStandard(qr)
	}
	d.nsdecoder = nonstandard.Decoder{}
	if err := d.decoder.Add(uqr); err != nil {
		// Handle fragmented UR (common in high-density QRs)
		logutil.DebugLog("UR decode error (fragment?), retrying: %v", err)
		// Reset and retry with partial accumulation
		d.decoder = ur.Decoder{} // Reset
		d.decoder.Add(uqr)       // Try again
		// Allow partial progress—store fragments for later
		typ, enc, partialErr := d.decoder.Result()
		if partialErr != nil && partialErr != io.EOF {
			logutil.DebugLog("Partial UR result error: %v", partialErr)
			d.decoder = ur.Decoder{} // Reset again if still broken
			return nil, false
		}
		if enc == nil {
			return nil, false // Still building—wait for more fragments
		}
		// Try parsing the partial or full result
		v, err := urtypes.Parse(typ, enc)
		if err == nil {
			if desc, ok := v.(urtypes.OutputDescriptor); ok {
				logutil.DebugLog("Parsed descriptor (partial/full): %+v", desc)
				return desc, true
			}
			return v, true
		}
		logutil.DebugLog("UR parse failed, trying nonstandard: %v", err)
		return d.parseNonStandard(qr)
	}
	typ, enc, err := d.decoder.Result()
	if err != nil {
		logutil.DebugLog("UR result error: %v", err)
		d.decoder = ur.Decoder{}
		return nil, false
	}
	if enc == nil {
		return nil, false
	}
	d.decoder = ur.Decoder{}
	v, err := urtypes.Parse(typ, enc)
	if err != nil {
		logutil.DebugLog("UR parse failed, trying nonstandard: %v", err)
		return d.parseNonStandard(qr)
	}
	return v, true
}

// ScanScreen handles QR scanning flow.
type ScanScreen struct {
	Title string
	Lead  string
}

func (s *ScanScreen) Scan(ctx *Context, ops op.Ctx) (any, bool) {
	var (
		feed, feed2, gray *image.Gray
		cameraErr         error
		decoder           QRDecoder
	)
	inp := new(InputTracker)
	for {
		const cameraFrameScale = 3
		for {
			e, ok := inp.Next(ctx, Button1, Button2)
			if !ok {
				break
			}
			if !inp.Clicked(e.Button) {
				continue
			}
			switch e.Button {
			case Button1:
				return nil, false
			case Button2:
				ctx.RotateCamera = !ctx.RotateCamera
			}
		}

		dims := ctx.Platform.DisplaySize()
		if feed == nil || dims != feed.Bounds().Size() {
			feed = image.NewGray(image.Rectangle{Max: dims})
			copy := *feed
			feed2 = &copy
			gray = new(image.Gray)
		}
		ctx.Platform.CameraFrame(dims.Mul(cameraFrameScale))
		for {
			f, ok := ctx.FrameEvent()
			if !ok {
				break
			}
			cameraErr = f.Error
			if cameraErr == nil {
				ycbcr := f.Image.(*image.YCbCr)
				*gray = image.Gray{Pix: ycbcr.Y, Stride: ycbcr.YStride, Rect: ycbcr.Bounds()}

				// Swap image (but not backing store) to ensure the graphics backend treats
				// it as dirty.
				feed, feed2 = feed2, feed
				scaleRot(feed, gray, ctx.RotateCamera)
				results, _ := ctx.Platform.ScanQR(gray)
				for _, res := range results {
					if v, ok := decoder.parseQR(res); ok {
						return v, true
					}
				}
			}
		}
		th := &cameraTheme
		r := layout.Rectangle{Max: dims}

		op.ImageOp(ops, feed, false)

		corners := assets.CameraCorners.Add(ops.Begin(), image.Rect(0, 0, 132, 132), false)
		op.Position(ops, ops.End(), r.Center(corners.Size()))

		underlay := assets.ButtonFocused
		background := func(ops op.Ctx, w op.CallOp, dst image.Rectangle, pos image.Point) {
			underlay.Add(ops.Begin(), dst, true)
			op.ColorOp(ops, color.NRGBA{A: theme.overlayMask})
			op.Position(ops, ops.End(), image.Point{})
			op.Position(ops, w, pos)
		}

		title := layoutTitle(ctx, ops.Begin(), dims.X, th.Text, "%s", s.Title)
		title.Min.Y += 4
		title.Max.Y -= 4
		background(ops, ops.End(), title, image.Point{})

		// Camera error, if any.
		if err := cameraErr; err != nil {
			sz := widget.Labelwf(ops.Begin(), ctx.Styles.body, dims.X-2*16, th.Text, "%s", err.Error())
			op.Position(ops, ops.End(), r.Center(sz))
		}

		width := dims.X - 2*8
		// Lead text.
		sz := widget.Labelwf(ops.Begin(), ctx.Styles.lead, width, th.Text, "%s", s.Lead)
		top, footer := r.CutBottom(sz.Y + 2*12)
		pos := footer.Center(sz)
		background(ops, ops.End(), image.Rectangle{Min: pos, Max: pos.Add(sz)}, pos)

		// Progress
		if progress := decoder.Progress(); progress > 0 {
			sz = widget.Labelwf(ops.Begin(), ctx.Styles.lead, width, th.Text, "%d%%", progress)
			_, percent := top.CutBottom(sz.Y)
			pos := percent.Center(sz)
			background(ops, ops.End(), image.Rectangle{Min: pos, Max: pos.Add(sz)}, pos)
		}

		nav := func(btn Button, icn image.RGBA64Image) {
			nav := layoutNavigation(ctx, inp, ops.Begin(), th, dims, []NavButton{{Button: btn, Style: StyleSecondary, Icon: icn}}...)
			nav = image.Rectangle(layout.Rectangle(nav).Shrink(underlay.Padding()).Shrink(-2, -4, -2, -2))
			background(ops, ops.End(), nav, image.Point{})
		}
		nav(Button1, assets.IconBack)
		nav(Button2, assets.IconFlip)
		ctx.Frame()
	}
}
