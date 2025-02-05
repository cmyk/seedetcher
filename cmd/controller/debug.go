//go:build debug

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime/pprof"
	"strings"
	"golang.org/x/sys/unix" // cmyk Correct import for system calls
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
	log.Printf("DEBUG: Received command: %q\n", cmd)
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
	default:
		fmt.Printf("Passing through command: %s\n", cmd)
		execCommand(cmd)
	}
	// default:
	// 	log.Printf("debug: unrecognized command: %s", cmd)
	// }
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

    // Ensure the command runs with the dynamic linker
    command := exec.Command("/lib/ld-musl-armhf.so.1", append([]string{parts[0]}, parts[1:]...)...)
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
	fmt.Println("Press Ctrl+D to exit the shell.")

	// Set PATH manually
	os.Setenv("PATH", "/bin:/usr/bin:/sbin:/usr/sbin")
	fmt.Println("PATH set to:", os.Getenv("PATH"))

	// Ensure /dev/tty exists for proper shell execution
	if _, err := os.Stat("/dev/tty"); os.IsNotExist(err) {
		fmt.Println("Creating /dev/tty for proper shell execution...")
		unix.Mknod("/dev/tty", unix.S_IFCHR|0666, int(unix.Mkdev(5, 0)))
		unix.Chmod("/dev/tty", 0666)
	}

	// Execute the shell
	cmd := exec.Command("/bin/sh", "-i")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		fmt.Println("Error starting shell:", err)
	}
	fmt.Println("Shell exited.")
}