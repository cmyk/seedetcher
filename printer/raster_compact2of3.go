package printer

import (
	"encoding/hex"
	"fmt"
	"image"
	"strings"

	"github.com/kortschak/qr"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/descriptor/compact2of3"
	"seedetcher.com/descriptor/shard"
	"seedetcher.com/seedqr"
)

func renderCompact2of3PlateBitmap(mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx, shareNum, totalShares int, opts RasterOptions, descQR string) (*image.Paletted, error) {
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
	// Tracking values use the same convention as the rest of the renderer:
	// 0.04 == +40%. So +80% => 0.08, +100% => 0.10.
	metaTrackPx := 0.08 * 10.0 * dpi / 72.0
	metaTrackPxNm := 0 * 10.0 * dpi / 72.0
	wordTrackPx := 0.10 * 11.0 * dpi / 72.0
	wordLeadingMM := 9.8 * 25.4 / 72.0

	const (
		topMarginMM          = 3.0
		topLeftXMM           = 8.5
		topRightRightMM      = plateSizeMM - 3.0
		leftPathXMM          = 3.0
		wordsStartTopCapYMM  = 7.0
		leftWordsXMM         = 11.5
		rightWordsXMM        = 41.5
		descQRSizeMM         = 50.0
		seedQRSizeMM         = 33.0
		qrPairRightMarginMM  = 3.0
		rightMetaBlockRight  = 87.0
		rightMetaBlockBottom = 41.0
		rightMetaBaselineGap = 4.0
		rightMetaSetRightMM  = 2.0
	)

	// Top line: fingerprint (left) and label (right).
	fpText := strings.ToUpper(fmt.Sprintf("%08x", desc.Keys[keyIdx].MasterFingerprint))
	topBaselineY := topMarginMM + capBaselineOffsetMM(metaFace, dpi)
	drawTrackedText(canvas, metaFace, dpi, topLeftXMM, topBaselineY, fpText, metaTrackPx)
	label := strings.ToUpper(walletLabel())
	labelW := trackedTextWidthMM(metaFace, dpi, label, metaTrackPx)
	drawTrackedText(canvas, metaFace, dpi, topRightRightMM-labelW, topBaselineY, label, metaTrackPx)

	// Left vertical top metadata: path/script/network.
	path := strings.ToUpper(derivationPathForKey(desc.Keys[keyIdx], desc.Script))
	path = strings.ReplaceAll(path, "'", "H")
	leftMeta := fmt.Sprintf("%s/%s/NET:%s", path, descriptorScriptTag(desc.Script), descriptorNetworkTag(desc.Keys[keyIdx].Network))
	drawTextRotatedCCW90Tracked(canvas, metaFace, dpi, leftPathXMM, topMarginMM, leftMeta, blackIdx, metaTrackPx)

	// Left vertical bottom marker: plate index and wallet threshold, e.g. 1/3(2/3).
	nm := fmt.Sprintf("%d/%d(%d/%d)", keyIdx+1, len(desc.Keys), desc.Threshold, len(desc.Keys))
	// Anchor using untracked text height so tracking tweaks don't shift vertical placement.
	_, nmRotH := rotatedTextSizeMM(metaFace, dpi, nm)
	nmY := plateSizeMM - topMarginMM - nmRotH
	if nmY < topMarginMM {
		nmY = topMarginMM
	}
	drawTextRotatedCCW90Tracked(canvas, metaFace, dpi, leftPathXMM, nmY, nm, blackIdx, metaTrackPxNm)

	// Bottom QR layout: right descriptor QR + adjacent seed QR.
	// Leave a dedicated right strip for vertical metadata.
	descQRX := plateSizeMM - qrPairRightMarginMM - descQRSizeMM + 2
	descQRY := plateSizeMM - descQRSizeMM
	seedQRX := descQRX - seedQRSizeMM + 2
	seedQRY := plateSizeMM - seedQRSizeMM

	// Word list block. Compact split for 24-word seeds follows the design:
	// left column 1..14, right column 15..24.
	wordStartBaselineY := wordsStartTopCapYMM + capBaselineOffsetMM(wordFace, dpi)
	numColW := trackedTextWidthMM(wordFace, dpi, "24", wordTrackPx)
	spaceW := trackedTextWidthMM(wordFace, dpi, " ", wordTrackPx) + 0.4
	yLeft := wordStartBaselineY
	yRight := wordStartBaselineY
	leftCount := len(mnemonic) / 2
	if len(mnemonic) == 24 {
		leftCount = 14
	}
	// Respect requested 10pt leading, but shrink only if needed to fit each QR top.
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

	// Right metadata block, starting at descriptor QR safe-zone top.
	wid := ""
	sid := ""
	if strings.HasPrefix(strings.ToUpper(qrContent), compact2of3.Prefix) {
		if sh, err := compact2of3.Decode(strings.ToUpper(qrContent)); err == nil {
			wid = strings.ToUpper(hex.EncodeToString(sh.WalletID[:4]))
			sid = strings.ToUpper(hex.EncodeToString(sh.SetID[:4]))
		}
	} else if strings.HasPrefix(strings.ToUpper(qrContent), shard.Prefix) {
		if sh, err := shard.Decode(strings.ToUpper(qrContent)); err == nil {
			wid = strings.ToUpper(hex.EncodeToString(sh.WalletID[:4]))
			sid = strings.ToUpper(hex.EncodeToString(sh.SetID[:4]))
		}
	}
	if wid == "" {
		wid = strings.ToUpper(fmt.Sprintf("%08x", desc.Keys[keyIdx].MasterFingerprint))
	}
	if sid == "" {
		set := shard.DeriveSetID(desc.Encode(), uint8(desc.Threshold), uint8(len(desc.Keys)))
		sid = strings.ToUpper(hex.EncodeToString(set[:4]))
	}
	rightLines := []string{
		"SEEDETCHER.COM",
		"WID:" + wid,
		"SET:" + sid,
	}
	lineHs := make([]float64, len(rightLines))
	lineWs := make([]float64, len(rightLines))
	for i, line := range rightLines {
		w, h := rotatedTextSizeMMTracked(metaFace, dpi, line, metaTrackPx)
		lineWs[i], lineHs[i] = w, h
	}
	// Right metadata columns are positioned by baseline offsets from the
	// right plate edge, per compact-layout spec.
	setBaselineX := plateSizeMM - rightMetaSetRightMM
	widBaselineX := setBaselineX - rightMetaBaselineGap
	urlBaselineX := widBaselineX - rightMetaBaselineGap
	baselines := []float64{setBaselineX, widBaselineX, urlBaselineX}
	for i, line := range rightLines {
		x := baselines[i] - lineWs[i]
		if x < 3.0 {
			x = 3.0
		}
		y := rightMetaBlockBottom - lineHs[i]
		if y < topMarginMM {
			y = topMarginMM
		}
		drawTextRotatedCCW90Tracked(canvas, metaFace, dpi, x, y, line, blackIdx, metaTrackPx)
	}

	if opts.Invert {
		invertInterior(canvas, border)
	}
	applyPostProcess(canvas, opts)
	return canvas, nil
}
