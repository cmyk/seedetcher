package printer

import (
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/kortschak/qr"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/seedqr"
	"seedetcher.com/version"
)

const (
	sceneVersion = "v0"
	sceneBlack   = "#000000"
)

type SeedSceneBuilder struct {
	Mnemonics []bip39.Mnemonic
	Desc      *urtypes.OutputDescriptor
}

func (b SeedSceneBuilder) Build() (*PlateDocument, error) {
	if len(b.Mnemonics) == 0 {
		return nil, fmt.Errorf("no mnemonics provided")
	}
	totalShares := len(b.Mnemonics)
	isSinglesigDesc := b.Desc != nil && len(b.Desc.Keys) == 1 && b.Desc.Type == urtypes.Singlesig
	if b.Desc != nil && len(b.Desc.Keys) > 0 && !isSinglesigDesc {
		totalShares = len(b.Desc.Keys)
	}
	seedLayout := defaultSeedPlateLayout(totalShares, isSinglesigDesc)
	if b.Desc == nil || isSinglesigDesc {
		seedLayout.ShareNum = 1
		seedLayout.ShareTotal = 1
	}
	if isSinglesigDesc {
		path := strings.ToUpper(derivationPathForKey(b.Desc.Keys[0], b.Desc.Script))
		seedLayout.RightMetaText = fmt.Sprintf("%s/%s/NET:%s", path, b.Desc.Script.Tag(), descriptorNetworkTag(b.Desc.Keys[0].Network))
	}

	doc := &PlateDocument{
		Version: sceneVersion,
		Scenes:  make([]PlateScene, 0, totalShares),
	}
	for i := 0; i < totalShares; i++ {
		mnemonic := b.Mnemonics[i%len(b.Mnemonics)]
		scene, err := buildSeedPlateScene(mnemonic, i+1, totalShares, seedLayout)
		if err != nil {
			return nil, err
		}
		scene.Name = fmt.Sprintf("seed_%02d", i+1)
		doc.Scenes = append(doc.Scenes, scene)
	}
	return doc, nil
}

func buildSeedPlateScene(mnemonic bip39.Mnemonic, shareNum, totalShares int, layout seedPlateLayout) (PlateScene, error) {
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
	wordFace := loadFace(14, dpi)
	metaFace := loadFace(11, dpi)
	const (
		marginMM    = 3.0
		wordTrackEm = 0.12
		numTrackEm  = 0.05
		numWordGap  = 0.5
	)
	leadingMM := 15.2 * 25.4 / 72.0
	wordTrackPx := wordTrackEm * 14.0 * dpi / 72.0
	numTrackPx := numTrackEm * 14.0 * dpi / 72.0
	metaTrackPx := 0.04 * 11.0 * dpi / 72.0
	wordStartBaseline := marginMM + capBaselineOffsetMM(wordFace, dpi)

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
		mask.Primitives = append(mask.Primitives,
			newSceneText(layout.LeftColXMM+numColWMM-numW, yLeft, num, 14, numTrackEm, TextDirHorizontal, TextAnchorBaselineLeft),
			newSceneText(layout.LeftColXMM+numColWMM+spaceWMM, yLeft, word, 14, wordTrackEm, TextDirHorizontal, TextAnchorBaselineLeft),
		)
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
		mask.Primitives = append(mask.Primitives,
			newSceneText(layout.RightColXMM+numColWMM-numW, yRight, num, 14, numTrackEm, TextDirHorizontal, TextAnchorBaselineLeft),
			newSceneText(layout.RightColXMM+numColWMM+spaceWMM, yRight, word, 14, wordTrackEm, TextDirHorizontal, TextAnchorBaselineLeft),
		)
		yRight += leadingMM
	}

	seed := bip39.MnemonicSeed(mnemonic, "")
	if seed != nil {
		qrContent := seedqr.QR(mnemonic)
		if len(qrContent) > 0 {
			qrCode, err := qr.Encode(string(qrContent), qr.M)
			if err == nil {
				mask.Primitives = append(mask.Primitives, sceneQRModules(qrCode, layout.QRLeftMM+0.5, seedQRYMM(seedQRSizeMM), seedQRSizeMM, plateQROptions{
					QuietModules:      0,
					Shape:             plateQRCircle,
					KeepIslandsSquare: true,
				})...)
			}
		}
		title := walletLabel()
		titleW := trackedTextWidthMM(metaFace, dpi, title, metaTrackPx)
		mask.Primitives = append(mask.Primitives, newSceneText(plateSizeMM-marginMM-titleW, plateSizeMM-marginMM, title, 11, 0.04, TextDirHorizontal, TextAnchorBaselineLeft))
	}

	showShareNum := shareNum
	showShareTotal := totalShares
	if layout.ShareNum > 0 && layout.ShareTotal > 0 {
		showShareNum = layout.ShareNum
		showShareTotal = layout.ShareTotal
	}
	shareText := fmt.Sprintf("%d/%d", showShareNum, showShareTotal)
	_, shareRotH := rotatedTextSizeMMTracked(metaFace, dpi, shareText, metaTrackPx)
	mask.Primitives = append(mask.Primitives, newSceneText(marginMM, plateSizeMM-marginMM-shareRotH, shareText, 11, 0.04, TextDirVerticalUp, TextAnchorTopLeft))

	if fp := seedFingerprintHex(seed); fp != "" {
		_, fpRotH := rotatedTextSizeMMTracked(metaFace, dpi, fp, metaTrackPx)
		mask.Primitives = append(mask.Primitives, newSceneText(marginMM, (plateSizeMM-fpRotH)/2, fp, 11, 0.04, TextDirVerticalUp, TextAnchorTopLeft))
	}

	verText := version.String()
	_, verRotH := rotatedTextSizeMMTracked(metaFace, dpi, verText, metaTrackPx)
	verY := marginMM
	if verY+verRotH > plateSizeMM-marginMM {
		verY = plateSizeMM - marginMM - verRotH
	}
	mask.Primitives = append(mask.Primitives, newSceneText(marginMM, verY, verText, 11, 0.04, TextDirVerticalUp, TextAnchorTopLeft))

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
		mask.Primitives = append(mask.Primitives, newSceneText(metaX, metaY, meta, 11, 0.04, TextDirVerticalUp, TextAnchorTopLeft))
	}

	return PlateScene{
		WidthMM:  plateSizeMM,
		HeightMM: plateSizeMM,
		Layers:   []SceneLayer{mask},
	}, nil
}

func seedFingerprintHex(seed []byte) string {
	if len(seed) == 0 {
		return ""
	}
	masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		return ""
	}
	masterPubKey, err := masterKey.Neuter()
	if err != nil {
		return ""
	}
	pubKey, err := masterPubKey.ECPubKey()
	if err != nil {
		return ""
	}
	fp := btcutil.Hash160(pubKey.SerializeCompressed())[:4]
	return fmt.Sprintf("%X", fp)
}
