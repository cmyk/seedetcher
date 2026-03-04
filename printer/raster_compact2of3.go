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
	wordTrackPx := 0.04 * 11.0 * dpi / 72.0
	wordLeadingMM := 9.8 * 25.4 / 72.0

	const (
		topMarginMM         = 3.0
		topLeftXMM          = 8.5
		topRightRightMM     = plateSizeMM - 3.0
		leftPathXMM         = 3.0
		wordsStartTopCapYMM = 8.0
		col1WordsXMM        = 8.0
		col2WordsXMM        = 34.0
		col3WordsXMM        = 61.0
		descQRSizeMM        = 59.0
		seedQRSizeMM        = 27.0
		qrPairRightMarginMM = 3.0
	)

	fpText := strings.ToUpper(fmt.Sprintf("%08x", desc.Keys[keyIdx].MasterFingerprint))
	topBaselineY := topMarginMM + capBaselineOffsetMM(metaFace, dpi)
	DrawMetaLine(canvas, dpi, topLeftXMM, topBaselineY, metaFace, metaTrackPx, fpText)
	label := strings.ToUpper(walletLabel())
	labelW := trackedTextWidthMM(metaFace, dpi, label, metaTrackPx)
	DrawMetaLine(canvas, dpi, topRightRightMM-labelW, topBaselineY, metaFace, metaTrackPx, label)

	path := strings.ToUpper(derivationPathForKey(desc.Keys[keyIdx], desc.Script))
	leftMeta := fmt.Sprintf("%s/%s/NET:%s", path, desc.Script.Tag(), descriptorNetworkTag(desc.Keys[keyIdx].Network))
	DrawRotatedLabel(canvas, dpi, leftPathXMM, topMarginMM, metaFace, metaTrackPx, blackIdx, leftMeta)

	nm := fmt.Sprintf("%d/%d(%d/%d)", keyIdx+1, len(desc.Keys), desc.Threshold, len(desc.Keys))
	_, nmRotH := rotatedTextSizeMM(metaFace, dpi, nm)
	nmY := plateSizeMM - topMarginMM - nmRotH
	if nmY < topMarginMM {
		nmY = topMarginMM
	}
	DrawRotatedLabel(canvas, dpi, leftPathXMM, nmY, metaFace, metaTrackPxNm, blackIdx, nm)

	descQRX := plateSizeMM - qrPairRightMarginMM - descQRSizeMM + 3
	descQRY := plateSizeMM - descQRSizeMM
	seedQRX := descQRX - seedQRSizeMM + 2.5
	seedQRY := plateSizeMM - seedQRSizeMM

	wordStartBaselineY := wordsStartTopCapYMM + capBaselineOffsetMM(wordFace, dpi)
	numColW := trackedTextWidthMM(wordFace, dpi, "24", wordTrackPx)
	spaceW := trackedTextWidthMM(wordFace, dpi, " ", wordTrackPx) + 0.1 // space between numbers col and word col
	y1 := wordStartBaselineY
	y2 := wordStartBaselineY
	y3 := wordStartBaselineY
	leading := wordLeadingMM
	col1Count := len(mnemonic) / 2
	col2Count := len(mnemonic) - col1Count
	col3Count := 0
	if len(mnemonic) == 24 {
		col1Count, col2Count, col3Count = 10, 7, 7
	} else if len(mnemonic) == 12 {
		col1Count, col2Count, col3Count = 6, 6, 0
	}
	for i := 0; i < len(mnemonic); i++ {
		if mnemonic[i] == -1 {
			continue
		}
		num := fmt.Sprintf("%d", i+1)
		word := strings.ToUpper(bip39.LabelFor(mnemonic[i]))
		numW := trackedTextWidthMM(wordFace, dpi, num, wordTrackPx)
		if i < col1Count {
			drawTrackedText(canvas, wordFace, dpi, col1WordsXMM+numColW-numW, y1, num, wordTrackPx)
			drawTrackedText(canvas, wordFace, dpi, col1WordsXMM+numColW+spaceW, y1, word, wordTrackPx)
			y1 += leading
			continue
		}
		if i < col1Count+col2Count {
			drawTrackedText(canvas, wordFace, dpi, col2WordsXMM+numColW-numW, y2, num, wordTrackPx)
			drawTrackedText(canvas, wordFace, dpi, col2WordsXMM+numColW+spaceW, y2, word, wordTrackPx)
			y2 += leading
			continue
		}
		if col3Count > 0 {
			drawTrackedText(canvas, wordFace, dpi, col3WordsXMM+numColW-numW, y3, num, wordTrackPx)
			drawTrackedText(canvas, wordFace, dpi, col3WordsXMM+numColW+spaceW, y3, word, wordTrackPx)
			y3 += leading
		}
	}

	// Compact warning block in lower-left free space.
	warnFace := loadFace(10, dpi)
	warnTrackPx := 0.04 * 10.0 * dpi / 72.0
	warnLeadingMM := 9.7 * 25.4 / 72.0
	descrX := 18.0
	descrBaselineY := 47.0 + capBaselineOffsetMM(warnFace, dpi)
	DrawMetaLine(canvas, dpi, descrX, descrBaselineY, warnFace, warnTrackPx, "DESCR→")

	warnX := 9.0
	warnBaselineY := 50.0 + capBaselineOffsetMM(warnFace, dpi)
	_ = DrawTextBlock(canvas, dpi, TextBlock{
		Face:      warnFace,
		Tracking:  warnTrackPx,
		LeadingMM: warnLeadingMM,
		WidthMM:   24.0,
		Align:     TextAlignStart,
		OriginXMM: warnX,
		OriginYMM: warnBaselineY,
	}, "↑\nNEVER SCAN\nWITH ONLINE\nDEVICE↓")

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
