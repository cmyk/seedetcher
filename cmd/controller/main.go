package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"seedetcher.com/bip39"
	"seedetcher.com/gui"
	"seedetcher.com/logutil"
	"seedetcher.com/printer"
	"seedetcher.com/testutils"
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
	f := testutils.DefineFlags()
	flag.Parse()

	if len(os.Args) > 1 && os.Args[1] == "--test-createPageLayout" {
		if len(os.Args) != 3 {
			fmt.Fprintf(os.Stderr, "Usage: %s --test-createPageLayout <tempDir>\n", os.Args[0])
			os.Exit(1)
		}
		testCreatePageLayout(f, os.Args[2])
		os.Exit(0)
	}
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "controller: %v\n", err)
		os.Exit(2)
	}
}

// Debug function to test createPageLayout
func testCreatePageLayout(f *testutils.Flags, tempDir string) {
	config, ok := testutils.WalletConfigs[f.WalletType]
	if !ok {
		fmt.Printf("Invalid wallet type: %s\n", f.WalletType)
		os.Exit(1)
	}
	mnemonics, desc, err := testutils.ParseWallet(config, f.Mnemonic, f.Descriptor)
	if err != nil {
		fmt.Printf("Error parsing wallet: %v\n", err)
		os.Exit(1)
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	logutil.DebugLog("Memory before plates: HeapAlloc=%.2f MB", float64(m.HeapAlloc)/1024/1024)
	file, err := os.Create("/tmp/test_output.pdf")
	if err != nil {
		fmt.Printf("Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()
	paperSize := printer.PaperSize(f.PaperSize)
	seedPaths, descPaths, tempDir, err := printer.CreatePlates(file, mnemonics, desc, 0, false, false)
	if err != nil {
		fmt.Printf("Error generating PDF: %v\n", err)
		os.Exit(1)
	}
	runtime.ReadMemStats(&m)
	logutil.DebugLog("Memory after plates: HeapAlloc=%.2f MB", float64(m.HeapAlloc)/1024/1024)
	logutil.DebugLog("Before CreatePageLayout: seedPaths=%v, descPaths=%v, tempDir=%s", seedPaths, descPaths, tempDir)
	for _, path := range seedPaths {
		if path != "" {
			if info, err := os.Stat(path); err == nil {
				logutil.DebugLog("Seed file %s exists, size: %d bytes", path, info.Size())
			} else {
				logutil.DebugLog("Seed file %s stat failed: %v", path, err)
			}
		}
	}
	for _, path := range descPaths {
		if path != "" {
			if info, err := os.Stat(path); err == nil {
				logutil.DebugLog("Desc file %s exists, size: %d bytes", path, info.Size())
			} else {
				logutil.DebugLog("Desc file %s stat failed: %v", path, err)
			}
		}
	}
	if err := printer.CreatePageLayout(file, tempDir, paperSize, seedPaths, descPaths); err != nil {
		logutil.DebugLog("CreatePageLayout failed: %v", err)
		fmt.Printf("Error merging PDF: %v\n", err)
		os.Exit(1)
	}
	runtime.ReadMemStats(&m)
	logutil.DebugLog("Memory after merge: HeapAlloc=%.2f MB", float64(m.HeapAlloc)/1024/1024)
	logutil.DebugLog("Merged PDF saved to /tmp/test_output.pdf")
	for _, path := range seedPaths {
		if path != "" {
			os.Remove(path)
		}
	}
	for _, path := range descPaths {
		if path != "" {
			os.Remove(path)
		}
	}
	os.RemoveAll(tempDir)

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
