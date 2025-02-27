package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"seedetcher.com/print"
)

func main() {
	mnemonic := flag.String("mnemonic", "", "12-word mnemonic phrase")
	output := flag.String("o", "./plates", "Output directory")
	paperSize := flag.String("papersize", "A4", "Paper size (A4 or Letter)")
	flag.Parse()

	if *mnemonic == "" {
		fmt.Println("Error: Mnemonic is required")
		flag.Usage()
		os.Exit(1)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(*output, 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Generate PDF for back side (simplified to always render back plate)
	filename := fmt.Sprintf("plate-side-back.pdf")
	file, err := os.Create(filepath.Join(*output, filename))
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

	fmt.Printf("Generated %s\n", filename)
}
