package urxor2of3

import (
	"bytes"
	"testing"

	"seedetcher.com/bc/ur"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/testutils"
)

func testDescriptor2of3(t *testing.T) *urtypes.OutputDescriptor {
	t.Helper()
	cfg := testutils.WalletConfigs["multisig-mainnet-2of3"]
	_, desc, err := testutils.ParseWallet(cfg, "", "")
	if err != nil {
		t.Fatalf("parse wallet: %v", err)
	}
	if desc == nil {
		t.Fatal("descriptor is nil")
	}
	return desc
}

func TestSplitDescriptorProducesThreeURMultipartShares(t *testing.T) {
	desc := testDescriptor2of3(t)
	shares, err := SplitDescriptor(desc)
	if err != nil {
		t.Fatalf("split descriptor: %v", err)
	}
	if len(shares) != TotalShares {
		t.Fatalf("got %d shares, want %d", len(shares), TotalShares)
	}
	for i, s := range shares {
		typ, _, seqLen, ok := ParseShare(s)
		if !ok {
			t.Fatalf("share %d not multipart ur: %q", i+1, s)
		}
		if typ != "crypto-output" {
			t.Fatalf("share %d type=%q", i+1, typ)
		}
		if seqLen != RequiredShares {
			t.Fatalf("share %d seqLen=%d", i+1, seqLen)
		}
	}
}

func TestAllPairsRecoverCanonicalPayload(t *testing.T) {
	desc := testDescriptor2of3(t)
	shares, err := SplitDescriptor(desc)
	if err != nil {
		t.Fatalf("split descriptor: %v", err)
	}
	expected, err := canonicalURPayload(desc)
	if err != nil {
		t.Fatalf("canonical payload: %v", err)
	}

	pairs := [][2]int{{0, 1}, {0, 2}, {1, 2}}
	for _, p := range pairs {
		got, err := Combine([]string{shares[p[0]], shares[p[1]]})
		if err != nil {
			t.Fatalf("combine pair %v: %v", p, err)
		}
		if !bytes.Equal(got, expected) {
			t.Fatalf("pair %v payload mismatch", p)
		}
	}
}

func TestSplitCanonicalizationStableAcrossReorderAndMissingChildren(t *testing.T) {
	desc := testDescriptor2of3(t)
	reordered := *desc
	reordered.Keys = append([]urtypes.KeyDescriptor(nil), desc.Keys...)
	reordered.Keys[0], reordered.Keys[2] = reordered.Keys[2], reordered.Keys[0]
	for i := range reordered.Keys {
		reordered.Keys[i].Children = nil
	}
	a, err := SplitDescriptor(desc)
	if err != nil {
		t.Fatalf("split A: %v", err)
	}
	b, err := SplitDescriptor(&reordered)
	if err != nil {
		t.Fatalf("split B: %v", err)
	}
	for i := 0; i < TotalShares; i++ {
		if a[i] != b[i] {
			t.Fatalf("share %d mismatch", i+1)
		}
	}
}

func TestCombineRejectsSingleShare(t *testing.T) {
	desc := testDescriptor2of3(t)
	shares, err := SplitDescriptor(desc)
	if err != nil {
		t.Fatalf("split descriptor: %v", err)
	}
	if _, err := Combine([]string{shares[0]}); err != ErrInsufficientShares {
		t.Fatalf("got err=%v want ErrInsufficientShares", err)
	}
}

func TestCombinedPayloadParsesAsCryptoOutput(t *testing.T) {
	desc := testDescriptor2of3(t)
	shares, err := SplitDescriptor(desc)
	if err != nil {
		t.Fatalf("split descriptor: %v", err)
	}
	payload, err := Combine([]string{shares[0], shares[2]})
	if err != nil {
		t.Fatalf("combine: %v", err)
	}
	enc := ur.Encode("crypto-output", payload, 1, 1)
	var d ur.Decoder
	if err := d.Add(enc); err != nil {
		t.Fatalf("decoder add: %v", err)
	}
	typ, out, err := d.Result()
	if err != nil {
		t.Fatalf("decoder result: %v", err)
	}
	if typ != "crypto-output" {
		t.Fatalf("typ=%q", typ)
	}
	if !bytes.Equal(out, payload) {
		t.Fatal("decoded payload mismatch")
	}
}
