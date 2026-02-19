package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	testMode := flag.Bool("test-createPageLayout", false, "Run canonical bitmap page rendering test mode")
	flag.Parse()

	if *testMode {
		if err := runCLI(f); err != nil {
			fmt.Fprintf(os.Stderr, "controller CLI: %v\n", err)
			os.Exit(2)
		}
		os.Exit(0)
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "controller: %v\n", err)
		os.Exit(2)
	}
}

func runCLI(f *testutils.Flags) error {
	config, ok := testutils.WalletConfigs[f.WalletType]
	if !ok {
		return fmt.Errorf("invalid wallet type: %s", f.WalletType)
	}
	mnemonics, desc, err := testutils.ParseWallet(config, f.Mnemonic, f.Descriptor)
	if err != nil {
		return fmt.Errorf("error parsing wallet: %v", err)
	}
	if f.Verbose {
		logutil.DebugLog("Processing %s wallet", config.Name)
	}

	if f.WalletName != "" {
		printer.SetWalletLabel(f.WalletName)
	}
	printer.SetDescriptorQRSize(f.DescQRMM)

	opts := printer.RasterOptions{
		DPI:           float64(f.DPI),
		Mirror:        f.Mirror,
		Invert:        f.Invert,
		EtchStatsPage: f.EtchStatsPage,
	}
	seedImgs, descImgs, err := printer.CreatePlateBitmaps(mnemonics, desc, 0, opts, nil)
	if err != nil {
		return fmt.Errorf("render bitmaps: %w", err)
	}
	pages, err := printer.ComposePages(seedImgs, descImgs, printer.PaperSize(f.PaperSize), opts.DPI, nil)
	if err != nil {
		return fmt.Errorf("compose pages: %w", err)
	}
	if opts.EtchStatsPage {
		report, err := printer.BuildEtchStatsReport(seedImgs, descImgs, opts.DPI, printer.PaperSize(f.PaperSize))
		if err != nil {
			return fmt.Errorf("build etch stats report: %w", err)
		}
		statsPage, err := printer.RenderEtchStatsPage(report, printer.PaperSize(f.PaperSize), opts.DPI)
		if err != nil {
			return fmt.Errorf("render etch stats page: %w", err)
		}
		pages = append(pages, statsPage)
	}

	const outPDF = "/tmp/test_output.pdf"
	pdfFile, err := os.Create(outPDF)
	if err != nil {
		return fmt.Errorf("failed to create output PDF: %v", err)
	}
	if err := printer.WritePDFRaster(pdfFile, pages, printer.PaperSize(f.PaperSize)); err != nil {
		pdfFile.Close()
		return fmt.Errorf("write PDF: %w", err)
	}
	if err := pdfFile.Close(); err != nil {
		return fmt.Errorf("close output PDF: %w", err)
	}
	if f.Verbose {
		logutil.DebugLog("PDF generated at %s", outPDF)
	}

	pclPath := strings.TrimSpace(f.PCLOut)
	if pclPath != "" {
		if strings.HasSuffix(pclPath, "/") || isDir(pclPath) {
			pclPath = filepath.Join(strings.TrimRight(pclPath, "/"), config.Name+".pcl")
		}
		if err := os.MkdirAll(filepath.Dir(pclPath), 0o755); err != nil {
			return fmt.Errorf("create PCL output directory: %w", err)
		}
		pclFile, err := os.Create(pclPath)
		if err != nil {
			return fmt.Errorf("create PCL output file: %w", err)
		}
		if err := printer.WritePCL(pclFile, pages, opts.DPI, printer.PaperSize(f.PaperSize), nil); err != nil {
			pclFile.Close()
			return fmt.Errorf("write PCL: %w", err)
		}
		if err := pclFile.Close(); err != nil {
			return fmt.Errorf("close PCL output file: %w", err)
		}
		if f.Verbose {
			logutil.DebugLog("PCL generated at %s", pclPath)
		}
	}

	return nil
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

// isDir reports whether the given path exists and is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
