package printer

import (
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"sync"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/kortschak/qr"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/descriptor/shard"
	"seedetcher.com/seedqr"
	"seedetcher.com/version"
)

// RasterOptions controls the bitmap output used for raw printer jobs.
type RasterOptions struct {
	DPI    float64 // target resolution; defaults to 600 if unset
	Mirror bool    // mirror horizontally (for toner transfer)
	Invert bool    // swap black/white for negative output
}

const (
	plateSizeMM   = 90.0
	borderWidthMM = 0.2
)

var (
	bwPalette = color.Palette{color.White, color.Black}

	fontOnce     sync.Once
	fontFaceData *opentype.Font
	fontErr      error
	faceMu       sync.Mutex
	faceCache    = make(map[[2]float64]font.Face) // key: {sizePt, dpi}

	fontOnceMedium     sync.Once
	fontFaceDataMedium *opentype.Font
	fontErrMedium      error
	faceMuMedium       sync.Mutex
	faceCacheMedium    = make(map[[2]float64]font.Face) // key: {sizePt, dpi}

	shardSetMu     sync.RWMutex
	forcedShardSet *[16]byte
)

// CreatePlateBitmaps renders seed/descriptor plates to 1-bit bitmaps using the existing layout.
func CreatePlateBitmaps(mnemonics []bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, opts RasterOptions, progress ProgressFunc) ([]*image.Paletted, []*image.Paletted, error) {
	totalShares := len(mnemonics)
	if desc != nil && len(desc.Keys) > 0 {
		totalShares = len(desc.Keys)
	}

	seedImgs := make([]*image.Paletted, totalShares)
	var descImgs []*image.Paletted
	hasDesc := desc != nil && len(desc.Keys) > 0
	if hasDesc {
		descImgs = make([]*image.Paletted, totalShares)
	}
	var shardQRCodes []string
	if hasDesc {
		var err error
		shardQRCodes, err = descriptorShardQRCodes(desc, totalShares)
		if err != nil {
			return nil, nil, err
		}
	}

	for i := 0; i < totalShares; i++ {
		mnemonic := mnemonics[i%len(mnemonics)]
		seedImg, err := RenderSeedPlateBitmap(mnemonic, i+1, totalShares, opts)
		if err != nil {
			return nil, nil, err
		}
		seedImgs[i] = seedImg

		if hasDesc {
			descKeyIdx := i % len(desc.Keys)
			descQR := ""
			if i < len(shardQRCodes) {
				descQR = shardQRCodes[i]
			}
			descImg, err := RenderDescriptorPlateBitmap(desc, descKeyIdx, i+1, totalShares, opts, descQR)
			if err != nil {
				return nil, nil, err
			}
			descImgs[i] = descImg
		}

		if progress != nil {
			progress(StagePrepare, int64(i+1), int64(totalShares))
		}
	}

	return seedImgs, descImgs, nil
}

// RenderSeedPlateBitmap mirrors the PDF layout at 600dpi as a 1-bit paletted image.
func RenderSeedPlateBitmap(mnemonic bip39.Mnemonic, shareNum, totalShares int, opts RasterOptions) (*image.Paletted, error) {
	dpi := opts.dpi()
	canvas := newPlateCanvas(dpi)
	blackIdx := uint8(1)

	border := mmToPx(borderWidthMM, dpi)
	if border < 1 {
		border = 1
	}
	strokeRect(canvas, 0, 0, canvas.Bounds().Dx(), canvas.Bounds().Dy(), border, blackIdx)

	shareFace := loadFaceMedium(6, dpi)
	mainFace := loadFace(8, dpi)

	drawText(canvas, shareFace, dpi, 5.0, 5.0, fmt.Sprintf("%d/%d", shareNum, totalShares))

	seed := bip39.MnemonicSeed(mnemonic, "")
	if seed != nil {
		masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
		if err == nil {
			if masterPubKey, err := masterKey.Neuter(); err == nil {
				if pubKey, err := masterPubKey.ECPubKey(); err == nil {
					fp := btcutil.Hash160(pubKey.SerializeCompressed())[:4]
					fingerprintHex := fmt.Sprintf("%X", fp)
					fpWidth := textWidthMM(shareFace, dpi, fingerprintHex)
					fpX := (plateSizeMM - fpWidth) / 2
					verWidth := textWidthMM(shareFace, dpi, version.String())
					verX := plateSizeMM - 5.0 - verWidth
					drawText(canvas, shareFace, dpi, fpX, 5.0, fingerprintHex)
					drawText(canvas, shareFace, dpi, verX, 5.0, version.String())
				}
			}
		}
	}

	// Word columns
	yLeft := 15.0
	for i := 0; i < 16 && i < len(mnemonic); i++ {
		if mnemonic[i] == -1 {
			continue
		}
		word := strings.ToUpper(bip39.LabelFor(mnemonic[i]))
		drawText(canvas, mainFace, dpi, 12.0, yLeft, fmt.Sprintf("%2d %s", i+1, word))
		yLeft += 4.0
	}
	yRight := 15.0
	for i := 16; i < 24 && i < len(mnemonic); i++ {
		if mnemonic[i] == -1 {
			continue
		}
		word := strings.ToUpper(bip39.LabelFor(mnemonic[i]))
		drawText(canvas, mainFace, dpi, 45.0, yRight, fmt.Sprintf("%2d %s", i+1, word))
		yRight += 4.0
	}

	qrRegions := []image.Rectangle{}
	if seed != nil {
		qrContent := seedqr.QR(mnemonic)
		if len(qrContent) > 0 {
			qrCode, err := qr.Encode(string(qrContent), qr.M)
			if err == nil {
				qrSize := 28.0
				const quiet = 4
				step := qrSize / float64(qrCode.Size+2*quiet)
				offset := float64(quiet) * step
				qrX := 48.5 - offset
				// Align QR bottom to the 16th word baseline (yLeft base + 15*4mm).
				qrY := (15.0 + float64(15)*4.0) - qrSize
				drawQR(canvas, qrCode, dpi, qrX, qrY, qrSize, blackIdx)
				qrRegions = append(qrRegions, image.Rect(mmToPx(qrX, dpi), mmToPx(qrY, dpi), mmToPx(qrX+qrSize, dpi), mmToPx(qrY+qrSize, dpi)))
			}
		}

		title := walletLabel()
		titleFace := loadFaceMedium(6, dpi)
		titleY := plateSizeMM - 3.0
		drawCenteredText(canvas, titleFace, dpi, titleY, title)
	}

	if opts.Invert {
		invertExcept(canvas, qrRegions)
	}
	applyPostProcess(canvas, opts)
	return canvas, nil
}

// RenderDescriptorPlateBitmap mirrors the descriptor PDF layout at 600dpi as a 1-bit paletted image.
func RenderDescriptorPlateBitmap(desc *urtypes.OutputDescriptor, keyIdx, shareNum, totalShares int, opts RasterOptions, qrPayload string) (*image.Paletted, error) {
	if desc == nil {
		return nil, fmt.Errorf("descriptor is nil")
	}
	dpi := opts.dpi()
	canvas := newPlateCanvas(dpi)
	blackIdx := uint8(1)

	border := mmToPx(borderWidthMM, dpi)
	if border < 1 {
		border = 1
	}
	strokeRect(canvas, 0, 0, canvas.Bounds().Dx(), canvas.Bounds().Dy(), border, blackIdx)

	smallFace := loadFaceMedium(6, dpi)
	mainFace := loadFace(8, dpi)
	drawText(canvas, smallFace, dpi, 5.0, 5.0, fmt.Sprintf("%d/%d", shareNum, totalShares))
	pathStr := derivationPathForKey(desc.Keys[keyIdx], desc.Script)
	pathWidth := textWidthMM(smallFace, dpi, fmt.Sprintf("Path:%s", pathStr))
	pathX := plateSizeMM - 5.0 - pathWidth
	drawText(canvas, smallFace, dpi, pathX, 5.0, fmt.Sprintf("Path:%s", pathStr))

	key := desc.Keys[keyIdx]
	allText := fmt.Sprintf("Type:%v/Script:%s/Threshold:%d/Keys:%d/Key%d:%s",
		desc.Type, strings.Replace(desc.Script.String(), " ", "", -1), desc.Threshold, len(desc.Keys), keyIdx+1, key.String())

	lines := wrapText(mainFace, dpi, allText, plateSizeMM-10.0)
	lineHeightPx := float64(mainFace.Metrics().Height.Ceil())
	lineHeightMM := lineHeightPx * 25.4 / dpi
	lineSpacing := 3.5
	y := 10.0
	for i, line := range lines {
		drawText(canvas, mainFace, dpi, 5.0, y, line)
		if i < len(lines)-1 {
			y += lineSpacing
		}
	}
	qrContent := qrPayload
	if qrContent == "" {
		qrContent = createDescriptorQR(desc)
	}
	if len(qrContent) == 0 {
		return nil, fmt.Errorf("empty descriptor QR content")
	}
	var shMeta *shard.Share
	if strings.HasPrefix(strings.ToUpper(qrContent), shard.Prefix) {
		if sh, err := shard.Decode(strings.ToUpper(qrContent)); err == nil {
			shMeta = &sh
		}
	}
	qrCode, err := qr.Encode(qrContent, descriptorQRECC)
	if err != nil {
		return nil, err
	}

	textLines := float64(len(lines))
	textBlockHeight := lineHeightMM
	if textLines > 1 {
		textBlockHeight += (textLines - 1) * lineSpacing
	}
	textBottom := 7.0 + textBlockHeight
	qrGap := 2.0    // gap between text and QR
	qrBottom := 1.0 // bottom margin
	qrSize := plateSizeMM - textBottom - qrGap - qrBottom
	if qrSize > descriptorQRSizeMM && descriptorQRSizeMM > 0 {
		qrSize = descriptorQRSizeMM
	}
	if qrSize < 5.0 {
		qrSize = 5.0 // Prevent degenerate QR
	}
	qrX := (plateSizeMM - qrSize) / 2
	qrY := textBottom + qrGap
	drawQR(canvas, qrCode, dpi, qrX, qrY, qrSize, blackIdx)
	if shMeta != nil {
		wid := strings.ToUpper(hex.EncodeToString(shMeta.WalletID[:4]))
		sid := strings.ToUpper(hex.EncodeToString(shMeta.SetID[:4]))
		meta := fmt.Sprintf("WID:%s SET:%s %d/%d", wid, sid, shMeta.Index, shMeta.Threshold)
		drawRotatedSideMeta(canvas, smallFace, dpi, qrX, qrY, qrSize, meta, blackIdx)
	}
	qrRegions := []image.Rectangle{
		image.Rect(mmToPx(qrX, dpi), mmToPx(qrY, dpi), mmToPx(qrX+qrSize, dpi), mmToPx(qrY+qrSize, dpi)),
	}
	if opts.Invert {
		invertExcept(canvas, qrRegions)
	}
	applyPostProcess(canvas, opts)
	return canvas, nil
}

func descriptorShardQRCodes(desc *urtypes.OutputDescriptor, totalShares int) ([]string, error) {
	if desc == nil {
		return nil, fmt.Errorf("descriptor is nil")
	}
	if totalShares <= 0 {
		return nil, fmt.Errorf("invalid share count: %d", totalShares)
	}
	threshold := desc.Threshold
	if threshold == 1 && totalShares == 1 {
		qr := createDescriptorQR(desc)
		if qr == "" {
			return nil, fmt.Errorf("empty descriptor QR content")
		}
		return []string{qr}, nil
	}
	if threshold < 2 || threshold > totalShares {
		return nil, fmt.Errorf("invalid descriptor threshold %d for %d shares", threshold, totalShares)
	}
	payload := desc.Encode()
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty descriptor payload")
	}
	opts := shard.SplitOptions{
		Threshold: uint8(threshold),
		Total:     uint8(totalShares),
	}
	if setID, ok := forcedDescriptorShardSetID(); ok {
		opts.SetID = setID
	}
	shares, err := shard.SplitPayloadBytes(payload, opts)
	if err != nil {
		return nil, fmt.Errorf("split descriptor payload: %w", err)
	}
	out := make([]string, len(shares))
	for i, sh := range shares {
		enc, err := shard.Encode(sh)
		if err != nil {
			return nil, fmt.Errorf("encode share %d: %w", i+1, err)
		}
		out[i] = enc
	}
	return out, nil
}

// SetDescriptorShardSetID forces the descriptor shard set_id used during plate
// generation. Pass nil to clear and return to random per-job set IDs.
func SetDescriptorShardSetID(id *[16]byte) {
	shardSetMu.Lock()
	defer shardSetMu.Unlock()
	if id == nil {
		forcedShardSet = nil
		return
	}
	v := *id
	forcedShardSet = &v
}

func forcedDescriptorShardSetID() ([16]byte, bool) {
	shardSetMu.RLock()
	defer shardSetMu.RUnlock()
	if forcedShardSet == nil {
		return [16]byte{}, false
	}
	return *forcedShardSet, true
}

// SavePNG writes a paletted image to disk.
func SavePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// ---- helpers ----

func (o RasterOptions) dpi() float64 {
	if o.DPI <= 0 {
		return 600
	}
	return o.DPI
}

func newPlateCanvas(dpi float64) *image.Paletted {
	sizePx := mmToPx(plateSizeMM, dpi)
	return image.NewPaletted(image.Rect(0, 0, sizePx, sizePx), bwPalette)
}

func mmToPx(mm, dpi float64) int {
	return int(math.Round(mm / 25.4 * dpi))
}

func mmToPxFloat(mm, dpi float64) float64 {
	return mm / 25.4 * dpi
}

func textWidthMM(face font.Face, dpi float64, text string) float64 {
	d := font.Drawer{
		Face: face,
	}
	wPx := d.MeasureString(text).Round()
	return float64(wPx) * 25.4 / dpi
}

func strokeRect(img *image.Paletted, x, y, w, h, thickness int, idx uint8) {
	fillRect(img, x, y, w, thickness, idx)             // top
	fillRect(img, x, y+h-thickness, w, thickness, idx) // bottom
	fillRect(img, x, y, thickness, h, idx)             // left
	fillRect(img, x+w-thickness, y, thickness, h, idx) // right
}

func fillRect(img *image.Paletted, x, y, w, h int, idx uint8) {
	b := img.Bounds()
	x0, y0 := clamp(x, b.Min.X, b.Max.X), clamp(y, b.Min.Y, b.Max.Y)
	x1, y1 := clamp(x+w, b.Min.X, b.Max.X), clamp(y+h, b.Min.Y, b.Max.Y)
	if x1 <= x0 || y1 <= y0 {
		return
	}
	for yy := y0; yy < y1; yy++ {
		row := img.Pix[yy*img.Stride:]
		for xx := x0; xx < x1; xx++ {
			row[xx] = idx
		}
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func drawText(img *image.Paletted, face font.Face, dpi, xMm, yMm float64, text string) {
	d := font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.Black),
		Face: face,
		Dot: fixed.Point26_6{
			X: fixed.I(int(math.Round(mmToPxFloat(xMm, dpi)))),
			Y: fixed.I(int(math.Round(mmToPxFloat(yMm, dpi)))),
		},
	}
	d.DrawString(text)
}

func drawCenteredText(img *image.Paletted, face font.Face, dpi, yMm float64, text string) {
	d := font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.Black),
		Face: face,
	}
	textWidth := d.MeasureString(text).Round()
	xPx := (img.Bounds().Dx() - textWidth) / 2
	d.Dot = fixed.Point26_6{
		X: fixed.I(xPx),
		Y: fixed.I(int(math.Round(mmToPxFloat(yMm, dpi)))),
	}
	d.DrawString(text)
}

func drawRotatedSideMeta(img *image.Paletted, face font.Face, dpi, qrX, qrY, qrSize float64, text string, idx uint8) {
	if text == "" {
		return
	}
	const (
		sideMarginMM = 1.2
		qrGapMM      = 1.2
	)
	rotWmm, rotHmm := rotatedTextSizeMM(face, dpi, text)
	if rotWmm <= 0 || rotHmm <= 0 {
		return
	}
	leftAvail := (qrX - qrGapMM) - sideMarginMM
	rightAvail := (plateSizeMM - sideMarginMM) - (qrX + qrSize + qrGapMM)
	if rotWmm > leftAvail && rotWmm > rightAvail {
		return // no safe strip wide enough; never overlap QR
	}
	xMm := sideMarginMM
	if rightAvail >= rotWmm && rightAvail >= leftAvail {
		xMm = qrX + qrSize + qrGapMM
	} else {
		xMm = sideMarginMM + (leftAvail-rotWmm)/2
	}
	yMm := qrY + (qrSize-rotHmm)/2
	if yMm < sideMarginMM {
		yMm = sideMarginMM
	}
	maxY := plateSizeMM - sideMarginMM - rotHmm
	if yMm > maxY {
		yMm = maxY
	}
	drawTextRotatedCW90(img, face, dpi, xMm, yMm, text, idx)
}

func rotatedTextSizeMM(face font.Face, dpi float64, text string) (wMm, hMm float64) {
	if text == "" {
		return 0, 0
	}
	d := font.Drawer{Face: face}
	wPx := d.MeasureString(text).Ceil()
	if wPx <= 0 {
		return 0, 0
	}
	m := face.Metrics()
	hPx := (m.Ascent + m.Descent).Ceil()
	if hPx <= 0 {
		return 0, 0
	}
	// Rotated CW 90: width/height swap.
	return float64(hPx) * 25.4 / dpi, float64(wPx) * 25.4 / dpi
}

func drawTextRotatedCW90(img *image.Paletted, face font.Face, dpi, xMm, yMm float64, text string, idx uint8) {
	d := font.Drawer{Face: face}
	srcW := d.MeasureString(text).Ceil()
	if srcW <= 0 {
		return
	}
	metrics := face.Metrics()
	srcH := (metrics.Ascent + metrics.Descent).Ceil()
	if srcH <= 0 {
		return
	}

	src := image.NewAlpha(image.Rect(0, 0, srcW, srcH))
	d = font.Drawer{
		Dst:  src,
		Src:  image.NewUniform(color.Alpha{A: 0xff}),
		Face: face,
		Dot: fixed.Point26_6{
			X: 0,
			Y: fixed.I(metrics.Ascent.Ceil()),
		},
	}
	d.DrawString(text)

	x0 := mmToPx(xMm, dpi)
	y0 := mmToPx(yMm, dpi)
	b := img.Bounds()
	for sy := 0; sy < srcH; sy++ {
		for sx := 0; sx < srcW; sx++ {
			if src.AlphaAt(sx, sy).A == 0 {
				continue
			}
			dx := srcH - 1 - sy
			dy := sx
			x := x0 + dx
			y := y0 + dy
			if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
				continue
			}
			img.Pix[y*img.Stride+x] = idx
		}
	}
}

func drawQR(img *image.Paletted, code *qr.Code, dpi, xMm, yMm, sizeMm float64, idx uint8) {
	if code == nil {
		return
	}
	x0 := mmToPx(xMm, dpi)
	y0 := mmToPx(yMm, dpi)
	sizePx := mmToPx(sizeMm, dpi)
	const quiet = 4
	step := float64(sizePx) / float64(code.Size+2*quiet)
	offset := int(math.Round(float64(quiet) * step))

	for y := 0; y < code.Size; y++ {
		yStart := y0 + offset + int(math.Round(float64(y)*step))
		yEnd := y0 + offset + int(math.Round(float64(y+1)*step))
		for x := 0; x < code.Size; x++ {
			if !code.Black(x, y) {
				continue
			}
			xStart := x0 + offset + int(math.Round(float64(x)*step))
			xEnd := x0 + offset + int(math.Round(float64(x+1)*step))
			fillRect(img, xStart, yStart, xEnd-xStart, yEnd-yStart, idx)
		}
	}
}

// wrapText performs character-level wrapping to ensure long descriptors fit even without spaces.
func wrapText(face font.Face, dpi float64, text string, maxWidthMm float64) []string {
	var lines []string
	if maxWidthMm <= 0 {
		return []string{text}
	}
	maxPx := int(math.Round(mmToPxFloat(maxWidthMm, dpi)))
	if maxPx <= 0 {
		return []string{text}
	}

	d := font.Drawer{Face: face}
	var buf []rune
	for _, r := range text {
		buf = append(buf, r)
		if d.MeasureString(string(buf)).Ceil() > maxPx {
			// Overflow: push previous run and start new line with current rune.
			if len(buf) > 1 {
				lines = append(lines, string(buf[:len(buf)-1]))
				buf = buf[len(buf)-1:]
			} else {
				// Single rune too wide; force as line.
				lines = append(lines, string(buf))
				buf = buf[:0]
			}
		}
	}
	if len(buf) > 0 {
		lines = append(lines, string(buf))
	}
	return lines
}

func loadFace(sizePt, dpi float64) font.Face {
	key := [2]float64{sizePt, dpi}
	faceMu.Lock()
	if face, ok := faceCache[key]; ok {
		faceMu.Unlock()
		return face
	}
	faceMu.Unlock()

	fontOnce.Do(func() {
		data := loadFontData(martianMono)
		if data == nil {
			fontErr = fmt.Errorf("font data %s not found", martianMono)
			return
		}
		fontFaceData, fontErr = opentype.Parse(data)
	})

	if fontErr == nil && fontFaceData != nil {
		if face, err := opentype.NewFace(fontFaceData, &opentype.FaceOptions{
			Size:    sizePt,
			DPI:     dpi,
			Hinting: font.HintingFull,
		}); err == nil {
			faceMu.Lock()
			faceCache[key] = face
			faceMu.Unlock()
			return face
		}
	}
	return basicfont.Face7x13
}

func applyPostProcess(img *image.Paletted, opts RasterOptions) {
	if opts.Mirror {
		mirrorHorizontal(img)
	}
}

func invertExcept(img *image.Paletted, keep []image.Rectangle) {
	keepMask := make([]bool, img.Bounds().Dx()*img.Bounds().Dy())
	for _, r := range keep {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				idx := y*img.Stride + x
				if idx >= 0 && idx < len(keepMask) {
					keepMask[idx] = true
				}
			}
		}
	}
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		row := img.Pix[y*img.Stride:]
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			idx := y*img.Stride + x
			if keepMask[idx] {
				continue
			}
			if row[x] == 0 {
				row[x] = 1
			} else if row[x] == 1 {
				row[x] = 0
			}
		}
	}
}

func mirrorHorizontal(img *image.Paletted) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	for y := 0; y < h; y++ {
		row := img.Pix[y*img.Stride:]
		for x := 0; x < w/2; x++ {
			row[x], row[w-1-x] = row[w-1-x], row[x]
		}
	}
}

func loadFaceMedium(sizePt, dpi float64) font.Face {
	key := [2]float64{sizePt, dpi}
	faceMuMedium.Lock()
	if face, ok := faceCacheMedium[key]; ok {
		faceMuMedium.Unlock()
		return face
	}
	faceMuMedium.Unlock()

	fontOnceMedium.Do(func() {
		data := loadFontData(martianMonoMedium)
		if data == nil {
			fontErrMedium = fmt.Errorf("font data %s not found", martianMonoMedium)
			return
		}
		fontFaceDataMedium, fontErrMedium = opentype.Parse(data)
	})

	if fontErrMedium == nil && fontFaceDataMedium != nil {
		if face, err := opentype.NewFace(fontFaceDataMedium, &opentype.FaceOptions{
			Size:    sizePt,
			DPI:     dpi,
			Hinting: font.HintingFull,
		}); err == nil {
			faceMuMedium.Lock()
			faceCacheMedium[key] = face
			faceMuMedium.Unlock()
			return face
		}
	}

	return loadFace(sizePt, dpi)
}
