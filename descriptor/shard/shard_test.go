package shard

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"

	"seedetcher.com/bc/urtypes"
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

func TestSplitCombinePayloadRoundTrip(t *testing.T) {
	payload := "UR:CRYPTO-OUTPUT/TESTPAYLOAD123"
	shares, err := SplitPayload(payload, SplitOptions{Threshold: 2, Total: 3})
	if err != nil {
		t.Fatalf("split payload failed: %v", err)
	}
	got, err := CombinePayload([]Share{shares[0], shares[2]})
	if err != nil {
		t.Fatalf("combine payload failed: %v", err)
	}
	if got != payload {
		t.Fatalf("payload mismatch\\n got: %s\\nwant: %s", got, payload)
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
	var setA [16]byte
	var setB [16]byte
	setA[0] = 0x01
	setB[0] = 0x02

	a, err := Split(desc, SplitOptions{Threshold: 2, Total: 3, SetID: setA})
	if err != nil {
		t.Fatalf("split A failed: %v", err)
	}
	b, err := Split(desc, SplitOptions{Threshold: 2, Total: 3, SetID: setB})
	if err != nil {
		t.Fatalf("split B failed: %v", err)
	}
	if _, err := Combine([]Share{a[0], b[1]}); err == nil {
		t.Fatal("expected set mismatch error")
	}
}

func TestSplitDefaultSetIDDeterministic(t *testing.T) {
	desc := testDescriptor(t)
	want, err := CanonicalizeDescriptor(desc)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	wantSet := DeriveSetID([]byte(want), 2, 3)

	a, err := Split(desc, SplitOptions{Threshold: 2, Total: 3})
	if err != nil {
		t.Fatalf("split A failed: %v", err)
	}
	b, err := Split(desc, SplitOptions{Threshold: 2, Total: 3})
	if err != nil {
		t.Fatalf("split B failed: %v", err)
	}
	for i := range a {
		if a[i].SetID != wantSet {
			t.Fatalf("share %d unexpected set id", i)
		}
		if b[i].SetID != wantSet {
			t.Fatalf("share %d unexpected set id in second split", i)
		}
		if a[i].SetID != b[i].SetID {
			t.Fatalf("share %d set id mismatch", i)
		}
		if a[i].Index != b[i].Index {
			t.Fatalf("share %d index mismatch", i)
		}
		if string(a[i].Data) != string(b[i].Data) {
			t.Fatalf("share %d payload bytes mismatch", i)
		}
		aEnc, err := MarshalBinary(a[i])
		if err != nil {
			t.Fatalf("marshal A[%d]: %v", i, err)
		}
		bEnc, err := MarshalBinary(b[i])
		if err != nil {
			t.Fatalf("marshal B[%d]: %v", i, err)
		}
		if string(aEnc) != string(bEnc) {
			t.Fatalf("share %d encoded mismatch", i)
		}
	}
}

func TestDeriveSetIDVariesWithPayloadAndParams(t *testing.T) {
	p1 := []byte("payload-one")
	p2 := []byte("payload-two")

	id11 := DeriveSetID(p1, 2, 3)
	id12 := DeriveSetID(p1, 3, 3)
	id13 := DeriveSetID(p1, 2, 4)
	id21 := DeriveSetID(p2, 2, 3)

	if id11 == id12 {
		t.Fatal("set id should differ when threshold differs")
	}
	if id11 == id13 {
		t.Fatal("set id should differ when total differs")
	}
	if id11 == id21 {
		t.Fatal("set id should differ when payload differs")
	}
}

func TestDescriptorPayloadCanonicalizationIgnoresSortedMultiKeyOrder(t *testing.T) {
	cfg := testutils.WalletConfigs["multisig-mainnet-2of3"]
	_, desc, err := testutils.ParseWallet(cfg, "", "")
	if err != nil {
		t.Fatalf("parse wallet: %v", err)
	}
	if desc == nil || desc.Type != urtypes.SortedMulti || len(desc.Keys) < 2 {
		t.Fatal("missing sortedmulti test descriptor")
	}

	orig := desc.Encode()
	reordered := *desc
	reordered.Keys = append([]urtypes.KeyDescriptor(nil), desc.Keys...)
	reordered.Keys[0], reordered.Keys[1] = reordered.Keys[1], reordered.Keys[0]
	reorderedPayload := reordered.Encode()

	idA := DeriveSetID(orig, uint8(desc.Threshold), uint8(len(desc.Keys)))
	idB := DeriveSetID(reorderedPayload, uint8(reordered.Threshold), uint8(len(reordered.Keys)))
	if idA != idB {
		t.Fatal("set id mismatch for reordered sortedmulti keys")
	}

	a, err := SplitPayloadBytes(orig, SplitOptions{Threshold: uint8(desc.Threshold), Total: uint8(len(desc.Keys))})
	if err != nil {
		t.Fatalf("split A: %v", err)
	}
	b, err := SplitPayloadBytes(reorderedPayload, SplitOptions{Threshold: uint8(reordered.Threshold), Total: uint8(len(reordered.Keys))})
	if err != nil {
		t.Fatalf("split B: %v", err)
	}
	if len(a) != len(b) {
		t.Fatalf("share count mismatch: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].SetID != b[i].SetID {
			t.Fatalf("share %d set id mismatch", i)
		}
		if !bytes.Equal(a[i].Data, b[i].Data) {
			t.Fatalf("share %d data mismatch", i)
		}
		ae, err := MarshalBinary(a[i])
		if err != nil {
			t.Fatalf("marshal A[%d]: %v", i, err)
		}
		be, err := MarshalBinary(b[i])
		if err != nil {
			t.Fatalf("marshal B[%d]: %v", i, err)
		}
		if !bytes.Equal(ae, be) {
			t.Fatalf("share %d encoding mismatch", i)
		}
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
