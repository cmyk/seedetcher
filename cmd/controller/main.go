package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"seedetcher.com/bc/urtypes"
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
	printer.SetCompactDescriptor2of3Enabled(f.Compact2of3)
	defer printer.SetCompactDescriptor2of3Enabled(false)
	if adjusted, note, err := adjustDPILowMem(mnemonics, desc, printer.PaperSize(f.PaperSize), opts, f.Compact2of3); err != nil {
		return err
	} else if adjusted != opts.DPI {
		opts.DPI = adjusted
		fmt.Fprintln(os.Stderr, note)
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

func adjustDPILowMem(mnemonics []bip39.Mnemonic, desc *urtypes.OutputDescriptor, paper printer.PaperSize, opts printer.RasterOptions, compact2of3 bool) (float64, string, error) {
	avail, err := memAvailableBytes()
	if err != nil || avail <= 0 {
		return opts.DPI, "", nil
	}

	estimate := func(dpi float64) (int64, error) {
		if dpi <= 0 {
			return 0, fmt.Errorf("invalid dpi: %.0f", dpi)
		}
		totalShares, seedPlates, descPlates := rasterPlateCounts(mnemonics, desc, opts, compact2of3)
		if totalShares <= 0 || seedPlates <= 0 {
			return 0, fmt.Errorf("no shares to render")
		}
		pageCount := rasterPageCount(seedPlates, descPlates, paper)
		if opts.EtchStatsPage {
			pageCount++
		}
		pw, ph, err := paperPixelDims(paper, dpi)
		if err != nil {
			return 0, err
		}
		side := mmToPx(90.0, dpi)
		plateBytes := int64(side * side)
		pageBytes := int64(pw * ph)
		base := plateBytes*int64(seedPlates+descPlates) + pageBytes*int64(pageCount)
		// Safety factor for transient allocations in compose/encode path.
		estimate := int64(float64(base)*1.35) + 24*1024*1024
		return estimate, nil
	}

	const budgetRatio = 0.75
	budget := int64(float64(avail) * budgetRatio)
	need, err := estimate(opts.DPI)
	if err != nil {
		return 0, "", err
	}
	if need <= budget {
		return opts.DPI, "", nil
	}

	if opts.DPI > 600 {
		need600, err := estimate(600)
		if err != nil {
			return 0, "", err
		}
		if need600 <= budget {
			return 600, fmt.Sprintf("controller CLI: requested %.0fdpi needs ~%dMB with %dMB available; using 600dpi to avoid OOM", opts.DPI, need/(1024*1024), avail/(1024*1024)), nil
		}
	}

	return 0, "", fmt.Errorf("insufficient RAM for %.0fdpi render: need ~%dMB, available %dMB (budget %dMB)", opts.DPI, need/(1024*1024), avail/(1024*1024), budget/(1024*1024))
}

func rasterPlateCounts(mnemonics []bip39.Mnemonic, desc *urtypes.OutputDescriptor, opts printer.RasterOptions, compact2of3 bool) (totalShares, seedPlates, descPlates int) {
	totalShares = len(mnemonics)
	if totalShares <= 0 {
		return 0, 0, 0
	}
	isSinglesigDesc := desc != nil && len(desc.Keys) == 1 && desc.Type == urtypes.Singlesig
	includeSinglesigDescriptorSide := isSinglesigDesc && opts.SinglesigLayout == printer.SinglesigLayoutSeedWithDescriptorQR
	if desc != nil && len(desc.Keys) > 0 && !isSinglesigDesc {
		totalShares = len(desc.Keys)
	}

	seedPlates = totalShares
	hasDesc := desc != nil && len(desc.Keys) > 0 && (!isSinglesigDesc || includeSinglesigDescriptorSide)
	if hasDesc {
		descPlates = totalShares
	}
	compactSingleSided := hasDesc &&
		compact2of3 &&
		desc != nil &&
		desc.Type == urtypes.SortedMulti &&
		desc.Threshold == 2 &&
		len(desc.Keys) == 3 &&
		totalShares == 3
	if compactSingleSided {
		descPlates = 0
	}
	return totalShares, seedPlates, descPlates
}

func rasterPageCount(seedPlates, descPlates int, paper printer.PaperSize) int {
	slots := seedPlates + descPlates
	perPage := 6
	if paper == printer.PaperLetter {
		perPage = 4
	}
	if perPage <= 0 {
		perPage = 1
	}
	pages := (slots + perPage - 1) / perPage
	if pages <= 0 {
		pages = 1
	}
	return pages
}

func mmToPx(mm, dpi float64) int {
	return int(math.Round(mm * dpi / 25.4))
}

func paperPixelDims(p printer.PaperSize, dpi float64) (int, int, error) {
	var wmm, hmm float64
	switch p {
	case printer.PaperA4:
		wmm, hmm = 210, 297
	case printer.PaperLetter:
		wmm, hmm = 216, 279
	default:
		return 0, 0, fmt.Errorf("unsupported paper size: %v", p)
	}
	w := mmToPx(wmm, dpi)
	h := mmToPx(hmm, dpi)
	if w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("invalid paper dimensions: %dx%d", w, h)
	}
	return w, h, nil
}

func memAvailableBytes() (int64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "MemAvailable:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			break
		}
		v, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, err
		}
		// /proc/meminfo reports kB.
		return v * 1024, nil
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("MemAvailable not found in /proc/meminfo")
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
