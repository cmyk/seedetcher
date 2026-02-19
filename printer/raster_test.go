package printer

import (
	"strings"
	"testing"

	"seedetcher.com/descriptor/compact2of3"
	"seedetcher.com/descriptor/shard"
	"seedetcher.com/testutils"
)

func TestDescriptorShardQRCodesAreSE1AndConsistentSet(t *testing.T) {
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
	var base shard.Share
	for i, q := range qrs {
		if !strings.HasPrefix(strings.ToUpper(q), shard.Prefix) {
			t.Fatalf("share %d has non-shard payload: %q", i+1, q)
		}
		sh, err := shard.Decode(strings.ToUpper(q))
		if err != nil {
			t.Fatalf("decode share %d: %v", i+1, err)
		}
		if i == 0 {
			base = sh
		}
		if sh.SetID != base.SetID {
			t.Fatalf("share %d set_id mismatch", i+1)
		}
		if sh.Threshold != uint8(desc.Threshold) || sh.Total != uint8(len(desc.Keys)) {
			t.Fatalf("share %d threshold/total mismatch: got %d/%d want %d/%d", i+1, sh.Threshold, sh.Total, desc.Threshold, len(desc.Keys))
		}
	}
}

func TestDescriptorShardQRCodesRespectForcedSetID(t *testing.T) {
	cfg := testutils.WalletConfigs["multisig-3of5"]
	_, desc, err := testutils.ParseWallet(cfg, "", "")
	if err != nil {
		t.Fatalf("parse wallet: %v", err)
	}
	if desc == nil {
		t.Fatal("missing descriptor")
	}
	set := [16]byte{1, 2, 3, 4}
	SetDescriptorShardSetID(&set)
	defer SetDescriptorShardSetID(nil)

	qrs, err := descriptorShardQRCodes(desc, len(desc.Keys))
	if err != nil {
		t.Fatalf("descriptorShardQRCodes: %v", err)
	}
	for i, q := range qrs {
		sh, err := shard.Decode(strings.ToUpper(q))
		if err != nil {
			t.Fatalf("decode share %d: %v", i+1, err)
		}
		if sh.SetID != set {
			t.Fatalf("share %d set_id mismatch", i+1)
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
	if strings.HasPrefix(strings.ToUpper(qrs[0]), shard.Prefix) {
		t.Fatalf("singlesig descriptor QR unexpectedly sharded: %q", qrs[0])
	}
}

func TestDescriptorShardQRCodesCompact2of3WhenEnabled(t *testing.T) {
	cfg := testutils.WalletConfigs["multisig-mainnet-2of3"]
	_, desc, err := testutils.ParseWallet(cfg, "", "")
	if err != nil {
		t.Fatalf("parse wallet: %v", err)
	}
	if desc == nil {
		t.Fatal("missing descriptor")
	}
	SetCompactDescriptor2of3Enabled(true)
	defer SetCompactDescriptor2of3Enabled(false)

	qrs, err := descriptorShardQRCodes(desc, len(desc.Keys))
	if err != nil {
		t.Fatalf("descriptorShardQRCodes compact: %v", err)
	}
	if len(qrs) != 3 {
		t.Fatalf("got %d qrs, want 3", len(qrs))
	}
	shares := make([]compact2of3.Share, 0, 3)
	for i, q := range qrs {
		if !strings.HasPrefix(strings.ToUpper(q), compact2of3.Prefix) {
			t.Fatalf("share %d has non-compact payload: %q", i+1, q)
		}
		sh, err := compact2of3.Decode(strings.ToUpper(q))
		if err != nil {
			t.Fatalf("decode compact share %d: %v", i+1, err)
		}
		shares = append(shares, sh)
	}
	got, err := compact2of3.CombineToDescriptorPayload([]compact2of3.Share{shares[0], shares[2]})
	if err != nil {
		t.Fatalf("combine compact shares: %v", err)
	}
	want := desc.Encode()
	if string(got) != string(want) {
		t.Fatal("recovered compact payload mismatch")
	}
}
