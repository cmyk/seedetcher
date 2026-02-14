package printer

import (
	"strings"
	"testing"

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
