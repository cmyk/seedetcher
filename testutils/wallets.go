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
		},
		Descriptor: "wpkh([7d10e19c/84h/1h/0h]tpubDDc8Aqia8wM4wePyxmwGsHaeVy3o5a1eazxyii8B2YceajqRtuVDvDUL3BCQXqM5pXbFkUozTX3SXFc8Sc3RdGEjfPcJRe6NgVREYvVztuX/<0;1>/*)#crv0xrff",
	},
	"singlesig-nested-p2sh-p2wpkh": {
		Name: "singlesig-nested-p2sh-p2wpkh",
		Mnemonics: []string{
			"cash zoo picture text skill steel dragon remove imitate fatal close train recipe april extra void obey sell train chaos noble rice typical below",
		},
		Descriptor: "sh(wpkh([7d10e19c/49h/1h/0h]tpubDDc8Aqia8wM4wePyxmwGsHaeVy3o5a1eazxyii8B2YceajqRtuVDvDUL3BCQXqM5pXbFkUozTX3SXFc8Sc3RdGEjfPcJRe6NgVREYvVztuX/<0;1>/*))",
	},
	"singlesig-longwords": {
		Name: "singlesig-longwords",
		Mnemonics: []string{
			"abstract accident acoustic announce artefact attitude bachelor broccoli business category champion cinnamon congress consider convince cupboard daughter december decorate decrease describe dinosaur disagree begin",
		},
		Descriptor: "",
	},
	"seed-12": {
		Name: "seed-12",
		Mnemonics: []string{
			"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
		},
		Descriptor: "",
	},
	"seed-15": {
		Name: "seed-15",
		Mnemonics: []string{
			"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon address",
		},
		Descriptor: "",
	},
	"seed-18": {
		Name: "seed-18",
		Mnemonics: []string{
			"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon agent",
		},
		Descriptor: "",
	},
	"seed-21": {
		Name: "seed-21",
		Mnemonics: []string{
			"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon admit",
		},
		Descriptor: "",
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
	"multisig-mainnet-2of3": {
		Name: "multisig-mainnet-2of3",
		Mnemonics: []string{
			"truly mouse crystal game narrow tent exclude silver bench price sail various cereal deny wife manual dish also trick refuse trial salute harvest fat",
			"output wife day wrap office depend reduce mention lemon always proof body unit arrow wisdom clock because bar first decorate novel elbow curve split",
			"retreat lab leg hammer turkey affair actor raven resist dose advance pretty vague choice tube credit catalog secret usage bean album detect empty drip",
		},
		Descriptor: "wsh(sortedmulti(2,[9b36c8e8/48h/0h/0h/2h]xpub6EyxtxsfiKHufP3QywbagVzs9q5vQdxUeoBET2mwxgNihDkCsdoBAvP4A1kKfEMSre4mugaYLmm9RZhmWMPRP7nmYTLLYVp9scbjHfHEaXp/<0;1>/*,[3a40e049/48h/0h/0h/2h]xpub6EJTfe6udAtK4hUdKWBnoTaNTwNKhcmaHg6u2en5XqUo6zwF7FGqwokfuJrVQorQPjzZiS4u5rfgJ5u1VJENBLPVvj89kK8oQeqmF573uyw/<0;1>/*,[a5972a4e/48h/0h/0h/2h]xpub6E52T6BkCNLgK1nUJa5ewQ3JgHPpGmZNqf7QyKMsvpZMjFhiNqvgN5GHMJHsbt839JBxpLe8fP9nfUerBy3dpJSwdDHLvzTo8tDnYrFJnb1/<0;1>/*))",
	},
	"multisig-nested-2of3": {
		Name: "multisig-nested-2of3",
		Mnemonics: []string{
			"truly mouse crystal game narrow tent exclude silver bench price sail various cereal deny wife manual dish also trick refuse trial salute harvest fat",
			"output wife day wrap office depend reduce mention lemon always proof body unit arrow wisdom clock because bar first decorate novel elbow curve split",
			"retreat lab leg hammer turkey affair actor raven resist dose advance pretty vague choice tube credit catalog secret usage bean album detect empty drip",
		},
		Descriptor: "sh(wsh(sortedmulti(2,[3a40e049/48h/1h/0h/2h]tpubDEjEpeK6KLHjAQ5cKbxZncFjR6jXUqQfiLpDyKtpNJrJCsqj2LeiMjRUjwduWPUnSngsTjEs58WJX5rnMkLCMdKb8Eed3z32g5d99Nfi6Wz/<0;1>/*,[9b36c8e8/48h/1h/0h/2h]tpubDEWg8TmjbEhCdj3zbYytQrPtS141uPxN2m3msBJokZCDawHFvWG78mmithyEN92jez6588ATkBE2pkPNAct9MmPx94GahYqEa8Xq7j2eoPw/<0;1>/*,[a5972a4e/48h/1h/0h/2h]tpubDDwEPDnfMxf2tuGMrLoQmdY3L8xmoTtUVBkHkagPq1xLvNs6CfXui74mYtauBd8eKXkSQo6dQyzh7UtvnmsppyuuKqXMjvRCqfDyA8DvcHb/<0;1>/*)))",
	},
	"multisig-3of5": {
		Name: "multisig-3of5",
		Mnemonics: []string{
			"cat prefer album ancient injury video detect since place evidence cement ice sign avoid behind snake enrich view lab comfort twist bless opera luggage",
			"sight wise ski enough clinic salon rocket around also sleep garment venue rain float practice erosion property panel bright ridge patrol bind arrest decline",
			"reject bronze turn sniff solar scorpion hunt spatial soda animal kit cart horror divide fan bargain sport chronic canvas height odor mass edit phrase",
			"giraffe sort priority fluid glove session sadness dolphin hockey piece plug work public slim provide payment dragon toddler clinic lecture blue wheat flight resemble",
			"glory benefit idle repair high bleak garbage excite dial canvas divide little flower fly genre mountain omit father squeeze blanket leave sweet update magic",
		},
		Descriptor: "wsh(sortedmulti(3,[8de74615/48h/1h/0h/2h]tpubDEGTwBTu9vFk2BrjkFJwB6rJzrQv1Kx4Gh48XqvEs8brQYJiEwyk8cExRZxyt4yX2AktEda37tF6PuVG2qrBqJ9txpniBvbWQrh6FjUSNxj/<0;1>/*,[f8cb7260/48h/1h/0h/2h]tpubDEtiTPBLv4XbofTgWQSfJpe2sh7YY9nXC4smGTwsZMrYQqfVVYa9vhnCMPMH12xxyrH7L7A8NaQ1GLX5T2HLTGbVYwRGmR7cGF6nFHfBUUa/<0;1>/*,[cc5e307c/48h/1h/0h/2h]tpubDF2CFuGD3Tr9xjPJQqfARBY7ACFZyx5P84HBvw6bJdr4MjHrBjky2BzKFRarjbB6VENW7CZn3X92eyeHcHoNG75tjMWJAxVepveXuF3hNod/<0;1>/*,[11544443/48h/1h/0h/2h]tpubDDzFJfPfv9tz2nCLQ7gViGF8zbC9yiyFHoGCgWZywHv3GVrqDZnrk4C5MZoyQE1q2yHfozPqRc4HiQ7zBDhHGQPs1XmhRVEVSzDZKWTuQt6/<0;1>/*,[fe45e5a2/48h/1h/0h/2h]tpubDFE3XKXzf7aWrHYtcHWA2sZJGQLiNKzupvxppuUXyBkhoM4q57gGeJezP6ckgAfP99u7AMewH4b41VhGmJ7ksG24HTcLM2Ty6jRqzubRsoY/<0;1>/*))",
	},
	"multisig-2of4": {
		Name: "multisig-2of4",
		Mnemonics: []string{
			"cat prefer album ancient injury video detect since place evidence cement ice sign avoid behind snake enrich view lab comfort twist bless opera luggage",
			"sight wise ski enough clinic salon rocket around also sleep garment venue rain float practice erosion property panel bright ridge patrol bind arrest decline",
			"reject bronze turn sniff solar scorpion hunt spatial soda animal kit cart horror divide fan bargain sport chronic canvas height odor mass edit phrase",
			"giraffe sort priority fluid glove session sadness dolphin hockey piece plug work public slim provide payment dragon toddler clinic lecture blue wheat flight resemble",
		},
		Descriptor: "wsh(sortedmulti(2,[8de74615/48h/1h/0h/2h]tpubDEGTwBTu9vFk2BrjkFJwB6rJzrQv1Kx4Gh48XqvEs8brQYJiEwyk8cExRZxyt4yX2AktEda37tF6PuVG2qrBqJ9txpniBvbWQrh6FjUSNxj/<0;1>/*,[f8cb7260/48h/1h/0h/2h]tpubDEtiTPBLv4XbofTgWQSfJpe2sh7YY9nXC4smGTwsZMrYQqfVVYa9vhnCMPMH12xxyrH7L7A8NaQ1GLX5T2HLTGbVYwRGmR7cGF6nFHfBUUa/<0;1>/*,[cc5e307c/48h/1h/0h/2h]tpubDF2CFuGD3Tr9xjPJQqfARBY7ACFZyx5P84HBvw6bJdr4MjHrBjky2BzKFRarjbB6VENW7CZn3X92eyeHcHoNG75tjMWJAxVepveXuF3hNod/<0;1>/*,[11544443/48h/1h/0h/2h]tpubDDzFJfPfv9tz2nCLQ7gViGF8zbC9yiyFHoGCgWZywHv3GVrqDZnrk4C5MZoyQE1q2yHfozPqRc4HiQ7zBDhHGQPs1XmhRVEVSzDZKWTuQt6/<0;1>/*))",
	},
	"multisig-2of2": {
		Name: "multisig-2of2",
		Mnemonics: []string{
			"truly mouse crystal game narrow tent exclude silver bench price sail various cereal deny wife manual dish also trick refuse trial salute harvest fat",
			"output wife day wrap office depend reduce mention lemon always proof body unit arrow wisdom clock because bar first decorate novel elbow curve split",
		},
		Descriptor: "wsh(sortedmulti(2,[3a40e049/48h/1h/0h/2h]tpubDEjEpeK6KLHjAQ5cKbxZncFjR6jXUqQfiLpDyKtpNJrJCsqj2LeiMjRUjwduWPUnSngsTjEs58WJX5rnMkLCMdKb8Eed3z32g5d99Nfi6Wz/<0;1>/*,[9b36c8e8/48h/1h/0h/2h]tpubDEWg8TmjbEhCdj3zbYytQrPtS141uPxN2m3msBJokZCDawHFvWG78mmithyEN92jez6588ATkBE2pkPNAct9MmPx94GahYqEa8Xq7j2eoPw/<0;1>/*))",
	},
	"multisig-3of4": {
		Name: "multisig-3of4",
		Mnemonics: []string{
			"cat prefer album ancient injury video detect since place evidence cement ice sign avoid behind snake enrich view lab comfort twist bless opera luggage",
			"sight wise ski enough clinic salon rocket around also sleep garment venue rain float practice erosion property panel bright ridge patrol bind arrest decline",
			"reject bronze turn sniff solar scorpion hunt spatial soda animal kit cart horror divide fan bargain sport chronic canvas height odor mass edit phrase",
			"giraffe sort priority fluid glove session sadness dolphin hockey piece plug work public slim provide payment dragon toddler clinic lecture blue wheat flight resemble",
		},
		Descriptor: "wsh(sortedmulti(3,[8de74615/48h/1h/0h/2h]tpubDEGTwBTu9vFk2BrjkFJwB6rJzrQv1Kx4Gh48XqvEs8brQYJiEwyk8cExRZxyt4yX2AktEda37tF6PuVG2qrBqJ9txpniBvbWQrh6FjUSNxj/<0;1>/*,[f8cb7260/48h/1h/0h/2h]tpubDEtiTPBLv4XbofTgWQSfJpe2sh7YY9nXC4smGTwsZMrYQqfVVYa9vhnCMPMH12xxyrH7L7A8NaQ1GLX5T2HLTGbVYwRGmR7cGF6nFHfBUUa/<0;1>/*,[cc5e307c/48h/1h/0h/2h]tpubDF2CFuGD3Tr9xjPJQqfARBY7ACFZyx5P84HBvw6bJdr4MjHrBjky2BzKFRarjbB6VENW7CZn3X92eyeHcHoNG75tjMWJAxVepveXuF3hNod/<0;1>/*,[11544443/48h/1h/0h/2h]tpubDDzFJfPfv9tz2nCLQ7gViGF8zbC9yiyFHoGCgWZywHv3GVrqDZnrk4C5MZoyQE1q2yHfozPqRc4HiQ7zBDhHGQPs1XmhRVEVSzDZKWTuQt6/<0;1>/*))",
	},
	"multisig-4of7": {
		Name: "multisig-4of7",
		Mnemonics: []string{
			"quote film dawn robust settle trust lesson day farm list silk analyst carbon mercy immense bounce extra raccoon identify audit gap dragon wife where",
			"problem stable pyramid toy unfold kit breeze jacket web chase lawsuit stomach mercy stadium run agent crash crawl rely master secret town aisle saddle",
			"endorse price dutch receive van ensure stadium display legend route diamond security torch cruise fix blanket wrap dwarf diamond enter galaxy side pelican gasp",
			"jeans mother buddy avocado attack broken shoot retire shift nice duty piece parade sudden auction cute guilt engine record various crumble curve answer mistake",
			"improve shallow final orbit scatter about conduct bulb price method jar regular father tiny baby napkin obtain palace motion defy arrange drip little animal",
			"curve exile mistake bread skin social iron trend mimic interest impact notable tray lunch flavor undo donate change hero give rebuild trumpet awake duck",
			"box spring sugar short ice chalk filter cute ripple crumble divert purity swarm memory chapter strong sketch script glad wage crawl actual bamboo flash",
		},
		Descriptor: "wsh(sortedmulti(4,[7c39a50c/48h/1h/0h/2h]tpubDFNm755hHqXY5tj7c9EzdP79jxbyaA87pxrbXZTPsdM1kxz5Pm3XTsWSHS85Jjb6LAQvSWYRUGuUjasZdWraMzgJppUFYCFMQwBtHm3RuJs/<0;1>/*,[98744c48/48h/1h/0h/2h]tpubDFkSMkWnjxRjvvB4yPbJtpT29nXqh7uMYe5XYCjJ1LcMoQuBvh8dFkhFzNvtq7wPmoESjjPn38KBNG6nBxENN9cGUczZQSVHjb5uhc6LoM6/<0;1>/*,[ec49c717/48h/1h/0h/2h]tpubDFi8m5ejnMjnGPiG1szxpdA3CqBJuLVu9trBV8Zr8uHsGQxQ5YPZi53QF7ptfCZSQRRQWsiLMUaqVrTKhvSFhDto8RohC9wqNYQc5mvkdQ7/<0;1>/*,[32a4aafb/48h/1h/0h/2h]tpubDExTKwT9EGr3KVkbvvdtAn64UoLKJ8SjvucJASnFSf4hm4wL99oj3tLCDtVxfe6CHjhYuYShYUM7mKkALxFThAMYjEHcYsYwCo9vtrD31eP/<0;1>/*,[74f2fd01/48h/1h/0h/2h]tpubDFEdRbQaCn4D7xuWhSnYZn1ibDAKcxiXrnasaZ9Zx82DA1Dw97HXGgWkFAUYD8w174mZcN3xNu9Qr19zURmSRRm2Fcg4SCnfRt7kwYyBCZW/<0;1>/*,[2bda0361/48h/1h/0h/2h]tpubDFLQaNaXwuKef6XKVfq1kiuNFLMRzeKD3gERKyzAtoQDTFV3f3wu5ihpvyusaLsVTbGxaPhxpWhD5YHcJhuFZ2f4eKuB3vGXcPeqSCohu8j/<0;1>/*,[d228c784/48h/1h/0h/2h]tpubDEcvH31exWqVNgqTFteAHb98aLt9yADWMp9iEk1ZSq2h69ogbUADnVcwwQzajjPEiUqRfmyD4MTWKdmVhX8SfJiwF6UimavDAXN7Siai1gF/<0;1>/*))",
	},
	"multisig-5of7": {
		Name: "multisig-5of7",
		Mnemonics: []string{
			"quote film dawn robust settle trust lesson day farm list silk analyst carbon mercy immense bounce extra raccoon identify audit gap dragon wife where",
			"problem stable pyramid toy unfold kit breeze jacket web chase lawsuit stomach mercy stadium run agent crash crawl rely master secret town aisle saddle",
			"endorse price dutch receive van ensure stadium display legend route diamond security torch cruise fix blanket wrap dwarf diamond enter galaxy side pelican gasp",
			"jeans mother buddy avocado attack broken shoot retire shift nice duty piece parade sudden auction cute guilt engine record various crumble curve answer mistake",
			"improve shallow final orbit scatter about conduct bulb price method jar regular father tiny baby napkin obtain palace motion defy arrange drip little animal",
			"curve exile mistake bread skin social iron trend mimic interest impact notable tray lunch flavor undo donate change hero give rebuild trumpet awake duck",
			"box spring sugar short ice chalk filter cute ripple crumble divert purity swarm memory chapter strong sketch script glad wage crawl actual bamboo flash",
		},
		Descriptor: "wsh(sortedmulti(5,[7c39a50c/48h/1h/0h/2h]tpubDFNm755hHqXY5tj7c9EzdP79jxbyaA87pxrbXZTPsdM1kxz5Pm3XTsWSHS85Jjb6LAQvSWYRUGuUjasZdWraMzgJppUFYCFMQwBtHm3RuJs/<0;1>/*,[98744c48/48h/1h/0h/2h]tpubDFkSMkWnjxRjvvB4yPbJtpT29nXqh7uMYe5XYCjJ1LcMoQuBvh8dFkhFzNvtq7wPmoESjjPn38KBNG6nBxENN9cGUczZQSVHjb5uhc6LoM6/<0;1>/*,[ec49c717/48h/1h/0h/2h]tpubDFi8m5ejnMjnGPiG1szxpdA3CqBJuLVu9trBV8Zr8uHsGQxQ5YPZi53QF7ptfCZSQRRQWsiLMUaqVrTKhvSFhDto8RohC9wqNYQc5mvkdQ7/<0;1>/*,[32a4aafb/48h/1h/0h/2h]tpubDExTKwT9EGr3KVkbvvdtAn64UoLKJ8SjvucJASnFSf4hm4wL99oj3tLCDtVxfe6CHjhYuYShYUM7mKkALxFThAMYjEHcYsYwCo9vtrD31eP/<0;1>/*,[74f2fd01/48h/1h/0h/2h]tpubDFEdRbQaCn4D7xuWhSnYZn1ibDAKcxiXrnasaZ9Zx82DA1Dw97HXGgWkFAUYD8w174mZcN3xNu9Qr19zURmSRRm2Fcg4SCnfRt7kwYyBCZW/<0;1>/*,[2bda0361/48h/1h/0h/2h]tpubDFLQaNaXwuKef6XKVfq1kiuNFLMRzeKD3gERKyzAtoQDTFV3f3wu5ihpvyusaLsVTbGxaPhxpWhD5YHcJhuFZ2f4eKuB3vGXcPeqSCohu8j/<0;1>/*,[d228c784/48h/1h/0h/2h]tpubDEcvH31exWqVNgqTFteAHb98aLt9yADWMp9iEk1ZSq2h69ogbUADnVcwwQzajjPEiUqRfmyD4MTWKdmVhX8SfJiwF6UimavDAXN7Siai1gF/<0;1>/*))",
	},
	"multisig-7of10": {
		Name: "multisig-7of10",
		Mnemonics: []string{
			"quote film dawn robust settle trust lesson day farm list silk analyst carbon mercy immense bounce extra raccoon identify audit gap dragon wife where",
			"problem stable pyramid toy unfold kit breeze jacket web chase lawsuit stomach mercy stadium run agent crash crawl rely master secret town aisle saddle",
			"endorse price dutch receive van ensure stadium display legend route diamond security torch cruise fix blanket wrap dwarf diamond enter galaxy side pelican gasp",
			"jeans mother buddy avocado attack broken shoot retire shift nice duty piece parade sudden auction cute guilt engine record various crumble curve answer mistake",
			"improve shallow final orbit scatter about conduct bulb price method jar regular father tiny baby napkin obtain palace motion defy arrange drip little animal",
			"curve exile mistake bread skin social iron trend mimic interest impact notable tray lunch flavor undo donate change hero give rebuild trumpet awake duck",
			"box spring sugar short ice chalk filter cute ripple crumble divert purity swarm memory chapter strong sketch script glad wage crawl actual bamboo flash",
			"bronze radio include regret sock lava wolf replace people hill grape ability deny fabric skin response choose wood auction midnight script another organ useful",
			"suffer lake approve churn during occur narrow provide jungle open fold olympic maid because glimpse wealth reduce inquiry boost lottery denial shiver laundry frown",
			"tray rent regular fine future globe put exhibit catch struggle elite crush cement title vibrant coral danger just catch glare another gate jump noise",
		},
		Descriptor: "wsh(sortedmulti(7,[7c39a50c/48h/1h/0h/2h]tpubDFNm755hHqXY5tj7c9EzdP79jxbyaA87pxrbXZTPsdM1kxz5Pm3XTsWSHS85Jjb6LAQvSWYRUGuUjasZdWraMzgJppUFYCFMQwBtHm3RuJs/<0;1>/*,[98744c48/48h/1h/0h/2h]tpubDFkSMkWnjxRjvvB4yPbJtpT29nXqh7uMYe5XYCjJ1LcMoQuBvh8dFkhFzNvtq7wPmoESjjPn38KBNG6nBxENN9cGUczZQSVHjb5uhc6LoM6/<0;1>/*,[ec49c717/48h/1h/0h/2h]tpubDFi8m5ejnMjnGPiG1szxpdA3CqBJuLVu9trBV8Zr8uHsGQxQ5YPZi53QF7ptfCZSQRRQWsiLMUaqVrTKhvSFhDto8RohC9wqNYQc5mvkdQ7/<0;1>/*,[32a4aafb/48h/1h/0h/2h]tpubDExTKwT9EGr3KVkbvvdtAn64UoLKJ8SjvucJASnFSf4hm4wL99oj3tLCDtVxfe6CHjhYuYShYUM7mKkALxFThAMYjEHcYsYwCo9vtrD31eP/<0;1>/*,[74f2fd01/48h/1h/0h/2h]tpubDFEdRbQaCn4D7xuWhSnYZn1ibDAKcxiXrnasaZ9Zx82DA1Dw97HXGgWkFAUYD8w174mZcN3xNu9Qr19zURmSRRm2Fcg4SCnfRt7kwYyBCZW/<0;1>/*,[2bda0361/48h/1h/0h/2h]tpubDFLQaNaXwuKef6XKVfq1kiuNFLMRzeKD3gERKyzAtoQDTFV3f3wu5ihpvyusaLsVTbGxaPhxpWhD5YHcJhuFZ2f4eKuB3vGXcPeqSCohu8j/<0;1>/*,[d228c784/48h/1h/0h/2h]tpubDEcvH31exWqVNgqTFteAHb98aLt9yADWMp9iEk1ZSq2h69ogbUADnVcwwQzajjPEiUqRfmyD4MTWKdmVhX8SfJiwF6UimavDAXN7Siai1gF/<0;1>/*,[4f9f152c/48h/1h/0h/2h]tpubDF2UdW9N1BcebkqAMTYvs9LxPta6znNBxTFYR2DKUjLivv51xkMU1VQJEsvCMXyFSZZBkVtLrJaZpeGQpjJhCuYs5exF9tG3hq52TELLigM/<0;1>/*,[5236d7fc/48h/1h/0h/2h]tpubDFCj736R62FxQhdSWzQ7J8ue1JcgszzpjZ7hFT3sx1CT6hDdAjVXRXAwqkc2iNL3REz5Ews6prz7LNwow7F41ebLzoyVQDWKgoVrzZhYqUB/<0;1>/*,[e0e2c342/48h/1h/0h/2h]tpubDEUnLeuvCmm17R2NpWpFzAk7DVkUgZV2KuyEAp9oZFfVntbyhyf3QYpqiW7gv14k3nA6W116owMjT6RcfPNA7dKprKAnQ8ie8qFNtke99Wa/<0;1>/*))",
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
