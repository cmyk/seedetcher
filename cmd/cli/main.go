package main

import (
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/nonstandard"
	"seedetcher.com/printer"
)

func main() {
	mnemonic := flag.String("mnemonic", "", "12- or 24-word mnemonic phrase (space-separated)")
	descriptor := flag.String("descriptor", "", "Raw descriptor string (e.g., 'wsh(sortedmulti(2,[fingerprint/path]tpub...))')")
	output := flag.String("o", "/home/cmyk/PDF", "Output directory")
	paperSize := flag.String("papersize", "A4", "Paper size (A4 or Letter)")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	walletType := flag.String("w", "multisig", "Wallet type (single or multisig)")
	flag.Parse()

	usr, err := user.Current()
	if err != nil {
		fmt.Printf("Error getting current user: %v\n", err)
		os.Exit(1)
	}
	outputDir := strings.Replace(*output, "~", usr.HomeDir, 1)
	if *verbose {
		fmt.Println("Expanded output directory to:", outputDir)
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	walletConfigsMap := map[string]walletConfig{
		"singlesig": {
			name:       "singlesig",
			mnemonics:  []string{"cash zoo picture text skill steel dragon remove imitate fatal close train recipe april extra void obey sell train chaos noble rice typical below", "cash zoo picture text skill steel dragon remove imitate fatal close train recipe april extra void obey sell train chaos noble rice typical below", "cash zoo picture text skill steel dragon remove imitate fatal close train recipe april extra void obey sell train chaos noble rice typical below"},
			descriptor: "wpkh([7d10e19c/84h/1h/0h]tpubDDc8Aqia8wM4wePyxmwGsHaeVy3o5a1eazxyii8B2YceajqRtuVDvDUL3BCQXqM5pXbFkUozTX3SXFc8Sc3RdGEjfPcJRe6NgVREYvVztuX/<0;1>/*)#crv0xrff",
			outputFile: filepath.Join(outputDir, "singlesig.pdf"),
		},
		"multisig": {
			name:       "multisig",
			mnemonics:  []string{"truly mouse crystal game narrow tent exclude silver bench price sail various cereal deny wife manual dish also trick refuse trial salute harvest fat", "output wife day wrap office depend reduce mention lemon always proof body unit arrow wisdom clock because bar first decorate novel elbow curve split", "retreat lab leg hammer turkey affair actor raven resist dose advance pretty vague choice tube credit catalog secret usage bean album detect empty drip"},
			descriptor: "wsh(sortedmulti(2,[3a40e049/48h/1h/0h/2h]tpubDEjEpeK6KLHjAQ5cKbxZncFjR6jXUqQfiLpDyKtpNJrJCsqj2LeiMjRUjwduWPUnSngsTjEs58WJX5rnMkLCMdKb8Eed3z32g5d99Nfi6Wz/<0;1>/*,[9b36c8e8/48h/1h/0h/2h]tpubDEWg8TmjbEhCdj3zbYytQrPtS141uPxN2m3msBJokZCDawHFvWG78mmithyEN92jez6588ATkBE2pkPNAct9MmPx94GahYqEa8Xq7j2eoPw/<0;1>/*,[a5972a4e/48h/1h/0h/2h]tpubDDwEPDnfMxf2tuGMrLoQmdY3L8xmoTtUVBkHkagPq1xLvNs6CfXui74mYtauBd8eKXkSQo6dQyzh7UtvnmsppyuuKqXMjvRCqfDyA8DvcHb/<0;1>/*))#vhd8qaqn",
			outputFile: filepath.Join(outputDir, "multisig.pdf"),
		},
	}

	config, ok := walletConfigsMap[*walletType]
	if !ok {
		fmt.Printf("Invalid wallet type. Use 'singlesig' or 'multisig'\n")
		os.Exit(1)
	}
	walletConfigs := []walletConfig{config} // Wrap in slice for the loop

	mnemonicProvided := *mnemonic != ""
	for i := range walletConfigs {
		config := &walletConfigs[i]
		if mnemonicProvided {
			config.mnemonics = []string{*mnemonic, *mnemonic, *mnemonic}
			if *verbose {
				fmt.Println("Using provided mnemonic for all plates:", *mnemonic)
			}
		}
		if *descriptor != "" {
			config.descriptor = *descriptor
		}
	}

	for _, config := range walletConfigs {
		if *verbose {
			fmt.Printf("Processing %s wallet with descriptor: %s\n", config.name, config.descriptor)
		}
		var desc *urtypes.OutputDescriptor
		if config.descriptor != "" {
			if *verbose {
				fmt.Printf("Parsing %s descriptor: %s\n", config.name, config.descriptor)
			}
			d, err := nonstandard.OutputDescriptor([]byte(config.descriptor))
			if err != nil {
				fmt.Printf("Error parsing %s descriptor: %v\n", config.name, err)
				os.Exit(1)
			}
			desc = &d
			if *verbose {
				fmt.Printf("%s descriptor parsed: Type=%v, Script=%s, Keys=%d, Threshold=%d\n",
					config.name, desc.Type, desc.Script.String(), len(desc.Keys), desc.Threshold)
			}
		}

		// Parse mnemonics
		mnemonics := make([]bip39.Mnemonic, 3)
		for i, mnem := range config.mnemonics {
			mnemWords := strings.Fields(mnem)
			if len(mnemWords) != 12 && len(mnemWords) != 24 {
				fmt.Printf("Error: Mnemonic %d for %s must be 12 or 24 words, got %d\n", i+1, config.name, len(mnemWords))
				os.Exit(1)
			}
			m := make(bip39.Mnemonic, len(mnemWords))
			for j, w := range mnemWords {
				word, ok := bip39.ClosestWord(w)
				if !ok || bip39.LabelFor(word) != w {
					fmt.Printf("Error: Invalid word in %s mnemonic %d at position %d: %s\n", config.name, i+1, j+1, w)
					os.Exit(1)
				}
				m[j] = word
			}
			if !m.Valid() {
				fmt.Printf("Error: Invalid %s mnemonic %d (checksum failed)\n", config.name, i+1)
				os.Exit(1)
			}
			mnemonics[i] = m
			if *verbose {
				fmt.Printf("%s mnemonic %d validated successfully\n", config.name, i+1)
			}
		}

		// Create output file
		file, err := os.Create(config.outputFile)
		if err != nil {
			fmt.Printf("Error creating PDF file %s: %v\n", config.outputFile, err)
			os.Exit(1)
		}
		defer file.Close()

		// Generate PDF
		if err := printer.CreatePlates(file, mnemonics, desc, 0, printer.PaperSize(*paperSize), false, false); err != nil {
			fmt.Printf("Error generating PDF %s: %v\n", config.outputFile, err)
			os.Exit(1)
		}
		// Remove redundant CreatePageLayout call
		if *verbose {
			fmt.Printf("Generated %s\n", config.outputFile)
		}
	}

	fmt.Println("PDF generation completed")
}

type walletConfig struct {
	name       string
	mnemonics  []string
	descriptor string
	outputFile string
}
