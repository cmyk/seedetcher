package printer

import (
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
	"seedetcher.com/seedqr"
	"seedetcher.com/version"
)

// RasterOptions controls the bitmap output used for raw printer jobs.
type RasterOptions struct {
	DPI    float64 // target resolution; defaults to 600 if unset
	Mirror bool    // mirror horizontally (for toner transfer)
	Invert bool    // swap black/white for negative output
	// SinglesigLayout controls singlesig plate rendering strategy.
	// Zero value keeps the current default: seed + descriptor info (single-sided).
	SinglesigLayout SinglesigLayoutMode
	// EtchStatsPage appends an additional page with per-plate coverage metrics.
	EtchStatsPage bool
}

type SinglesigLayoutMode uint8

const (
	// Default zero-value behavior.
	SinglesigLayoutSeedWithInfo SinglesigLayoutMode = iota
	SinglesigLayoutSeedOnly
	SinglesigLayoutSeedWithDescriptorQR
)

type plateQRShape uint8

const (
	plateQRSquare plateQRShape = iota
	plateQRCircle
)

type plateQROptions struct {
	QuietModules      int
	Shape             plateQRShape
	KeepIslandsSquare bool
}

const (
	plateSizeMM   = 90.0
	borderWidthMM = 0.2
	// Relative circle diameter for non-island QR modules on plate render.
	// 1.0 fills the whole module cell, smaller values leave more white margin.
	plateQRDotScale = 0.7
)

var (
	bwPalette = color.Palette{color.White, color.Black}

	plateFontPrimary = "font/seedetcher/SeedEtcher-Regular.ttf"

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
	compactMu      sync.RWMutex
	compact2of3On  bool
)

type seedPlateLayout struct {
	LeftColXMM    float64
	RightColXMM   float64
	QRLeftMM      float64
	RightMetaText string
	ShareNum      int
	ShareTotal    int
}

func defaultSeedPlateLayout(totalShares int, singlesigVariant bool) seedPlateLayout {
	layout := seedPlateLayout{
		LeftColXMM:  10.0,
		RightColXMM: 49.0,
		QRLeftMM:    49.0,
	}
	if totalShares == 1 || singlesigVariant {
		layout.LeftColXMM = 8.0
		layout.RightColXMM = 47.0
		layout.QRLeftMM = 47.0
	}
	return layout
}

// CreatePlateBitmaps renders seed/descriptor plates to 1-bit bitmaps using the existing layout.
func CreatePlateBitmaps(mnemonics []bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, opts RasterOptions, progress ProgressFunc) ([]*image.Paletted, []*image.Paletted, error) {
	totalShares := len(mnemonics)
	isSinglesigDesc := desc != nil && len(desc.Keys) == 1 && desc.Type == urtypes.Singlesig
	includeSinglesigInfo := isSinglesigDesc && opts.SinglesigLayout == SinglesigLayoutSeedWithInfo
	includeSinglesigDescriptorSide := isSinglesigDesc && opts.SinglesigLayout == SinglesigLayoutSeedWithDescriptorQR
	isSinglesigJob := desc == nil || isSinglesigDesc
	if desc != nil && len(desc.Keys) > 0 && !isSinglesigDesc {
		totalShares = len(desc.Keys)
	}
	// Singlesig seed-side variant: give space for optional right-edge metadata.
	seedLayout := defaultSeedPlateLayout(totalShares, isSinglesigDesc)
	if isSinglesigJob {
		// Plate marker is wallet-key pagination, not physical copy count.
		seedLayout.ShareNum = 1
		seedLayout.ShareTotal = 1
	}
	if includeSinglesigInfo {
		path := strings.ToUpper(derivationPathForKey(desc.Keys[0], desc.Script))
		seedLayout.RightMetaText = fmt.Sprintf("%s/%s/NET:%s", path, desc.Script.Tag(), descriptorNetworkTag(desc.Keys[0].Network))
	}

	seedImgs := make([]*image.Paletted, totalShares)
	var descImgs []*image.Paletted
	hasDesc := desc != nil && len(desc.Keys) > 0 && (!isSinglesigDesc || includeSinglesigDescriptorSide)
	if hasDesc {
		descImgs = make([]*image.Paletted, totalShares)
	}
	var shardQRPayloads [][]string
	if hasDesc {
		if isSinglesigDesc && includeSinglesigDescriptorSide {
			qrPayload := createDescriptorQR(desc)
			if qrPayload == "" {
				return nil, nil, fmt.Errorf("empty descriptor QR content")
			}
			shardQRPayloads = make([][]string, totalShares)
			for i := range shardQRPayloads {
				shardQRPayloads[i] = []string{qrPayload}
			}
		} else {
			shardQRPayloads = make([][]string, totalShares)
			for i := 0; i < totalShares; i++ {
				descKeyIdx := i % len(desc.Keys)
				payloads, err := descriptorShardQRPayloadsForShare(desc, totalShares, descKeyIdx)
				if err != nil {
					return nil, nil, err
				}
				shardQRPayloads[i] = payloads
			}
		}
	}
	compactSingleSided := hasDesc &&
		CompactDescriptor2of3Enabled() &&
		desc.Type == urtypes.SortedMulti &&
		desc.Threshold == 2 &&
		len(desc.Keys) == 3 &&
		totalShares == 3 &&
		len(shardQRPayloads) == 3
	if compactSingleSided {
		descImgs = nil
	}

	for i := 0; i < totalShares; i++ {
		mnemonic := mnemonics[i%len(mnemonics)]
		seedImg, err := renderSeedPlateBitmapWithLayout(mnemonic, i+1, totalShares, opts, seedLayout)
		if err != nil {
			return nil, nil, err
		}
		if compactSingleSided {
			descKeyIdx := i % len(desc.Keys)
			sharePayload := ""
			if i < len(shardQRPayloads) && len(shardQRPayloads[i]) > 0 {
				sharePayload = shardQRPayloads[i][0]
			}
			seedImg, err = renderCompact2of3PlateBitmap(mnemonic, desc, descKeyIdx, opts, sharePayload)
			if err != nil {
				return nil, nil, err
			}
		}
		seedImgs[i] = seedImg

		if hasDesc && !compactSingleSided {
			descKeyIdx := i % len(desc.Keys)
			var descQRs []string
			if i < len(shardQRPayloads) {
				descQRs = shardQRPayloads[i]
			}
			descImg, err := RenderDescriptorPlateBitmap(desc, descKeyIdx, i+1, totalShares, opts, descQRs)
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
	return renderSeedPlateBitmapWithLayout(mnemonic, shareNum, totalShares, opts, defaultSeedPlateLayout(totalShares, false))
}

// RenderSeedPlateBitmapWithDescriptor renders a seed plate and applies
// descriptor-derived singlesig metadata when a singlesig descriptor is provided.
func RenderSeedPlateBitmapWithDescriptor(mnemonic bip39.Mnemonic, shareNum, totalShares int, desc *urtypes.OutputDescriptor, opts RasterOptions) (*image.Paletted, error) {
	isSinglesigDesc := desc != nil && len(desc.Keys) == 1 && desc.Type == urtypes.Singlesig
	layout := defaultSeedPlateLayout(totalShares, isSinglesigDesc)
	if isSinglesigDesc {
		path := strings.ToUpper(derivationPathForKey(desc.Keys[0], desc.Script))
		layout.RightMetaText = fmt.Sprintf("%s/%s/NET:%s", path, desc.Script.Tag(), descriptorNetworkTag(desc.Keys[0].Network))
		// Marker is wallet-key pagination, not physical copy count.
		layout.ShareNum = 1
		layout.ShareTotal = 1
	}
	return renderSeedPlateBitmapWithLayout(mnemonic, shareNum, totalShares, opts, layout)
}

// RenderCompact2of3PlateBitmap renders a single-sided compact 2-of-3 plate
// containing both seed and descriptor-share QR payloads.
func RenderCompact2of3PlateBitmap(mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, opts RasterOptions, descQR string) (*image.Paletted, error) {
	return renderCompact2of3PlateBitmap(mnemonic, desc, keyIdx, opts, descQR)
}

func renderSeedPlateBitmapWithLayout(mnemonic bip39.Mnemonic, shareNum, totalShares int, opts RasterOptions, layout seedPlateLayout) (*image.Paletted, error) {
	dpi := opts.dpi()
	canvas := newPlateCanvas(dpi)
	blackIdx := uint8(1)

	border := mmToPx(borderWidthMM, dpi)
	if border < 1 {
		border = 1
	}
	strokeRect(canvas, 0, 0, canvas.Bounds().Dx(), canvas.Bounds().Dy(), border, blackIdx)

	wordFace := loadFace(14, dpi)
	metaFace := loadFace(11, dpi)
	const (
		marginMM    = 3.0
		wordTrackEm = 0.12 // word-list tracking
		numTrackEm  = 0.05 // tighter tracking for index numbers
		numWordGap  = 0.5  // extra gutter (mm) between number and word columns
	)
	leadingMM := 15.2 * 25.4 / 72.0
	wordTrackPx := wordTrackEm * 14.0 * dpi / 72.0
	numTrackPx := numTrackEm * 14.0 * dpi / 72.0
	metaTrackPx := 0.04 * 11.0 * dpi / 72.0 // Affinity tracking as percent of em
	wordStartBaseline := marginMM + capBaselineOffsetMM(wordFace, dpi)

	seed := bip39.MnemonicSeed(mnemonic, "")
	var fingerprintHex string
	if seed != nil {
		masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
		if err == nil {
			if masterPubKey, err := masterKey.Neuter(); err == nil {
				if pubKey, err := masterPubKey.ECPubKey(); err == nil {
					fp := btcutil.Hash160(pubKey.SerializeCompressed())[:4]
					fingerprintHex = fmt.Sprintf("%X", fp)
				}
			}
		}
	}

	// Word columns: right-aligned numbers + one space + left-aligned words.
	numColWMM := trackedTextWidthMM(wordFace, dpi, "24", numTrackPx)
	spaceWMM := trackedTextWidthMM(wordFace, dpi, " ", wordTrackPx) + numWordGap
	yLeft := wordStartBaseline
	for i := 0; i < 16 && i < len(mnemonic); i++ {
		if mnemonic[i] == -1 {
			continue
		}
		num := fmt.Sprintf("%d", i+1)
		word := strings.ToUpper(bip39.LabelFor(mnemonic[i]))
		numW := trackedTextWidthMM(wordFace, dpi, num, numTrackPx)
		drawTrackedText(canvas, wordFace, dpi, layout.LeftColXMM+numColWMM-numW, yLeft, num, numTrackPx)
		drawTrackedText(canvas, wordFace, dpi, layout.LeftColXMM+numColWMM+spaceWMM, yLeft, word, wordTrackPx)
		yLeft += leadingMM
	}
	yRight := wordStartBaseline
	for i := 16; i < 24 && i < len(mnemonic); i++ {
		if mnemonic[i] == -1 {
			continue
		}
		num := fmt.Sprintf("%d", i+1)
		word := strings.ToUpper(bip39.LabelFor(mnemonic[i]))
		numW := trackedTextWidthMM(wordFace, dpi, num, numTrackPx)
		drawTrackedText(canvas, wordFace, dpi, layout.RightColXMM+numColWMM-numW, yRight, num, numTrackPx)
		drawTrackedText(canvas, wordFace, dpi, layout.RightColXMM+numColWMM+spaceWMM, yRight, word, wordTrackPx)
		yRight += leadingMM
	}

	if seed != nil {
		qrContent := seedqr.QR(mnemonic)
		if len(qrContent) > 0 {
			qrCode, err := qr.Encode(string(qrContent), qr.M)
			if err == nil {
				qrSize := 28.0
				qrX := layout.QRLeftMM + 0.5
				// Fixed vertical anchor for seed QR: keep bottom 12mm above plate edge.
				qrY := plateSizeMM - 12.0 - qrSize
				drawPlateQR(canvas, qrCode, dpi, qrX, qrY, qrSize, blackIdx, plateQROptions{
					QuietModules:      0,
					Shape:             plateQRCircle,
					KeepIslandsSquare: true,
				})
			}
		}

		title := walletLabel()
		titleW := trackedTextWidthMM(metaFace, dpi, title, metaTrackPx)
		titleX := plateSizeMM - marginMM - titleW
		titleY := plateSizeMM - marginMM
		drawTrackedText(canvas, metaFace, dpi, titleX, titleY, title, metaTrackPx)
	}

	showShareNum := shareNum
	showShareTotal := totalShares
	if layout.ShareNum > 0 && layout.ShareTotal > 0 {
		showShareNum = layout.ShareNum
		showShareTotal = layout.ShareTotal
	}
	shareText := fmt.Sprintf("%d/%d", showShareNum, showShareTotal)
	_, shareRotH := rotatedTextSizeMMTracked(metaFace, dpi, shareText, metaTrackPx)
	shareX := marginMM
	shareY := plateSizeMM - marginMM - shareRotH
	drawTextRotatedCCW90Tracked(canvas, metaFace, dpi, shareX, shareY, shareText, blackIdx, metaTrackPx)

	if fingerprintHex != "" {
		_, fpRotH := rotatedTextSizeMMTracked(metaFace, dpi, fingerprintHex, metaTrackPx)
		fpX := marginMM
		fpY := (plateSizeMM - fpRotH) / 2
		drawTextRotatedCCW90Tracked(canvas, metaFace, dpi, fpX, fpY, fingerprintHex, blackIdx, metaTrackPx)
	}

	verText := version.String()
	_, verRotH := rotatedTextSizeMMTracked(metaFace, dpi, verText, metaTrackPx)
	verX := marginMM
	verY := marginMM
	if verY+verRotH > plateSizeMM-marginMM {
		verY = plateSizeMM - marginMM - verRotH
	}
	drawTextRotatedCCW90Tracked(canvas, metaFace, dpi, verX, verY, verText, blackIdx, metaTrackPx)
	if layout.RightMetaText != "" {
		meta := strings.ToUpper(layout.RightMetaText)
		metaRotW, metaRotH := rotatedInkSizeMMTracked(metaFace, dpi, meta, metaTrackPx)
		metaX := plateSizeMM - marginMM - metaRotW
		if metaX < marginMM {
			metaX = marginMM
		}
		metaY := marginMM
		if metaY+metaRotH > plateSizeMM-marginMM {
			metaY = plateSizeMM - marginMM - metaRotH
		}
		drawTextRotatedCCW90Tracked(canvas, metaFace, dpi, metaX, metaY, meta, blackIdx, metaTrackPx)
	}

	if opts.Invert {
		invertInterior(canvas, border)
	}
	applyPostProcess(canvas, opts)
	return canvas, nil
}

func descriptorNetworkTag(net *chaincfg.Params) string {
	if net != nil && net.Net == chaincfg.MainNetParams.Net {
		return "MAIN"
	}
	return "TEST"
}

// DescriptorShardQRCodes returns descriptor QR payloads (or shard payloads) for each share.
// This is exported for batched host-mode printing paths that need deterministic per-share payloads.
func DescriptorShardQRCodes(desc *urtypes.OutputDescriptor, totalShares int) ([]string, error) {
	return descriptorShardQRCodes(desc, totalShares)
}

// DescriptorShardQRPayloadsForShare returns one or more descriptor QR payloads for a
// given share index. UR/XOR families such as 3-of-5 return two payloads.
func DescriptorShardQRPayloadsForShare(desc *urtypes.OutputDescriptor, totalShares, keyIdx int) ([]string, error) {
	return descriptorShardQRPayloadsForShare(desc, totalShares, keyIdx)
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

// SetCompactDescriptor2of3Enabled toggles compact single-sided 2-of-3 plate rendering.
func SetCompactDescriptor2of3Enabled(on bool) {
	compactMu.Lock()
	defer compactMu.Unlock()
	compact2of3On = on
}

// CompactDescriptor2of3Enabled reports whether compact single-sided 2-of-3
// rendering is enabled.
func CompactDescriptor2of3Enabled() bool {
	compactMu.RLock()
	defer compactMu.RUnlock()
	return compact2of3On
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

func trimNonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func quietZoneMM(code *qr.Code, qrSizeMM float64, quietModules int) float64 {
	if quietModules <= 0 || code == nil || code.Size <= 0 || qrSizeMM <= 0 {
		return 0
	}
	totalModules := float64(code.Size + 2*quietModules)
	moduleMM := qrSizeMM / totalModules
	return moduleMM * float64(quietModules)
}

func drawTrackedText(img *image.Paletted, face font.Face, dpi, xMm, yMm float64, text string, trackingPx float64) {
	d := font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.Black),
		Face: face,
		Dot: fixed.Point26_6{
			X: fixed.I(int(math.Round(mmToPxFloat(xMm, dpi)))),
			Y: fixed.I(int(math.Round(mmToPxFloat(yMm, dpi)))),
		},
	}
	rs := []rune(text)
	trackFixed := fixed.I(int(math.Round(trackingPx)))
	for i, r := range rs {
		d.DrawString(string(r))
		if i < len(rs)-1 && trackingPx > 0 {
			d.Dot.X += trackFixed
		}
	}
}

func trackedTextWidthMM(face font.Face, dpi float64, text string, trackingPx float64) float64 {
	rs := []rune(text)
	if len(rs) == 0 {
		return 0
	}
	var width fixed.Int26_6
	trackFixed := fixed.I(int(math.Round(trackingPx)))
	for i, r := range rs {
		if adv, ok := face.GlyphAdvance(r); ok {
			width += adv
		}
		if i < len(rs)-1 && trackingPx > 0 {
			width += trackFixed
		}
	}
	return float64(width.Ceil()) * 25.4 / dpi
}

func rotatedTextSizeMM(face font.Face, dpi float64, text string) (wMm, hMm float64) {
	return rotatedTextSizeMMTracked(face, dpi, text, 0)
}

func rotatedTextSizeMMTracked(face font.Face, dpi float64, text string, trackingPx float64) (wMm, hMm float64) {
	if text == "" {
		return 0, 0
	}
	wPx := trackedTextWidthPx(face, text, trackingPx)
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

func rotatedInkSizeMMTracked(face font.Face, dpi float64, text string, trackingPx float64) (wMm, hMm float64) {
	src := rasterizeTextAlpha(face, text, trackingPx)
	if src == nil {
		return 0, 0
	}
	minX, minY, maxX, maxY, ok := alphaInkBounds(src)
	if !ok {
		return 0, 0
	}
	trimW := maxX - minX + 1
	trimH := maxY - minY + 1
	return float64(trimH) * 25.4 / dpi, float64(trimW) * 25.4 / dpi
}

func drawTextRotatedCCW90Tracked(img *image.Paletted, face font.Face, dpi, xMm, yMm float64, text string, idx uint8, trackingPx float64) {
	src := rasterizeTextAlpha(face, text, trackingPx)
	if src == nil {
		return
	}
	minX, minY, maxX, maxY, ok := alphaInkBounds(src)
	if !ok {
		return
	}
	trimW := maxX - minX + 1
	trimH := maxY - minY + 1

	x0 := mmToPx(xMm, dpi)
	y0 := mmToPx(yMm, dpi)
	b := img.Bounds()
	for sy := 0; sy < trimH; sy++ {
		for sx := 0; sx < trimW; sx++ {
			tx := minX + sx
			ty := minY + sy
			if src.AlphaAt(tx, ty).A == 0 {
				continue
			}
			dx := sy
			dy := trimW - 1 - sx
			x := x0 + dx
			y := y0 + dy
			if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
				continue
			}
			img.Pix[y*img.Stride+x] = idx
		}
	}
}

func alphaInkBounds(src *image.Alpha) (minX, minY, maxX, maxY int, ok bool) {
	b := src.Bounds()
	minX, minY = b.Max.X, b.Max.Y
	maxX, maxY = b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := src.Pix[y*src.Stride:]
		for x := b.Min.X; x < b.Max.X; x++ {
			if row[x] == 0 {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
			ok = true
		}
	}
	return
}

func trackedTextWidthPx(face font.Face, text string, trackingPx float64) int {
	rs := []rune(text)
	if len(rs) == 0 {
		return 0
	}
	var width fixed.Int26_6
	trackFixed := fixed.I(int(math.Round(trackingPx)))
	for i, r := range rs {
		if adv, ok := face.GlyphAdvance(r); ok {
			width += adv
		}
		if i < len(rs)-1 && trackingPx > 0 {
			width += trackFixed
		}
	}
	return width.Ceil()
}

func rasterizeTextAlpha(face font.Face, text string, trackingPx float64) *image.Alpha {
	if text == "" {
		return nil
	}
	srcW := trackedTextWidthPx(face, text, trackingPx)
	if srcW <= 0 {
		return nil
	}
	metrics := face.Metrics()
	srcH := (metrics.Ascent + metrics.Descent).Ceil()
	if srcH <= 0 {
		return nil
	}
	src := image.NewAlpha(image.Rect(0, 0, srcW, srcH))
	d := font.Drawer{
		Dst:  src,
		Src:  image.NewUniform(color.Alpha{A: 0xff}),
		Face: face,
		Dot: fixed.Point26_6{
			X: 0,
			Y: fixed.I(metrics.Ascent.Ceil()),
		},
	}
	rs := []rune(text)
	trackFixed := fixed.I(int(math.Round(trackingPx)))
	for i, r := range rs {
		d.DrawString(string(r))
		if i < len(rs)-1 && trackingPx > 0 {
			d.Dot.X += trackFixed
		}
	}
	return src
}

func capBaselineOffsetMM(face font.Face, dpi float64) float64 {
	// Anchor by uppercase cap height so the visible top of uppercase letters
	// sits on the requested margin.
	b, _ := font.BoundString(face, "H")
	minYpx := float64(b.Min.Y) / 64.0
	if minYpx >= 0 {
		return float64(face.Metrics().Ascent.Ceil()) * 25.4 / dpi
	}
	return (-minYpx) * 25.4 / dpi
}

func drawPlateQR(img *image.Paletted, code *qr.Code, dpi, xMm, yMm, sizeMm float64, idx uint8, opts plateQROptions) {
	if code == nil {
		return
	}
	if opts.QuietModules < 0 {
		opts.QuietModules = 0
	}
	x0 := mmToPx(xMm, dpi)
	y0 := mmToPx(yMm, dpi)
	sizePx := mmToPx(sizeMm, dpi)
	quiet := opts.QuietModules
	step := float64(sizePx) / float64(code.Size+2*quiet)
	offset := int(math.Round(float64(quiet) * step))
	var islandMask []bool
	if opts.KeepIslandsSquare {
		islandMask = buildQRIslandMask(code)
	}

	for y := 0; y < code.Size; y++ {
		yStart := y0 + offset + int(math.Round(float64(y)*step))
		yEnd := y0 + offset + int(math.Round(float64(y+1)*step))
		for x := 0; x < code.Size; x++ {
			if !code.Black(x, y) {
				continue
			}
			xStart := x0 + offset + int(math.Round(float64(x)*step))
			xEnd := x0 + offset + int(math.Round(float64(x+1)*step))

			useSquare := opts.Shape == plateQRSquare
			if opts.KeepIslandsSquare && islandMask[y*code.Size+x] {
				useSquare = true
			}
			if useSquare {
				fillRect(img, xStart, yStart, xEnd-xStart, yEnd-yStart, idx)
			} else {
				fillModuleCircle(img, xStart, yStart, xEnd, yEnd, idx)
			}
		}
	}
}

func fillModuleCircle(img *image.Paletted, xStart, yStart, xEnd, yEnd int, idx uint8) {
	w := xEnd - xStart
	h := yEnd - yStart
	if w <= 0 || h <= 0 {
		return
	}
	b := img.Bounds()
	if xEnd <= b.Min.X || xStart >= b.Max.X || yEnd <= b.Min.Y || yStart >= b.Max.Y {
		return
	}
	d := w
	if h < d {
		d = h
	}
	scaled := int(math.Round(float64(d) * plateQRDotScale))
	if scaled < 1 {
		scaled = 1
	}
	d = scaled
	if d <= 1 {
		fillRect(img, xStart, yStart, w, h, idx)
		return
	}
	cx2 := 2*xStart + w
	cy2 := 2*yStart + h
	r := d - 1
	r2 := r * r
	y0 := clamp(yStart, b.Min.Y, b.Max.Y)
	y1 := clamp(yEnd, b.Min.Y, b.Max.Y)
	x0 := clamp(xStart, b.Min.X, b.Max.X)
	x1 := clamp(xEnd, b.Min.X, b.Max.X)
	for y := y0; y < y1; y++ {
		dy2 := 2*y + 1 - cy2
		row := img.Pix[y*img.Stride:]
		for x := x0; x < x1; x++ {
			dx2 := 2*x + 1 - cx2
			if dx2*dx2+dy2*dy2 <= r2 {
				row[x] = idx
			}
		}
	}
}

func buildQRIslandMask(code *qr.Code) []bool {
	size := code.Size
	mask := make([]bool, size*size)
	markRect := func(x0, y0, w, h int) {
		for y := y0; y < y0+h; y++ {
			if y < 0 || y >= size {
				continue
			}
			row := y * size
			for x := x0; x < x0+w; x++ {
				if x < 0 || x >= size {
					continue
				}
				mask[row+x] = true
			}
		}
	}

	// Finder patterns are always 7x7 in 3 corners.
	markRect(0, 0, 7, 7)
	markRect(size-7, 0, 7, 7)
	markRect(0, size-7, 7, 7)

	// Detect alignment patterns directly from the encoded module matrix.
	// This avoids version-center math drift and keeps islands stable across sizes.
	for cy := 2; cy <= size-3; cy++ {
		for cx := 2; cx <= size-3; cx++ {
			if !isAlignmentCenter(code, cx, cy) {
				continue
			}
			markRect(cx-2, cy-2, 5, 5)
		}
	}
	return mask
}

func isAlignmentCenter(code *qr.Code, cx, cy int) bool {
	size := code.Size
	if cx-2 < 0 || cy-2 < 0 || cx+2 >= size || cy+2 >= size {
		return false
	}
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			ax := absInt(dx)
			ay := absInt(dy)
			wantBlack := ax == 2 || ay == 2 || (ax == 0 && ay == 0)
			if code.Black(cx+dx, cy+dy) != wantBlack {
				return false
			}
		}
	}
	return true
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func wrapTextTracked(face font.Face, dpi float64, text string, maxWidthMm float64, trackingPx float64) []string {
	var lines []string
	if maxWidthMm <= 0 {
		return []string{text}
	}
	maxPx := int(math.Round(mmToPxFloat(maxWidthMm, dpi)))
	if maxPx <= 0 {
		return []string{text}
	}

	var buf []rune
	for _, r := range text {
		buf = append(buf, r)
		if trackedTextWidthPx(face, string(buf), trackingPx) > maxPx {
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
		data := loadFirstFontData(plateFontPrimary, martianMono)
		if data == nil {
			fontErr = fmt.Errorf("font data not found (tried %s, %s)", plateFontPrimary, martianMono)
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

func invertAll(img *image.Paletted) {
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		row := img.Pix[y*img.Stride:]
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			if row[x] == 0 {
				row[x] = 1
			} else if row[x] == 1 {
				row[x] = 0
			}
		}
	}
}

// invertInterior flips black/white inside the plate while preserving the outer border.
func invertInterior(img *image.Paletted, borderPx int) {
	if borderPx <= 0 {
		invertAll(img)
		return
	}
	b := img.Bounds()
	x0 := b.Min.X + borderPx
	y0 := b.Min.Y + borderPx
	x1 := b.Max.X - borderPx
	y1 := b.Max.Y - borderPx
	if x0 >= x1 || y0 >= y1 {
		return
	}
	for y := y0; y < y1; y++ {
		row := img.Pix[y*img.Stride:]
		for x := x0; x < x1; x++ {
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
		data := loadFirstFontData(martianMonoMedium, martianMono, plateFontPrimary)
		if data == nil {
			fontErrMedium = fmt.Errorf("font data not found (tried %s, %s, %s)", martianMonoMedium, martianMono, plateFontPrimary)
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

func loadFirstFontData(paths ...string) []byte {
	for _, p := range paths {
		if p == "" {
			continue
		}
		if data := loadFontData(p); data != nil {
			return data
		}
	}
	return nil
}
