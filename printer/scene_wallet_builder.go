package printer

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/kortschak/qr"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
)

type WalletSceneBuilder struct {
	Mnemonics       []bip39.Mnemonic
	Desc            *urtypes.OutputDescriptor
	SinglesigLayout SinglesigLayoutMode
}

func (b WalletSceneBuilder) Build() (*PlateDocument, error) {
	if len(b.Mnemonics) == 0 {
		return nil, fmt.Errorf("no mnemonics provided")
	}
	totalShares := len(b.Mnemonics)
	isSinglesigDesc := b.Desc != nil && len(b.Desc.Keys) == 1 && b.Desc.Type == urtypes.Singlesig
	if b.Desc != nil && len(b.Desc.Keys) > 0 && !isSinglesigDesc {
		totalShares = len(b.Desc.Keys)
	}

	seedBuilder := SeedSceneBuilder{
		Mnemonics: b.Mnemonics,
		Desc:      b.Desc,
	}
	seedDoc, err := seedBuilder.Build()
	if err != nil {
		return nil, err
	}

	includeSinglesigDescriptorSide := isSinglesigDesc && b.SinglesigLayout == SinglesigLayoutSeedWithDescriptorQR
	hasDescScenes := b.Desc != nil && len(b.Desc.Keys) > 0 && (!isSinglesigDesc || includeSinglesigDescriptorSide)
	if !hasDescScenes {
		return seedDoc, nil
	}

	for i := 0; i < totalShares; i++ {
		keyIdx := i % len(b.Desc.Keys)
		qrPayloads, err := descriptorScenePayloadsForShare(b.Desc, totalShares, keyIdx, isSinglesigDesc, includeSinglesigDescriptorSide)
		if err != nil {
			return nil, err
		}
		scene, err := buildDescriptorPlateScene(b.Desc, keyIdx, i+1, totalShares, qrPayloads)
		if err != nil {
			return nil, err
		}
		scene.Name = fmt.Sprintf("desc_%02d", i+1)
		seedDoc.Scenes = append(seedDoc.Scenes, scene)
	}
	return seedDoc, nil
}

func descriptorScenePayloadsForShare(desc *urtypes.OutputDescriptor, totalShares, keyIdx int, isSinglesigDesc, includeSinglesigDescriptorSide bool) ([]string, error) {
	if isSinglesigDesc {
		if !includeSinglesigDescriptorSide {
			return nil, nil
		}
		q := createDescriptorQR(desc)
		if q == "" {
			return nil, fmt.Errorf("empty descriptor QR content")
		}
		return []string{q}, nil
	}
	return descriptorShardQRPayloadsForShare(desc, totalShares, keyIdx)
}

func buildDescriptorPlateScene(desc *urtypes.OutputDescriptor, keyIdx, shareNum, totalShares int, qrPayloads []string) (PlateScene, error) {
	if desc == nil {
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
	face := loadFace(11, dpi)
	trackPx := 0.04 * 11.0 * dpi / 72.0
	trackEM := 0.04
	margin := descriptorSingleQRLayout.MarginMM
	ascentMM := capBaselineOffsetMM(face, dpi)
	lineSpacing := descriptorSingleQRLayout.LineGapMM
	maxMetaWidth := plateSizeMM - 2*margin

	key := desc.Keys[keyIdx]
	typeTag := fmt.Sprintf("TYPE:%s", desc.Type.Tag())
	scriptTag := fmt.Sprintf("SCRIPT:%s", desc.Script.Tag())
	netTag := fmt.Sprintf("NET:%s", descriptorNetworkTag(key.Network))
	thresholdTag := fmt.Sprintf("THRESHOLD:%d", desc.Threshold)
	keysTag := fmt.Sprintf("KEYS:%d", len(desc.Keys))
	keyTag := fmt.Sprintf("KEY:%d", keyIdx+1)
	line1 := strings.Join([]string{typeTag, scriptTag, netTag}, " / ")
	line2 := strings.Join([]string{thresholdTag, keysTag, keyTag}, " / ")
	if trackedTextWidthMM(face, dpi, line1, trackPx) > maxMetaWidth ||
		trackedTextWidthMM(face, dpi, line2, trackPx) > maxMetaWidth {
		line1 = strings.Join([]string{typeTag, scriptTag, netTag}, "/")
		line2 = strings.Join([]string{thresholdTag, keysTag, keyTag}, "/")
	}
	if trackedTextWidthMM(face, dpi, line1, trackPx) > maxMetaWidth ||
		trackedTextWidthMM(face, dpi, line2, trackPx) > maxMetaWidth {
		line1 = strings.Join([]string{typeTag, scriptTag}, "/")
		line2 = strings.Join([]string{netTag, fmt.Sprintf("THR:%d", desc.Threshold), keysTag, keyTag}, "/")
	}
	y := margin + ascentMM
	mask.Primitives = append(mask.Primitives,
		newSceneText(margin, y, line1, 11, trackEM, TextDirHorizontal, TextAnchorBaselineLeft),
		newSceneText(margin, y+lineSpacing, line2, 11, trackEM, TextDirHorizontal, TextAnchorBaselineLeft),
	)

	qrPayloads = trimNonEmpty(qrPayloads)
	if len(qrPayloads) == 0 {
		qrPayloads = []string{createDescriptorQR(desc)}
	}
	if len(qrPayloads) == 0 {
		return PlateScene{}, fmt.Errorf("empty descriptor QR content")
	}
	dual := len(qrPayloads) == 2
	pathText := fmt.Sprintf("PATH:%s", derivationPathForKey(desc.Keys[keyIdx], desc.Script))

	if dual {
		pathY := y + 2*lineSpacing + descriptorSingleQRLayout.PathGapMM
		mask.Primitives = append(mask.Primitives, newSceneText(margin, pathY, pathText, 11, trackEM, TextDirHorizontal, TextAnchorBaselineLeft))
		guide := fmt.Sprintf("RECOVER: SCAN BOTH QRS FROM >=%d PLATES", desc.Threshold)
		gLines := wrapTrackedParagraphs(face, dpi, guide, plateSizeMM-2*margin, trackPx)
		gy := pathY + lineSpacing + descriptorDualQRLayout.GuideGapMM
		for i, line := range gLines {
			mask.Primitives = append(mask.Primitives, newSceneText(margin, gy+float64(i)*descriptorDualQRLayout.LineGapMM, line, 11, trackEM, TextDirHorizontal, TextAnchorBaselineLeft))
		}
	}

	switch len(qrPayloads) {
	case 1:
		code, err := qr.Encode(qrPayloads[0], descriptorQRECC)
		if err != nil {
			return PlateScene{}, err
		}
		qrX, qrY, qrSize := descriptorSingleQRPlacement(descriptorQRSizeMM)
		mask.Primitives = append(mask.Primitives, sceneQRModules(code, qrX, qrY, qrSize, plateQROptions{
			QuietModules:      descriptorDualQRLayout.QuietModules,
			Shape:             plateQRCircle,
			KeepIslandsSquare: true,
		})...)
	case 2:
		qrL, err := qr.Encode(qrPayloads[0], descriptorQRECC)
		if err != nil {
			return PlateScene{}, err
		}
		qrR, err := qr.Encode(qrPayloads[1], descriptorQRECC)
		if err != nil {
			return PlateScene{}, err
		}
		qrSize, leftX, rightX, qrY := descriptorDualQRPlacement(qrL, qrR)
		mask.Primitives = append(mask.Primitives, sceneQRModules(qrL, leftX, qrY, qrSize, plateQROptions{
			QuietModules:      descriptorDualQRLayout.QuietModules,
			Shape:             plateQRCircle,
			KeepIslandsSquare: true,
		})...)
		mask.Primitives = append(mask.Primitives, sceneQRModules(qrR, rightX, qrY, qrSize, plateQROptions{
			QuietModules:      descriptorDualQRLayout.QuietModules,
			Shape:             plateQRCircle,
			KeepIslandsSquare: true,
		})...)
	default:
		return PlateScene{}, fmt.Errorf("unsupported descriptor QR payload count: %d", len(qrPayloads))
	}

	if !dual {
		_, pathRotH := rotatedTextSizeMMTracked(face, dpi, pathText, trackPx)
		pathY := plateSizeMM - margin - pathRotH
		if pathY < margin {
			pathY = margin
		}
		mask.Primitives = append(mask.Primitives, newSceneText(margin, pathY, pathText, 11, trackEM, TextDirVerticalUp, TextAnchorTopLeft))
	}

	if len(qrPayloads) == 1 {
		if shMeta := decodeShardMeta(qrPayloads[0]); shMeta != nil {
			wid := strings.ToUpper(hex.EncodeToString(shMeta.WalletID[:4]))
			sid := strings.ToUpper(hex.EncodeToString(shMeta.SetID[:4]))
			meta := fmt.Sprintf("WID:%s SET:%s %d/%d", wid, sid, shMeta.Index, shMeta.Threshold)
			metaRotW, metaRotH := rotatedInkSizeMMTracked(face, dpi, meta, trackPx)
			metaX := plateSizeMM - margin - metaRotW
			if metaX < margin {
				metaX = margin
			}
			metaY := plateSizeMM - margin - metaRotH
			if metaY < margin {
				metaY = margin
			}
			mask.Primitives = append(mask.Primitives, newSceneText(metaX, metaY, meta, 11, trackEM, TextDirVerticalUp, TextAnchorTopLeft))
		}
	}

	_ = shareNum
	_ = totalShares
	return PlateScene{
		WidthMM:  plateSizeMM,
		HeightMM: plateSizeMM,
		Layers:   []SceneLayer{mask},
	}, nil
}
