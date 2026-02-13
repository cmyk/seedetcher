package shard

import (
	"math/rand"
	"strings"
	"testing"

	"seedetcher.com/testutils"
)

func testDescriptor(t *testing.T) string {
	t.Helper()
	cfg := testutils.WalletConfigs["multisig"]
	if cfg.Descriptor == "" {
		t.Fatal("missing test descriptor")
	}
	return cfg.Descriptor
}

func TestCanonicalizeDescriptor(t *testing.T) {
	raw := "  \n" + strings.ReplaceAll(testDescriptor(t), "(", " ( ") + " \t\n"
	got, err := CanonicalizeDescriptor(raw)
	if err != nil {
		t.Fatalf("canonicalize failed: %v", err)
	}
	want := testDescriptor(t)
	if got != want {
		t.Fatalf("canonical mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestCanonicalizeDescriptorRequiresChecksum(t *testing.T) {
	d := testDescriptor(t)
	d = d[:strings.LastIndex(d, "#")]
	if _, err := CanonicalizeDescriptor(d); err == nil {
		t.Fatal("expected error for missing checksum")
	}
}

func TestSplitCombineRoundTrip(t *testing.T) {
	desc := testDescriptor(t)
	shares, err := Split(desc, SplitOptions{Threshold: 2, Total: 3})
	if err != nil {
		t.Fatalf("split failed: %v", err)
	}
	if len(shares) != 3 {
		t.Fatalf("got %d shares, want 3", len(shares))
	}
	got, err := Combine([]Share{shares[0], shares[2]})
	if err != nil {
		t.Fatalf("combine failed: %v", err)
	}
	want, _ := CanonicalizeDescriptor(desc)
	if got != want {
		t.Fatalf("reconstruction mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestCombineFailsBelowThreshold(t *testing.T) {
	desc := testDescriptor(t)
	shares, err := Split(desc, SplitOptions{Threshold: 3, Total: 5})
	if err != nil {
		t.Fatalf("split failed: %v", err)
	}
	if _, err := Combine([]Share{shares[0], shares[1]}); err == nil {
		t.Fatal("expected threshold error")
	}
}

func TestCombineRejectsMismatchedSet(t *testing.T) {
	desc := testDescriptor(t)
	a, err := Split(desc, SplitOptions{Threshold: 2, Total: 3})
	if err != nil {
		t.Fatalf("split A failed: %v", err)
	}
	b, err := Split(desc, SplitOptions{Threshold: 2, Total: 3})
	if err != nil {
		t.Fatalf("split B failed: %v", err)
	}
	if _, err := Combine([]Share{a[0], b[1]}); err == nil {
		t.Fatal("expected set mismatch error")
	}
}

func TestCombineRejectsDuplicateIndex(t *testing.T) {
	desc := testDescriptor(t)
	shares, err := Split(desc, SplitOptions{Threshold: 2, Total: 3})
	if err != nil {
		t.Fatalf("split failed: %v", err)
	}
	dup := shares[0]
	if _, err := Combine([]Share{shares[0], dup}); err == nil {
		t.Fatal("expected duplicate index error")
	}
}

func TestEncodeDecodeAndCorruption(t *testing.T) {
	desc := testDescriptor(t)
	shares, err := Split(desc, SplitOptions{Threshold: 2, Total: 3})
	if err != nil {
		t.Fatalf("split failed: %v", err)
	}

	enc, err := Encode(shares[0])
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	decoded, err := Decode(enc)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if decoded.Index != shares[0].Index || decoded.Threshold != shares[0].Threshold || decoded.Total != shares[0].Total {
		t.Fatal("decoded share metadata mismatch")
	}

	bin, err := MarshalBinary(shares[0])
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	bin[len(bin)-1] ^= 0x01
	if _, err := UnmarshalBinary(bin); err == nil {
		t.Fatal("expected checksum mismatch on corrupted binary")
	}
}

func TestSplitCombineRandomized(t *testing.T) {
	corpus := []string{
		testutils.WalletConfigs["singlesig"].Descriptor,
		testutils.WalletConfigs["multisig"].Descriptor,
		"wsh(sortedmulti(2,[dc567276/48h/0h/0h/2h]xpub6DiYrfRwNnjeX4vHsWMajJVFKrbEEnu8gAW9vDuQzgTWEsEHE16sGWeXXUV1LBWQE1yCTmeprSNcqZ3W74hqVdgDbtYHUv3eM4W2TEUhpan/0/*,[f245ae38/48h/0h/0h/2h]xpub6DnT4E1fT8VxuAZW29avMjr5i99aYTHBp9d7fiLnpL5t4JEprQqPMbTw7k7rh5tZZ2F5g8PJpssqrZoebzBChaiJrmEvWwUTEMAbHsY39Ge/0/*,[c5d87297/48h/0h/0h/2h]xpub6DjrnfAyuonMaboEb3ZQZzhQ2ZEgaKV2r64BFmqymZqJqviLTe1JzMr2X2RfQF892RH7MyYUbcy77R7pPu1P71xoj8cDUMNhAMGYzKR4noZ/0/*))#hfwurrvt",
	}
	for i, d := range corpus {
		if d == "" {
			t.Fatalf("empty descriptor in corpus at %d", i)
		}
	}

	r := rand.New(rand.NewSource(1))
	for iter := 0; iter < 100; iter++ {
		desc := corpus[r.Intn(len(corpus))]
		total := uint8(2 + r.Intn(7)) // 2..8
		threshold := uint8(2 + r.Intn(int(total)-1))
		shares, err := Split(desc, SplitOptions{Threshold: threshold, Total: total})
		if err != nil {
			t.Fatalf("iter %d: split failed: %v", iter, err)
		}

		perm := r.Perm(len(shares))
		pickCount := int(threshold) + r.Intn(len(shares)-int(threshold)+1)
		picked := make([]Share, 0, pickCount)
		for _, idx := range perm[:pickCount] {
			picked = append(picked, shares[idx])
		}

		got, err := Combine(picked)
		if err != nil {
			t.Fatalf("iter %d: combine failed: %v", iter, err)
		}
		want, _ := CanonicalizeDescriptor(desc)
		if got != want {
			t.Fatalf("iter %d: reconstruction mismatch", iter)
		}
	}
}
