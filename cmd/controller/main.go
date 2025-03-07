// command controller is the user interface for printing SeedEtcher backup plates.
// It runs on a Raspberry Pi Zero, in the same configuration as SeedSigner.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"seedetcher.com/bip39"
	"seedetcher.com/gui"
	"seedetcher.com/logutil"
	"seedetcher.com/printer" // Ensure printer package is imported for CreatePlates and createPageLayout
)

var platform *Platform // Global platform variable

func initPlatform() (*Platform, error) {
	// Set libcamera environment variables to redirect logs and suppress terminal output
	os.Setenv("LIBCAMERA_LOG_LEVEL", "ERROR")
	os.Setenv("LIBCAMERA_LOG_FILE", "/log/libcamera.log")
	os.Setenv("LIBCAMERA_LOG_OUTPUT", "")
	os.Setenv("LIBCAMERA_PROVIDER_LOG", "0")
	os.Setenv("LD_LIBRARY_PATH", "/lib")
	logutil.DebugLog("Set libcamera environment variables: LOG_LEVEL=ERROR, LOG_FILE=/log/libcamera.log")

	// Initialize platform using the existing Init function (assumed in platform_rpi.go)
	p, err := Init()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize platform: %v", err)
	}
	platform = p
	return platform, nil
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--test-createPageLayout" {
		if len(os.Args) != 3 {
			fmt.Fprintf(os.Stderr, "Usage: %s --test-createPageLayout <tempDir>\n", os.Args[0])
			os.Exit(1)
		}
		tempDir := os.Args[2]
		testCreatePageLayout(tempDir)
		os.Exit(0)
	}
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "controller: %v\n", err)
		os.Exit(2)
	}
}

// Debug function to test createPageLayout
// Debug function to test createPageLayout
func testCreatePageLayout(tempDir string) {
	validMnemonics := []bip39.Mnemonic{
		mustParseMnemonic("truly mouse crystal game narrow tent exclude silver bench price sail various cereal deny wife manual dish also trick refuse trial salute harvest fat"),
		mustParseMnemonic("output wife day wrap office depend reduce mention lemon always proof body unit arrow wisdom clock because bar first decorate novel elbow curve split"),
		mustParseMnemonic("retreat lab leg hammer turkey affair actor raven resist dose advance pretty vague choice tube credit catalog secret usage bean album detect empty drip"),
	}
	file, err := os.Create(filepath.Join(tempDir, "test_output.pdf"))
	if err != nil {
		fmt.Printf("Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()
	paperSize := printer.PaperA4 // Default to A4 for testing
	if err := printer.CreatePlates(file, validMnemonics, nil, 0, paperSize, false, false); err != nil {
		fmt.Printf("Error generating PDF: %v\n", err)
		os.Exit(1)
	}
	if err := printer.CreatePageLayout(file, tempDir, paperSize); err != nil {
		fmt.Printf("Error merging PDF: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Test succeeded")
	os.Exit(0)
}

// Helper function to parse mnemonic and exit on error
func mustParseMnemonic(mnemonic string) bip39.Mnemonic {
	m, err := bip39.ParseMnemonic(mnemonic)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse mnemonic: %v\n", err)
		os.Exit(1)
	}
	return m
}

func run() error {
	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))
	version := os.Getenv("sh_version")
	p, err := initPlatform()
	if err != nil {
		return err
	}
	for range gui.Run(p, version) {
	}
	return nil
}

var debug = false

// Move these to main.go, not Platform
func (p *Platform) Debug() bool {
	return debug
}

func (p *Platform) Now() time.Time {
	return time.Now()
}
