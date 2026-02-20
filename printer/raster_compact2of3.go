package printer

import (
	"fmt"
	"image"
	"strings"

	"github.com/kortschak/qr"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/seedqr"
)

func renderCompact2of3PlateBitmap(mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, opts RasterOptions, descQR string) (*image.Paletted, error) {
	dpi := opts.dpi()
	canvas := newPlateCanvas(dpi)
	blackIdx := uint8(1)

	border := mmToPx(borderWidthMM, dpi)
	if border < 1 {
		border = 1
	}
	strokeRect(canvas, 0, 0, canvas.Bounds().Dx(), canvas.Bounds().Dy(), border, blackIdx)

	metaFace := loadFace(10, dpi)
	wordFace := loadFace(11, dpi)
	metaTrackPx := 0.08 * 10.0 * dpi / 72.0
	metaTrackPxNm := 0 * 10.0 * dpi / 72.0
	wordTrackPx := 0.10 * 11.0 * dpi / 72.0
	wordLeadingMM := 9.8 * 25.4 / 72.0

	const (
		topMarginMM         = 3.0
		topLeftXMM          = 8.5
		topRightRightMM     = plateSizeMM - 3.0
		leftPathXMM         = 3.0
		wordsStartTopCapYMM = 7.0
		leftWordsXMM        = 11.5
		rightWordsXMM       = 41.5
		descQRSizeMM        = 50.0
		seedQRSizeMM        = 33.0
		qrPairRightMarginMM = 3.0
	)

	fpText := strings.ToUpper(fmt.Sprintf("%08x", desc.Keys[keyIdx].MasterFingerprint))
	topBaselineY := topMarginMM + capBaselineOffsetMM(metaFace, dpi)
	drawTrackedText(canvas, metaFace, dpi, topLeftXMM, topBaselineY, fpText, metaTrackPx)
	label := strings.ToUpper(walletLabel())
	labelW := trackedTextWidthMM(metaFace, dpi, label, metaTrackPx)
	drawTrackedText(canvas, metaFace, dpi, topRightRightMM-labelW, topBaselineY, label, metaTrackPx)

	path := strings.ToUpper(derivationPathForKey(desc.Keys[keyIdx], desc.Script))
	path = strings.ReplaceAll(path, "'", "H")
	leftMeta := fmt.Sprintf("%s/%s/NET:%s", path, descriptorScriptTag(desc.Script), descriptorNetworkTag(desc.Keys[keyIdx].Network))
	drawTextRotatedCCW90Tracked(canvas, metaFace, dpi, leftPathXMM, topMarginMM, leftMeta, blackIdx, metaTrackPx)

	nm := fmt.Sprintf("%d/%d(%d/%d)", keyIdx+1, len(desc.Keys), desc.Threshold, len(desc.Keys))
	_, nmRotH := rotatedTextSizeMM(metaFace, dpi, nm)
	nmY := plateSizeMM - topMarginMM - nmRotH
	if nmY < topMarginMM {
		nmY = topMarginMM
	}
	drawTextRotatedCCW90Tracked(canvas, metaFace, dpi, leftPathXMM, nmY, nm, blackIdx, metaTrackPxNm)

	descQRX := plateSizeMM - qrPairRightMarginMM - descQRSizeMM + 2
	descQRY := plateSizeMM - descQRSizeMM
	seedQRX := descQRX - seedQRSizeMM + 2
	seedQRY := plateSizeMM - seedQRSizeMM

	wordStartBaselineY := wordsStartTopCapYMM + capBaselineOffsetMM(wordFace, dpi)
	numColW := trackedTextWidthMM(wordFace, dpi, "24", wordTrackPx)
	spaceW := trackedTextWidthMM(wordFace, dpi, " ", wordTrackPx) + 0.4
	yLeft := wordStartBaselineY
	yRight := wordStartBaselineY
	leftCount := len(mnemonic) / 2
	if len(mnemonic) == 24 {
		leftCount = 14
	}
	leftLeading := wordLeadingMM
	rightLeading := wordLeadingMM
	for i := 0; i < len(mnemonic); i++ {
		if mnemonic[i] == -1 {
			continue
		}
		num := fmt.Sprintf("%d", i+1)
		word := strings.ToUpper(bip39.LabelFor(mnemonic[i]))
		numW := trackedTextWidthMM(wordFace, dpi, num, wordTrackPx)
		if i < leftCount {
			drawTrackedText(canvas, wordFace, dpi, leftWordsXMM+numColW-numW, yLeft, num, wordTrackPx)
			drawTrackedText(canvas, wordFace, dpi, leftWordsXMM+numColW+spaceW, yLeft, word, wordTrackPx)
			yLeft += leftLeading
		} else {
			drawTrackedText(canvas, wordFace, dpi, rightWordsXMM+numColW-numW, yRight, num, wordTrackPx)
			drawTrackedText(canvas, wordFace, dpi, rightWordsXMM+numColW+spaceW, yRight, word, wordTrackPx)
			yRight += rightLeading
		}
	}

	seedPayload := seedqr.QR(mnemonic)
	if len(seedPayload) > 0 {
		if seedCode, err := qr.Encode(string(seedPayload), qr.M); err == nil {
			drawPlateQR(canvas, seedCode, dpi, seedQRX, seedQRY, seedQRSizeMM, blackIdx, plateQROptions{
				QuietModules:      4,
				Shape:             plateQRCircle,
				KeepIslandsSquare: true,
			})
		}
	}

	qrContent := descQR
	if qrContent == "" {
		qrContent = createDescriptorQR(desc)
	}
	if qrContent == "" {
		return nil, fmt.Errorf("empty descriptor QR content")
	}
	descCode, err := qr.Encode(qrContent, descriptorQRECC)
	if err != nil {
		return nil, err
	}
	drawPlateQR(canvas, descCode, dpi, descQRX, descQRY, descQRSizeMM, blackIdx, plateQROptions{
		QuietModules:      4,
		Shape:             plateQRCircle,
		KeepIslandsSquare: true,
	})

	if opts.Invert {
		invertInterior(canvas, border)
	}
	applyPostProcess(canvas, opts)
	return canvas, nil
}
