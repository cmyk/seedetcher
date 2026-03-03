package printer

import (
	"encoding/hex"
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/kortschak/qr"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/descriptor/shard"
)

// RenderDescriptorPlateBitmap mirrors the descriptor PDF layout at 600dpi as a 1-bit paletted image.
func RenderDescriptorPlateBitmap(desc *urtypes.OutputDescriptor, keyIdx, shareNum, totalShares int, opts RasterOptions, qrPayloads []string) (*image.Paletted, error) {
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
	typeTag := fmt.Sprintf("TYPE:%s", desc.Type.Tag())
	scriptTag := fmt.Sprintf("SCRIPT:%s", desc.Script.Tag())
	netTag := fmt.Sprintf("NET:%s", descriptorNetworkTag(key.Network))
	thresholdTag := fmt.Sprintf("THRESHOLD:%d", desc.Threshold)
	keysTag := fmt.Sprintf("KEYS:%d", len(desc.Keys))
	keyTag := fmt.Sprintf("KEY:%d", keyIdx+1)

	margin := descriptorSingleQRLayout.MarginMM
	ascentMM := capBaselineOffsetMM(descriptorFace, dpi)
	maxMetaWidth := plateSizeMM - 2*margin
	line1 := strings.Join([]string{typeTag, scriptTag, netTag}, " / ")
	line2 := strings.Join([]string{thresholdTag, keysTag, keyTag}, " / ")
	// Deterministic fixed-line layout; avoid mid-token wrapping.
	if trackedTextWidthMM(descriptorFace, dpi, line1, descTrackPx) > maxMetaWidth ||
		trackedTextWidthMM(descriptorFace, dpi, line2, descTrackPx) > maxMetaWidth {
		line1 = strings.Join([]string{typeTag, scriptTag, netTag}, "/")
		line2 = strings.Join([]string{thresholdTag, keysTag, keyTag}, "/")
	}
	if trackedTextWidthMM(descriptorFace, dpi, line1, descTrackPx) > maxMetaWidth ||
		trackedTextWidthMM(descriptorFace, dpi, line2, descTrackPx) > maxMetaWidth {
		line1 = strings.Join([]string{typeTag, scriptTag}, "/")
		line2 = strings.Join([]string{netTag, fmt.Sprintf("THR:%d", desc.Threshold), keysTag, keyTag}, "/")
	}
	lineSpacing := descriptorSingleQRLayout.LineGapMM
	y := margin + ascentMM
	res := DrawTextBlock(canvas, dpi, TextBlock{
		Face:      descriptorFace,
		Tracking:  descTrackPx,
		LeadingMM: lineSpacing,
		WidthMM:   maxMetaWidth,
		Align:     TextAlignStart,
		OriginXMM: margin,
		OriginYMM: y,
	}, line1+"\n"+line2)
	y = res.NextBaselineYMM - lineSpacing

	if len(qrPayloads) == 0 {
		qrPayloads = []string{createDescriptorQR(desc)}
	}
	qrPayloads = trimNonEmpty(qrPayloads)
	if len(qrPayloads) == 0 {
		return nil, fmt.Errorf("empty descriptor QR content")
	}

	dualQRLayout := len(qrPayloads) == 2
	if dualQRLayout {
		y += descriptorSingleQRLayout.PathGapMM
		DrawMetaLine(canvas, dpi, margin, y, descriptorFace, descTrackPx, pathText)
		guide := fmt.Sprintf("RECOVER: SCAN BOTH QRS FROM >=%d PLATES", desc.Threshold)
		gy := y + lineSpacing + descriptorDualQRLayout.GuideGapMM
		_ = DrawTextBlock(canvas, dpi, TextBlock{
			Face:      descriptorFace,
			Tracking:  descTrackPx,
			LeadingMM: descriptorDualQRLayout.LineGapMM,
			WidthMM:   plateSizeMM - 2*margin,
			Align:     TextAlignStart,
			OriginXMM: margin,
			OriginYMM: gy,
		}, guide)
	}

	switch len(qrPayloads) {
	case 1:
		qrCode, err := qr.Encode(qrPayloads[0], descriptorQRECC)
		if err != nil {
			return nil, err
		}
		qrX, qrY, qrSize := descriptorSingleQRPlacement(descriptorQRSizeMM)
		drawPlateQR(canvas, qrCode, dpi, qrX, qrY, qrSize, blackIdx, plateQROptions{
			QuietModules:      descriptorDualQRLayout.QuietModules,
			Shape:             plateQRCircle,
			KeepIslandsSquare: true,
		})
	case 2:
		qrL, err := qr.Encode(qrPayloads[0], descriptorQRECC)
		if err != nil {
			return nil, err
		}
		qrR, err := qr.Encode(qrPayloads[1], descriptorQRECC)
		if err != nil {
			return nil, err
		}
		qrSize, leftX, rightX, qrY := descriptorDualQRPlacement(qrL, qrR)
		drawPlateQR(canvas, qrL, dpi, leftX, qrY, qrSize, blackIdx, plateQROptions{
			QuietModules:      descriptorDualQRLayout.QuietModules,
			Shape:             plateQRCircle,
			KeepIslandsSquare: true,
		})
		drawPlateQR(canvas, qrR, dpi, rightX, qrY, qrSize, blackIdx, plateQROptions{
			QuietModules:      descriptorDualQRLayout.QuietModules,
			Shape:             plateQRCircle,
			KeepIslandsSquare: true,
		})
	default:
		return nil, fmt.Errorf("unsupported descriptor QR payload count: %d", len(qrPayloads))
	}

	if !dualQRLayout {
		_, pathRotH := rotatedTextSizeMMTracked(descriptorFace, dpi, pathText, descTrackPx)
		pathX := margin
		pathY := plateSizeMM - margin - pathRotH
		if pathY < margin {
			pathY = margin
		}
		DrawRotatedLabel(canvas, dpi, pathX, pathY, descriptorFace, descTrackPx, blackIdx, pathText)
	}

	if len(qrPayloads) == 1 {
		if shMeta := decodeShardMeta(qrPayloads[0]); shMeta != nil {
			wid := strings.ToUpper(hex.EncodeToString(shMeta.WalletID[:4]))
			sid := strings.ToUpper(hex.EncodeToString(shMeta.SetID[:4]))
			meta := fmt.Sprintf("WID:%s SET:%s %d/%d", wid, sid, shMeta.Index, shMeta.Threshold)
			metaRotW, metaRotH := rotatedInkSizeMMTracked(descriptorFace, dpi, meta, descTrackPx)
			metaX := plateSizeMM - margin - metaRotW
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
			DrawRotatedLabel(canvas, dpi, metaX, metaY, descriptorFace, descTrackPx, blackIdx, meta)
		}
	}

	if opts.Invert {
		invertInterior(canvas, border)
	}
	applyPostProcess(canvas, opts)
	return canvas, nil
}

func decodeShardMeta(payload string) *shard.Share {
	if !strings.HasPrefix(strings.ToUpper(payload), shard.Prefix) {
		return nil
	}
	sh, err := shard.Decode(strings.ToUpper(payload))
	if err != nil {
		return nil
	}
	return &sh
}

func descriptorSingleQRPlacement(sizeOverride float64) (x, y, size float64) {
	size = sizeOverride
	if size <= 0 {
		size = descriptorSingleQRLayout.DefaultSize
	}
	if size < 5.0 {
		size = 5.0
	}
	x = descriptorSingleQRLayout.QRXMM
	y = descriptorSingleQRLayout.QRYMM
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x+size > plateSizeMM {
		x = plateSizeMM - size
	}
	if y+size > plateSizeMM {
		y = plateSizeMM - size
	}
	return x, y, size
}

func descriptorDualQRPlacement(qrL, qrR *qr.Code) (size, leftX, rightX, y float64) {
	usableH := plateSizeMM - descriptorDualQRLayout.QRTopLimitMM
	size = math.Min(plateSizeMM/2, usableH)
	quietModules := descriptorDualQRLayout.QuietModules
	for i := 0; i < descriptorDualQRLayout.OverlapIters; i++ {
		overlap := math.Min(quietZoneMM(qrL, size, quietModules), quietZoneMM(qrR, size, quietModules))
		maxByWidth := (plateSizeMM + overlap) / 2
		next := math.Min(maxByWidth, usableH)
		if math.Abs(next-size) < descriptorDualQRLayout.OverlapEpsilon {
			break
		}
		size = next
	}
	if size < 5.0 {
		size = 5.0
	}
	overlap := math.Min(quietZoneMM(qrL, size, quietModules), quietZoneMM(qrR, size, quietModules))
	leftX = 0.0
	rightX = leftX + size - overlap
	y = plateSizeMM - size
	return size, leftX, rightX, y
}
