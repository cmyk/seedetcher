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
	"seedetcher.com/descriptor/urxor2of3"
	"seedetcher.com/seedqr"
	"seedetcher.com/version"
)

// RasterOptions controls the bitmap output used for raw printer jobs.
type RasterOptions struct {
	DPI    float64 // target resolution; defaults to 600 if unset
	Mirror bool    // mirror horizontally (for toner transfer)
	Invert bool    // swap black/white for negative output
	// EtchStatsPage appends an additional page with per-plate coverage metrics.
	EtchStatsPage bool
}

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
	if isSinglesigDesc {
		path := strings.ToUpper(derivationPathForKey(desc.Keys[0], desc.Script))
		path = strings.ReplaceAll(path, "'", "H")
		seedLayout.RightMetaText = fmt.Sprintf("%s/%s/NET:%s", path, descriptorScriptTag(desc.Script), descriptorNetworkTag(desc.Keys[0].Network))
	}

	seedImgs := make([]*image.Paletted, totalShares)
	var descImgs []*image.Paletted
	hasDesc := desc != nil && len(desc.Keys) > 0 && !isSinglesigDesc
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
		seedImg, err := renderSeedPlateBitmapWithLayout(mnemonic, i+1, totalShares, opts, seedLayout)
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
	return renderSeedPlateBitmapWithLayout(mnemonic, shareNum, totalShares, opts, defaultSeedPlateLayout(totalShares, false))
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

	descriptorFace := loadFace(11, dpi)
	pathStr := derivationPathForKey(desc.Keys[keyIdx], desc.Script)
	pathText := fmt.Sprintf("PATH:%s", pathStr)
	descTrackPx := 0.04 * 11.0 * dpi / 72.0 // Affinity tracking as percent of em

	key := desc.Keys[keyIdx]
	allText := fmt.Sprintf("TYPE:%s/SCRIPT:%s/NET:%s/THRESHOLD:%d/KEYS:%d/KEY:%d",
		descriptorTypeTag(desc.Type), descriptorScriptTag(desc.Script), descriptorNetworkTag(key.Network), desc.Threshold, len(desc.Keys), keyIdx+1)

	const margin = 3.0
	ascentMM := capBaselineOffsetMM(descriptorFace, dpi)
	lines := wrapTextTracked(descriptorFace, dpi, allText, plateSizeMM-2*margin, descTrackPx)
	lineSpacing := 4.2
	y := margin + ascentMM
	for i, line := range lines {
		drawTrackedText(canvas, descriptorFace, dpi, margin, y, line, descTrackPx)
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

	// Anchor QR sizing to the actual rendered text bottom:
	// last baseline + font descent.
	descentMM := float64(descriptorFace.Metrics().Descent.Ceil()) * 25.4 / dpi
	lastBaselineY := y
	textBottom := lastBaselineY + descentMM
	qrGap := 2.0    // gap between text and QR
	qrBottom := 3.0 // bottom safe margin for etched plate layout
	// Keep descriptor QR at or below the b0.3 layout target. If an explicit
	// override is set smaller, honor that.
	qrMaxMM := 80.0
	if descriptorQRSizeMM > 0 && descriptorQRSizeMM < qrMaxMM {
		qrMaxMM = descriptorQRSizeMM
	}
	// sizeMm is the full QR footprint (data modules + quiet zone).
	maxByTop := plateSizeMM - qrBottom - (textBottom + qrGap)
	qrSize := qrMaxMM
	if qrSize > maxByTop {
		qrSize = maxByTop
	}
	if qrSize < 5.0 {
		qrSize = 5.0 // Prevent degenerate QR
	}
	qrX := (plateSizeMM - qrSize) / 2
	qrY := plateSizeMM - qrBottom - qrSize
	drawPlateQR(canvas, qrCode, dpi, qrX, qrY, qrSize, blackIdx, plateQROptions{
		QuietModules:      4,
		Shape:             plateQRCircle,
		KeepIslandsSquare: true,
	})
	// Left-side vertical derivation path: 3mm from left and bottom edges.
	_, pathRotH := rotatedTextSizeMMTracked(descriptorFace, dpi, pathText, descTrackPx)
	pathX := margin
	pathY := plateSizeMM - margin - pathRotH
	if pathY < margin {
		pathY = margin
	}
	drawTextRotatedCCW90Tracked(canvas, descriptorFace, dpi, pathX, pathY, pathText, blackIdx, descTrackPx)
	if shMeta != nil {
		wid := strings.ToUpper(hex.EncodeToString(shMeta.WalletID[:4]))
		sid := strings.ToUpper(hex.EncodeToString(shMeta.SetID[:4]))
		meta := fmt.Sprintf("WID:%s SET:%s %d/%d", wid, sid, shMeta.Index, shMeta.Threshold)
		// Vertical WID/SET line: 3mm right of the QR safe-zone edge.
		metaRotW, metaRotH := rotatedInkSizeMMTracked(descriptorFace, dpi, meta, descTrackPx)
		metaX := qrX + qrSize + margin
		if metaX+metaRotW > plateSizeMM-margin {
			metaX = plateSizeMM - margin - metaRotW
		}
		if metaX < margin {
			metaX = margin
		}
		metaY := plateSizeMM - margin - metaRotH
		if metaY < margin {
			metaY = margin
		}
		if metaY+metaRotH > plateSizeMM-margin {
			metaY = plateSizeMM - margin - metaRotH
		}
		drawTextRotatedCCW90Tracked(canvas, descriptorFace, dpi, metaX, metaY, meta, blackIdx, descTrackPx)
	}
	if opts.Invert {
		invertInterior(canvas, border)
	}
	applyPostProcess(canvas, opts)
	return canvas, nil
}

func descriptorTypeTag(t urtypes.MultisigType) string {
	switch t {
	case urtypes.Singlesig:
		return "SINGLESIG"
	case urtypes.SortedMulti:
		return "SORTEDMULTI"
	default:
		return fmt.Sprintf("TYPE%d", int(t))
	}
}

func descriptorScriptTag(s urtypes.Script) string {
	switch s {
	case urtypes.P2SH:
		return "P2SH"
	case urtypes.P2SH_P2WSH:
		return "P2SH-P2WSH"
	case urtypes.P2SH_P2WPKH:
		return "P2SH-P2WPKH"
	case urtypes.P2PKH:
		return "P2PKH"
	case urtypes.P2WSH:
		return "P2WSH"
	case urtypes.P2WPKH:
		return "P2WPKH"
	case urtypes.P2TR:
		return "P2TR"
	default:
		return "UNKNOWN"
	}
}

func descriptorNetworkTag(net *chaincfg.Params) string {
	if net != nil && net.Net == chaincfg.MainNetParams.Net {
		return "MAIN"
	}
	return "TEST"
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
	if desc.Type == urtypes.SortedMulti && threshold == 2 && totalShares == 3 {
		shares, err := urxor2of3.SplitDescriptor(desc)
		if err != nil {
			return nil, fmt.Errorf("split ur/xor descriptor shares: %w", err)
		}
		return shares, nil
	}
	if threshold > math.MaxUint8 {
		return nil, fmt.Errorf("descriptor threshold too large: %d", threshold)
	}
	if totalShares > math.MaxUint8 {
		return nil, fmt.Errorf("descriptor share count too large: %d", totalShares)
	}
	payload := desc.Encode()
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty descriptor payload")
	}
	threshold8 := uint8(threshold)
	totalShares8 := uint8(totalShares)
	opts := shard.SplitOptions{
		Threshold: threshold8,
		Total:     totalShares8,
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

// DescriptorShardQRCodes returns descriptor QR payloads (or shard payloads) for each share.
// This is exported for batched host-mode printing paths that need deterministic per-share payloads.
func DescriptorShardQRCodes(desc *urtypes.OutputDescriptor, totalShares int) ([]string, error) {
	return descriptorShardQRCodes(desc, totalShares)
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

func drawTextRotatedCW90(img *image.Paletted, face font.Face, dpi, xMm, yMm float64, text string, idx uint8) {
	drawTextRotatedCW90Tracked(img, face, dpi, xMm, yMm, text, idx, 0)
}

func drawTextRotatedCW90Tracked(img *image.Paletted, face font.Face, dpi, xMm, yMm float64, text string, idx uint8, trackingPx float64) {
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
			dx := trimH - 1 - sy
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

func drawTextRotatedCCW90(img *image.Paletted, face font.Face, dpi, xMm, yMm float64, text string, idx uint8) {
	drawTextRotatedCCW90Tracked(img, face, dpi, xMm, yMm, text, idx, 0)
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

func inFinder(x, y, fx, fy int) bool {
	return x >= fx && x < fx+7 && y >= fy && y < fy+7
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func alignmentPatternCenters(size int) []int {
	version := (size - 17) / 4
	if version < 2 {
		return nil
	}
	num := version/7 + 2
	if num <= 0 {
		return nil
	}
	step := 0
	if version == 32 {
		step = 26
	} else {
		step = ((size - 13) / (num - 1))
		if step%2 == 1 {
			step++
		}
	}
	centers := make([]int, num)
	centers[0] = 6
	for i := num - 1; i > 0; i-- {
		centers[i] = size - 7 - (num-1-i)*step
	}
	return centers
}

// wrapText performs character-level wrapping to ensure long descriptors fit even without spaces.
func wrapText(face font.Face, dpi float64, text string, maxWidthMm float64) []string {
	return wrapTextTracked(face, dpi, text, maxWidthMm, 0)
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
