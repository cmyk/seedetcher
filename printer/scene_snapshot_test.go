package printer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"seedetcher.com/testutils"
	"seedetcher.com/version"
)

func TestWalletSceneSnapshots(t *testing.T) {
	cases := []struct {
		wallet     string
		compact2of3 bool
		wantScenes int
		wantHash   string
	}{
		{
			wallet:      "singlesig",
			compact2of3: false,
			wantScenes:  1,
			wantHash:    "6fd6d6a121d41d72f24d6101aaa89f800ae36495f16f808d74d66165f5cf93fb",
		},
		{
			wallet:      "multisig-mainnet-2of3",
			compact2of3: false,
			wantScenes:  6,
			wantHash:    "f9e0fcc598bfff542a7c6d99afdec9094e1502e56917bde69c40ed4297c8935c",
		},
		{
			wallet:      "multisig-3of5",
			compact2of3: false,
			wantScenes:  10,
			wantHash:    "e028083dd4d32ce0aebc1dda96bd0818f13ed1778bd7326ca93b4ee51b716c59",
		},
		{
			wallet:      "multisig",
			compact2of3: true,
			wantScenes:  3,
			wantHash:    "7113fd7248bc84f7138ee541443d4ded31ee076d5cc51c11725831f7ce6ff3cd",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.wallet, func(t *testing.T) {
			SetWalletLabel(DefaultWalletLabel)
			SetDescriptorQRSize(80.0)
			SetCompactDescriptor2of3Enabled(tc.compact2of3)

			cfg, ok := testutils.WalletConfigs[tc.wallet]
			if !ok {
				t.Fatalf("wallet fixture not found: %s", tc.wallet)
			}
			mnemonics, desc, err := testutils.ParseWallet(cfg, "", "")
			if err != nil {
				t.Fatalf("ParseWallet(%s): %v", tc.wallet, err)
			}
			doc, err := WalletSceneBuilder{
				Mnemonics:       mnemonics,
				Desc:            desc,
				SinglesigLayout: SinglesigLayoutSeedWithInfo,
			}.Build()
			if err != nil {
				t.Fatalf("WalletSceneBuilder.Build(%s): %v", tc.wallet, err)
			}
			if len(doc.Scenes) != tc.wantScenes {
				t.Fatalf("scene count mismatch: got=%d want=%d", len(doc.Scenes), tc.wantScenes)
			}

			snapshotNormalizeVersion(doc, version.String())
			b, err := json.Marshal(doc)
			if err != nil {
				t.Fatalf("json.Marshal(%s): %v", tc.wallet, err)
			}
			gotHash := sha256.Sum256(b)
			gotHex := hex.EncodeToString(gotHash[:])
			if gotHex != tc.wantHash {
				t.Fatalf("scene snapshot hash mismatch:\n got=%s\nwant=%s", gotHex, tc.wantHash)
			}
		})
	}
}

func snapshotNormalizeVersion(doc *PlateDocument, versionText string) {
	for si := range doc.Scenes {
		for li := range doc.Scenes[si].Layers {
			for pi := range doc.Scenes[si].Layers[li].Primitives {
				snapshotNormalizePrimitive(&doc.Scenes[si].Layers[li].Primitives[pi], versionText)
			}
		}
	}
}

func snapshotNormalizePrimitive(p *ScenePrimitive, versionText string) {
	if p.Kind == PrimitiveText && p.Text == versionText {
		p.Text = "<VERSION>"
	}
	for i := range p.Children {
		snapshotNormalizePrimitive(&p.Children[i], versionText)
	}
}
