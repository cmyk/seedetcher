package printer

import (
	"fmt"
	"image"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
)

func layoutSpecForSelection(sel LayoutSelection) LayoutSpec {
	for _, spec := range layoutRegistry() {
		if spec.Name() == sel.Name {
			return spec
		}
	}
	return ClassicLayoutSpec{}
}

func (ClassicLayoutSpec) RenderBitmaps(mnemonics []bip39.Mnemonic, desc *urtypes.OutputDescriptor, opts RasterOptions, sel LayoutSelection, progress ProgressFunc) ([]*image.Paletted, []*image.Paletted, error) {
	totalShares := sel.TotalShares
	seedLayout := sel.SeedLayout(desc)

	seedImgs := make([]*image.Paletted, totalShares)
	var descImgs []*image.Paletted
	if sel.HasDescriptorSide {
		descImgs = make([]*image.Paletted, totalShares)
	}
	shardQRPayloads, err := DescriptorPayloadsByShare(desc, totalShares, sel.IsSinglesigDesc, sel.IncludeSinglesigDescriptorQR)
	if err != nil {
		return nil, nil, err
	}

	prepareTotal := int64(totalShares)
	if sel.HasDescriptorSide {
		prepareTotal *= 2
	}
	prepareDone := int64(0)

	for i := 0; i < totalShares; i++ {
		mnemonic := mnemonics[i%len(mnemonics)]
		seedImg, err := renderSeedPlateBitmapWithLayout(mnemonic, i+1, totalShares, opts, seedLayout)
		if err != nil {
			return nil, nil, err
		}
		seedImgs[i] = seedImg
		prepareDone++
		if progress != nil && prepareTotal > 0 {
			progress(StagePrepare, prepareDone, prepareTotal)
		}

		if sel.HasDescriptorSide {
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
			prepareDone++
			if progress != nil && prepareTotal > 0 {
				progress(StagePrepare, prepareDone, prepareTotal)
			}
		}
	}

	return seedImgs, descImgs, nil
}

func (Compact2of3LayoutSpec) RenderBitmaps(mnemonics []bip39.Mnemonic, desc *urtypes.OutputDescriptor, opts RasterOptions, sel LayoutSelection, progress ProgressFunc) ([]*image.Paletted, []*image.Paletted, error) {
	if desc == nil || len(desc.Keys) == 0 {
		return nil, nil, fmt.Errorf("descriptor is nil")
	}
	totalShares := sel.TotalShares
	seedImgs := make([]*image.Paletted, totalShares)
	shardQRPayloads, err := DescriptorPayloadsByShare(desc, totalShares, sel.IsSinglesigDesc, sel.IncludeSinglesigDescriptorQR)
	if err != nil {
		return nil, nil, err
	}
	prepareTotal := int64(totalShares)
	prepareDone := int64(0)
	for i := 0; i < totalShares; i++ {
		mnemonic := mnemonics[i%len(mnemonics)]
		descKeyIdx := i % len(desc.Keys)
		sharePayload := ""
		if i < len(shardQRPayloads) && len(shardQRPayloads[i]) > 0 {
			sharePayload = shardQRPayloads[i][0]
		}
		seedImg, err := renderCompact2of3PlateBitmap(mnemonic, desc, descKeyIdx, opts, sharePayload)
		if err != nil {
			return nil, nil, err
		}
		seedImgs[i] = seedImg
		prepareDone++
		if progress != nil && prepareTotal > 0 {
			progress(StagePrepare, prepareDone, prepareTotal)
		}
	}
	return seedImgs, nil, nil
}

func (ClassicLayoutSpec) BuildScenes(mnemonics []bip39.Mnemonic, desc *urtypes.OutputDescriptor, singlesigLayout SinglesigLayoutMode, sel LayoutSelection) (*PlateDocument, error) {
	seedBuilder := SeedSceneBuilder{
		Mnemonics:       mnemonics,
		Desc:            desc,
		SinglesigLayout: singlesigLayout,
	}
	seedDoc, err := seedBuilder.Build()
	if err != nil {
		return nil, err
	}
	if !sel.HasDescriptorSide {
		return seedDoc, nil
	}
	qrPayloadsByShare, err := DescriptorPayloadsByShare(desc, sel.TotalShares, sel.IsSinglesigDesc, sel.IncludeSinglesigDescriptorQR)
	if err != nil {
		return nil, err
	}
	for i := 0; i < sel.TotalShares; i++ {
		keyIdx := i % len(desc.Keys)
		qrPayloads := []string(nil)
		if i < len(qrPayloadsByShare) {
			qrPayloads = qrPayloadsByShare[i]
		}
		scene, err := buildDescriptorPlateScene(desc, keyIdx, i+1, sel.TotalShares, qrPayloads)
		if err != nil {
			return nil, err
		}
		scene.Name = fmt.Sprintf("desc_%02d", i+1)
		seedDoc.Scenes = append(seedDoc.Scenes, scene)
	}
	return seedDoc, nil
}

func (Compact2of3LayoutSpec) BuildScenes(mnemonics []bip39.Mnemonic, desc *urtypes.OutputDescriptor, _ SinglesigLayoutMode, sel LayoutSelection) (*PlateDocument, error) {
	if desc == nil || len(desc.Keys) == 0 {
		return nil, fmt.Errorf("descriptor is nil")
	}
	shardQRPayloads, err := DescriptorPayloadsByShare(desc, sel.TotalShares, sel.IsSinglesigDesc, sel.IncludeSinglesigDescriptorQR)
	if err != nil {
		return nil, err
	}
	doc := &PlateDocument{
		Version: sceneVersion,
		Scenes:  make([]PlateScene, 0, sel.TotalShares),
	}
	for i := 0; i < sel.TotalShares; i++ {
		keyIdx := i % len(desc.Keys)
		descQR := ""
		if i < len(shardQRPayloads) && len(shardQRPayloads[i]) > 0 {
			descQR = shardQRPayloads[i][0]
		}
		scene, err := buildCompact2of3SeedScene(mnemonics[i%len(mnemonics)], desc, keyIdx, descQR)
		if err != nil {
			return nil, err
		}
		scene.Name = fmt.Sprintf("seed_%02d", i+1)
		doc.Scenes = append(doc.Scenes, scene)
	}
	return doc, nil
}
