package printer

import (
	"fmt"
	"strings"

	"seedetcher.com/bc/urtypes"
)

type RenderContext struct {
	MnemonicCount   int
	Desc            *urtypes.OutputDescriptor
	SinglesigLayout SinglesigLayoutMode
	Compact2of3     bool
}

type LayoutSelection struct {
	Name                         string
	TotalShares                  int
	IsSinglesigDesc              bool
	IncludeSinglesigInfo         bool
	IncludeSinglesigDescriptorQR bool
	HasDescriptorSide            bool
	UseCompact2of3               bool
}

func SelectLayout(ctx RenderContext) LayoutSelection {
	totalShares := ctx.MnemonicCount
	isSinglesigDesc := ctx.Desc != nil && len(ctx.Desc.Keys) == 1 && ctx.Desc.Type == urtypes.Singlesig
	if ctx.Desc != nil && len(ctx.Desc.Keys) > 0 && !isSinglesigDesc {
		totalShares = len(ctx.Desc.Keys)
	}
	includeSinglesigInfo := isSinglesigDesc && ctx.SinglesigLayout == SinglesigLayoutSeedWithInfo
	includeSinglesigDescriptorQR := isSinglesigDesc && ctx.SinglesigLayout == SinglesigLayoutSeedWithDescriptorQR
	hasDesc := ctx.Desc != nil && len(ctx.Desc.Keys) > 0 && (!isSinglesigDesc || includeSinglesigDescriptorQR)
	useCompact2of3 := hasDesc &&
		ctx.Compact2of3 &&
		ctx.Desc.Type == urtypes.SortedMulti &&
		ctx.Desc.Threshold == 2 &&
		len(ctx.Desc.Keys) == 3 &&
		totalShares == 3
	name := "classic"
	if useCompact2of3 {
		name = "compact-2of3"
	}
	return LayoutSelection{
		Name:                         name,
		TotalShares:                  totalShares,
		IsSinglesigDesc:              isSinglesigDesc,
		IncludeSinglesigInfo:         includeSinglesigInfo,
		IncludeSinglesigDescriptorQR: includeSinglesigDescriptorQR,
		HasDescriptorSide:            hasDesc,
		UseCompact2of3:               useCompact2of3,
	}
}

func (s LayoutSelection) SeedLayout(desc *urtypes.OutputDescriptor) seedPlateLayout {
	layout := defaultSeedPlateLayout(s.TotalShares, s.IsSinglesigDesc)
	if desc == nil || s.IsSinglesigDesc {
		layout.ShareNum = 1
		layout.ShareTotal = 1
	}
	if s.IncludeSinglesigInfo && desc != nil && len(desc.Keys) > 0 {
		path := strings.ToUpper(derivationPathForKey(desc.Keys[0], desc.Script))
		layout.RightMetaText = fmt.Sprintf("%s/%s/NET:%s", path, desc.Script.Tag(), descriptorNetworkTag(desc.Keys[0].Network))
	}
	return layout
}

func DescriptorPayloadsByShare(desc *urtypes.OutputDescriptor, totalShares int, isSinglesigDesc, includeSinglesigDescriptorSide bool) ([][]string, error) {
	if desc == nil || len(desc.Keys) == 0 {
		return nil, nil
	}
	payloads := make([][]string, totalShares)
	if isSinglesigDesc && includeSinglesigDescriptorSide {
		qrPayload := createDescriptorQR(desc)
		if qrPayload == "" {
			return nil, fmt.Errorf("empty descriptor QR content")
		}
		for i := range payloads {
			payloads[i] = []string{qrPayload}
		}
		return payloads, nil
	}
	for i := 0; i < totalShares; i++ {
		descKeyIdx := i % len(desc.Keys)
		p, err := descriptorShardQRPayloadsForShare(desc, totalShares, descKeyIdx)
		if err != nil {
			return nil, err
		}
		payloads[i] = p
	}
	return payloads, nil
}
