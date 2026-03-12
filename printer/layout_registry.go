package printer

import (
	"fmt"
	"image"
	"strings"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
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

type LayoutSpec interface {
	Name() string
	Supports(ctx RenderContext, sel LayoutSelection) bool
	Apply(ctx RenderContext, sel LayoutSelection) LayoutSelection
	RenderBitmaps(mnemonics []bip39.Mnemonic, desc *urtypes.OutputDescriptor, opts RasterOptions, sel LayoutSelection, progress ProgressFunc) ([]*image.Paletted, []*image.Paletted, error)
	BuildScenes(mnemonics []bip39.Mnemonic, desc *urtypes.OutputDescriptor, singlesigLayout SinglesigLayoutMode, sel LayoutSelection) (*PlateDocument, error)
}

type ClassicLayoutSpec struct{}

func (ClassicLayoutSpec) Name() string { return "classic" }
func (ClassicLayoutSpec) Supports(_ RenderContext, _ LayoutSelection) bool {
	return true
}
func (ClassicLayoutSpec) Apply(_ RenderContext, sel LayoutSelection) LayoutSelection {
	sel.Name = "classic"
	sel.UseCompact2of3 = false
	return sel
}

type Compact2of3LayoutSpec struct{}

func (Compact2of3LayoutSpec) Name() string { return "compact-2of3" }
func (Compact2of3LayoutSpec) Supports(ctx RenderContext, sel LayoutSelection) bool {
	if !ctx.Compact2of3 || !sel.HasDescriptorSide || ctx.Desc == nil {
		return false
	}
	return ctx.Desc.Type == urtypes.SortedMulti &&
		ctx.Desc.Threshold == 2 &&
		len(ctx.Desc.Keys) == 3 &&
		sel.TotalShares == 3
}
func (Compact2of3LayoutSpec) Apply(_ RenderContext, sel LayoutSelection) LayoutSelection {
	sel.Name = "compact-2of3"
	sel.UseCompact2of3 = true
	return sel
}

func baseSelection(ctx RenderContext) LayoutSelection {
	totalShares := ctx.MnemonicCount
	isSinglesigDesc := ctx.Desc != nil && len(ctx.Desc.Keys) == 1 && ctx.Desc.Type == urtypes.Singlesig
	if ctx.Desc != nil && len(ctx.Desc.Keys) > 0 && !isSinglesigDesc {
		totalShares = len(ctx.Desc.Keys)
	}
	includeSinglesigInfo := isSinglesigDesc && ctx.SinglesigLayout == SinglesigLayoutSeedWithInfo
	includeSinglesigDescriptorQR := isSinglesigDesc && ctx.SinglesigLayout == SinglesigLayoutSeedWithDescriptorQR
	hasDesc := ctx.Desc != nil && len(ctx.Desc.Keys) > 0 && (!isSinglesigDesc || includeSinglesigDescriptorQR)
	return LayoutSelection{
		Name:                         "classic",
		TotalShares:                  totalShares,
		IsSinglesigDesc:              isSinglesigDesc,
		IncludeSinglesigInfo:         includeSinglesigInfo,
		IncludeSinglesigDescriptorQR: includeSinglesigDescriptorQR,
		HasDescriptorSide:            hasDesc,
		UseCompact2of3:               false,
	}
}

func layoutRegistry() []LayoutSpec {
	// Priority order matters: more specific first.
	return []LayoutSpec{
		Compact2of3LayoutSpec{},
		ClassicLayoutSpec{},
	}
}

func SelectLayout(ctx RenderContext) LayoutSelection {
	sel := baseSelection(ctx)
	for _, spec := range layoutRegistry() {
		if spec.Supports(ctx, sel) {
			return spec.Apply(ctx, sel)
		}
	}
	return sel
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
