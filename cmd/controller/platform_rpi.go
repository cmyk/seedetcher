//go:build linux && arm

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	rdebug "runtime/debug"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/driver/drm"
	"seedetcher.com/driver/libcamera"
	"seedetcher.com/driver/wshat"
	"seedetcher.com/gui"
	"seedetcher.com/logutil"
	"seedetcher.com/printer"
	"seedetcher.com/zbar"
)

// Debug hooks (ensure unique per build tag if needed, but keep as is for now).
var (
	initHook func(p *Platform) error
)

// queryPrinterCapabilities sends a PJL query and parses the response for PCL/PostScript support
func queryPrinterCapabilities(w io.Writer, r io.Reader) (supportsPCL, supportsPostScript bool, err error) {
	// PJL query for printer language
	query := []byte("\033%-12345X@PJL INFO VARIABLES\r\n\033%-12345X")
	if _, err = w.Write(query); err != nil {
		logutil.DebugLog("PJL query failed: %v", err)
		return false, false, err
	}

	// Read response (simplified, assumes line-based response)
	buf := make([]byte, 1024)
	n, err := r.Read(buf)
	if err != nil {
		logutil.DebugLog("Failed to read PJL response: %v", err)
		return false, false, err
	}
	response := string(buf[:n])
	logutil.DebugLog("PJL response: %s", response)

	// Parse for PCL and PostScript (simplified, adjust for actual printer response)
	supportsPCL = strings.Contains(response, "PCL")
	supportsPostScript = strings.Contains(response, "POSTSCRIPT")
	return supportsPCL, supportsPostScript, nil
}

type Platform struct {
	display *drm.LCD
	events  chan gui.Event
	wakeups chan struct{}
	timer   *time.Timer
	camera  struct {
		frames chan gui.FrameEvent
		out    chan gui.FrameEvent
		frame  *gui.FrameEvent
		close  func()
		active bool
	}
	printerCached      io.Writer
	supportsPCL        bool
	supportsPostScript bool
	printing           bool // Add flag to track printing state
}

func Init() (*Platform, error) {
	log.Println("Running platform_rpi.go")
	_ = mountFS()
	p := &Platform{
		events:  make(chan gui.Event, 10),
		wakeups: make(chan struct{}, 1),
	}
	c := &p.camera
	c.frames = make(chan gui.FrameEvent)
	c.out = make(chan gui.FrameEvent)
	// Set printing temporarily during initialization to prevent debug redirection
	p.printing = true
	if initHook != nil {
		if err := initHook(p); err != nil {
			p.printing = false
			log.Printf("debug: %v", err)
			return nil, err
		}
	}
	p.printing = false // Reset after initHook
	if err := p.initSDCardNotifier(); err != nil {
		return nil, err
	}
	if err := p.initPrinterNotifier(); err != nil {
		logutil.DebugLog("Printer notifier init failed: %v", err)
	}
	if err := wshat.Open(p.events); err != nil {
		return nil, err
	}
	d, err := drm.Open()
	if err != nil {
		logutil.DebugLog("Failed to initialize display: %v; continuing in headless mode", err)
		p.display = nil // Set display to nil but proceed
	} else {
		p.display = d
		logutil.DebugLog("Display initialized successfully")
	}
	return p, nil
}

func (p *Platform) Wakeup() {
	select {
	case p.wakeups <- struct{}{}:
	default:
	}
}

func (p *Platform) PrinterStatus() (bool, string) {
	for i := 0; i < 3; i++ {
		matches, _ := filepath.Glob("/dev/usb/lp*")
		if len(matches) > 0 {
			return true, readPrinterModel()
		}
		if i < 2 {
			time.Sleep(50 * time.Millisecond)
		}
	}
	return false, ""
}

func (p *Platform) AppendEvents(deadline time.Time, evts []gui.Event) []gui.Event {
	c := &p.camera
	if c.close != nil {
		if c.frame != nil {
			c.out <- *c.frame
			c.frame = nil
		}
		if !c.active {
			c.close()
			c.close = nil
		}
		c.active = false
	}
	for {
		runtime.Gosched()
		select {
		case e := <-p.events:
			evts = append(evts, e)
		case f := <-c.frames:
			c.frame = &f
			evts = append(evts, f.Event())
		default:
			if len(evts) > 0 {
				return evts
			}
			d := time.Until(deadline)
			if p.timer == nil {
				p.timer = time.NewTimer(d)
			} else if !p.timer.Stop() {
				select {
				case <-p.timer.C:
				default:
				}
			}
			if d <= 0 {
				p.Wakeup()
			} else {
				p.timer.Reset(d)
			}
			select {
			case e := <-p.events:
				evts = append(evts, e)
			case f := <-c.frames:
				c.frame = &f
				evts = append(evts, f.Event())
			case <-p.timer.C:
				return evts
			case <-p.wakeups:
				return evts
			}
		}
	}
}

func (p *Platform) DisplaySize() image.Point {
	if p.display == nil {
		logutil.DebugLog("No display available, using default size 240x240")
		return image.Point{X: 240, Y: 240} // Default size for headless mode
	}
	return p.display.Size()
}

func (p *Platform) Dirty(r image.Rectangle) error {
	if p.display == nil {
		logutil.DebugLog("No display, skipping Dirty call")
		return nil
	}
	return p.display.Dirty(r)
}

func (p *Platform) NextChunk() (draw.RGBA64Image, bool) {
	if p.display == nil {
		logutil.DebugLog("No display, skipping NextChunk")
		return nil, false
	}
	return p.display.NextChunk()
}

var frameCounter int

func (p *Platform) ScanQR(img *image.Gray) ([][]byte, error) {
	frameCounter++
	if frameCounter%10 != 0 {
		return nil, nil
	}
	return zbar.Scan(img)
}

func (p *Platform) CameraFrame(dims image.Point) {
	c := &p.camera
	if c.close == nil {
		c.close = libcamera.Open(dims, p.camera.frames, p.camera.out)
	}
	c.active = true
}

func (p *Platform) Printer() io.Writer {
	// If we previously failed and cached a non-PCL writer, but lp0 exists now,
	// clear the cache and try again.
	if p.printerCached != nil {
		if p.supportsPCL {
			return p.printerCached
		}
		if _, err := os.Stat("/dev/usb/lp0"); err == nil {
			if f, ok := p.printerCached.(*os.File); ok && f != os.Stderr {
				_ = f.Close()
			}
			p.printerCached = nil
		} else {
			return p.printerCached
		}
	}
	// Prefer usblp (host-mode printing). Fall back to gadget serial if present.
	paths := []string{"/dev/usb/lp0", "/dev/ttyGS1"}
	for _, dev := range paths {
		printer, err := os.OpenFile(dev, os.O_RDWR, 0)
		if err != nil {
			logutil.DebugLog("Failed to open %s: %v", dev, err)
			continue
		}
		logutil.DebugLog("Opened printer device %s", dev)
		// Assume PCL-capable when using usblp.
		if strings.Contains(dev, "lp0") {
			p.supportsPCL = true
		} else {
			p.supportsPCL = false
		}
		p.supportsPostScript = false
		p.printerCached = printer
		logutil.DebugLog("Printer initialized: dev=%s PCL=%v PS=%v", dev, p.supportsPCL, p.supportsPostScript)
		return printer
	}
	p.printerCached = os.Stderr
	return p.printerCached
}

func summarizeCommandOutput(out []byte) string {
	text := strings.TrimSpace(string(out))
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	const maxLines = 24
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

func runCommandWithOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	summary := summarizeCommandOutput(out)
	if err != nil {
		if summary != "" {
			return summary, fmt.Errorf("%s %s failed: %w\n%s", name, strings.Join(args, " "), err, summary)
		}
		return "", fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
	}
	return summary, nil
}

func nixMountIsRAMBacked() bool {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return false
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		if fields[1] != "/nix" {
			continue
		}
		src, fstype := fields[0], fields[2]
		if src == "/run/hbp-ram-runtime/nix" {
			return true
		}
		if fstype == "tmpfs" {
			return true
		}
	}
	return false
}

type mmcMount struct {
	dev    string
	target string
}

func mountedMMCPartitions() ([]mmcMount, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	re := regexp.MustCompile(`^/dev/mmcblk0p[0-9]+$`)
	seen := make(map[string]struct{})
	var out []mmcMount
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		dev, target := fields[0], fields[1]
		if !re.MatchString(dev) {
			continue
		}
		key := dev + "@" + target
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, mmcMount{dev: dev, target: target})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func bindNixFromRAM() error {
	if _, err := os.Stat("/run/hbp-ram-runtime/nix"); err != nil {
		return fmt.Errorf("missing RAM nix root: %w", err)
	}
	if _, err := runCommandWithOutput("mount", "--bind", "/run/hbp-ram-runtime/nix", "/nix"); err != nil {
		return err
	}
	return nil
}

func formatMMCMounts(entries []mmcMount) string {
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		parts = append(parts, fmt.Sprintf("%s -> %s", e.dev, e.target))
	}
	return strings.Join(parts, ", ")
}

func detachMountTarget(target string) bool {
	if target == "" {
		return false
	}
	if err := unix.Unmount(target, 0); err == nil {
		return true
	}
	if err := unix.Unmount(target, unix.MNT_DETACH); err == nil {
		return true
	}
	if _, err := runCommandWithOutput("umount", target); err == nil {
		return true
	}
	if _, err := runCommandWithOutput("umount", "-l", target); err == nil {
		return true
	}
	return false
}

func detachSDCardMountsFallback(restoreRAMNix bool) error {
	syscall.Sync()
	if _, err := exec.LookPath("blockdev"); err == nil {
		_, _ = runCommandWithOutput("blockdev", "--flushbufs", "/dev/mmcblk0")
	}

	for pass := 0; pass < 6; pass++ {
		parts, err := mountedMMCPartitions()
		if err != nil {
			return fmt.Errorf("scan mmc mounts: %w", err)
		}
		if len(parts) == 0 {
			if restoreRAMNix && !nixMountIsRAMBacked() {
				if err := bindNixFromRAM(); err != nil {
					return fmt.Errorf("restore RAM /nix bind: %w", err)
				}
			}
			return nil
		}

		progress := false

		// If /nix is currently RAM-backed but a lower mmc mount still exists on /nix,
		// drop the top layer once so the lower mount can be detached.
		if nixMountIsRAMBacked() {
			for _, p := range parts {
				if p.target == "/nix" {
					if detachMountTarget("/nix") {
						logutil.DebugLog("HBP prep fallback: unmounted top /nix layer to expose lower mount")
						progress = true
					}
					break
				}
			}
		}

		parts, err = mountedMMCPartitions()
		if err != nil {
			return fmt.Errorf("scan mmc mounts: %w", err)
		}
		for _, p := range parts {
			if detachMountTarget(p.target) {
				progress = true
			}
		}

		if restoreRAMNix && !nixMountIsRAMBacked() {
			if err := bindNixFromRAM(); err == nil {
				progress = true
			}
		}

		if !progress {
			break
		}
	}

	syscall.Sync()
	remain, err := mountedMMCPartitions()
	if err != nil {
		return fmt.Errorf("scan remaining mmc mounts: %w", err)
	}
	if len(remain) > 0 {
		for _, p := range remain {
			_ = detachMountTarget(p.target)
		}
		remain, _ = mountedMMCPartitions()
		if len(remain) == 0 {
			return nil
		}
		return fmt.Errorf("fallback detach incomplete: mounted partitions remain: %s", formatMMCMounts(remain))
	}
	return nil
}

func (p *Platform) PrepareHBPForSDRemoval() error {
	if _, err := os.Stat("/bin/cups-spike-ram-feasibility"); err != nil {
		return fmt.Errorf("missing /bin/cups-spike-ram-feasibility: %w", err)
	}

	out, err := runCommandWithOutput("/bin/cups-spike-ram-feasibility", "stage", "core")
	if out != "" {
		logutil.DebugLog("HBP prep stage output:\n%s", out)
	}
	if err != nil {
		return err
	}

	out, err = runCommandWithOutput("/bin/cups-spike-ram-feasibility", "detach-sd")
	if out != "" {
		logutil.DebugLog("HBP prep detach output:\n%s", out)
	}
	if err != nil {
		msg := err.Error()
		if nixMountIsRAMBacked() && (strings.Contains(msg, "/nix is not RAM-backed") || strings.Contains(msg, "mmc partitions still mounted")) {
			logutil.DebugLog("HBP prep: detach helper failed with RAM-backed /nix, applying fallback SD detach")
			return detachSDCardMountsFallback(true)
		}
		return err
	}

	return nil
}

func (p *Platform) PrepareSDForRemoval() error {
	// PCL/PS-only flow: make SD removal safe by detaching mmc-backed mounts.
	// Best-effort stop of cupsd first so unmount can complete cleanly.
	if _, err := exec.LookPath("killall"); err == nil {
		if out, err := runCommandWithOutput("killall", "cupsd"); err == nil && out != "" {
			logutil.DebugLog("SD prep: stopped cupsd:\n%s", out)
		}
	}
	return detachSDCardMountsFallback(false)
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
		if opts.DPI > 600 {
			// Allow 1200 only for one-page jobs to avoid multi-page OOM spikes.
			if estimateJobPages(desc, paper, opts) > 1 {
				logutil.DebugLog("HBP path: forcing 600 DPI for multi-page job")
				opts.DPI = 600
			}
		}
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
		// Host-mode PCL path: render and send in page-sized batches to reduce peak RAM.
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

func (p *Platform) initSDCardNotifier() error {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC)
	if err != nil {
		return fmt.Errorf("inotify_init1: %w", err)
	}
	f := os.NewFile(uintptr(fd), "inotify")
	var flags uint32 = unix.IN_CREATE | unix.IN_DELETE
	const dev = "/dev"
	if _, err = unix.InotifyAddWatch(fd, dev, flags); err != nil {
		f.Close()
		return fmt.Errorf("inotify_add_watch: %w", err)
	}
	const sdcName = "mmcblk0"
	inserted := true
	if _, err := os.Stat(filepath.Join(dev, sdcName)); os.IsNotExist(err) {
		inserted = false
	}
	go func() {
		defer f.Close()
		p.events <- gui.SDCardEvent{
			Inserted: inserted,
		}.Event()
		var buf [(unix.SizeofInotifyEvent + unix.PathMax + 1) * 100]byte
		for {
			n, err := f.Read(buf[:])
			if err != nil {
				panic(err)
			}
			evts := buf[:n]
			for len(evts) > 0 {
				evt := (*unix.InotifyEvent)(unsafe.Pointer(&evts[0]))
				evts = evts[unix.SizeofInotifyEvent:]
				var name string
				if evt.Len > 0 {
					nameb := evts[:evt.Len-1]
					evts = evts[evt.Len:]
					nameb = bytes.TrimRight(nameb, "\000")
					name = string(nameb)
				}
				if name == sdcName {
					switch {
					case evt.Mask&unix.IN_CREATE != 0:
						p.events <- gui.SDCardEvent{Inserted: true}.Event()
					case evt.Mask&unix.IN_DELETE != 0:
						p.events <- gui.SDCardEvent{Inserted: false}.Event()
					}
				}
			}
		}
	}()
	return nil
}

func (p *Platform) initPrinterNotifier() error {
	const devDir = "/dev/usb"
	_ = os.MkdirAll(devDir, 0o755)
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return fmt.Errorf("printer inotify_init1: %w", err)
	}
	f := os.NewFile(uintptr(fd), "inotify-printer")
	var flags uint32 = unix.IN_CREATE | unix.IN_DELETE
	if _, err = unix.InotifyAddWatch(fd, devDir, flags); err != nil {
		f.Close()
		return fmt.Errorf("inotify_add_watch (%s): %w", devDir, err)
	}

	initialModel := readPrinterModel()
	initial := initialModel != ""
	go func() {
		defer f.Close()
		p.events <- gui.PrinterEvent{Connected: initial, Model: initialModel}.Event()
		p.Wakeup()
		var buf [(unix.SizeofInotifyEvent + unix.PathMax + 1) * 20]byte
		for {
			n, err := f.Read(buf[:])
			if err != nil {
				logutil.DebugLog("printer notifier read err: %v", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			evts := buf[:n]
			for len(evts) > 0 {
				evt := (*unix.InotifyEvent)(unsafe.Pointer(&evts[0]))
				evts = evts[unix.SizeofInotifyEvent:]
				var name string
				if evt.Len > 0 {
					nameb := evts[:evt.Len-1]
					evts = evts[evt.Len:]
					nameb = bytes.TrimRight(nameb, "\000")
					name = string(nameb)
				}
				if !strings.HasPrefix(name, "lp") {
					continue
				}
				switch {
				case evt.Mask&unix.IN_CREATE != 0:
					model := readPrinterModel()
					p.printerCached = nil
					p.supportsPCL = false
					logutil.DebugLog("Printer event: connected model=%s", model)
					p.events <- gui.PrinterEvent{Connected: true, Model: model}.Event()
					p.Wakeup()
				case evt.Mask&unix.IN_DELETE != 0:
					p.printerCached = nil
					p.supportsPCL = false
					logutil.DebugLog("Printer event: disconnected")
					p.events <- gui.PrinterEvent{Connected: false}.Event()
					p.Wakeup()
				}
			}
		}
	}()

	// Fallback poll in case inotify misses events.
	go func() {
		prevConnected, prevModel := initial, initialModel
		for {
			connected, model := p.PrinterStatus()
			if connected != prevConnected || (model != "" && model != prevModel) {
				prevConnected, prevModel = connected, model
				p.printerCached = nil
				p.supportsPCL = false
				logutil.DebugLog("Printer poll: connected=%v model=%s", connected, model)
				p.events <- gui.PrinterEvent{Connected: connected, Model: model}.Event()
				p.Wakeup()
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()
	return nil
}

func readPrinterModel() string {
	paths := []string{}
	if matches, err := filepath.Glob("/sys/class/usb/lp*/device/ieee1284_id"); err == nil {
		paths = append(paths, matches...)
	}
	if matches, err := filepath.Glob("/sys/bus/usb/devices/*/ieee1284_id"); err == nil {
		paths = append(paths, matches...)
	}
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			if m := parseIEEE1284(string(data)); m != "" {
				return m
			}
		}
	}
	// fallback to product string
	for _, p := range []string{"/sys/class/usb/lp0/device/product"} {
		if data, err := os.ReadFile(p); err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return ""
}

func parseIEEE1284(s string) string {
	fields := strings.Split(strings.TrimSpace(s), ";")
	var mfg, mdl string
	for _, f := range fields {
		parts := strings.SplitN(f, ":", 2)
		if len(parts) != 2 {
			continue
		}
		switch strings.ToUpper(parts[0]) {
		case "MFG":
			mfg = parts[1]
		case "MDL":
			mdl = parts[1]
		}
	}
	if mfg != "" && mdl != "" {
		return fmt.Sprintf("%s %s", mfg, mdl)
	}
	if mdl != "" {
		return mdl
	}
	return ""
}

func mountFS() error {
	devices := []struct {
		path string
		fs   string
	}{
		{"/dev", "devtmpfs"},
		{"/sys", "sysfs"},
		{"/proc", "proc"},
	}
	for _, dev := range devices {
		if err := os.MkdirAll(dev.path, 0o644); err != nil {
			return fmt.Errorf("platform: %w", err)
		}
		if err := syscall.Mount(dev.fs, dev.path, dev.fs, 0, ""); err != nil {
			return fmt.Errorf("platform: mount %s: %w", dev.path, err)
		}
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func releaseMemory() {
	runtime.GC()
	rdebug.FreeOSMemory()
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
