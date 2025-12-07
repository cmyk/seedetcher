package main

import (
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"seedetcher.com/printer"
	"seedetcher.com/testutils"
)

func main() {
	f := testutils.DefineFlags()
	flag.Parse()

	usr, err := user.Current()
	if err != nil {
		fmt.Printf("Error getting current user: %v\n", err)
		os.Exit(1)
	}
	outputDir := strings.Replace(f.Output, "~", usr.HomeDir, 1)
	pngDir := ""
	if f.BitmapDir != "" {
		pngDir = strings.Replace(f.BitmapDir, "~", usr.HomeDir, 1)
	}
	if f.Verbose {
		fmt.Println("Expanded output directory to:", outputDir)
		if pngDir != "" {
			fmt.Println("Expanded PNG directory to:", pngDir)
		}
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}
	if pngDir != "" {
		if err := os.MkdirAll(pngDir, 0755); err != nil {
			fmt.Printf("Error creating PNG output directory: %v\n", err)
			os.Exit(1)
		}
	}

	config, ok := testutils.WalletConfigs[f.WalletType]
	if !ok {
		fmt.Printf("Invalid wallet type. Use 'singlesig' or 'multisig'\n")
		os.Exit(1)
	}
	mnemonics, desc, err := testutils.ParseWallet(config, f.Mnemonic, f.Descriptor)
	if err != nil {
		fmt.Printf("Error parsing wallet: %v\n", err)
		os.Exit(1)
	}
	if f.Verbose {
		fmt.Printf("Processing %s wallet with descriptor: %v\n", config.Name, desc != nil)
	}

	printer.SetDescriptorQRSize(f.DescQRMM)
	file, err := os.Create(filepath.Join(outputDir, config.Name+".pdf"))
	if err != nil {
		fmt.Printf("Error creating PDF file %s: %v\n", config.Name+".pdf", err)
		os.Exit(1)
	}
	defer file.Close()

	seedPaths, descPaths, tempDir, err := printer.CreatePlates(file, mnemonics, desc, 0, false, false)
	if err != nil {
		fmt.Printf("Error generating plates: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempDir) // Move cleanup here
	if err := printer.CreatePageLayout(file, tempDir, printer.PaperSize(f.PaperSize), seedPaths, descPaths); err != nil {
		fmt.Printf("Error merging PDF: %v\n", err)
		os.Exit(1)
	}
	if f.Verbose {
		fmt.Printf("Generated %s.pdf\n", config.Name)
	}
	fmt.Println("PDF generation completed")

	if pngDir != "" {
		opts := printer.RasterOptions{
			DPI:    float64(f.DPI),
			Mirror: f.Mirror,
			Invert: f.Invert,
		}
		seedImgs, descImgs, err := printer.CreatePlateBitmaps(mnemonics, desc, 0, opts)
		if err != nil {
			fmt.Printf("Error creating PNG plates: %v\n", err)
			os.Exit(1)
		}
		for i, img := range seedImgs {
			path := filepath.Join(pngDir, fmt.Sprintf("%s_seed_%d.png", config.Name, i+1))
			if err := printer.SavePNG(path, img); err != nil {
				fmt.Printf("Error writing %s: %v\n", path, err)
				os.Exit(1)
			}
		}
		if descImgs != nil {
			for i, img := range descImgs {
				if img == nil {
					continue
				}
				path := filepath.Join(pngDir, fmt.Sprintf("%s_desc_%d.png", config.Name, i+1))
				if err := printer.SavePNG(path, img); err != nil {
					fmt.Printf("Error writing %s: %v\n", path, err)
					os.Exit(1)
				}
			}
		}
		if f.Verbose {
			fmt.Printf("Generated PNG plates at %s (mirror=%v invert=%v dpi=%d)\n", pngDir, f.Mirror, f.Invert, f.DPI)
		}
	}
}

type walletConfig struct {
	name       string
	mnemonics  []string
	descriptor string
	outputFile string
}
