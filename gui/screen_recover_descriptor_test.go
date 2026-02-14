package gui

import (
	"strings"
	"testing"

	"seedetcher.com/descriptor/shard"
	"seedetcher.com/testutils"
)

func TestRecoverFlowReconstructsAndRendersQR(t *testing.T) {
	cfg := testutils.WalletConfigs["multisig-3of5"]
	_, desc, err := testutils.ParseWallet(cfg, "", "")
	if err != nil {
		t.Fatalf("parse wallet: %v", err)
	}
	if desc == nil {
		t.Fatal("missing descriptor")
	}
	payload := desc.Encode()
	shares, err := shard.SplitPayloadBytes(payload, shard.SplitOptions{
		Threshold: uint8(desc.Threshold),
		Total:     uint8(len(desc.Keys)),
	})
	if err != nil {
		t.Fatalf("split payload: %v", err)
	}
	gotPayload, err := shard.CombinePayloadBytes([]shard.Share{shares[0], shares[2], shares[4]})
	if err != nil {
		t.Fatalf("combine payload: %v", err)
	}
	if string(gotPayload) == "" {
		t.Fatal("empty reconstructed payload")
	}
	recoveredUR, err := safeEncodeDescriptorUR(gotPayload)
	if err != nil {
		t.Fatalf("encode ur: %v", err)
	}
	if !strings.HasPrefix(strings.ToLower(recoveredUR), "ur:crypto-output/") {
		t.Fatalf("unexpected ur prefix: %q", recoveredUR)
	}
	img := renderQRImageRect(recoveredUR, 240, 240)
	if img.Bounds().Dx() != 240 || img.Bounds().Dy() != 240 {
		t.Fatalf("unexpected qr image size: %v", img.Bounds().Size())
	}
}
