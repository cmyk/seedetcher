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
	"seedetcher.com/backup"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/driver/drm"
	"seedetcher.com/driver/libcamera"
	"seedetcher.com/driver/mjolnir"
	"seedetcher.com/driver/wshat"
	"seedetcher.com/engrave"
	"seedetcher.com/gui"
	"seedetcher.com/logutil"
	"seedetcher.com/printer"
	"seedetcher.com/zbar"
)

// Debug hooks (ensure unique per build tag if needed, but keep as is for now).
var (
	engraverHook func() io.ReadWriteCloser
	initHook     func(p *Platform) error
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
	if err := wshat.Open(p.events); err != nil {
		return nil, err
	}
	d, err := drm.Open()
	if err != nil {
		return nil, err
	}
	p.display = d
	return p, nil
}

func (p *Platform) Wakeup() {
	select {
	case p.wakeups <- struct{}{}:
	default:
	}
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
	return p.display.Size()
}

func (p *Platform) Dirty(r image.Rectangle) error {
	return p.display.Dirty(r)
}

func (p *Platform) NextChunk() (draw.RGBA64Image, bool) {
	return p.display.NextChunk()
}

func (p *Platform) PlateSizes() []backup.PlateSize {
	return []backup.PlateSize{backup.SquarePlate, backup.LargePlate}
}

func (p *Platform) EngraverParams() engrave.Params {
	return mjolnir.Params
}

func (p *Platform) Engraver() (gui.Engraver, error) {
	var dev io.ReadWriteCloser
	if engraverHook == nil {
		var err error
		dev, err = mjolnir.Open("")
		if err != nil {
			return nil, err
		}
	} else {
		dev = engraverHook()
	}
	return &engraver{dev: dev}, nil
}

type engraver struct {
	dev io.ReadWriteCloser
}

func (e *engraver) Engrave(sz backup.PlateSize, plan engrave.Plan, quit <-chan struct{}) error {
	const x = 97
	y := 0
	switch sz {
	case backup.SquarePlate:
		y = 49
	}
	mm := mjolnir.Params.Millimeter
	plan = engrave.Offset(x*mm, y*mm, plan)
	return mjolnir.Engrave(e.dev, mjolnir.Options{}, plan, quit)
}

func (e *engraver) Close() {
	e.dev.Close()
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
	logutil.DebugLog("Attempting to open /dev/ttyGS1")
	printer, err := os.OpenFile("/dev/ttyGS1", os.O_RDWR, 0) // RDWR for reading response
	if err != nil {
		logutil.DebugLog("Failed to open /dev/ttyGS1: %v, falling back to stderr", err)
		p.printerCached = os.Stderr
		return p.printerCached
	}
	logutil.DebugLog("Successfully opened /dev/ttyGS1")
	// Skip PJL query for now—assume no PCL/PS support
	p.supportsPCL = false
	p.supportsPostScript = false
	p.printerCached = printer
	logutil.DebugLog("Printer initialized without query, PCL: %v, PS: %v", p.supportsPCL, p.supportsPostScript)
	return printer
}

func (p *Platform) CreatePlates(mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int) error {
	logutil.DebugLog("Entering CreatePlates with mnemonic length: %d, desc: %v, keyIdx: %d", len(mnemonic), desc != nil, keyIdx)
	printerDev := p.Printer()
	if printerDev == nil {
		logutil.DebugLog("Printer is nil")
		return fmt.Errorf("no printer available")
	}
	logutil.DebugLog("Printer acquired, preparing to write PDF")

	p.printing = true                     // Set printing flag
	defer func() { p.printing = false }() // Reset on exit

	var buf bytes.Buffer
	seedPaths, descPaths, tempDir, err := printer.CreatePlates(&buf, []bip39.Mnemonic{mnemonic, mnemonic, mnemonic}, desc, keyIdx, p.supportsPCL, p.supportsPostScript)
	if err != nil {
		logutil.DebugLog("PDF generation failed: %v", err)
		return err
	}
	logutil.DebugLog("Generated PDF buffer, size: %d bytes", buf.Len())
	if buf.Len() > 0 {
		logutil.DebugLog("First 20 bytes of generated PDF: %x", buf.Bytes()[:min(20, buf.Len())])
	} else {
		logutil.DebugLog("Generated PDF buffer is empty (expected, as files are written to disk)")
	}
	if err := os.WriteFile("/log/debug_pdf.bin", buf.Bytes(), 0644); err != nil {
		logutil.DebugLog("Failed to write debug PDF file: %v", err)
	}

	data := make([]byte, buf.Len())
	copy(data, buf.Bytes())
	logutil.DebugLog("Copied PDF data, size: %d bytes", len(data))
	if len(data) > 0 {
		logutil.DebugLog("First 20 bytes of copied PDF data: %x", data[:min(20, len(data))])
	} else {
		logutil.DebugLog("Copied PDF data is empty (expected, as files are written to disk)")
	}
	if len(data) == 0 {
		logutil.DebugLog("Data is empty, cannot write to printer")
		return fmt.Errorf("no data to write to printer")
	}

	if err := printer.CreatePageLayout(printerDev, tempDir, printer.PaperA4, seedPaths, descPaths); err != nil { // Default to PaperA4
		logutil.DebugLog("Failed to merge PDF: %v", err)
		return err
	}

	// Clean up temp files after CreatePageLayout
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

	const chunkSize = 1024
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[i:end]
		logutil.DebugLog("Preparing chunk %d, size: %d bytes", i/chunkSize, len(chunk))
		if len(chunk) > 0 {
			logutil.DebugLog("First 20 bytes of chunk %d: %x", i/chunkSize, chunk[:min(20, len(chunk))])
		} else {
			logutil.DebugLog("Chunk %d is empty", i/chunkSize)
		}
		n, err := printerDev.Write(chunk)
		if err != nil {
			logutil.DebugLog("Write chunk %d failed: %v, wrote %d bytes", i/chunkSize, err, n)
			return err
		}
		logutil.DebugLog("Wrote chunk %d, %d bytes", i/chunkSize, n)
	}
	time.Sleep(2 * time.Second)
	return nil
}

func (p *Platform) initSDCardNotifier() error {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
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
