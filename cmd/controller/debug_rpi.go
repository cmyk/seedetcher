//go:build debug && linux && arm

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
	"seedetcher.com/driver/mjolnir"
)

const (
	dmesgEnabled = false
	serialPath   = "/dev/ttyGS1"
)

var screenshotCounter int

func init() {
	initHook = dbgInit
	engraverHook = func() io.ReadWriteCloser {
		return mjolnir.NewSimulator()
	}
}

// dbgInit initializes serial communication.
func dbgInit(p *Platform) error {
	serial, err := openSerial(serialPath)
	if err != nil {
		log.Printf("ERROR: Failed to open %s: %v", serialPath, err)
		return err
	}
	log.Printf("DEBUG: Successfully opened %s", serialPath)

	// Redirect stderr and stdout
	unix.Dup2(int(serial.Fd()), syscall.Stderr)
	unix.Dup2(int(serial.Fd()), syscall.Stdout)

	go func() {
		defer serial.Close()
		if err := runSerial(p, serial); err != nil {
			log.Printf("DEBUG: Serial communication failed: %v", err)
		}
	}()

	// Optional kernel message logging
	if dmesgEnabled {
		kmsg, err := os.Open("/dev/kmsg")
		if err != nil {
			return err
		}
		go func() {
			defer kmsg.Close()
			io.Copy(os.Stderr, kmsg)
		}()
	}

	return nil
}

func runSerial(p *Platform, s io.Reader) error {
    r := bufio.NewReader(s)

    for {
        line, err := r.ReadString('\n')
        if err != nil {
            return err
        }
        line = strings.TrimSpace(line)

        var binSize int64
        if _, err := fmt.Sscanf(line, "reload %d", &binSize); err == nil {
            binFile := "/reload-a"
            if binFile == os.Args[0] {
                binFile = "/reload-b"
            }

            // ✅ Correctly read binary data
            binaryReader := io.LimitReader(s, binSize)
            if err := writeReloader(binaryReader, binFile, binSize); err != nil {
                return err
            }

            // ✅ Ensure we execute the new binary
            if err := syscall.Exec(binFile, []string{binFile}, nil); err != nil {
                log.Printf("Exec failed: %v", err)
                return fmt.Errorf("%s: %w", binFile, err)
            }
            continue
        }

        // ✅ Only print if it’s text (avoids printing binary junk)
        log.Printf("DEBUG: Received command: [%s]", line)

        // ✅ Handle known commands
        switch line {
        case "screenshot":
            if p.display == nil {
                break
            }
            screenshotCounter++
            name := fmt.Sprintf("screenshot%d.png", screenshotCounter)
            dumpImage(name, p.display.Framebuffer())
        default:
            for _, e := range debugCommand(line) {
                p.events <- e.Event()
            }
        }
    }
}



// takeScreenshot captures the screen.
func takeScreenshot(p *Platform) {
	if p.display == nil {
		return
	}
	screenshotCounter++
	filename := fmt.Sprintf("screenshot%d.png", screenshotCounter)
	dumpImage(filename, p.display.Framebuffer())
	log.Printf("DEBUG: Screenshot saved as %s", filename)
}

// openSerial opens and configures the serial port.
func openSerial(path string) (*os.File, error) {
	log.Printf("DEBUG: Attempting to open serial [%s]", path)

	serial, err := os.OpenFile(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0666)
	if err != nil {
		log.Printf("ERROR: Failed to open [%s]: %v", path, err)
		return nil, err
	}

	// Skip termios settings for ttyGS1
	if strings.Contains(path, "GS1") {
		log.Printf("WARNING: Skipping termios settings for [%s]", path)
		return serial, nil
	}

	if err := configureSerial(serial); err != nil {
		serial.Close()
		return nil, err
	}

	return serial, nil
}

// configureSerial applies termios settings.
func configureSerial(serial *os.File) error {
	conn, err := serial.SyscallConn()
	if err != nil {
		log.Printf("ERROR: SyscallConn failed: %v", err)
		return err
	}

	var errno syscall.Errno
	err = conn.Control(func(fd uintptr) {
		// Check ioctl support
		if _, _, errno = unix.Syscall6(unix.SYS_IOCTL, fd, uintptr(unix.TCGETS), 0, 0, 0, 0); errno != 0 {
			log.Printf("WARNING: ioctl not supported, skipping termios settings.")
			return
		}

		// Apply termios settings
		t := unix.Termios{
			Iflag:  0, // Disable input processing
			Oflag:  0, // Disable output processing
			Cflag:  unix.CREAD | unix.CLOCAL | unix.CS8,
			Lflag:  0, // Disable local modes
			Ispeed: 115200,
			Ospeed: 115200,
		}
		t.Cc[unix.VMIN] = 1
		t.Cc[unix.VTIME] = 0

		if _, _, errno = unix.Syscall6(unix.SYS_IOCTL, fd, uintptr(unix.TCSETS), uintptr(unsafe.Pointer(&t)), 0, 0, 0); errno != 0 {
			log.Printf("WARNING: Failed to apply termios settings: %v", errno)
		}
	})

	return err
}

// writeReloader writes the new binary.
func writeReloader(reader io.Reader, binFile string, size int64) error {
	bin, err := os.OpenFile(binFile, os.O_CREATE|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	defer bin.Close()
	_, err = io.CopyN(bin, reader, size)
	return err
}

// dumpImage saves an image to disk.
func dumpImage(name string, img image.Image) {
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		log.Printf("ERROR: Screenshot encode failed: %v", err)
		return
	}
	if err := dumpFile(name, buf); err != nil {
		log.Printf("ERROR: Screenshot save failed: %v", err)
	}
}

// dumpFile writes a file to disk.
func dumpFile(path string, r io.Reader) error {
	const mntDir = "/mnt"
	if err := os.MkdirAll(mntDir, 0o644); err != nil {
		return fmt.Errorf("mkdir %s: %w", mntDir, err)
	}
	if err := syscall.Mount("/dev/mmcblk0p1", mntDir, "vfat", 0, ""); err != nil {
		return fmt.Errorf("mount /dev/mmcblk0p1: %w", err)
	}
	defer syscall.Unmount(mntDir, 0)

	fullPath := filepath.Join(mntDir, path)
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, r)
	return err
}