package urxor2of3

import (
	"bytes"
	"fmt"
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
	if len(shares) != len(desc.Keys) {
		t.Fatalf("got %d shares, want %d", len(shares), len(desc.Keys))
	}
	for i, s := range shares {
		typ, _, seqLen, ok := ParseShare(s)
		if !ok {
			t.Fatalf("share %d not multipart ur: %q", i+1, s)
		}
		if typ != "crypto-output" {
			t.Fatalf("share %d type=%q", i+1, typ)
		}
		if seqLen != desc.Threshold {
			t.Fatalf("share %d seqLen=%d want %d", i+1, seqLen, desc.Threshold)
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
	for i := range len(desc.Keys) {
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

func TestSplitDescriptorForShareProducesTwoFragmentsFor3of5(t *testing.T) {
	cfg := testutils.WalletConfigs["multisig-3of5"]
	_, desc, err := testutils.ParseWallet(cfg, "", "")
	if err != nil {
		t.Fatalf("parse wallet: %v", err)
	}
	if desc == nil {
		t.Fatal("descriptor is nil")
	}
	for i := range desc.Keys {
		frags, err := SplitDescriptorForShare(desc, i)
		if err != nil {
			t.Fatalf("split share %d: %v", i+1, err)
		}
		if len(frags) != 2 {
			t.Fatalf("share %d: got %d fragments want 2", i+1, len(frags))
		}
	}
}

func TestCombineRecovers3of5FromThreeShares(t *testing.T) {
	cfg := testutils.WalletConfigs["multisig-3of5"]
	_, desc, err := testutils.ParseWallet(cfg, "", "")
	if err != nil {
		t.Fatalf("parse wallet: %v", err)
	}
	if desc == nil {
		t.Fatal("descriptor is nil")
	}
	expected, err := canonicalURPayload(desc)
	if err != nil {
		t.Fatalf("canonical payload: %v", err)
	}
	var parts []string
	for _, idx := range []int{0, 2, 4} {
		frags, err := SplitDescriptorForShare(desc, idx)
		if err != nil {
			t.Fatalf("split share %d: %v", idx+1, err)
		}
		parts = append(parts, frags...)
	}
	got, err := Combine(parts)
	if err != nil {
		t.Fatalf("combine 3-of-5: %v", err)
	}
	if !bytes.Equal(got, expected) {
		t.Fatal("combined payload mismatch")
	}
}

func TestCombineRecovers3of5AllTenCombinations(t *testing.T) {
	cfg := testutils.WalletConfigs["multisig-3of5"]
	_, desc, err := testutils.ParseWallet(cfg, "", "")
	if err != nil {
		t.Fatalf("parse wallet: %v", err)
	}
	if desc == nil {
		t.Fatal("descriptor is nil")
	}
	expected, err := canonicalURPayload(desc)
	if err != nil {
		t.Fatalf("canonical payload: %v", err)
	}
	for a := 0; a < len(desc.Keys)-2; a++ {
		for b := a + 1; b < len(desc.Keys)-1; b++ {
			for c := b + 1; c < len(desc.Keys); c++ {
				t.Run(fmt.Sprintf("%d-%d-%d", a+1, b+1, c+1), func(t *testing.T) {
					var parts []string
					for _, idx := range []int{a, b, c} {
						frags, err := SplitDescriptorForShare(desc, idx)
						if err != nil {
							t.Fatalf("split share %d: %v", idx+1, err)
						}
						parts = append(parts, frags...)
					}
					got, err := Combine(parts)
					if err != nil {
						t.Fatalf("combine shares (%d,%d,%d): %v", a+1, b+1, c+1, err)
					}
					if !bytes.Equal(got, expected) {
						t.Fatalf("payload mismatch for shares (%d,%d,%d)", a+1, b+1, c+1)
					}
				})
			}
		}
	}
}

func TestStableSharePayloadVectors(t *testing.T) {
	cases := []struct {
		name      string
		walletKey string
		want      [][]string // per-share payloads
	}{
		{
			name:      "2of3_mainnet",
			walletKey: "multisig-mainnet-2of3",
			want: [][]string{
				{"UR:CRYPTO-OUTPUT/1-2/LPADAOCFADFZCYFPNLSAHHHDNBTAADMETAADMSOEADAOAOLSTAADDLOXAXHDCLAOMHEYADJKJEGWOLLRPAPEHFKOFDHPYACTNBDNAAENFWSWWFPROXPEURDEZMTOEHJYAAHDCXLOTSSKCEDKRSDLSBTELRDTSAOESSUYNSHSDWGUKKDWLPEHJNWSZOEMCSCNEOVEAYAMTAADDYOEADLOCSDYYKAEYKAEYKAOYKAOCYFTFZVTGAAYCYISLRBYTITAADDLOXAXHDCLAXNBMHKNLDGYCXVLLNNNDWFMFNADRNTIEHJTHTAOFYPARLBSPYMOPRRHQDIDLDMYDAAAHDCXYALEWTMUJNCACM"},
				{"UR:CRYPTO-OUTPUT/2-2/LPAOAOCFADFZCYFPNLSAHHHDNBMOPDZEAATIKKTEOEURRSKGSWYLYKWNSFFNLELKPELRRSWSBGFXROVAPMKSAMTAADDYOEADLOCSDYYKAEYKAEYKAOYKAOCYNDENSPVSAYCYSKDMFHLKTAADDLOXAXHDCLAXHGMSLUSWTSGWDPGWCFECKERFOSWKSOMUBAPMSSCWTSFDLTFEAHTNJKDYFZGHUERTAAHDCXCWFGHFADWYJZAXLPCAGWKORFTNKEUYNLBTCXDLTIVEIMDMDPDEPRWYKBMEEOYARSAMTAADDYOEADLOCSDYYKAEYKAEYKAOYKAOCYONMSDRGLAYCYGAAEKKTPIDLNGSEO"},
				{"UR:CRYPTO-OUTPUT/3-2/LPAXAOCFADFZCYFPNLSAHHHDNBGRPTJLUTTTWYJSOTUTRYYACTYNTNGOTKIEPYMNFHRPRNNSKKBNCKIDCETSGDPEGAJEHTCKDEEOEESRFWEOWFFLOLHTUTEYIEYAYTNSBNFWVWOLVSGASKDAMHLUSPLUONDRMDECGWCAGRDMADCEHNCFYTLGSGCWEYOXCMMNYLZMURGLHYFYECKSJPROHDIEDNRTWNHDTLCFQDGHCWTYDWVLSFBZGOCKETSBPSAOMKCPLRDWLOSKINMNRYGMFRRSHYJPREIYMUETVWAEMNJPDYVAFWEYPAPAFWBSHYMHFLRKPTSTCKONJEBNFWINYAWFDEGRWSJKWT"},
			},
		},
		{
			name:      "3of5_testnet",
			walletKey: "multisig-3of5",
			want: [][]string{
				{
					"UR:CRYPTO-OUTPUT/1-6/LPADAMCFAOEHCYZTWLDWLNHDHYTAADMETAADMSOEADAXAOLPTAADDLONAXHDCLAXGWWZHNLEKKPDCHVYPLATLEGEFXTESAETFZVTFHFYJZBNGYBZFHGYCNPEAXFPBYLFAAHDCXTLJTLEIHJTEMWTSENTWLVECPPYZTADEMINAHIHBSDIBZJLFRNTENDLBBGEIYMHMNAHTAADEHOYAOADAMFEPDFNCS",
					"UR:CRYPTO-OUTPUT/206-6/LPCSTOAMCFAOEHCYZTWLDWLNHDHYTTFROTENOSSPHPMEONVYCNJOWMIHOEWKHKLDWZESOEKEUTSOZMLGBWTSHNFRWZPYSTVSDAGRGDETMTFMKEADOSREVSFLWDDAKTJZSBCNUTGRKIPFNTPFFWDNMDMSVSCWCESEIOTYESNBKPBDFEAMJNENMUSBGWGWUTBBVSRFGEJSUYHHIMIHVTLYLRHNGDFTGUCX",
				},
				{
					"UR:CRYPTO-OUTPUT/2-6/LPAOAMCFAOEHCYZTWLDWLNHDHYTAADDYOEADLOCSDYYKADYKAEYKAOYKAOCYBYGHFYFXAYCYBDBAINDYTAADDLONAXHDCLAOZOZMOTWYHGFHSBEYMKKITPLPGTEYMKFRMKRLRLAASFJNISRPCYSEMSCECTURETNSAAHDCXKTRYMEMWGRWSIOVACTGLNESBPECYPKCHVEPTAAJZBNLUBBCELBROHFJP",
					"UR:CRYPTO-OUTPUT/85-6/LPCSGOAMCFAOEHCYZTWLDWLNHDHYIEBGWTWSDMPMNYYTIMSTGRCAMTCSKOTTNTDTJLRNPDWPSGINEYJZVEVWEHIOUTFPLUZSWLGOENKPOTMYGUJKMSZSIEMEIEJTLPSKFZJZBSLNDTTNATSNAAOYAYSBTILYKNTNBACEVTRKTKFMBBCETEQDFDWZPENSHHWNVOLTIMOLMEUYZMQDKBGDWFHFGSCMCKNE",
				},
				{
					"UR:CRYPTO-OUTPUT/3-6/LPAXAMCFAOEHCYZTWLDWLNHDHYMYMOGRWEGSKBCNKKAHTAADEHOYAOADAMTAADDYOEADLOCSDYYKADYKAEYKAOYKAOCYLGVDFGBZAYCYEHCFEHCMTAADDLONAXHDCLAOJSFNTDNTYALOJSJEOSHKAYGSJTFLYTMDFWVTAARNBDLRBGADOLTDSBSTPDINCWWEAAHDCXMNPALNATVSGMWZGDDNYAGUQZ",
					"UR:CRYPTO-OUTPUT/16-6/LPBEAMCFAOEHCYZTWLDWLNHDHYHDPLZTCEPSASHKPMNSNLFHBZVYCYTYDKGTEMKGGTVODTGMZEIESGSEOTIASASWAHSRMDAABKFSINBDRELKTAGULRRFFLCKCLQZOYONPANBSBLBCHSSHYPSWZHSRDROCYHYCSPLHHWYKIKEJZRHLRDWJZVYTIBDMSTETDASLDIECYLBFGTSDNZTZTFTATHNSAEYBN",
				},
				{
					"UR:CRYPTO-OUTPUT/4-6/LPAAAMCFAOEHCYZTWLDWLNHDHYQDDMVAIHTOSKHTCEAHLRAHVTCPDPWFWZGRDLJKHTZSDABETLAHTAADEHOYAOADAMTAADDYOEADLOCSDYYKADYKAEYKAOYKAOCYSFHYDYKEAYCYMSPSWSCWTAADDLONAXHDCLAOZSRLTLBWVYUTBYMOLSJKEEVYUYHFVDVLKOJZAHLYHHPYSASTKTDMCWLGIOREKK",
					"UR:CRYPTO-OUTPUT/507-6/LPCFADZOAMCFAOEHCYZTWLDWLNHDHYLTPDTPKKWDFMHNTPGOESTSFPRSIHHFWTNYNLMTURVTZTURWZAAVWTBBAMWCMOEPKLPFYRTYNRDMUIDHDHTZOLSWKMWPFSGJECATLWZSGHFDMVELRKSPTNEMTBTAYROIMLRAEJTMOLYLRRFRYGDLADILBDSVAMSPTDNSSPKOEROFGPAFYVSBAAAHDIDDWEHPFIMSS",
				},
				{
					"UR:CRYPTO-OUTPUT/5-6/LPAHAMCFAOEHCYZTWLDWLNHDHYFTRKRHGWSKAAHDCXFNZTCFLPDEGMLPCPHEMEZSDMRDJONEVONNNNTEFEOEZOECPDUTKEBYVLJZTALPRKAHTAADEHOYAOADAMTAADDYOEADLOCSDYYKADYKAEYKAOYKAOCYYASBJPHNAYCYLNCLMUTETAADDLONAXHDCLAOFXFWJTDAFGCMLKUYBKMHKECPDLTNWY",
					"UR:CRYPTO-OUTPUT/117-6/LPCSKPAMCFAOEHCYZTWLDWLNHDHYHDPLHLIOPSCMVLNSIMNYGWSFBZEMLRDABSATDWFGGUFPSALKSAQZBETYIHIODTFEFDKOFMPACPYKOYMNRSFXJYCNMHRFEEJLSTDECEDPGWHHPLRECNGUJYURGDWPESWPIHAONLOXRLIMIDTYGTCTFZMTWLBTLDWSIABTWPYKHYLSNNENTDKOGYKPDLCAPEYKJKRP",
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, desc, err := testutils.ParseWallet(testutils.WalletConfigs[tc.walletKey], "", "")
			if err != nil {
				t.Fatalf("parse wallet: %v", err)
			}
			if desc == nil {
				t.Fatal("descriptor is nil")
			}
			if len(tc.want) != len(desc.Keys) {
				t.Fatalf("bad test vector shape: got %d shares want %d", len(tc.want), len(desc.Keys))
			}
			for i := range desc.Keys {
				got, err := SplitDescriptorForShare(desc, i)
				if err != nil {
					t.Fatalf("split share %d: %v", i+1, err)
				}
				if len(got) != len(tc.want[i]) {
					t.Fatalf("share %d fragments=%d want %d", i+1, len(got), len(tc.want[i]))
				}
				for j := range got {
					if got[j] != tc.want[i][j] {
						t.Fatalf("share %d fragment %d mismatch", i+1, j+1)
					}
				}
			}
		})
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
