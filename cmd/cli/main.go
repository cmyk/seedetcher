package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"seedetcher.com/bip39"
	"seedetcher.com/print"
)

func main() {
	mnemonic := flag.String("mnemonic", "", "12- or 24-word mnemonic phrase (space-separated)")
	output := flag.String("o", "./plates", "Output directory")
	paperSize := flag.String("papersize", "A4", "Paper size (A4 or Letter)")
	flag.Parse()

	if *mnemonic == "" {
		fmt.Println("Error: Mnemonic is required")
		flag.Usage()
		os.Exit(1)
	}

	// Parse and validate mnemonic
	mnemonicWords := strings.Fields(*mnemonic)
	if len(mnemonicWords) != 12 && len(mnemonicWords) != 24 {
		fmt.Printf("Error: Mnemonic must be 12 or 24 words, got %d\n", len(mnemonicWords))
		os.Exit(1)
	}
	m := make(bip39.Mnemonic, len(mnemonicWords))
	for i, w := range mnemonicWords {
		word, ok := bip39.ClosestWord(w)
		if !ok {
			fmt.Printf("Error: Invalid word at position %d: %s\n", i+1, w)
			os.Exit(1)
		}
		m[i] = word
	}
	if !m.Valid() {
		fmt.Println("Error: Invalid mnemonic (checksum failed)")
		os.Exit(1)
	}
	fmt.Println("Mnemonic validated successfully")

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(*output, 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Generate PDF with validated mnemonic, no descriptor, default keyIdx 0, and paper size
	outputPath := filepath.Join(*output, "plate-side-back.pdf")
	file, err := os.Create(outputPath)
	if err != nil {
		fmt.Printf("Error creating PDF file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	ps := print.PaperSize(*paperSize)
	if err := print.PrintPDF(file, m, nil, 0, ps); err != nil { // Pass mnemonic, nil descriptor, keyIdx 0, paperSize
		fmt.Printf("Error generating PDF: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Generated %s\n", outputPath)
}
