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

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/kortschak/qr"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
)

// RasterOptions controls the bitmap output used for raw printer jobs.
type RasterOptions struct {
	DPI    float64 // target resolution; defaults to 600 if unset
	Mirror bool    // mirror horizontally (for toner transfer)
	Invert bool    // swap black/white for negative output
	// PrinterLang selects the host printer language path.
	// Default zero value is PCL.
	PrinterLang PrinterLanguage
	// SinglesigLayout controls singlesig plate rendering strategy.
	// Zero value keeps the current default: seed + descriptor info (single-sided).
	SinglesigLayout SinglesigLayoutMode
	// EtchStatsPage appends an additional page with per-plate coverage metrics.
	EtchStatsPage bool
}

type PrinterLanguage uint8

const (
	PrinterLangPCL PrinterLanguage = iota
	PrinterLangPS
)

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
	// PatternCornerRadiusRatio rounds structural QR modules (finder/alignment)
	// as a fraction of module size. 0 keeps sharp corners; max useful is 0.5.
	PatternCornerRadiusRatio float64
}

const (
	plateSizeMM   = 90.0
	borderWidthMM = 0.2
	// Relative circle diameter for non-island QR modules on plate render.
	// 1.0 fills the whole module cell, smaller values leave more white margin.
	plateQRDotScale = 0.7
	// Default structural-module corner radius ratio (code-only switch).
	// Keep at 0.0 for current sharp-corner behavior.
	plateQRPatternCornerRadiusRatio = 0.5
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

// RenderCompact2of3PlateBitmap renders a single-sided compact 2-of-3 plate
// containing both seed and descriptor-share QR payloads.
func RenderCompact2of3PlateBitmap(mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, opts RasterOptions, descQR string) (*image.Paletted, error) {
	return renderCompact2of3PlateBitmap(mnemonic, desc, keyIdx, opts, descQR)
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
