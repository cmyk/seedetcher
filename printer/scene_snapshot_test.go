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
			wantHash:    "c8fabb706a6912bc263bf1413e05be9a7a5ade3de7f28b11f68c2f6a4b0b598b",
		},
		{
			wallet:      "multisig-mainnet-2of3",
			compact2of3: false,
			wantScenes:  6,
			wantHash:    "0c825d956b5523a068dc29ad18368b6546b7361193938f196e5623e219c63a99",
		},
		{
			wallet:      "multisig-3of5",
			compact2of3: false,
			wantScenes:  10,
			wantHash:    "382a077ff054d9c25e4dbf683e93629695e5ef0c0ab215eeca86655de536e076",
		},
		{
			wallet:      "multisig",
			compact2of3: true,
			wantScenes:  3,
			wantHash:    "2d81801128c2378e43ba4054a7bd633586c802d23c66aa853e833dd3e032daf2",
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
