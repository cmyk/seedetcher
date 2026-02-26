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

type hostRenderPlan struct {
	descForHost        *urtypes.OutputDescriptor
	totalShares        int
	compactSingleSided bool
	shardQRPayloads    [][]string
}

func prepareHostRenderPlan(desc *urtypes.OutputDescriptor, totalMnemonicCount int, isSinglesigDesc, singlesigWithDescriptorSide bool) (hostRenderPlan, error) {
	plan := hostRenderPlan{
		descForHost: desc,
		totalShares: totalMnemonicCount,
	}
	if isSinglesigDesc && !singlesigWithDescriptorSide {
		// Singlesig descriptor is seed-side metadata only; no descriptor-side plates.
		plan.descForHost = nil
	}
	if plan.descForHost != nil && len(plan.descForHost.Keys) > 0 && !isSinglesigDesc {
		plan.totalShares = len(plan.descForHost.Keys)
	}
	if plan.totalShares <= 0 {
		return hostRenderPlan{}, fmt.Errorf("no shares to print")
	}

	plan.compactSingleSided = plan.descForHost != nil &&
		printer.CompactDescriptor2of3Enabled() &&
		plan.descForHost.Type == urtypes.SortedMulti &&
		plan.descForHost.Threshold == 2 &&
		len(plan.descForHost.Keys) == 3 &&
		plan.totalShares == 3

	if plan.descForHost != nil && len(plan.descForHost.Keys) > 0 {
		if isSinglesigDesc && singlesigWithDescriptorSide {
			qrPayload := printer.DescriptorQRPayload(plan.descForHost)
			if qrPayload == "" {
				return hostRenderPlan{}, fmt.Errorf("render: empty singlesig descriptor qr payload")
			}
			plan.shardQRPayloads = make([][]string, plan.totalShares)
			for i := range plan.shardQRPayloads {
				plan.shardQRPayloads[i] = []string{qrPayload}
			}
		} else {
			var err error
			plan.shardQRPayloads = make([][]string, plan.totalShares)
			for i := 0; i < plan.totalShares; i++ {
				descKeyIdx := i % len(plan.descForHost.Keys)
				plan.shardQRPayloads[i], err = printer.DescriptorShardQRPayloadsForShare(plan.descForHost, plan.totalShares, descKeyIdx)
				if err != nil {
					return hostRenderPlan{}, fmt.Errorf("render: descriptor shard qrs: %w", err)
				}
			}
		}
	}

	return plan, nil
}

type hostRenderedBatch struct {
	seedBatch []*image.Paletted
	descBatch []*image.Paletted
	statsRows []printer.EtchPlateStat
}

func renderHostBatch(
	mnemonics []bip39.Mnemonic,
	seedInfoDesc *urtypes.OutputDescriptor,
	plan hostRenderPlan,
	isSinglesigJob bool,
	singlesigWithInfo bool,
	opts printer.RasterOptions,
	start, end int,
	progress func(stage printer.PrintStage, current, total int64),
	prepareDone *int64,
	prepareTotal int64,
	collectStats bool,
) (hostRenderedBatch, error) {
	batchSize := end - start
	out := hostRenderedBatch{
		seedBatch: make([]*image.Paletted, 0, batchSize),
	}
	if plan.descForHost != nil && !plan.compactSingleSided {
		out.descBatch = make([]*image.Paletted, 0, batchSize)
	}
	if collectStats {
		statsCap := batchSize
		if plan.descForHost != nil && !plan.compactSingleSided {
			statsCap *= 2
		}
		out.statsRows = make([]printer.EtchPlateStat, 0, statsCap)
	}

	for i := start; i < end; i++ {
		m := mnemonics[i%len(mnemonics)]
		seedShareNum, seedShareTotal := i+1, plan.totalShares
		if isSinglesigJob {
			seedShareNum, seedShareTotal = 1, 1
		}
		var seedDesc *urtypes.OutputDescriptor
		if singlesigWithInfo {
			seedDesc = seedInfoDesc
		}
		seedImg, err := printer.RenderSeedPlateBitmapWithDescriptor(m, seedShareNum, seedShareTotal, seedDesc, opts)
		if err != nil {
			return hostRenderedBatch{}, fmt.Errorf("render: seed plate %d: %w", i+1, err)
		}
		if plan.compactSingleSided {
			descKeyIdx := i % len(plan.descForHost.Keys)
			descQR := ""
			if i < len(plan.shardQRPayloads) && len(plan.shardQRPayloads[i]) > 0 {
				descQR = plan.shardQRPayloads[i][0]
			}
			seedImg, err = printer.RenderCompact2of3PlateBitmap(m, plan.descForHost, descKeyIdx, opts, descQR)
			if err != nil {
				return hostRenderedBatch{}, fmt.Errorf("render: compact plate %d: %w", i+1, err)
			}
		}
		out.seedBatch = append(out.seedBatch, seedImg)
		if collectStats {
			out.statsRows = append(out.statsRows, printer.ComputeEtchPlateStat(seedImg, i+1, "seed", opts.DPI))
		}
		*prepareDone = *prepareDone + 1
		if progress != nil && prepareTotal > 0 {
			progress(printer.StagePrepare, *prepareDone, prepareTotal)
		}

		if plan.descForHost != nil && !plan.compactSingleSided {
			descKeyIdx := i % len(plan.descForHost.Keys)
			var descQRs []string
			if i < len(plan.shardQRPayloads) {
				descQRs = plan.shardQRPayloads[i]
			}
			descImg, err := printer.RenderDescriptorPlateBitmap(plan.descForHost, descKeyIdx, i+1, plan.totalShares, opts, descQRs)
			if err != nil {
				return hostRenderedBatch{}, fmt.Errorf("render: descriptor plate %d: %w", i+1, err)
			}
			out.descBatch = append(out.descBatch, descImg)
			if collectStats {
				out.statsRows = append(out.statsRows, printer.ComputeEtchPlateStat(descImg, i+1, "descriptor", opts.DPI))
			}
			*prepareDone = *prepareDone + 1
			if progress != nil && prepareTotal > 0 {
				progress(printer.StagePrepare, *prepareDone, prepareTotal)
			}
		}
	}

	return out, nil
}

func hostSharesPerBatch(plan hostRenderPlan) int {
	sharesPerBatch := 3 // A4 with descriptor side (2x3 slots -> 3 shares/page).
	if plan.descForHost == nil || plan.compactSingleSided {
		sharesPerBatch = 6 // seed-only path (2x3 slots -> 6 shares/page).
	}
	if sharesPerBatch < 1 {
		return 1
	}
	return sharesPerBatch
}

func hostPrepareTotal(plan hostRenderPlan, totalShares int) int64 {
	prepareTotal := int64(totalShares)
	if plan.descForHost != nil && !plan.compactSingleSided {
		prepareTotal *= 2
	}
	return prepareTotal
}

func hostStatsCap(plan hostRenderPlan, totalShares int) int {
	statsCap := totalShares
	if plan.descForHost != nil && !plan.compactSingleSided {
		statsCap *= 2
	}
	return statsCap
}

func buildStatsPageFromRows(statsRows []printer.EtchPlateStat, dpi float64, paper printer.PaperSize) (*image.Paletted, error) {
	report, err := printer.BuildEtchStatsReportFromStats(statsRows, dpi, paper)
	if err != nil {
		return nil, err
	}
	return printer.RenderEtchStatsPage(report, paper, dpi)
}

type hostBatchRunResult struct {
	numBatches int
	statsRows  []printer.EtchPlateStat
}

func runHostRenderBatches(
	mnemonics []bip39.Mnemonic,
	seedInfoDesc *urtypes.OutputDescriptor,
	plan hostRenderPlan,
	isSinglesigJob bool,
	singlesigWithInfo bool,
	opts printer.RasterOptions,
	progress func(stage printer.PrintStage, current, total int64),
	collectStats bool,
	onBatch func(batch hostRenderedBatch, start, end, batchIndex, numBatches int) error,
) (hostBatchRunResult, error) {
	totalShares := plan.totalShares
	sharesPerBatch := hostSharesPerBatch(plan)
	numBatches := (totalShares + sharesPerBatch - 1) / sharesPerBatch
	if numBatches < 1 {
		numBatches = 1
	}
	prepareDone := int64(0)
	prepareTotal := hostPrepareTotal(plan, totalShares)
	var statsRows []printer.EtchPlateStat
	if collectStats {
		statsRows = make([]printer.EtchPlateStat, 0, hostStatsCap(plan, totalShares))
	}

	batchIndex := 0
	for start := 0; start < totalShares; start += sharesPerBatch {
		end := start + sharesPerBatch
		if end > totalShares {
			end = totalShares
		}
		batch, err := renderHostBatch(
			mnemonics,
			seedInfoDesc,
			plan,
			isSinglesigJob,
			singlesigWithInfo,
			opts,
			start,
			end,
			progress,
			&prepareDone,
			prepareTotal,
			collectStats,
		)
		if err != nil {
			return hostBatchRunResult{}, err
		}
		if collectStats {
			statsRows = append(statsRows, batch.statsRows...)
		}
		batchIndex++
		if onBatch != nil {
			if err := onBatch(batch, start, end, batchIndex, numBatches); err != nil {
				return hostBatchRunResult{}, err
			}
		}
	}

	return hostBatchRunResult{
		numBatches: numBatches,
		statsRows:  statsRows,
	}, nil
}

type pclNeed600RetryError struct {
	cause error
}

func (e *pclNeed600RetryError) Error() string {
	if e == nil || e.cause == nil {
		return "pcl 1200->600 retry required"
	}
	return e.cause.Error()
}

func (e *pclNeed600RetryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (p *Platform) CreatePlates(ctx *gui.Context, mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paper printer.PaperSize, opts printer.RasterOptions) error {
	logutil.DebugLog("Entering CreatePlates with mnemonic length: %d, desc: %v, keyIdx: %d", len(mnemonic), desc != nil, keyIdx)

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
	if opts.PrinterLang == printer.PrinterLangBrotherHBP {
		if opts.DPI != 600 {
			logutil.DebugLog("HBP path: forcing 600 DPI")
		}
		opts.DPI = 600
		return p.createPlatesHBP(ctx, mnemonics, desc, keyIdx, paper, opts, progress)
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
		plan, err := prepareHostRenderPlan(desc, len(mnemonics), isSinglesigDesc, singlesigWithDescriptorSide)
		if err != nil {
			return err
		}
		totalShares := plan.totalShares
		composeMarked := false
		sendDone := int64(0)
		sendTotal := int64(0)
		sendBatchBytes := int64(-1)
		runResult, err := runHostRenderBatches(
			mnemonics,
			desc,
			plan,
			isSinglesigJob,
			singlesigWithInfo,
			opts,
			progress,
			opts.EtchStatsPage,
			func(batch hostRenderedBatch, start, end, _, numBatches int) error {
				if !composeMarked && progress != nil {
					progress(printer.StageCompose, 1, 1)
					composeMarked = true
				}
				if sendBatchBytes < 0 {
					var err error
					sendBatchBytes, err = printer.EstimatePCLPlatesBytes(batch.seedBatch, batch.descBatch, opts.DPI, paper)
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
				if err := printer.WritePCLPlates(printerDev, batch.seedBatch, batch.descBatch, opts.DPI, paper, batchProgress); err != nil {
					if sendDone == 0 && opts.DPI > 600 && isDeviceWriteEIO(err) {
						return &pclNeed600RetryError{cause: err}
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
				return nil
			},
		)
		if err != nil {
			var need600 *pclNeed600RetryError
			if errors.As(err, &need600) {
				logutil.DebugLog("PCL host path: write failed at %.0fdpi with EIO; retrying at 600dpi", opts.DPI)
				p.hostPCLForce600 = true
				opts.DPI = 600
				printerDev = p.Printer()
				if printerDev == nil {
					return fmt.Errorf("no printer available after 1200->600 fallback")
				}
				goto retryPCL
			}
			return err
		}
		if progress != nil && !composeMarked {
			progress(printer.StageCompose, 1, 1)
			composeMarked = true
		}
		if opts.EtchStatsPage {
			statsPage, err := buildStatsPageFromRows(runResult.statsRows, opts.DPI, paper)
			if err != nil {
				return fmt.Errorf("stats: build/render page: %w", err)
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
	_ = ctx
	_ = keyIdx
	isSinglesigDesc := desc != nil && len(desc.Keys) == 1 && desc.Type == urtypes.Singlesig
	singlesigWithDescriptorSide := isSinglesigDesc && opts.SinglesigLayout == printer.SinglesigLayoutSeedWithDescriptorQR
	singlesigWithInfo := isSinglesigDesc && opts.SinglesigLayout == printer.SinglesigLayoutSeedWithInfo
	isSinglesigJob := desc == nil || isSinglesigDesc
	plan, err := prepareHostRenderPlan(desc, len(mnemonics), isSinglesigDesc, singlesigWithDescriptorSide)
	if err != nil {
		return err
	}
	totalShares := plan.totalShares

	maxSlotsPerPage := 6
	if paper == printer.PaperLetter {
		maxSlotsPerPage = 4
	}
	slotsPerShare := 1
	if plan.descForHost != nil && !plan.compactSingleSided {
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
	composeTotal := int64(numBatches)
	sendTotal := int64(numBatches)
	if opts.EtchStatsPage {
		composeTotal++
		sendTotal++
	}

	prepareDone := int64(0)
	prepareTotal := hostPrepareTotal(plan, totalShares)
	var statsRows []printer.EtchPlateStat
	if opts.EtchStatsPage {
		statsRows = make([]printer.EtchPlateStat, 0, hostStatsCap(plan, totalShares))
	}
	progressStep := int64(0)

	for start := 0; start < totalShares; start += sharesPerBatch {
		end := start + sharesPerBatch
		if end > totalShares {
			end = totalShares
		}
		batch, err := renderHostBatch(
			mnemonics,
			desc,
			plan,
			isSinglesigJob,
			singlesigWithInfo,
			opts,
			start,
			end,
			progress,
			&prepareDone,
			prepareTotal,
			opts.EtchStatsPage,
		)
		if err != nil {
			return err
		}
		if opts.EtchStatsPage {
			statsRows = append(statsRows, batch.statsRows...)
		}

		progressStep++
		if progress != nil && composeTotal > 0 {
			progress(printer.StageCompose, progressStep, composeTotal)
		}
		outFile, err := os.CreateTemp("/tmp", "seedetcher-hbp-*.pdf")
		if err != nil {
			return fmt.Errorf("hbp: create temp pdf: %w", err)
		}
		outPath := outFile.Name()
		if err := printer.WritePDFPlates(outFile, batch.seedBatch, batch.descBatch, paper, opts.DPI); err != nil {
			outFile.Close()
			_ = os.Remove(outPath)
			return fmt.Errorf("hbp: write temp pdf batch %d-%d: %w", start+1, end, err)
		}
		if err := outFile.Close(); err != nil {
			_ = os.Remove(outPath)
			return fmt.Errorf("hbp: close temp pdf batch %d-%d: %w", start+1, end, err)
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
		if progress != nil && sendTotal > 0 {
			progress(printer.StageSend, progressStep, sendTotal)
		}

		releaseMemory()
	}
	if opts.EtchStatsPage {
		statsPage, err := buildStatsPageFromRows(statsRows, opts.DPI, paper)
		if err != nil {
			return fmt.Errorf("stats: build/render page: %w", err)
		}
		progressStep++
		if progress != nil && composeTotal > 0 {
			progress(printer.StageCompose, progressStep, composeTotal)
		}

		outFile, err := os.CreateTemp("/tmp", "seedetcher-hbp-stats-*.pdf")
		if err != nil {
			return fmt.Errorf("hbp: create stats pdf: %w", err)
		}
		outPath := outFile.Name()
		if err := printer.WritePDFRaster(outFile, []*image.Paletted{statsPage}, paper); err != nil {
			outFile.Close()
			_ = os.Remove(outPath)
			return fmt.Errorf("hbp: write stats pdf: %w", err)
		}
		if err := outFile.Close(); err != nil {
			_ = os.Remove(outPath)
			return fmt.Errorf("hbp: close stats pdf: %w", err)
		}

		dpiArg := fmt.Sprintf("%.0f", opts.DPI)
		cmdOut, err := runCommandWithOutput("/bin/print-hbp-pdf", outPath, dpiArg)
		_ = os.Remove(outPath)
		if cmdOut != "" {
			logutil.DebugLog("HBP print helper output (stats page):\n%s", cmdOut)
		}
		if err != nil {
			return err
		}
		if progress != nil && sendTotal > 0 {
			progress(printer.StageSend, progressStep, sendTotal)
		}
		releaseMemory()
	}
	return nil
}

func (p *Platform) createPlatesPostScript(ctx *gui.Context, mnemonics []bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paper printer.PaperSize, opts printer.RasterOptions, progress func(stage printer.PrintStage, current, total int64)) error {
	_ = keyIdx
	isSinglesigDesc := desc != nil && len(desc.Keys) == 1 && desc.Type == urtypes.Singlesig
	singlesigWithDescriptorSide := isSinglesigDesc && opts.SinglesigLayout == printer.SinglesigLayoutSeedWithDescriptorQR
	singlesigWithInfo := isSinglesigDesc && opts.SinglesigLayout == printer.SinglesigLayoutSeedWithInfo
	isSinglesigJob := desc == nil || isSinglesigDesc
	plan, err := prepareHostRenderPlan(desc, len(mnemonics), isSinglesigDesc, singlesigWithDescriptorSide)
	if err != nil {
		return err
	}
	totalShares := plan.totalShares

	printerDev := p.Printer()
	if printerDev == nil {
		return fmt.Errorf("no printer available")
	}

	composeMarked := false
	sendDone := int64(0)
	sendTotal := int64(hostBatchCount(totalShares, hostSharesPerBatch(plan)))
	if sendTotal < 1 {
		sendTotal = 1
	}
	if opts.EtchStatsPage {
		sendTotal++
	}

	runResult, err := runHostRenderBatches(
		mnemonics,
		desc,
		plan,
		isSinglesigJob,
		singlesigWithInfo,
		opts,
		progress,
		opts.EtchStatsPage,
		func(batch hostRenderedBatch, start, end, _, _ int) error {
			if !composeMarked && progress != nil {
				progress(printer.StageCompose, 1, 1)
				composeMarked = true
			}
			if progress != nil && sendTotal > 0 {
				progress(printer.StageSend, sendDone, sendTotal)
			}
			if err := printer.WritePSPlates(printerDev, batch.seedBatch, batch.descBatch, paper, opts.DPI, nil, nil); err != nil {
				return fmt.Errorf("ps: write batch %d-%d: %w", start+1, end, err)
			}
			sendDone++
			if sendDone > sendTotal {
				sendDone = sendTotal
			}
			if progress != nil && sendTotal > 0 {
				progress(printer.StageSend, sendDone, sendTotal)
			}
			return nil
		},
	)
	if err != nil {
		return err
	}
	if progress != nil && !composeMarked {
		progress(printer.StageCompose, 1, 1)
		composeMarked = true
	}
	if opts.EtchStatsPage {
		statsPage, err := buildStatsPageFromRows(runResult.statsRows, opts.DPI, paper)
		if err != nil {
			return fmt.Errorf("stats: build/render page: %w", err)
		}
		if progress != nil && sendTotal > 0 {
			progress(printer.StageSend, sendDone, sendTotal)
		}
		if err := printer.WritePS(printerDev, []*image.Paletted{statsPage}, paper, nil); err != nil {
			return fmt.Errorf("stats: write ps page: %w", err)
		}
		sendDone++
		if sendDone > sendTotal {
			sendDone = sendTotal
		}
		if progress != nil && sendTotal > 0 {
			progress(printer.StageSend, sendDone, sendTotal)
		}
	}
	logutil.DebugLog("PS write complete (shares=%d dpi=%.0f, batched)", totalShares, opts.DPI)
	return nil
}

func hostBatchCount(totalShares, sharesPerBatch int) int {
	if sharesPerBatch < 1 {
		sharesPerBatch = 1
	}
	n := (totalShares + sharesPerBatch - 1) / sharesPerBatch
	if n < 1 {
		return 1
	}
	return n
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
