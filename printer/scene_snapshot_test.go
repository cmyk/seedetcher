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
		wantScenes int
		wantHash   string
	}{
		{
			wallet:     "singlesig",
			wantScenes: 1,
			wantHash:   "17e9fd052839062ee7b7c2ecc276cab80971ec6d06005de6d38a290bdb0a854d",
		},
		{
			wallet:     "multisig-mainnet-2of3",
			wantScenes: 6,
			wantHash:   "3c7656cfc0145a4cd7f1c7cbe9b3f8a0ab2c432f50c9faccf10f340f2bdb2b1e",
		},
		{
			wallet:     "multisig-3of5",
			wantScenes: 10,
			wantHash:   "2878e41d532630ee0106da75d3300aad4a7e5737fb31dfb5b085ed4dfc529f93",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.wallet, func(t *testing.T) {
			SetWalletLabel(DefaultWalletLabel)
			SetDescriptorQRSize(80.0)
			SetCompactDescriptor2of3Enabled(false)

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
