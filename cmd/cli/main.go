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
	if f.ListFixtures {
		fmt.Println(testutils.FixtureListText())
		return
	}

	if f.WalletName != "" {
		printer.SetWalletLabel(f.WalletName)
	}
	printer.SetCompactDescriptor2of3Enabled(f.Compact2of3)

	usr, err := user.Current()
	if err != nil {
		fmt.Printf("Error getting current user: %v\n", err)
		os.Exit(1)
	}
	config, wordProfileMnemonicOverride, err := testutils.ResolveWalletSelection(f)
	if err != nil {
		fmt.Printf("Error selecting wallet fixture: %v\n", err)
		os.Exit(1)
	}
	mnemonicOverride := f.Mnemonic
	if mnemonicOverride == "" {
		mnemonicOverride = wordProfileMnemonicOverride
	}
	mnemonics, desc, err := testutils.ParseWallet(config, mnemonicOverride, f.Descriptor)
	if err != nil {
		fmt.Printf("Error parsing wallet: %v\n", err)
		os.Exit(1)
	}
	singlesigLayout, err := parseSinglesigLayoutFlag(f.SinglesigLayout)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	if f.Verbose {
		fmt.Printf("Processing %s wallet with descriptor: %v\n", config.Name, desc != nil)
	}

	outputDir := strings.Replace(f.Output, "~", usr.HomeDir, 1)
	pngDir := ""
	if f.BitmapDir != "" {
		pngDir = strings.Replace(f.BitmapDir, "~", usr.HomeDir, 1)
	}
	pclPath := ""
	if f.PCLOut != "" {
		pclPath = strings.Replace(f.PCLOut, "~", usr.HomeDir, 1)
		// If the path is a directory (existing or ends with /), auto-name the file.
		if strings.HasSuffix(pclPath, "/") || isDir(pclPath) {
			base := fmt.Sprintf("%s.pcl", config.Name)
			pclPath = filepath.Join(strings.TrimRight(pclPath, "/"), base)
		}
	}
	sceneJSONPath := ""
	if f.SceneJSONOut != "" {
		sceneJSONPath = strings.Replace(f.SceneJSONOut, "~", usr.HomeDir, 1)
	}
	svgOutDir := ""
	if f.SVGOut != "" {
		svgOutDir = strings.Replace(f.SVGOut, "~", usr.HomeDir, 1)
	}
	gcodeOutDir := ""
	if f.GCodeOut != "" {
		gcodeOutDir = strings.Replace(f.GCodeOut, "~", usr.HomeDir, 1)
	}

	if f.Verbose {
		fmt.Println("Expanded output directory to:", outputDir)
		if pngDir != "" {
			fmt.Println("Expanded PNG directory to:", pngDir)
		}
		if pclPath != "" {
			fmt.Println("Expanded PCL path to:", pclPath)
		}
		if sceneJSONPath != "" {
			fmt.Println("Expanded scene JSON path to:", sceneJSONPath)
		}
		if svgOutDir != "" {
			fmt.Println("Expanded SVG directory to:", svgOutDir)
		}
		if gcodeOutDir != "" {
			fmt.Println("Expanded G-code directory to:", gcodeOutDir)
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
	if pclPath != "" {
		if err := os.MkdirAll(filepath.Dir(pclPath), 0755); err != nil {
			fmt.Printf("Error creating PCL output directory: %v\n", err)
			os.Exit(1)
		}
	}
	if sceneJSONPath != "" {
		if err := os.MkdirAll(filepath.Dir(sceneJSONPath), 0755); err != nil {
			fmt.Printf("Error creating scene JSON output directory: %v\n", err)
			os.Exit(1)
		}
	}
	if svgOutDir != "" {
		if err := os.MkdirAll(svgOutDir, 0755); err != nil {
			fmt.Printf("Error creating SVG output directory: %v\n", err)
			os.Exit(1)
		}
	}
	if gcodeOutDir != "" {
		if err := os.MkdirAll(gcodeOutDir, 0755); err != nil {
			fmt.Printf("Error creating G-code output directory: %v\n", err)
			os.Exit(1)
		}
	}

	printer.SetDescriptorQRSize(f.DescQRMM)
	opts := printer.RasterOptions{
		DPI:             float64(f.DPI),
		Mirror:          f.Mirror,
		Invert:          f.Invert,
		SinglesigLayout: singlesigLayout,
		EtchStatsPage:   f.EtchStatsPage,
	}
	if sceneJSONPath != "" || svgOutDir != "" || gcodeOutDir != "" {
		sceneDoc, err := printer.WalletSceneBuilder{
			Mnemonics:       mnemonics,
			Desc:            desc,
			SinglesigLayout: opts.SinglesigLayout,
		}.Build()
		if err != nil {
			fmt.Printf("Error building plate scene document: %v\n", err)
			os.Exit(1)
		}
		if sceneJSONPath != "" {
			if err := (printer.SceneJSONRenderer{}).Render(sceneDoc, sceneJSONPath); err != nil {
				fmt.Printf("Error writing scene JSON: %v\n", err)
				os.Exit(1)
			}
		}
		if svgOutDir != "" {
			if err := (printer.SceneSVGRenderer{}).Render(sceneDoc, svgOutDir); err != nil {
				fmt.Printf("Error writing scene SVG: %v\n", err)
				os.Exit(1)
			}
		}
		if gcodeOutDir != "" {
			sideScenes, err := filterScenesForSide(sceneDoc, f.GCodeSide)
			if err != nil {
				fmt.Printf("Error selecting G-code side: %v\n", err)
				os.Exit(1)
			}
			if err := (printer.SceneGCodeRenderer{
				LaserMaxS:      f.LaserMaxS,
				CutFeedMMMin:   f.LaserFeed,
				RapidFeedMMMin: f.RapidFeed,
				BedMM:          f.BedMM,
				PlateMM:        f.PlateMM,
				PlateOriginXMM: f.PlateOriginXMM,
				PlateOriginYMM: f.PlateOriginYMM,
			}).Render(sideScenes, gcodeOutDir); err != nil {
				fmt.Printf("Error writing scene G-code: %v\n", err)
				os.Exit(1)
			}
		}
		if f.Verbose {
			fmt.Printf("Generated %d seed scene(s)\n", len(sceneDoc.Scenes))
		}
	}
	seedImgs, descImgs, err := printer.CreatePlateBitmaps(mnemonics, desc, 0, opts, nil)
	if err != nil {
		fmt.Printf("Error creating raster plates: %v\n", err)
		os.Exit(1)
	}
	pages, err := printer.ComposePagesWithInvert(seedImgs, descImgs, printer.PaperSize(f.PaperSize), opts.DPI, opts.Invert, nil)
	if err != nil {
		fmt.Printf("Error composing pages: %v\n", err)
		os.Exit(1)
	}
	if opts.EtchStatsPage {
		report, err := printer.BuildEtchStatsReport(seedImgs, descImgs, opts.DPI, printer.PaperSize(f.PaperSize))
		if err != nil {
			fmt.Printf("Error building etch stats report: %v\n", err)
			os.Exit(1)
		}
		statsPage, err := printer.RenderEtchStatsPage(report, printer.PaperSize(f.PaperSize), opts.DPI)
		if err != nil {
			fmt.Printf("Error rendering etch stats page: %v\n", err)
			os.Exit(1)
		}
		pages = append(pages, statsPage)
	}

	file, err := os.Create(filepath.Join(outputDir, config.Name+".pdf"))
	if err != nil {
		fmt.Printf("Error creating PDF file %s: %v\n", config.Name+".pdf", err)
		os.Exit(1)
	}
	if err := printer.WritePDFRaster(file, pages, printer.PaperSize(f.PaperSize)); err != nil {
		file.Close()
		fmt.Printf("Error writing PDF: %v\n", err)
		os.Exit(1)
	}
	if err := file.Close(); err != nil {
		fmt.Printf("Error closing PDF file: %v\n", err)
		os.Exit(1)
	}
	if f.Verbose {
		fmt.Printf("Generated %s.pdf\n", config.Name)
	}
	fmt.Println("PDF generation completed")

	if pngDir != "" {
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
	if pclPath != "" {
		pclFile, err := os.Create(pclPath)
		if err != nil {
			fmt.Printf("Error creating PCL file %s: %v\n", pclPath, err)
			os.Exit(1)
		}
		if err := printer.WritePCL(pclFile, pages, opts.DPI, printer.PaperSize(f.PaperSize), nil); err != nil {
			pclFile.Close()
			fmt.Printf("Error writing PCL file: %v\n", err)
			os.Exit(1)
		}
		if err := pclFile.Close(); err != nil {
			fmt.Printf("Error closing PCL file: %v\n", err)
			os.Exit(1)
		}
		if f.Verbose {
			fmt.Printf("Generated PCL at %s (pages=%d mirror=%v invert=%v dpi=%d)\n", pclPath, len(pages), f.Mirror, f.Invert, f.DPI)
		}
	}
}

func parseSinglesigLayoutFlag(v string) (printer.SinglesigLayoutMode, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "seed-info":
		return printer.SinglesigLayoutSeedWithInfo, nil
	case "seed-only":
		return printer.SinglesigLayoutSeedOnly, nil
	case "seed-desc":
		return printer.SinglesigLayoutSeedWithDescriptorQR, nil
	default:
		return 0, fmt.Errorf("invalid -singlesig-layout: %s (allowed: seed-info, seed-only, seed-desc)", v)
	}
}

func filterScenesForSide(doc *printer.PlateDocument, side string) (*printer.PlateDocument, error) {
	if doc == nil {
		return nil, fmt.Errorf("scene document is nil")
	}
	mode := strings.ToLower(strings.TrimSpace(side))
	if mode == "" {
		mode = "both"
	}
	out := &printer.PlateDocument{
		Version: doc.Version,
		Scenes:  make([]printer.PlateScene, 0, len(doc.Scenes)),
	}
	switch mode {
	case "both":
		out.Scenes = append(out.Scenes, doc.Scenes...)
	case "seed":
		for _, s := range doc.Scenes {
			if strings.HasPrefix(s.Name, "seed_") {
				out.Scenes = append(out.Scenes, s)
			}
		}
	case "desc":
		for _, s := range doc.Scenes {
			if strings.HasPrefix(s.Name, "desc_") {
				out.Scenes = append(out.Scenes, s)
			}
		}
	default:
		return nil, fmt.Errorf("invalid -side: %s (allowed: seed, desc, both)", side)
	}
	if len(out.Scenes) == 0 {
		return nil, fmt.Errorf("no scenes selected for -side=%s", mode)
	}
	return out, nil
}

type walletConfig struct {
	name       string
	mnemonics  []string
	descriptor string
	outputFile string
}

// isDir reports whether the given path exists and is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
