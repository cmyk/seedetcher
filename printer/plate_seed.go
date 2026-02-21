package printer

import (
	"fmt"
	"image"
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
				qrX := layout.QRLeftMM + 0.5
				qrY := seedQRYMM(seedQRSizeMM)
				drawPlateQR(canvas, qrCode, dpi, qrX, qrY, seedQRSizeMM, blackIdx, plateQROptions{
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
	DrawRotatedLabel(canvas, dpi, shareX, shareY, metaFace, metaTrackPx, blackIdx, shareText)

	if fingerprintHex != "" {
		_, fpRotH := rotatedTextSizeMMTracked(metaFace, dpi, fingerprintHex, metaTrackPx)
		fpX := marginMM
		fpY := (plateSizeMM - fpRotH) / 2
		DrawRotatedLabel(canvas, dpi, fpX, fpY, metaFace, metaTrackPx, blackIdx, fingerprintHex)
	}

	verText := version.String()
	_, verRotH := rotatedTextSizeMMTracked(metaFace, dpi, verText, metaTrackPx)
	verX := marginMM
	verY := marginMM
	if verY+verRotH > plateSizeMM-marginMM {
		verY = plateSizeMM - marginMM - verRotH
	}
	DrawRotatedLabel(canvas, dpi, verX, verY, metaFace, metaTrackPx, blackIdx, verText)
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
		DrawRotatedLabel(canvas, dpi, metaX, metaY, metaFace, metaTrackPx, blackIdx, meta)
	}

	if opts.Invert {
		invertInterior(canvas, border)
	}
	applyPostProcess(canvas, opts)
	return canvas, nil
}
