package printer

import (
	"bytes"
	"strings"
	"testing"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/descriptor/urxor2of3"
	"seedetcher.com/testutils"
)

func TestDescriptorShardQRCodes2of3UseURXORAndRecover(t *testing.T) {
	cfg := testutils.WalletConfigs["multisig"]
	_, desc, err := testutils.ParseWallet(cfg, "", "")
	if err != nil {
		t.Fatalf("parse wallet: %v", err)
	}
	if desc == nil {
		t.Fatal("missing descriptor")
	}
	qrs, err := descriptorShardQRCodes(desc, len(desc.Keys))
	if err != nil {
		t.Fatalf("descriptorShardQRCodes: %v", err)
	}
	if len(qrs) != len(desc.Keys) {
		t.Fatalf("got %d qrs, want %d", len(qrs), len(desc.Keys))
	}
	want, err := urxor2of3.Combine([]string{qrs[0], qrs[1]})
	if err != nil {
		t.Fatalf("combine first two shares: %v", err)
	}
	for i, q := range qrs {
		typ, _, seqLen, ok := urxor2of3.ParseShare(q)
		if !ok || typ != "crypto-output" || seqLen != desc.Threshold {
			t.Fatalf("share %d has non ur/xor payload: %q", i+1, q)
		}
		got, err := urxor2of3.Combine([]string{qrs[i], qrs[(i+1)%len(qrs)]})
		if err != nil {
			t.Fatalf("combine pair for share %d: %v", i+1, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("share %d recovered payload mismatch", i+1)
		}
	}
	v, err := urtypes.Parse("crypto-output", want)
	if err != nil {
		t.Fatalf("parse recovered payload: %v", err)
	}
	if _, ok := v.(urtypes.OutputDescriptor); !ok {
		t.Fatalf("recovered payload type: %T", v)
	}
}

func TestDescriptorShardQRCodesUnsupportedFallbacksToFullDescriptorUR(t *testing.T) {
	cfg := testutils.WalletConfigs["multisig-7of10"]
	_, desc, err := testutils.ParseWallet(cfg, "", "")
	if err != nil {
		t.Fatalf("parse wallet: %v", err)
	}
	if desc == nil {
		t.Fatal("missing descriptor")
	}

	qrs, err := descriptorShardQRCodes(desc, len(desc.Keys))
	if err != nil {
		t.Fatalf("descriptorShardQRCodes: %v", err)
	}
	for i, q := range qrs {
		if strings.HasPrefix(strings.ToUpper(q), "SE1:") {
			t.Fatalf("share %d unexpectedly used SE1 fallback: %q", i+1, q)
		}
		typ, _, _, ok := urxor2of3.ParseShare(q)
		if ok && typ == "crypto-output" {
			t.Fatalf("share %d unexpectedly looks like multipart UR/XOR: %q", i+1, q)
		}
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(q)), "ur:crypto-output/") {
			t.Fatalf("share %d missing full descriptor UR prefix: %q", i+1, q)
		}
	}
}

func TestDescriptorShardQRCodesSinglesigUsesDescriptorQR(t *testing.T) {
	cfg := testutils.WalletConfigs["singlesig"]
	_, desc, err := testutils.ParseWallet(cfg, "", "")
	if err != nil {
		t.Fatalf("parse wallet: %v", err)
	}
	if desc == nil {
		t.Fatal("missing descriptor")
	}
	if desc.Threshold != 1 || len(desc.Keys) != 1 {
		t.Fatalf("unexpected singlesig descriptor params: threshold=%d keys=%d", desc.Threshold, len(desc.Keys))
	}

	qrs, err := descriptorShardQRCodes(desc, len(desc.Keys))
	if err != nil {
		t.Fatalf("descriptorShardQRCodes singlesig: %v", err)
	}
	if len(qrs) != 1 {
		t.Fatalf("got %d qrs, want 1", len(qrs))
	}
	if strings.HasPrefix(strings.ToUpper(qrs[0]), "SE1:") {
		t.Fatalf("singlesig descriptor QR unexpectedly sharded: %q", qrs[0])
	}
}
