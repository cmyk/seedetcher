package printer

import (
	"fmt"
	"strings"

	"github.com/kortschak/qr"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/seedqr"
)

func buildCompact2of3SeedScene(mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, descQR string) (PlateScene, error) {
	if desc == nil || len(desc.Keys) == 0 {
		return PlateScene{}, fmt.Errorf("descriptor is nil")
	}
	mask := SceneLayer{Tag: "mask", Visible: true}
	mask.Primitives = append(mask.Primitives, ScenePrimitive{
		Kind:        PrimitiveRect,
		XMM:         0,
		YMM:         0,
		WidthMM:     plateSizeMM,
		HeightMM:    plateSizeMM,
		StrokeColor: sceneBlack,
		StrokeMM:    borderWidthMM,
	})

	dpi := 600.0
	metaFace := loadFace(10, dpi)
	wordStyle := newCompact2of3WordStyle()
	metaTrackPx := 0.08 * 10.0 * dpi / 72.0
	metaTrackEm := 0.08

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
	mask.Primitives = append(mask.Primitives, newSceneText(topLeftXMM, topBaselineY, fpText, 10, metaTrackEm, TextDirHorizontal, TextAnchorBaselineLeft))
	label := strings.ToUpper(walletLabel())
	labelW := trackedTextWidthMM(metaFace, dpi, label, metaTrackPx)
	mask.Primitives = append(mask.Primitives, newSceneText(topRightRightMM-labelW, topBaselineY, label, 10, metaTrackEm, TextDirHorizontal, TextAnchorBaselineLeft))

	path := strings.ToUpper(derivationPathForKey(desc.Keys[keyIdx], desc.Script))
	leftMeta := fmt.Sprintf("%s/%s/NET:%s", path, desc.Script.Tag(), descriptorNetworkTag(desc.Keys[keyIdx].Network))
	mask.Primitives = append(mask.Primitives, newSceneText(leftPathXMM, topMarginMM, leftMeta, 10, metaTrackEm, TextDirVerticalUp, TextAnchorTopLeft))

	nm := fmt.Sprintf("%d/%d(%d/%d)", keyIdx+1, len(desc.Keys), desc.Threshold, len(desc.Keys))
	_, nmRotH := rotatedTextSizeMM(metaFace, dpi, nm)
	nmY := plateSizeMM - topMarginMM - nmRotH
	if nmY < topMarginMM {
		nmY = topMarginMM
	}
	mask.Primitives = append(mask.Primitives, newSceneText(leftPathXMM, nmY, nm, 10, 0, TextDirVerticalUp, TextAnchorTopLeft))

	descQRX := plateSizeMM - qrPairRightMarginMM - descQRSizeMM + 3
	descQRY := plateSizeMM - descQRSizeMM
	seedQRX := descQRX - seedQRSizeMM + 2.5
	seedQRY := plateSizeMM - seedQRSizeMM

	wordStartBaselineY := wordsStartTopCapYMM + wordStyle.baselineOffsetMM()
	y1 := wordStartBaselineY
	y2 := wordStartBaselineY
	y3 := wordStartBaselineY
	leading := wordStyle.leading()
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
		switch {
		case i < col1Count:
			mask.Primitives = append(mask.Primitives, wordStyle.linePrimitives(col1WordsXMM, y1, num, word, 0, 0)...)
			y1 += leading
		case i < col1Count+col2Count:
			mask.Primitives = append(mask.Primitives, wordStyle.linePrimitives(col2WordsXMM, y2, num, word, 0, 0)...)
			y2 += leading
		case col3Count > 0:
			mask.Primitives = append(mask.Primitives, wordStyle.linePrimitives(col3WordsXMM, y3, num, word, 0, 0)...)
			y3 += leading
		}
	}

	warnTrackEm := 0.04
	warnLeadingMM := 9.7 * 25.4 / 72.0
	descrX := 18.0
	descrBaselineY := 47.0 + capBaselineOffsetMM(metaFace, dpi)
	mask.Primitives = append(mask.Primitives, newSceneText(descrX, descrBaselineY, "DESCR→", 10, warnTrackEm, TextDirHorizontal, TextAnchorBaselineLeft))
	warnX := 9.0
	warnBaselineY := 50.0 + capBaselineOffsetMM(metaFace, dpi)
	for li, line := range []string{"↑", "NEVER SCAN", "WITH ONLINE", "DEVICE↓"} {
		mask.Primitives = append(mask.Primitives, newSceneText(warnX, warnBaselineY+float64(li)*warnLeadingMM, line, 10, warnTrackEm, TextDirHorizontal, TextAnchorBaselineLeft))
	}

	seedPayload := seedqr.QR(mnemonic)
	if len(seedPayload) > 0 {
		if seedCode, err := qr.Encode(string(seedPayload), qr.M); err == nil {
			mask.Primitives = append(mask.Primitives, sceneQRModules(seedCode, seedQRX, seedQRY, seedQRSizeMM, plateQROptions{
				QuietModules:      4,
				Shape:             plateQRCircle,
				KeepIslandsSquare: true,
			})...)
		}
	}

	qrContent := descQR
	if qrContent == "" {
		qrContent = createDescriptorQR(desc)
	}
	if qrContent == "" {
		return PlateScene{}, fmt.Errorf("empty descriptor QR content")
	}
	descCode, err := qr.Encode(qrContent, descriptorQRECC)
	if err != nil {
		return PlateScene{}, err
	}
	mask.Primitives = append(mask.Primitives, sceneQRModules(descCode, descQRX, descQRY, descQRSizeMM, plateQROptions{
		QuietModules:      4,
		Shape:             plateQRCircle,
		KeepIslandsSquare: true,
	})...)

	return PlateScene{
		WidthMM:  plateSizeMM,
		HeightMM: plateSizeMM,
		Layers:   []SceneLayer{mask},
	}, nil
}
