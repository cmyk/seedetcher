package testutils

import (
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/nonstandard"
)

type WalletConfig struct {
	Name       string
	Mnemonics  []string
	Descriptor string
}

var WalletConfigs = map[string]WalletConfig{
	"singlesig": {
		Name: "singlesig",
		Mnemonics: []string{
			"cash zoo picture text skill steel dragon remove imitate fatal close train recipe april extra void obey sell train chaos noble rice typical below",
			"cash zoo picture text skill steel dragon remove imitate fatal close train recipe april extra void obey sell train chaos noble rice typical below",
			"cash zoo picture text skill steel dragon remove imitate fatal close train recipe april extra void obey sell train chaos noble rice typical below",
		},
		Descriptor: "wpkh([7d10e19c/84h/1h/0h]tpubDDc8Aqia8wM4wePyxmwGsHaeVy3o5a1eazxyii8B2YceajqRtuVDvDUL3BCQXqM5pXbFkUozTX3SXFc8Sc3RdGEjfPcJRe6NgVREYvVztuX/<0;1>/*)#crv0xrff",
	},
	"multisig": {
		Name: "multisig",
		Mnemonics: []string{
			"truly mouse crystal game narrow tent exclude silver bench price sail various cereal deny wife manual dish also trick refuse trial salute harvest fat",
			"output wife day wrap office depend reduce mention lemon always proof body unit arrow wisdom clock because bar first decorate novel elbow curve split",
			"retreat lab leg hammer turkey affair actor raven resist dose advance pretty vague choice tube credit catalog secret usage bean album detect empty drip",
		},
		Descriptor: "wsh(sortedmulti(2,[3a40e049/48h/1h/0h/2h]tpubDEjEpeK6KLHjAQ5cKbxZncFjR6jXUqQfiLpDyKtpNJrJCsqj2LeiMjRUjwduWPUnSngsTjEs58WJX5rnMkLCMdKb8Eed3z32g5d99Nfi6Wz/<0;1>/*,[9b36c8e8/48h/1h/0h/2h]tpubDEWg8TmjbEhCdj3zbYytQrPtS141uPxN2m3msBJokZCDawHFvWG78mmithyEN92jez6588ATkBE2pkPNAct9MmPx94GahYqEa8Xq7j2eoPw/<0;1>/*,[a5972a4e/48h/1h/0h/2h]tpubDDwEPDnfMxf2tuGMrLoQmdY3L8xmoTtUVBkHkagPq1xLvNs6CfXui74mYtauBd8eKXkSQo6dQyzh7UtvnmsppyuuKqXMjvRCqfDyA8DvcHb/<0;1>/*))#vhd8qaqn",
	},
}

func ParseWallet(config WalletConfig, mnemonicOverride, descriptorOverride string) (mnemonics []bip39.Mnemonic, desc *urtypes.OutputDescriptor, err error) {
	mnemonics = make([]bip39.Mnemonic, len(config.Mnemonics))
	for i, m := range config.Mnemonics {
		if mnemonicOverride != "" {
			m = mnemonicOverride
		}
		mnem, err := bip39.ParseMnemonic(m)
		if err != nil {
			return nil, nil, err
		}
		mnemonics[i] = mnem
	}
	descStr := config.Descriptor
	if descriptorOverride != "" {
		descStr = descriptorOverride
	}
	if descStr != "" {
		d, err := nonstandard.OutputDescriptor([]byte(descStr))
		if err != nil {
			return nil, nil, err
		}
		return mnemonics, &d, nil
	}
	return mnemonics, nil, nil
}
