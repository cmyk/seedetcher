//go:build linux && arm

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"seedetcher.com/logutil"
)

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
	if _, err := os.Stat("/bin/cups-spike-bootstrap"); err != nil {
		return fmt.Errorf("missing /bin/cups-spike-bootstrap: %w", err)
	}
	if _, err := os.Stat("/bin/cups-spike-ram-feasibility"); err != nil {
		return fmt.Errorf("missing /bin/cups-spike-ram-feasibility: %w", err)
	}

	out, err := runCommandWithOutput("/bin/cups-spike-bootstrap")
	if out != "" {
		logutil.DebugLog("HBP bootstrap output:\n%s", out)
	}
	if err != nil {
		return err
	}

	out, err = runCommandWithOutput("/bin/cups-spike-ram-feasibility", "stage", "core")
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
