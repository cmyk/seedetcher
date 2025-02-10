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

const dmesg = false

var screenshotCounter int

func init() {
	initHook = dbgInit
	engraverHook = func() io.ReadWriteCloser {
		return mjolnir.NewSimulator()
	}
}

func dbgInit(p *Platform) error {
	s, err := openSerial("/dev/ttyGS1")
	if err != nil {
		log.Printf("ERROR: Failed to open /dev/ttyGS1: %v", err)
		return err
	}
	log.Println("DEBUG: Successfully opened /dev/ttyGS1")
	// Redirect stderr and stdout
	unix.Dup2(int(s.Fd()), syscall.Stderr)
	unix.Dup2(int(s.Fd()), syscall.Stdout)
	go func() {
		defer s.Close()
		if err := runSerial(p, s); err != nil {
			log.Printf("DEBUG: serial communication failed: %v", err)
		}
	}()

	if dmesg {
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
		line = strings.TrimRight(line, "\r\n") // cmyk Remove both CR and LF cleanly
		if err != nil {
			fmt.Fprintf(os.Stderr, "DEBUG: Read error: %v\n", err)  // <== Add this line
			return err
		}
		
		fmt.Fprintf(os.Stderr, "DEBUG: Received line: [%s]\n", line)  // <- Add this to debug

		var binSize int64
		line = strings.TrimSpace(line)
		
		if _, err := fmt.Sscanf(line, "reload %d", &binSize); err == nil {
			binFile := "/reload-a"
			if binFile == os.Args[0] {
				binFile = "/reload-b"
			}
			if err := writeReloader(r, binFile, binSize); err != nil {
				return err
			}
			if err := syscall.Exec(binFile, []string{binFile}, nil); err != nil {
				log.Printf("Exec failed: %v", err)
				return fmt.Errorf("%s: %w", binFile, err)
			}
			continue
		}
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

func writeReloader(s io.Reader, binFile string, size int64) (ferr error) {
	bin, err := os.OpenFile(binFile, os.O_CREATE|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	defer func() {
		if err := bin.Close(); ferr == nil {
			ferr = err
		}
	}()
	_, err = io.CopyN(bin, s, size)
	return err
}

func dumpImage(name string, img image.Image) {
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		log.Printf("screenshot: failed to encode: %v", err)
		return
	}
	if err := dumpFile(name, buf); err != nil {
		log.Printf("screenshot: %s: %v", name, err)
		return
	}
	log.Printf("screenshot: dumped %s", name)
}

func dumpFile(path string, r io.Reader) (ferr error) {
	const mntDir = "/mnt"
	if err := os.MkdirAll(mntDir, 0o644); err != nil {
		return fmt.Errorf("mkdir %s: %w", mntDir, err)
	}
	if err := syscall.Mount("/dev/mmcblk0p1", mntDir, "vfat", 0, ""); err != nil {
		return fmt.Errorf("mount /dev/mmcblk0p1: %w", err)
	}
	defer func() {
		if err := syscall.Unmount(mntDir, 0); ferr == nil {
			ferr = err
		}
	}()
	path = filepath.Join(mntDir, path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o644); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); ferr == nil {
			ferr = err
		}
	}()
	_, err = io.Copy(f, r)
	return err
}

func openSerial(path string) (s *os.File, err error) {
	
	log.Printf("DEBUG: Attempting to open serial input at [%s]", path)

	s, err = os.OpenFile(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0666)
	if err != nil {
		log.Printf("ERROR: Failed to open [%s]: %v", path, err)
		return nil, err
	}
	defer func() {
		if err != nil && s != nil {
			s.Close()
		}
	}()

	
    // Skip termios settings for GS1
    if strings.Contains(path, "GS1") {
        log.Printf("WARNING: Skipping termios settings for [%s]", path)
        return s, nil
    }

	c, err := s.SyscallConn()
	if err != nil {
		log.Printf("ERROR: SyscallConn failed for [%s]: %v", path, err)
		return nil, err
	}
	var errno syscall.Errno
	err = c.Control(func(fd uintptr) {
		// Check if this device supports `ioctl`
		if _, _, errno = unix.Syscall6(unix.SYS_IOCTL, fd, uintptr(unix.TCGETS), 0, 0, 0, 0); errno != 0 {
			log.Printf("WARNING: ioctl not supported on [%s], skipping termios settings.", path)
			return
		}
		// Base settings
		cflagToUse := uint32(unix.CREAD | unix.CLOCAL | unix.CS8)
		t := unix.Termios{
			Iflag:  0, // cmyk Disable input processing
			Oflag:  0, // Disable output processing (fixes line issues)
			Cflag:  cflagToUse,
			Lflag:  0, // Disable local modes (canonical, echo, etc.)
			Ispeed: 115200,
			Ospeed: 115200,
		}
		t.Cc[unix.VMIN] = 1
		t.Cc[unix.VTIME] = 0

		if _, _, errno := unix.Syscall6(unix.SYS_IOCTL, fd, uintptr(unix.TCSETS), uintptr(unsafe.Pointer(&t)), 0, 0, 0); errno != 0 {
			log.Printf("WARNING: Failed to apply termios settings on [%s]: %v", path, errno)
			panic(errno)
		}
	})
	if err != nil {
		return nil, err
	}
	if errno != 0 {
		return nil, errno
	}

	return
}
