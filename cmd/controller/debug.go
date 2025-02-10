//go:build debug

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime/pprof"
	"strings"
	"time"
	"golang.org/x/sys/unix" // cmyk Correct import for system calls
	//"syscall"
	"seedetcher.com/gui"
)

func init() {
	debug = true
}

func click(btn gui.Button) []gui.ButtonEvent {
	return []gui.ButtonEvent{
		{
			Button:  btn,
			Pressed: true,
		},
		{
			Button:  btn,
			Pressed: false,
		},
	}
}

func debugCommand(cmd string) []gui.ButtonEvent {
	//log.Printf("DEBUG: Received command: %q\n", cmd)

	// Ignore Ctrl+C (SIGINT) so shell handles it
	if cmd == "\x03" {
		fmt.Println("Ignoring Ctrl+C")
		return nil
	}

	var evts []gui.ButtonEvent
	switch {
	case strings.HasPrefix(cmd, "runes "):
		cmd = strings.ToUpper(cmd[len("runes "):])
		for _, r := range cmd {
			if r == ' ' {
				evts = append(evts, click(gui.Button2)...)
				continue
			}
			evts = append(evts, gui.ButtonEvent{
				Button:  gui.Rune,
				Rune:    r,
				Pressed: true,
			})
		}
		evts = append(evts, click(gui.Button2)...)
	case strings.HasPrefix(cmd, "input "):
		cmd = cmd[len("input "):]
		for _, name := range strings.Split(cmd, " ") {
			name = strings.TrimSpace(name)
			var btn gui.Button
			switch name {
			case "up":
				btn = gui.Up
			case "down":
				btn = gui.Down
			case "left":
				btn = gui.Left
			case "right":
				btn = gui.Right
			case "center":
				btn = gui.Center
			case "b1":
				btn = gui.Button1
			case "b2":
				btn = gui.Button2
			case "b3":
				btn = gui.Button3
			default:
				log.Printf("debug: unknown button: %s", name)
				continue
			}
			evts = append(evts, click(btn)...)
		}
	case cmd == "goroutines":
		pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
	case cmd == "shell":
		log.Println("Starting interactive shell...")
		startShell()
	// default:
	// 	fmt.Printf("Passing through command: %s\n", cmd)
	// 	execCommand(cmd)
	// }
	default:
		log.Printf("debug: unrecognized command: %s", cmd)
	}
	return evts
}

func execCommand(cmdStr string) {
    fmt.Printf("DEBUG: Attempting to execute: %q\n", cmdStr)

    parts := strings.Fields(cmdStr)
    if len(parts) == 0 {
        fmt.Println("No command received.")
        return
    }

    fmt.Printf("DEBUG: Command parsed as: %v\n", parts)


    // Manually prepend "/bin" if command isn't an absolute path
    if !strings.HasPrefix(parts[0], "/") {
        parts[0] = "/bin/" + parts[0]
    }


    // Ensure the command runs with the dynamic linker
    // command := exec.Command("/lib/ld-musl-armhf.so.1", append([]string{parts[0]}, parts[1:]...)...)
	// Let's NOT ;)
	command := exec.Command(parts[0], parts[1:]...)
    command.Stdout = os.Stdout
    command.Stderr = os.Stderr
    command.Env = append(os.Environ(), "PATH=/bin:/usr/bin:/sbin:/usr/sbin")

    err := command.Run()
    if err != nil {
        fmt.Printf("Error executing command: %v\n", err)
    } else {
        fmt.Println("Command executed successfully.")
    }
}

func startShell() {
    log.Println("Starting interactive shell directly...")

    // Mount essential filesystems (if needed)
    exec.Command("/bin/mount", "-t", "devtmpfs", "devtmpfs", "/dev").Run()
    exec.Command("/bin/mount", "-t", "proc", "none", "/proc").Run()
    exec.Command("/bin/mount", "-t", "sysfs", "none", "/sys").Run()
    exec.Command("/bin/mount", "-t", "tmpfs", "tmpfs", "/run").Run()
    exec.Command("/bin/mkdir", "-p", "/dev/pts").Run()
    exec.Command("/bin/mount", "-t", "devpts", "none", "/dev/pts").Run()

    // Ensure /dev/ttyGS0 exists before launching a shell 
    for i := 0; i < 10; i++ {
        if _, err := os.Stat("/dev/ttyGS0"); err == nil {
            break
        }
        log.Println("Waiting for /dev/ttyGS0...")
        time.Sleep(1 * time.Second)
    }

    // Ensure /dev/console exists
    if _, err := os.Stat("/dev/console"); os.IsNotExist(err) {
        log.Println("Creating /dev/console...")
        exec.Command("/bin/mknod", "/dev/console", "c", fmt.Sprintf("%d", 5), fmt.Sprintf("%d", 1)).Run()
        exec.Command("/bin/chmod", "622", "/dev/console").Run()
    }

    // Force $PATH for all shell commands
    os.Setenv("PATH", "/bin:/usr/bin:/sbin:/usr/sbin")

    log.Println("Shell is about to start on ttyGS0...")

    // Open /dev/ttyGS0 as the shell input/output
    tty, err := os.OpenFile("/dev/ttyGS0", os.O_RDWR, 0666)
    if err != nil {
        log.Printf("ERROR: Failed to open /dev/ttyGS0: %v\n", err)
        return
    }
    defer tty.Close()

    // **🔧 Fix: Create a new session first**
    if _, err := unix.Setsid(); err != nil {
        log.Printf("ERROR: Failed to create new session: %v\n", err)
        return
    }

    // **🔧 Fix: Set ttyGS0 as controlling terminal**
    if err := unix.IoctlSetInt(int(tty.Fd()), unix.TIOCSCTTY, 0); err != nil {
        log.Printf("ERROR: Failed to set ttyGS0 as controlling terminal: %v\n", err)
        return
    }

    // **Start the shell using ttyGS0 explicitly as its terminal**
    cmd := exec.Command("/bin/sh", "-i")
    cmd.Stdin = tty
    cmd.Stdout = tty
    cmd.Stderr = tty
    cmd.Env = append(os.Environ(), "PATH=/bin:/usr/bin:/sbin:/usr/sbin")

    if err := cmd.Run(); err != nil {
        log.Printf("ERROR: Failed to start shell: %v\n", err)
    }
}