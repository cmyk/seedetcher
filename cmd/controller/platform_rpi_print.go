//go:build linux && arm

package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"os"
	"strings"
	"syscall"
	"time"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/gui"
	"seedetcher.com/logutil"
	"seedetcher.com/printer"
)

func isDeviceWriteEIO(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EIO) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "/dev/usb/lp0") && strings.Contains(msg, "input/output error")
}
func (p *Platform) CreatePlates(ctx *gui.Context, mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paper printer.PaperSize, opts printer.RasterOptions) error {
	logutil.DebugLog("Entering CreatePlates with mnemonic length: %d, desc: %v, keyIdx: %d", len(mnemonic), desc != nil, keyIdx)

	connected, _ := p.PrinterStatus()
	if !connected {
		return fmt.Errorf("printer not connected")
	}

	releaseMemory()
	p.printing = true
	defer func() {
		p.printing = false
		releaseMemory()
	}()

	var mnemonics []bip39.Mnemonic
	isSinglesigDesc := desc != nil && len(desc.Keys) == 1 && desc.Type == urtypes.Singlesig
	singlesigWithDescriptorSide := isSinglesigDesc && opts.SinglesigLayout == printer.SinglesigLayoutSeedWithDescriptorQR
	singlesigWithInfo := isSinglesigDesc && opts.SinglesigLayout == printer.SinglesigLayoutSeedWithInfo
	isSinglesigJob := desc == nil || isSinglesigDesc
	descForHost := desc
	if isSinglesigDesc && !singlesigWithDescriptorSide {
		// Singlesig descriptor is seed-side metadata only; no descriptor-side plates.
		descForHost = nil
	}
	if isSinglesigJob {
		// Singlesig default: print two identical seed plates.
		mnemonics = []bip39.Mnemonic{mnemonic, mnemonic}
	} else if ctx == nil { // Add this
		mnemonics = []bip39.Mnemonic{mnemonic} // Use passed mnemonic
	} else {
		mnemonics = make([]bip39.Mnemonic, len(desc.Keys))
		i := 0
		for _, k := range desc.Keys {
			if m, ok := ctx.Keystores[k.MasterFingerprint]; ok {
				mnemonics[i] = m
				i++
			}
		}
	}

	progress := func(stage printer.PrintStage, current, total int64) {
		if ctx != nil && ctx.PrintProgress != nil && total > 0 {
			ctx.PrintProgress(stage, current, total)
		}
	}

	if opts.DPI <= 0 {
		opts.DPI = 1200
	}
	hbpRuntimeReady := ctx != nil && ctx.HBPRuntimeReady
	if opts.PrinterLang == printer.PrinterLangBrotherHBP {
		if opts.DPI != 600 {
			logutil.DebugLog("HBP path: forcing 600 DPI")
		}
		opts.DPI = 600
		return p.createPlatesHBP(ctx, mnemonics, desc, keyIdx, paper, opts, progress)
	}
	if hbpRuntimeReady && opts.PrinterLang == printer.PrinterLangPS && opts.DPI > 600 {
		// PS rendering currently holds full raster pages in memory.
		if estimateJobPages(desc, paper, opts) > 1 {
			logutil.DebugLog("PS path: forcing 600 DPI for multi-page job (HBP runtime enabled)")
			opts.DPI = 600
		}
	}

	printerDev := p.Printer()
	if printerDev == nil {
		logutil.DebugLog("Printer is nil")
		return fmt.Errorf("no printer available")
	}
	if p.supportsPCL {
		logutil.DebugLog("Printer acquired (PCL), preparing to write job")
	} else {
		logutil.DebugLog("Printer acquired (non-PCL), using raster-to-PDF path")
	}

	if !p.supportsPCL && opts.DPI > 600 {
		// Gadget fallback path is heavier (raster->PDF); keep it conservative.
		opts.DPI = 600
	}
	if p.supportsPCL && opts.PrinterLang == printer.PrinterLangPS {
		return p.createPlatesPostScript(ctx, mnemonics, desc, keyIdx, paper, opts, progress)
	}
	if p.supportsPCL {
		if p.hostPCLForce600 && opts.DPI > 600 {
			logutil.DebugLog("PCL host path: forcing 600 DPI due to prior 1200 write failure")
			opts.DPI = 600
		}
		// Host-mode PCL path: render and send in page-sized batches to reduce peak RAM.
	retryPCL:
		totalShares := len(mnemonics)
		if descForHost != nil && len(descForHost.Keys) > 0 && !isSinglesigDesc {
			totalShares = len(descForHost.Keys)
		}
		if totalShares <= 0 {
			return fmt.Errorf("no shares to print")
		}
		compactSingleSided := descForHost != nil &&
			printer.CompactDescriptor2of3Enabled() &&
			descForHost.Type == urtypes.SortedMulti &&
			descForHost.Threshold == 2 &&
			len(descForHost.Keys) == 3 &&
			totalShares == 3
		var shardQRPayloads [][]string
		var err error
		if descForHost != nil && len(descForHost.Keys) > 0 {
			if isSinglesigDesc && singlesigWithDescriptorSide {
				qrPayload := printer.DescriptorQRPayload(descForHost)
				if qrPayload == "" {
					return fmt.Errorf("render: empty singlesig descriptor qr payload")
				}
				shardQRPayloads = make([][]string, totalShares)
				for i := range shardQRPayloads {
					shardQRPayloads[i] = []string{qrPayload}
				}
			} else {
				shardQRPayloads = make([][]string, totalShares)
				for i := 0; i < totalShares; i++ {
					descKeyIdx := i % len(descForHost.Keys)
					shardQRPayloads[i], err = printer.DescriptorShardQRPayloadsForShare(descForHost, totalShares, descKeyIdx)
					if err != nil {
						return fmt.Errorf("render: descriptor shard qrs: %w", err)
					}
				}
			}
		}
		sharesPerBatch := 3 // A4 with descriptor side (2x3 slots -> 3 shares/page).
		if descForHost == nil || compactSingleSided {
			sharesPerBatch = 6 // seed-only path (2x3 slots -> 6 shares/page).
		}
		if sharesPerBatch < 1 {
			sharesPerBatch = 1
		}
		numBatches := (totalShares + sharesPerBatch - 1) / sharesPerBatch
		if numBatches < 1 {
			numBatches = 1
		}
		prepareDone := int64(0)
		prepareTotal := int64(totalShares)
		if descForHost != nil && !compactSingleSided {
			prepareTotal *= 2
		}
		composeMarked := false
		sendDone := int64(0)
		sendTotal := int64(0)
		sendBatchBytes := int64(-1)
		var statsSeedImgs []*image.Paletted
		var statsDescImgs []*image.Paletted
		if opts.EtchStatsPage {
			statsSeedImgs = make([]*image.Paletted, 0, totalShares)
			if descForHost != nil && !compactSingleSided {
				statsDescImgs = make([]*image.Paletted, 0, totalShares)
			}
		}
		for start := 0; start < totalShares; start += sharesPerBatch {
			end := start + sharesPerBatch
			if end > totalShares {
				end = totalShares
			}
			batchSize := end - start
			seedBatch := make([]*image.Paletted, 0, batchSize)
			var descBatch []*image.Paletted
			if descForHost != nil && !compactSingleSided {
				descBatch = make([]*image.Paletted, 0, batchSize)
			}
			for i := start; i < end; i++ {
				m := mnemonics[i%len(mnemonics)]
				seedShareNum, seedShareTotal := i+1, totalShares
				if isSinglesigJob {
					seedShareNum, seedShareTotal = 1, 1
				}
				seedDesc := (*urtypes.OutputDescriptor)(nil)
				if singlesigWithInfo {
					seedDesc = desc
				}
				seedImg, err := printer.RenderSeedPlateBitmapWithDescriptor(m, seedShareNum, seedShareTotal, seedDesc, opts)
				if err != nil {
					return fmt.Errorf("render: seed plate %d: %w", i+1, err)
				}
				if compactSingleSided {
					descKeyIdx := i % len(descForHost.Keys)
					descQR := ""
					if i < len(shardQRPayloads) && len(shardQRPayloads[i]) > 0 {
						descQR = shardQRPayloads[i][0]
					}
					seedImg, err = printer.RenderCompact2of3PlateBitmap(m, descForHost, descKeyIdx, opts, descQR)
					if err != nil {
						return fmt.Errorf("render: compact plate %d: %w", i+1, err)
					}
				}
				seedBatch = append(seedBatch, seedImg)
				if opts.EtchStatsPage {
					statsSeedImgs = append(statsSeedImgs, seedImg)
				}
				prepareDone++
				if progress != nil && prepareTotal > 0 {
					progress(printer.StagePrepare, prepareDone, prepareTotal)
				}
				if descForHost != nil && !compactSingleSided {
					descKeyIdx := i % len(descForHost.Keys)
					var descQRs []string
					if i < len(shardQRPayloads) {
						descQRs = shardQRPayloads[i]
					}
					descImg, err := printer.RenderDescriptorPlateBitmap(descForHost, descKeyIdx, i+1, totalShares, opts, descQRs)
					if err != nil {
						return fmt.Errorf("render: descriptor plate %d: %w", i+1, err)
					}
					descBatch = append(descBatch, descImg)
					if opts.EtchStatsPage {
						statsDescImgs = append(statsDescImgs, descImg)
					}
					prepareDone++
					if progress != nil && prepareTotal > 0 {
						progress(printer.StagePrepare, prepareDone, prepareTotal)
					}
				}
			}
			if !composeMarked && progress != nil {
				progress(printer.StageCompose, 1, 1)
				composeMarked = true
			}
			if sendBatchBytes < 0 {
				var err error
				sendBatchBytes, err = printer.EstimatePCLPlatesBytes(seedBatch, descBatch, opts.DPI, paper)
				if err != nil {
					return fmt.Errorf("pcl: estimate batch %d-%d: %w", start+1, end, err)
				}
				sendTotal = sendBatchBytes * int64(numBatches)
			}
			baseDone := sendDone
			batchProgress := func(stage printer.PrintStage, current, total int64) {
				if stage != printer.StageSend || progress == nil || sendTotal <= 0 || total <= 0 {
					return
				}
				globalCurrent := baseDone + current
				if globalCurrent > sendTotal {
					globalCurrent = sendTotal
				}
				progress(printer.StageSend, globalCurrent, sendTotal)
			}
			if progress != nil && sendTotal > 0 {
				progress(printer.StageSend, sendDone, sendTotal)
			}
			if err := printer.WritePCLPlates(printerDev, seedBatch, descBatch, opts.DPI, paper, batchProgress); err != nil {
				if sendDone == 0 && opts.DPI > 600 && isDeviceWriteEIO(err) {
					logutil.DebugLog("PCL host path: write failed at %.0fdpi with EIO; retrying at 600dpi", opts.DPI)
					p.hostPCLForce600 = true
					opts.DPI = 600
					printerDev = p.Printer()
					if printerDev == nil {
						return fmt.Errorf("no printer available after 1200->600 fallback")
					}
					goto retryPCL
				}
				return fmt.Errorf("pcl: write batch %d-%d: %w", start+1, end, err)
			}
			sendDone += sendBatchBytes
			if sendDone > sendTotal {
				sendDone = sendTotal
			}
			if progress != nil && sendTotal > 0 {
				progress(printer.StageSend, sendDone, sendTotal)
			}
		}
		if progress != nil && !composeMarked {
			progress(printer.StageCompose, 1, 1)
			composeMarked = true
		}
		if opts.EtchStatsPage {
			report, err := printer.BuildEtchStatsReport(statsSeedImgs, statsDescImgs, opts.DPI, paper)
			if err != nil {
				return fmt.Errorf("stats: build report: %w", err)
			}
			statsPage, err := printer.RenderEtchStatsPage(report, paper, opts.DPI)
			if err != nil {
				return fmt.Errorf("stats: render page: %w", err)
			}
			if err := printer.WritePCL(printerDev, []*image.Paletted{statsPage}, opts.DPI, paper, progress); err != nil {
				return fmt.Errorf("stats: write pcl page: %w", err)
			}
		}
		logutil.DebugLog("PCL write complete (shares=%d dpi=%.0f, batched)", totalShares, opts.DPI)
		return nil
	}

	seedImgs, descImgs, err := printer.CreatePlateBitmaps(mnemonics, desc, keyIdx, opts, progress)
	if err != nil {
		return fmt.Errorf("render: plate bitmaps: %w", err)
	}

	pages, err := printer.ComposePages(seedImgs, descImgs, paper, opts.DPI, progress)
	if err != nil {
		return fmt.Errorf("render: compose pages: %w", err)
	}
	if opts.EtchStatsPage {
		report, err := printer.BuildEtchStatsReport(seedImgs, descImgs, opts.DPI, paper)
		if err != nil {
			return fmt.Errorf("stats: build report: %w", err)
		}
		statsPage, err := printer.RenderEtchStatsPage(report, paper, opts.DPI)
		if err != nil {
			return fmt.Errorf("stats: render page: %w", err)
		}
		pages = append(pages, statsPage)
	}

	// Fallback: serialize canonical raster pages as PDF (gadget capture/dev).
	var pdf bytes.Buffer
	if err := printer.WritePDFRaster(&pdf, pages, paper); err != nil {
		return fmt.Errorf("pdf: write: %w", err)
	}
	data := pdf.Bytes()
	logutil.DebugLog("Raster-based PDF generated, size: %d bytes", len(data))
	if len(data) == 0 {
		logutil.DebugLog("Generated PDF is empty")
		return fmt.Errorf("no data to write to printer")
	}

	const chunkSize = 1024
	total := int64(len(data))
	written := int64(0)
	if progress != nil && total > 0 {
		progress(printer.StageSend, 0, total)
	}
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[i:end]
		n, err := printerDev.Write(chunk)
		if err != nil {
			logutil.DebugLog("Write chunk %d failed: %v, wrote %d bytes", i/chunkSize, err, n)
			return err
		}
		logutil.DebugLog("Wrote chunk %d, %d bytes", i/chunkSize, n)
		written += int64(n)
		if progress != nil && total > 0 {
			progress(printer.StageSend, written, total)
		}
	}
	written = total
	if progress != nil && total > 0 {
		progress(printer.StageSend, written, total)
	}
	time.Sleep(2 * time.Second)
	return nil
}

func (p *Platform) createPlatesHBP(ctx *gui.Context, mnemonics []bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paper printer.PaperSize, opts printer.RasterOptions, progress func(stage printer.PrintStage, current, total int64)) error {
	if opts.EtchStatsPage {
		// Keep stats behavior intact for now; batching implementation below does not
		// retain all plate bitmaps needed by BuildEtchStatsReport.
		return p.createPlatesHBPLegacy(ctx, mnemonics, desc, keyIdx, paper, opts, progress)
	}

	isSinglesigDesc := desc != nil && len(desc.Keys) == 1 && desc.Type == urtypes.Singlesig
	singlesigWithDescriptorSide := isSinglesigDesc && opts.SinglesigLayout == printer.SinglesigLayoutSeedWithDescriptorQR
	singlesigWithInfo := isSinglesigDesc && opts.SinglesigLayout == printer.SinglesigLayoutSeedWithInfo
	isSinglesigJob := desc == nil || isSinglesigDesc
	totalShares := len(mnemonics)
	if desc != nil && len(desc.Keys) > 0 && !isSinglesigDesc {
		totalShares = len(desc.Keys)
	}
	if totalShares <= 0 {
		return fmt.Errorf("no shares to print")
	}

	compactSingleSided := desc != nil &&
		printer.CompactDescriptor2of3Enabled() &&
		desc.Type == urtypes.SortedMulti &&
		desc.Threshold == 2 &&
		len(desc.Keys) == 3 &&
		totalShares == 3

	var shardQRPayloads [][]string
	if desc != nil && len(desc.Keys) > 0 {
		if isSinglesigDesc && singlesigWithDescriptorSide {
			qrPayload := printer.DescriptorQRPayload(desc)
			if qrPayload == "" {
				return fmt.Errorf("render: empty singlesig descriptor qr payload")
			}
			shardQRPayloads = make([][]string, totalShares)
			for i := range shardQRPayloads {
				shardQRPayloads[i] = []string{qrPayload}
			}
		} else {
			var err error
			shardQRPayloads = make([][]string, totalShares)
			for i := 0; i < totalShares; i++ {
				descKeyIdx := i % len(desc.Keys)
				shardQRPayloads[i], err = printer.DescriptorShardQRPayloadsForShare(desc, totalShares, descKeyIdx)
				if err != nil {
					return fmt.Errorf("render: descriptor shard qrs: %w", err)
				}
			}
		}
	}

	maxSlotsPerPage := 6
	if paper == printer.PaperLetter {
		maxSlotsPerPage = 4
	}
	slotsPerShare := 1
	if desc != nil && !compactSingleSided {
		slotsPerShare = 2
	}
	sharesPerBatch := maxSlotsPerPage / slotsPerShare
	if sharesPerBatch < 1 {
		sharesPerBatch = 1
	}
	numBatches := (totalShares + sharesPerBatch - 1) / sharesPerBatch
	if numBatches < 1 {
		numBatches = 1
	}

	prepareDone := int64(0)
	prepareTotal := int64(totalShares)
	if desc != nil && !compactSingleSided {
		prepareTotal *= 2
	}
	for start := 0; start < totalShares; start += sharesPerBatch {
		end := start + sharesPerBatch
		if end > totalShares {
			end = totalShares
		}
		batchSize := end - start
		seedBatch := make([]*image.Paletted, 0, batchSize)
		var descBatch []*image.Paletted
		if desc != nil && !compactSingleSided {
			descBatch = make([]*image.Paletted, 0, batchSize)
		}
		for i := start; i < end; i++ {
			m := mnemonics[i%len(mnemonics)]
			seedShareNum, seedShareTotal := i+1, totalShares
			if isSinglesigJob {
				seedShareNum, seedShareTotal = 1, 1
			}
			seedDesc := (*urtypes.OutputDescriptor)(nil)
			if singlesigWithInfo {
				seedDesc = desc
			}
			seedImg, err := printer.RenderSeedPlateBitmapWithDescriptor(m, seedShareNum, seedShareTotal, seedDesc, opts)
			if err != nil {
				return fmt.Errorf("render: seed plate %d: %w", i+1, err)
			}
			if compactSingleSided {
				descKeyIdx := i % len(desc.Keys)
				descQR := ""
				if i < len(shardQRPayloads) && len(shardQRPayloads[i]) > 0 {
					descQR = shardQRPayloads[i][0]
				}
				seedImg, err = printer.RenderCompact2of3PlateBitmap(m, desc, descKeyIdx, opts, descQR)
				if err != nil {
					return fmt.Errorf("render: compact plate %d: %w", i+1, err)
				}
			}
			seedBatch = append(seedBatch, seedImg)
			prepareDone++
			if progress != nil && prepareTotal > 0 {
				progress(printer.StagePrepare, prepareDone, prepareTotal)
			}
			if desc != nil && !compactSingleSided {
				descKeyIdx := i % len(desc.Keys)
				var descQRs []string
				if i < len(shardQRPayloads) {
					descQRs = shardQRPayloads[i]
				}
				descImg, err := printer.RenderDescriptorPlateBitmap(desc, descKeyIdx, i+1, totalShares, opts, descQRs)
				if err != nil {
					return fmt.Errorf("render: descriptor plate %d: %w", i+1, err)
				}
				descBatch = append(descBatch, descImg)
				prepareDone++
				if progress != nil && prepareTotal > 0 {
					progress(printer.StagePrepare, prepareDone, prepareTotal)
				}
			}
		}

		outFile, err := os.CreateTemp("/tmp", "seedetcher-hbp-*.pdf")
		if err != nil {
			return fmt.Errorf("hbp: create temp pdf: %w", err)
		}
		outPath := outFile.Name()
		if err := printer.WritePDFPlates(outFile, seedBatch, descBatch, paper, opts.DPI); err != nil {
			outFile.Close()
			_ = os.Remove(outPath)
			return fmt.Errorf("hbp: write temp pdf batch %d-%d: %w", start+1, end, err)
		}
		if err := outFile.Close(); err != nil {
			_ = os.Remove(outPath)
			return fmt.Errorf("hbp: close temp pdf batch %d-%d: %w", start+1, end, err)
		}
		if progress != nil {
			progress(printer.StageCompose, int64((start/sharesPerBatch)+1), int64(numBatches))
		}

		dpiArg := fmt.Sprintf("%.0f", opts.DPI)
		cmdOut, err := runCommandWithOutput("/bin/print-hbp-pdf", outPath, dpiArg)
		_ = os.Remove(outPath)
		if cmdOut != "" {
			logutil.DebugLog("HBP print helper output (batch %d-%d):\n%s", start+1, end, cmdOut)
		}
		if err != nil {
			return err
		}
		if progress != nil {
			progress(printer.StageSend, int64((start/sharesPerBatch)+1), int64(numBatches))
		}

		seedBatch = nil
		descBatch = nil
		releaseMemory()
	}
	return nil
}

func (p *Platform) createPlatesHBPLegacy(ctx *gui.Context, mnemonics []bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paper printer.PaperSize, opts printer.RasterOptions, progress func(stage printer.PrintStage, current, total int64)) error {
	seedImgs, descImgs, err := printer.CreatePlateBitmaps(mnemonics, desc, keyIdx, opts, progress)
	if err != nil {
		return fmt.Errorf("render: plate bitmaps: %w", err)
	}

	pages, err := printer.ComposePages(seedImgs, descImgs, paper, opts.DPI, progress)
	if err != nil {
		return fmt.Errorf("render: compose pages: %w", err)
	}
	if opts.EtchStatsPage {
		report, err := printer.BuildEtchStatsReport(seedImgs, descImgs, opts.DPI, paper)
		if err != nil {
			return fmt.Errorf("stats: build report: %w", err)
		}
		statsPage, err := printer.RenderEtchStatsPage(report, paper, opts.DPI)
		if err != nil {
			return fmt.Errorf("stats: render page: %w", err)
		}
		pages = append(pages, statsPage)
	}

	outFile, err := os.CreateTemp("/tmp", "seedetcher-hbp-*.pdf")
	if err != nil {
		return fmt.Errorf("hbp: create temp pdf: %w", err)
	}
	outPath := outFile.Name()
	defer os.Remove(outPath)
	if err := printer.WritePDFRaster(outFile, pages, paper); err != nil {
		outFile.Close()
		return fmt.Errorf("hbp: write temp pdf: %w", err)
	}
	if err := outFile.Close(); err != nil {
		return fmt.Errorf("hbp: close temp pdf: %w", err)
	}

	if progress != nil {
		progress(printer.StageSend, 0, 1)
	}
	dpiArg := fmt.Sprintf("%.0f", opts.DPI)
	cmdOut, err := runCommandWithOutput("/bin/print-hbp-pdf", outPath, dpiArg)
	if cmdOut != "" {
		logutil.DebugLog("HBP print helper output:\n%s", cmdOut)
	}
	if err != nil {
		return err
	}
	if progress != nil {
		progress(printer.StageSend, 1, 1)
	}
	return nil
}

func (p *Platform) createPlatesPostScript(ctx *gui.Context, mnemonics []bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paper printer.PaperSize, opts printer.RasterOptions, progress func(stage printer.PrintStage, current, total int64)) error {
	seedImgs, descImgs, err := printer.CreatePlateBitmaps(mnemonics, desc, keyIdx, opts, progress)
	if err != nil {
		return fmt.Errorf("render: plate bitmaps: %w", err)
	}
	var extraPages []*image.Paletted
	if opts.EtchStatsPage {
		report, err := printer.BuildEtchStatsReport(seedImgs, descImgs, opts.DPI, paper)
		if err != nil {
			return fmt.Errorf("stats: build report: %w", err)
		}
		statsPage, err := printer.RenderEtchStatsPage(report, paper, opts.DPI)
		if err != nil {
			return fmt.Errorf("stats: render page: %w", err)
		}
		extraPages = append(extraPages, statsPage)
	}

	printerDev := p.Printer()
	if printerDev == nil {
		return fmt.Errorf("no printer available")
	}
	return printer.WritePSPlates(printerDev, seedImgs, descImgs, paper, opts.DPI, extraPages, progress)
}
func estimateJobPages(desc *urtypes.OutputDescriptor, paper printer.PaperSize, opts printer.RasterOptions) int {
	walletShares := 1
	if desc != nil {
		walletShares = len(desc.Keys)
	}
	maxSlotsPerPage := 6
	if paper == printer.PaperLetter {
		maxSlotsPerPage = 4
	}
	slotsPerShare := 2
	if desc == nil {
		slotsPerShare = 1
	}
	compactSingleSided := desc != nil &&
		printer.CompactDescriptor2of3Enabled() &&
		desc.Type == urtypes.SortedMulti &&
		desc.Threshold == 2 &&
		len(desc.Keys) == 3
	if compactSingleSided {
		slotsPerShare = 1
	}
	isSinglesig := desc != nil && desc.Type == urtypes.Singlesig && len(desc.Keys) == 1
	if isSinglesig && opts.SinglesigLayout != printer.SinglesigLayoutSeedWithDescriptorQR {
		slotsPerShare = 1
	}
	sharesPerPage := maxSlotsPerPage / slotsPerShare
	if sharesPerPage < 1 {
		sharesPerPage = 1
	}
	totalPages := (walletShares + sharesPerPage - 1) / sharesPerPage
	if totalPages < 1 {
		totalPages = 1
	}
	if opts.EtchStatsPage {
		totalPages++
	}
	return totalPages

}
