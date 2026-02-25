//go:build linux && arm

package main

import (
	"fmt"
	"image"
	"image/draw"
	"io"
	"log"
	"os"
	"runtime"
	rdebug "runtime/debug"
	"seedetcher.com/driver/drm"
	"seedetcher.com/driver/libcamera"
	"seedetcher.com/driver/wshat"
	"seedetcher.com/gui"
	"seedetcher.com/logutil"
	"seedetcher.com/zbar"
	"strings"
	"syscall"
	"time"
)

// Debug hooks (ensure unique per build tag if needed, but keep as is for now).
var (
	initHook func(p *Platform) error
)

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
	printerCached   io.Writer
	supportsPCL     bool
	hostPCLForce600 bool
	printing        bool // Add flag to track printing state
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
	// usblp can disconnect/re-enumerate between jobs; always reopen fresh in host mode
	// to avoid writing through a stale file descriptor.
	if p.printerCached != nil && p.supportsPCL {
		if f, ok := p.printerCached.(*os.File); ok && f != os.Stderr {
			_ = f.Close()
		}
		p.printerCached = nil
	}

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
		p.printerCached = printer
		logutil.DebugLog("Printer initialized: dev=%s PCL=%v", dev, p.supportsPCL)
		return printer
	}
	p.printerCached = os.Stderr
	return p.printerCached
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

func releaseMemory() {
	runtime.GC()
	rdebug.FreeOSMemory()
}
