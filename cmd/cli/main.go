package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	// Validate mnemonic length (based on print.go's expectation of 24 words, but we'll allow 12 too)
	mnemonicWords := strings.Fields(*mnemonic)
	if len(mnemonicWords) != 12 && len(mnemonicWords) != 24 {
		fmt.Printf("Error: Mnemonic must be 12 or 24 words, got %d\n", len(mnemonicWords))
		os.Exit(1)
	}

	// Create output directory
	if err := os.MkdirAll(*output, 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Generate PDF
	filename := "plate-side-back.pdf"
	filepath := filepath.Join(*output, filename)
	file, err := os.Create(filepath)
	if err != nil {
		fmt.Printf("Error creating PDF file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	paper := print.PaperSize(*paperSize)
	err = print.PrintPDF(file, *mnemonic, paper)
	if err != nil {
		fmt.Printf("Error generating PDF: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated %s\n", filepath)
}
