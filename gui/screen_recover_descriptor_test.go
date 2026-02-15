package gui

import (
	"strings"
	"testing"

	"seedetcher.com/bc/ur"
	"seedetcher.com/bc/urtypes"
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

func TestSafeEncodeDescriptorURLegacyOrderCompatibility(t *testing.T) {
	const newUR = "ur:crypto-output/taadmetaadmsoeadaoaolstaaddlonaxhdclaomheyadjkjegwollrpapehfkofdhpyactnbdnaaenfwswwfproxpeurdezmtoehjyaahdcxlotsskcedkrsdlsbtelrdtsaoessuynshsdwgukkdwlpehjnwszoemcscneoveayamtaaddyoeadlocsdyykaeykaeykaoykaocyftfzvtgaattaaddyoyadlrlfaeadwklawkaycyislrbytitaaddlonaxhdclaxhgmsluswtsgwdpgwcfeckerfoswksomubapmsscwtsfdltfeahtnjkdyfzghuertaahdcxcwfghfadwyjzaxlpcagwkorftnkeuynlbtcxdltiveimdmdpdeprwykbmeeoyarsamtaaddyoeadlocsdyykaeykaeykaoykaocyonmsdrglattaaddyoyadlrlfaeadwklawkaycygaaekktptaaddlonaxhdclaxnbmhknldgycxvllnnndwfmfnadrntiehjthtaofyparlbspymoprrhqdidldmydaaahdcxyalewtmopdzeaatikkteoeurrskgswylykwnsffnlelkpelrrswsbgfxrovapmksamtaaddyoeadlocsdyykaeykaeykaoykaocyndenspvsattaaddyoyadlrlfaeadwklawkaycyskdmfhlkadbdgwti"

	var d ur.Decoder
	if err := d.Add(newUR); err != nil {
		t.Fatalf("decode new ur: %v", err)
	}
	typ, enc, err := d.Result()
	if err != nil {
		t.Fatalf("new ur result: %v", err)
	}
	if typ != "crypto-output" {
		t.Fatalf("type=%q want crypto-output", typ)
	}
	got, err := safeEncodeDescriptorUR(enc)
	if err != nil {
		t.Fatalf("safeEncodeDescriptorUR: %v", err)
	}
	var out ur.Decoder
	if err := out.Add(got); err != nil {
		t.Fatalf("decode output ur: %v", err)
	}
	typ2, enc2, err := out.Result()
	if err != nil {
		t.Fatalf("output ur result: %v", err)
	}
	v2, err := urtypes.Parse(typ2, enc2)
	if err != nil {
		t.Fatalf("parse output ur: %v", err)
	}
	desc, ok := v2.(urtypes.OutputDescriptor)
	if !ok {
		t.Fatalf("parsed output type %T", v2)
	}
	for i := range desc.Keys {
		if len(desc.Keys[i].Children) != 0 {
			t.Fatalf("key %d children=%d want 0", i, len(desc.Keys[i].Children))
		}
	}
}
