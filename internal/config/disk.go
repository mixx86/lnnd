package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const mountInfoPath = "/proc/self/mountinfo"

// DiskDevice returns the block device name (as listed in /proc/diskstats)
// whose load the scheduler should track.
//
// When LNND_DISK_DEVICE is unset we fall back to the device holding the
// lnnd executable: it is the only path we know exists on every install,
// unlike a hard-coded name such as nvme0n1. `go run` builds into a tmpfs,
// so the data dir, where lnnd actually writes, is tried next.
func DiskDevice() (string, error) {
	if dev := os.Getenv("LNND_DISK_DEVICE"); dev != "" {
		return strings.TrimPrefix(dev, "/dev/"), nil
	}

	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	dev, exeErr := deviceOf(exe)
	if exeErr == nil {
		return dev, nil
	}

	dataDir, err := DataDir()
	if err != nil {
		return "", exeErr
	}
	dev, err = deviceOf(dataDir)
	if err != nil {
		return "", fmt.Errorf("%w; %w", exeErr, err)
	}
	return dev, nil
}

func deviceOf(path string) (string, error) {
	abs, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	path = abs

	f, err := os.Open(mountInfoPath)
	if err != nil {
		return "", fmt.Errorf("open mountinfo: %w", err)
	}
	defer func() { _ = f.Close() }()

	source, err := mountSource(bufio.NewScanner(f), path)
	if err != nil {
		return "", err
	}

	// Sources are often symlinks (/dev/mapper/luks-..., /dev/disk/by-uuid/...)
	// while /proc/diskstats only knows kernel names such as dm-0.
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil {
		return "", fmt.Errorf("resolve device %s: %w", source, err)
	}
	return filepath.Base(resolved), nil
}

// mountSource is driven by mountinfo rather than stat(2)'s st_dev because
// btrfs and overlayfs report anonymous device numbers (0:N) that do not map
// to any entry in /sys/dev/block.
func mountSource(sc *bufio.Scanner, path string) (string, error) {
	var bestMount, bestSource string
	for sc.Scan() {
		mount, source, ok := parseMountInfoLine(sc.Text())
		if !ok || !isUnder(path, mount) {
			continue
		}
		if len(mount) >= len(bestMount) {
			bestMount, bestSource = mount, source
		}
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("read mountinfo: %w", err)
	}
	if bestMount == "" {
		return "", fmt.Errorf("no mount found for %s", path)
	}
	if !strings.HasPrefix(bestSource, "/dev/") {
		return "", fmt.Errorf(
			"%s is on %s (%s), not a block device; set LNND_DISK_DEVICE",
			path, bestMount, bestSource,
		)
	}
	return bestSource, nil
}

// parseMountInfoLine follows proc(5): the optional fields before the "-"
// separator vary in number, so the source is located relative to it.
func parseMountInfoLine(line string) (mount, source string, ok bool) {
	fields := strings.Fields(line)
	sep := -1
	for i, f := range fields {
		if f == "-" {
			sep = i
			break
		}
	}
	if sep < 5 || sep+2 >= len(fields) {
		return "", "", false
	}
	return unescapeOctal(fields[4]), unescapeOctal(fields[sep+2]), true
}

// unescapeOctal undoes the kernel's \ooo escaping of spaces, tabs and
// backslashes in mountinfo paths.
func unescapeOctal(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+4 <= len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isUnder(path, mount string) bool {
	if mount == "/" {
		return true
	}
	return path == mount || strings.HasPrefix(path, mount+"/")
}
