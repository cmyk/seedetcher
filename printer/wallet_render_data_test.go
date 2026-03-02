package printer

import (
	"bytes"
	"fmt"
	"image"
	"testing"

	"seedetcher.com/bc/ur"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/descriptor/urxor2of3"
	"seedetcher.com/seedqr"
	"seedetcher.com/testutils"
)

func TestSeedQRPayloadRoundTripForWalletFixtures(t *testing.T) {
	for _, wallet := range []string{"singlesig", "multisig-mainnet-2of3", "multisig-7of10"} {
		cfg, ok := testutils.WalletConfigs[wallet]
		if !ok {
			t.Fatalf("wallet fixture not found: %s", wallet)
		}
		mnemonics, _, err := testutils.ParseWallet(cfg, "", "")
		if err != nil {
			t.Fatalf("ParseWallet(%s): %v", wallet, err)
		}
		for i, m := range mnemonics {
			got, ok := seedqr.Parse(seedqr.QR(m))
			if !ok {
				t.Fatalf("%s[%d]: seedqr roundtrip parse failed", wallet, i)
			}
			if !sameMnemonic(got, m) {
				t.Fatalf("%s[%d]: mnemonic mismatch after seedqr roundtrip", wallet, i)
			}
		}
	}
}

func TestDescriptorSharePayloadsRecoverOriginalDescriptor(t *testing.T) {
	for _, wallet := range []string{"singlesig", "multisig-mainnet-2of3", "multisig-7of10"} {
		cfg, ok := testutils.WalletConfigs[wallet]
		if !ok {
			t.Fatalf("wallet fixture not found: %s", wallet)
		}
		_, desc, err := testutils.ParseWallet(cfg, "", "")
		if err != nil {
			t.Fatalf("ParseWallet(%s): %v", wallet, err)
		}
		if desc == nil {
			continue
		}
		shares, err := collectDescriptorSharePayloads(desc)
		if err != nil {
			t.Fatalf("%s: collect shares: %v", wallet, err)
		}
		th := desc.Threshold
		if th < 1 || th > len(shares) {
			t.Fatalf("%s: invalid threshold=%d shares=%d", wallet, th, len(shares))
		}
		recovered, err := recoverDescriptorFromShares(shares[:th])
		if err != nil {
			t.Fatalf("%s: recover descriptor: %v", wallet, err)
		}
		assertDescriptorEquivalent(t, wallet, *desc, recovered)
	}
}

func TestComposePagesPreservesPlatePixels(t *testing.T) {
	const (
		wallet = "multisig-mainnet-2of3"
		dpi    = 150.0
		paper  = PaperA4
	)
	cfg, ok := testutils.WalletConfigs[wallet]
	if !ok {
		t.Fatalf("wallet fixture not found: %s", wallet)
	}
	mnemonics, desc, err := testutils.ParseWallet(cfg, "", "")
	if err != nil {
		t.Fatalf("ParseWallet(%s): %v", wallet, err)
	}
	seedPlates, descPlates, err := CreatePlateBitmaps(mnemonics, desc, 0, RasterOptions{DPI: dpi}, nil)
	if err != nil {
		t.Fatalf("CreatePlateBitmaps: %v", err)
	}
	pages, err := ComposePages(seedPlates, descPlates, paper, dpi, nil)
	if err != nil {
		t.Fatalf("ComposePages: %v", err)
	}
	plan, err := buildPlacementPlan(seedPlates, descPlates, paper, dpi, false, nil)
	if err != nil {
		t.Fatalf("buildPlacementPlan: %v", err)
	}
	if len(pages) != len(plan.pages) {
		t.Fatalf("page count mismatch: pages=%d plan=%d", len(pages), len(plan.pages))
	}
	for pi, p := range plan.pages {
		page := pages[pi]
		for si, slot := range p.slots {
			if slot.plate == nil {
				continue
			}
			if !pageRegionEqualsPlate(page, slot.plate, slot.x, slot.y) {
				t.Fatalf("page[%d] slot[%d]: composed page pixels differ from source plate", pi, si)
			}
		}
	}
}

func collectDescriptorSharePayloads(desc *urtypes.OutputDescriptor) ([][]string, error) {
	total := len(desc.Keys)
	out := make([][]string, total)
	if total == 0 {
		return nil, nil
	}
	for i := 0; i < total; i++ {
		payloads, err := DescriptorShardQRPayloadsForShare(desc, total, i)
		if err != nil {
			return nil, err
		}
		if len(payloads) == 0 {
			return nil, fmt.Errorf("share payloads are empty")
		}
		out[i] = payloads
	}
	return out, nil
}

func recoverDescriptorFromShares(selected [][]string) (urtypes.OutputDescriptor, error) {
	flat := make([]string, 0, len(selected)*2)
	for _, frags := range selected {
		flat = append(flat, frags...)
	}
	if len(flat) == 0 {
		return urtypes.OutputDescriptor{}, fmt.Errorf("no share payloads provided")
	}
	typ, _, seqLen, urxor := urxor2of3.ParseShare(flat[0])
	var payload []byte
	if urxor && typ == "crypto-output" && seqLen >= urxor2of3.MinShares {
		p, err := urxor2of3.Combine(flat)
		if err != nil {
			return urtypes.OutputDescriptor{}, err
		}
		payload = p
	} else {
		var d ur.Decoder
		for _, s := range flat {
			if err := d.Add(s); err != nil {
				return urtypes.OutputDescriptor{}, err
			}
		}
		outTyp, p, err := d.Result()
		if err != nil {
			return urtypes.OutputDescriptor{}, err
		}
		if outTyp != "crypto-output" {
			return urtypes.OutputDescriptor{}, fmt.Errorf("unexpected ur type: %s", outTyp)
		}
		payload = p
	}
	v, err := urtypes.Parse("crypto-output", payload)
	if err != nil {
		return urtypes.OutputDescriptor{}, err
	}
	parsed, ok := v.(urtypes.OutputDescriptor)
	if !ok {
		return urtypes.OutputDescriptor{}, fmt.Errorf("recovered payload is not output descriptor: %T", v)
	}
	return parsed, nil
}

func assertDescriptorEquivalent(t *testing.T, wallet string, want, got urtypes.OutputDescriptor) {
	t.Helper()
	if got.Type != want.Type {
		t.Fatalf("%s: descriptor type mismatch: got=%v want=%v", wallet, got.Type, want.Type)
	}
	if got.Script != want.Script {
		t.Fatalf("%s: descriptor script mismatch: got=%v want=%v", wallet, got.Script, want.Script)
	}
	if got.Threshold != want.Threshold {
		t.Fatalf("%s: descriptor threshold mismatch: got=%d want=%d", wallet, got.Threshold, want.Threshold)
	}
	if len(got.Keys) != len(want.Keys) {
		t.Fatalf("%s: descriptor key count mismatch: got=%d want=%d", wallet, len(got.Keys), len(want.Keys))
	}
	gotFP := descriptorFingerprintSet(got.Keys)
	wantFP := descriptorFingerprintSet(want.Keys)
	if len(gotFP) != len(wantFP) {
		t.Fatalf("%s: descriptor key fingerprint length mismatch: got=%d want=%d", wallet, len(gotFP), len(wantFP))
	}
	for i := range gotFP {
		if gotFP[i] != wantFP[i] {
			t.Fatalf("%s: descriptor key fingerprint set mismatch: got=%v want=%v", wallet, gotFP, wantFP)
		}
	}
}

func descriptorFingerprintSet(keys []urtypes.KeyDescriptor) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = fingerprintHex(k.MasterFingerprint)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

func pageRegionEqualsPlate(page, plate *image.Paletted, x, y int) bool {
	b := plate.Bounds()
	for py := 0; py < b.Dy(); py++ {
		pageRow := page.Pix[(y+py)*page.Stride+x : (y+py)*page.Stride+x+b.Dx()]
		plateRow := plate.Pix[(b.Min.Y+py)*plate.Stride+b.Min.X : (b.Min.Y+py)*plate.Stride+b.Min.X+b.Dx()]
		if !bytes.Equal(pageRow, plateRow) {
			return false
		}
	}
	return true
}

func fingerprintHex(v uint32) string {
	const hexd = "0123456789abcdef"
	out := [8]byte{}
	for i := 7; i >= 0; i-- {
		out[i] = hexd[v&0xF]
		v >>= 4
	}
	return string(out[:])
}

func sameMnemonic(a, b bip39.Mnemonic) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
