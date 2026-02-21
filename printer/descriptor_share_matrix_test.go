package printer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"seedetcher.com/bc/ur"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/descriptor/urxor2of3"
	"seedetcher.com/testutils"
)

type descriptorShareMode int

const (
	shareModeURXOR descriptorShareMode = iota
	shareModeFullUR
)

func TestDescriptorShareMatrix_RepresentativeScriptsNetworks(t *testing.T) {
	type baseCase struct {
		name      string
		walletKey string
		mutate    func(*urtypes.OutputDescriptor)
		expect    descriptorShareMode
	}
	bases := []baseCase{
		{name: "2of3", walletKey: "multisig", expect: shareModeURXOR},
		{name: "2of4", walletKey: "multisig-2of4", expect: shareModeURXOR},
		{name: "3of5", walletKey: "multisig-3of5", expect: shareModeURXOR},
		{
			name:      "nMinusOne_4of5",
			walletKey: "multisig-3of5",
			mutate: func(d *urtypes.OutputDescriptor) {
				d.Threshold = 4
			},
			expect: shareModeURXOR,
		},
		{name: "fallback_7of10", walletKey: "multisig-7of10", expect: shareModeFullUR},
	}

	scripts := []struct {
		name   string
		script urtypes.Script
	}{
		{name: "p2wsh", script: urtypes.P2WSH},
		{name: "p2sh_p2wsh", script: urtypes.P2SH_P2WSH},
	}
	networks := []struct {
		name string
		net  *chaincfg.Params
	}{
		{name: "main", net: &chaincfg.MainNetParams},
		{name: "test", net: &chaincfg.TestNet3Params},
	}

	for _, b := range bases {
		_, parsed, err := testutils.ParseWallet(testutils.WalletConfigs[b.walletKey], "", "")
		if err != nil {
			t.Fatalf("parse wallet %s: %v", b.walletKey, err)
		}
		if parsed == nil {
			t.Fatalf("wallet %s has no descriptor", b.walletKey)
		}

		for _, s := range scripts {
			for _, n := range networks {
				name := fmt.Sprintf("%s/%s/%s", b.name, s.name, n.name)
				t.Run(name, func(t *testing.T) {
					desc := cloneDescriptor(parsed)
					if b.mutate != nil {
						b.mutate(&desc)
					}
					desc.Script = s.script
					for i := range desc.Keys {
						desc.Keys[i].Network = n.net
					}

					payloadsByShare, mode := collectSharePayloads(t, &desc)
					if mode != b.expect {
						t.Fatalf("mode=%v want=%v", mode, b.expect)
					}

					selected := payloadsByShare[:desc.Threshold]
					recovered := recoverDescriptorPayload(t, selected, mode)
					v, err := urtypes.Parse("crypto-output", recovered)
					if err != nil {
						t.Fatalf("parse recovered payload: %v", err)
					}
					out, ok := v.(urtypes.OutputDescriptor)
					if !ok {
						t.Fatalf("recovered type %T", v)
					}
					if out.Type != desc.Type {
						t.Fatalf("type mismatch: got %v want %v", out.Type, desc.Type)
					}
					if out.Script != desc.Script {
						t.Fatalf("script mismatch: got %v want %v", out.Script, desc.Script)
					}
					if out.Threshold != desc.Threshold {
						t.Fatalf("threshold mismatch: got %d want %d", out.Threshold, desc.Threshold)
					}
					if len(out.Keys) != len(desc.Keys) {
						t.Fatalf("key count mismatch: got %d want %d", len(out.Keys), len(desc.Keys))
					}
					wantFP := fingerprintSet(desc.Keys)
					gotFP := fingerprintSet(out.Keys)
					if strings.Join(gotFP, ",") != strings.Join(wantFP, ",") {
						t.Fatalf("fingerprint set mismatch: got=%v want=%v", gotFP, wantFP)
					}
				})
			}
		}
	}
}

func collectSharePayloads(t *testing.T, desc *urtypes.OutputDescriptor) ([][]string, descriptorShareMode) {
	t.Helper()
	total := len(desc.Keys)
	all := make([][]string, total)
	mode := shareModeFullUR
	for i := 0; i < total; i++ {
		payloads, err := descriptorShardQRPayloadsForShare(desc, total, i)
		if err != nil {
			t.Fatalf("share %d payloads: %v", i+1, err)
		}
		if len(payloads) == 0 {
			t.Fatalf("share %d returned zero payloads", i+1)
		}
		all[i] = payloads
		if typ, _, seqLen, ok := urxor2of3.ParseShare(payloads[0]); ok && typ == "crypto-output" && seqLen >= urxor2of3.MinShares {
			mode = shareModeURXOR
		}
	}
	return all, mode
}

func recoverDescriptorPayload(t *testing.T, selected [][]string, mode descriptorShareMode) []byte {
	t.Helper()
	flat := make([]string, 0, len(selected)*2)
	for _, frags := range selected {
		flat = append(flat, frags...)
	}
	if mode == shareModeURXOR {
		payload, err := urxor2of3.Combine(flat)
		if err != nil {
			t.Fatalf("ur/xor combine: %v", err)
		}
		return payload
	}
	var d ur.Decoder
	for _, s := range flat {
		if err := d.Add(s); err != nil {
			t.Fatalf("ur add: %v", err)
		}
	}
	typ, payload, err := d.Result()
	if err != nil {
		t.Fatalf("ur result: %v", err)
	}
	if typ != "crypto-output" {
		t.Fatalf("ur type=%q want crypto-output", typ)
	}
	return payload
}

func cloneDescriptor(in *urtypes.OutputDescriptor) urtypes.OutputDescriptor {
	out := *in
	out.Keys = make([]urtypes.KeyDescriptor, len(in.Keys))
	for i := range in.Keys {
		k := in.Keys[i]
		k.KeyData = append([]byte(nil), k.KeyData...)
		k.ChainCode = append([]byte(nil), k.ChainCode...)
		k.DerivationPath = append(urtypes.Path(nil), k.DerivationPath...)
		k.Children = append([]urtypes.Derivation(nil), k.Children...)
		out.Keys[i] = k
	}
	return out
}

func fingerprintSet(keys []urtypes.KeyDescriptor) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = fmt.Sprintf("%08x", k.MasterFingerprint)
	}
	// tiny deterministic insertion sort; avoids pulling extra deps.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
