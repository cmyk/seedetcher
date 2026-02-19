package compact2of3

import (
	"bytes"
	"testing"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/testutils"
)

var goldenSE2Mainnet2of3 = [3]string{
	"SE2:KNCTEAIBAIBQIBIGA4EASCQLBQGQ4DYQK25DTQQBAEBAGAIBABEQASIEAKNTNSHIHJAOASNFS4VE5AAAAAYIAAAAACAAAAAAQAAAAAQCAAAAAAAAAAAACAABAAAAAAAAAAAAAAE3G3EORRJOH6GAHIEQPKEVCIHDQ2PCYPR4AG7NAMLOLIBEJMNXB6VZFMVZWNRITDZF7CFPBEVI7YCNA6OTULP3666G6727DTB4RKGK7BF754JEHOHGVV4D4DCFUMGC2VQNZK6RDYAKN6NNVBNJA54T7SALPZ33ZQHBREINHIQOFTBCYROIWGBBXDTWUEPV6ASLJZQN24AHIAM7KHXY3BK2IQSK2FW6JYS6YUPRQWI5GUNJ2XA",
	"SE2:KNCTEAIBAIBQIBIGA4EASCQLBQGQ4DYQK25DTQQBAEBAGAQCABEQASIEAKNTNSHIHJAOASNFS4VE5AAAAAYIAAAAACAAAAAAQAAAAAQCAAAAAAAAAAAACAABAAAAAAAAAAAAAAB2IDQES2EECHIAFEBSAFZWWT5GQSY26VTWJBN7QH5AFMCDMQWG6OZKJL67FD744MLURDL4KHBEX4X4XU4EFHBKFRG3TRQSYU3ZFSCTC3PP7M3RQIZT4QEHAJUMO4U3SVMLR7HH3UCQ6RERDQAVZHHHTIRFYI2DRZEEWRGPBVHYBXGWO2XXJK4ZRPGB7KZBQ47OEMCI76XQ2RVKAN23W5T7OQRROKFV2LX47AQAO7DW5ZGYU4I",
	"SE2:KNCTEAIBAIBQIBIGA4EASCQLBQGQ4DYQK25DTQQBAEBAGAYDABEQASIEAKNTNSHIHJAOASNFS4VE5AAAAAYIAAAAACAAAAAAQAAAAAQCAAAAAAAAAAAACAABAAAAAAAAAAAAAAFFS4VE4SIAPHMAGV4XRPDNOTZNJ4MTK7F4U72MTEYOVXCBXV2IQ5CQLWTTGBAFJXWADNDFMAPONQBYKHKPO26NU7G3TEGSAL6Q4RVC4LJIWLXH5EJT7C7UVS6LHPAT4VACI4KFTQDGO3Z2GCEKPCR3BBB7LX7EJ6FHKR4BUDNKCLEF5BHC5BNREWNMHARCKXEN6PJUU52WKOYJHWZ2SKBMMQQYCMRMC2TJDA27TFFEY2OZO2I",
}

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

func TestSplitEncodeDecodeCombine_AllPairs(t *testing.T) {
	desc := testDescriptor2of3(t)
	shares, err := SplitDescriptor(desc, SplitOptions{})
	if err != nil {
		t.Fatalf("split descriptor: %v", err)
	}
	if len(shares) != 3 {
		t.Fatalf("got %d shares, want 3", len(shares))
	}

	encoded := make([]string, 0, 3)
	for i, sh := range shares {
		s, err := Encode(sh)
		if err != nil {
			t.Fatalf("encode share %d: %v", i+1, err)
		}
		encoded = append(encoded, s)
	}
	decoded := make([]Share, 0, 3)
	for i, s := range encoded {
		sh, err := Decode(s)
		if err != nil {
			t.Fatalf("decode share %d: %v", i+1, err)
		}
		decoded = append(decoded, sh)
	}

	want := desc.Encode()
	pairs := [][2]int{{0, 1}, {0, 2}, {1, 2}}
	for _, p := range pairs {
		got, err := CombineToDescriptorPayload([]Share{decoded[p[0]], decoded[p[1]]})
		if err != nil {
			t.Fatalf("combine pair %v: %v", p, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("combine pair %v mismatch", p)
		}
	}
}

func TestCombineRejectsMismatchedSet(t *testing.T) {
	desc := testDescriptor2of3(t)
	a, err := SplitDescriptor(desc, SplitOptions{})
	if err != nil {
		t.Fatalf("split A: %v", err)
	}
	var forced [16]byte
	forced[0] = 0x42
	b, err := SplitDescriptor(desc, SplitOptions{SetID: forced})
	if err != nil {
		t.Fatalf("split B: %v", err)
	}
	if _, err := CombineToDescriptorPayload([]Share{a[0], b[1]}); err == nil {
		t.Fatal("expected mismatch error")
	}
}

func TestCompact2of3GoldenPayloadsStable(t *testing.T) {
	desc := testDescriptor2of3(t)
	var sid [16]byte
	for i := range sid {
		sid[i] = byte(i + 1)
	}
	shares, err := SplitDescriptor(desc, SplitOptions{SetID: sid})
	if err != nil {
		t.Fatalf("split descriptor: %v", err)
	}
	if len(shares) != 3 {
		t.Fatalf("got %d shares, want 3", len(shares))
	}
	for i := 0; i < 3; i++ {
		got, err := Encode(shares[i])
		if err != nil {
			t.Fatalf("encode share %d: %v", i+1, err)
		}
		if got != goldenSE2Mainnet2of3[i] {
			t.Fatalf("golden mismatch share %d\n got: %s\nwant: %s", i+1, got, goldenSE2Mainnet2of3[i])
		}
	}
}
