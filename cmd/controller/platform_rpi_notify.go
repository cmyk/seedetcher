//go:build linux && arm

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
	"seedetcher.com/gui"
	"seedetcher.com/logutil"
)

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
					p.hostPCLForce600 = false
					logutil.DebugLog("Printer event: connected model=%s", model)
					p.events <- gui.PrinterEvent{Connected: true, Model: model}.Event()
					p.Wakeup()
				case evt.Mask&unix.IN_DELETE != 0:
					p.printerCached = nil
					p.supportsPCL = false
					p.hostPCLForce600 = false
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
				p.hostPCLForce600 = false
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
