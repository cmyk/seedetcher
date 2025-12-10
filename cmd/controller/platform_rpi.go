//go:build linux && arm

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
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

// In platform_rpi.go, replace Printer function
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
func (p *Platform) CreatePlates(ctx *gui.Context, mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int) error {
	logutil.DebugLog("Entering CreatePlates with mnemonic length: %d, desc: %v, keyIdx: %d", len(mnemonic), desc != nil, keyIdx)
	printerDev := p.Printer()
	if printerDev == nil {
		logutil.DebugLog("Printer is nil")
		return fmt.Errorf("no printer available")
	}
	if p.supportsPCL {
		logutil.DebugLog("Printer acquired (PCL), preparing to write job")
	} else {
		logutil.DebugLog("Printer acquired (non-PCL), falling back to PDF path")
	}

	p.printing = true
	defer func() { p.printing = false }()

	tempFile, err := os.Create("/tmp/seedetcher-output.pdf")
	if err != nil {
		logutil.DebugLog("Failed to create temp file: %v", err)
		return err
	}
	defer tempFile.Close() // No os.Remove needed, will overwrite next run

	var mnemonics []bip39.Mnemonic
	if desc == nil {
		mnemonics = []bip39.Mnemonic{mnemonic}
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

	if p.supportsPCL {
		// Default to PCL in host mode (usblp).
		opts := printer.RasterOptions{
			DPI:    600, // Safe default for Zero; adjust if needed
			Mirror: true,
			Invert: true,
		}
		seedImgs, descImgs, err := printer.CreatePlateBitmaps(mnemonics, desc, keyIdx, opts, progress)
		if err != nil {
			return fmt.Errorf("pcl: plate bitmaps: %w", err)
		}
		pages, err := printer.ComposePages(seedImgs, descImgs, printer.PaperA4, opts.DPI, progress)
		if err != nil {
			return fmt.Errorf("pcl: compose pages: %w", err)
		}
		if err := printer.WritePCL(printerDev, pages, opts.DPI, printer.PaperA4, progress); err != nil {
			return fmt.Errorf("pcl: write: %w", err)
		}
		logutil.DebugLog("PCL write complete (pages=%d dpi=%.0f)", len(pages), opts.DPI)
		return nil
	}

	// Fallback: PDF path (gadget capture/dev)
	seedPaths, descPaths, tempDir, err := printer.CreatePlates(tempFile, mnemonics, desc, keyIdx, p.supportsPCL, p.supportsPostScript)
	if err != nil {
		logutil.DebugLog("PDF generation failed: %v", err)
		return err
	}
	logutil.DebugLog("Generated %d seed plates and %d desc plates in %s", len(seedPaths), len(descPaths), tempDir)

	if err := printer.CreatePageLayout(tempFile, tempDir, printer.PaperA4, seedPaths, descPaths); err != nil {
		logutil.DebugLog("Failed to merge PDF: %v", err)
		return err
	}

	data, err := os.ReadFile(tempFile.Name())
	if err != nil {
		logutil.DebugLog("Failed to read temp file: %v", err)
		return err
	}
	logutil.DebugLog("Merged PDF file, size: %d bytes", len(data))
	if len(data) == 0 {
		logutil.DebugLog("Merged PDF is empty")
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
