package printer

import (
	"bytes"
	"image"
	"testing"

	"seedetcher.com/testutils"
)

func TestWalletPageDataParityAcrossPrintBackends(t *testing.T) {
	const (
		wallet = "multisig-mainnet-2of3"
		dpi    = 150.0
		paper  = PaperA4
	)

	seedPlates, descPlates, pages := walletPagesForTest(t, wallet, dpi, paper)

	var pclComposed bytes.Buffer
	if err := WritePCL(&pclComposed, pages, dpi, paper, nil); err != nil {
		t.Fatalf("WritePCL: %v", err)
	}
	var pclDirect bytes.Buffer
	if err := WritePCLPlates(&pclDirect, seedPlates, descPlates, dpi, paper, nil); err != nil {
		t.Fatalf("WritePCLPlates: %v", err)
	}
	if !bytes.Equal(pclComposed.Bytes(), pclDirect.Bytes()) {
		t.Fatalf("PCL mismatch: composed=%d direct=%d", pclComposed.Len(), pclDirect.Len())
	}

	var psComposed bytes.Buffer
	if err := WritePS(&psComposed, pages, paper, nil); err != nil {
		t.Fatalf("WritePS: %v", err)
	}
	var psDirect bytes.Buffer
	if err := WritePSPlates(&psDirect, seedPlates, descPlates, paper, dpi, nil, nil); err != nil {
		t.Fatalf("WritePSPlates: %v", err)
	}
	if !bytes.Equal(psComposed.Bytes(), psDirect.Bytes()) {
		t.Fatalf("PS mismatch: composed=%d direct=%d", psComposed.Len(), psDirect.Len())
	}

	var pdf bytes.Buffer
	if err := WritePDFRaster(&pdf, pages, paper); err != nil {
		t.Fatalf("WritePDFRaster: %v", err)
	}
	if pdf.Len() == 0 {
		t.Fatal("WritePDFRaster: empty output")
	}
	if !bytes.HasPrefix(pdf.Bytes(), []byte("%PDF-")) {
		t.Fatal("WritePDFRaster: output is not a PDF header")
	}
}

func TestEtchStatsIncrementalMatchesLegacyForWalletPlates(t *testing.T) {
	const (
		wallet = "multisig-mainnet-2of3"
		dpi    = 150.0
		paper  = PaperA4
	)

	seedPlates, descPlates, _ := walletPagesForTest(t, wallet, dpi, paper)

	legacyReport, err := BuildEtchStatsReport(seedPlates, descPlates, dpi, paper)
	if err != nil {
		t.Fatalf("BuildEtchStatsReport: %v", err)
	}

	statsRows := make([]EtchPlateStat, 0, len(seedPlates)*2)
	for i, seed := range seedPlates {
		if seed != nil {
			statsRows = append(statsRows, ComputeEtchPlateStat(seed, i+1, "seed", dpi))
		}
		if i < len(descPlates) && descPlates[i] != nil {
			statsRows = append(statsRows, ComputeEtchPlateStat(descPlates[i], i+1, "descriptor", dpi))
		}
	}

	incrementalReport, err := BuildEtchStatsReportFromStats(statsRows, dpi, paper)
	if err != nil {
		t.Fatalf("BuildEtchStatsReportFromStats: %v", err)
	}

	if len(legacyReport.Stats) != len(incrementalReport.Stats) {
		t.Fatalf("stats length mismatch: legacy=%d incremental=%d", len(legacyReport.Stats), len(incrementalReport.Stats))
	}
	for i := range legacyReport.Stats {
		if legacyReport.Stats[i] != incrementalReport.Stats[i] {
			t.Fatalf("stats row %d mismatch: legacy=%+v incremental=%+v", i, legacyReport.Stats[i], incrementalReport.Stats[i])
		}
	}

	legacyPage, err := RenderEtchStatsPage(legacyReport, paper, dpi)
	if err != nil {
		t.Fatalf("RenderEtchStatsPage legacy: %v", err)
	}
	incrementalPage, err := RenderEtchStatsPage(incrementalReport, paper, dpi)
	if err != nil {
		t.Fatalf("RenderEtchStatsPage incremental: %v", err)
	}
	if !bytes.Equal(legacyPage.Pix, incrementalPage.Pix) {
		t.Fatal("rendered stats page mismatch between legacy and incremental paths")
	}
}

func walletPagesForTest(t *testing.T, wallet string, dpi float64, paper PaperSize) (seedPlates, descPlates, pages []*image.Paletted) {
	t.Helper()

	cfg, ok := testutils.WalletConfigs[wallet]
	if !ok {
		t.Fatalf("wallet fixture not found: %s", wallet)
	}
	mnemonics, desc, err := testutils.ParseWallet(cfg, "", "")
	if err != nil {
		t.Fatalf("ParseWallet(%s): %v", wallet, err)
	}
	seedPlates, descPlates, err = CreatePlateBitmaps(mnemonics, desc, 0, RasterOptions{DPI: dpi}, nil)
	if err != nil {
		t.Fatalf("CreatePlateBitmaps(%s): %v", wallet, err)
	}
	pages, err = ComposePages(seedPlates, descPlates, paper, dpi, nil)
	if err != nil {
		t.Fatalf("ComposePages(%s): %v", wallet, err)
	}
	return seedPlates, descPlates, pages
}
