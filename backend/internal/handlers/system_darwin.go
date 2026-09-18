//go:build darwin

package handlers

import (
	"context"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// cpuPercent samples `top` twice and reads the idle percentage.
func cpuPercent(ctx context.Context) float64 {
	out, err := exec.CommandContext(ctx, "top", "-l", "2", "-n", "0", "-s", "1").CombinedOutput()
	if err != nil {
		return -1
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "CPU usage:") {
			// e.g. "CPU usage: 12.34% user, 5.67% sys, 81.99% idle"
			for _, p := range strings.Split(line, ",") {
				p = strings.TrimSpace(p)
				if strings.HasSuffix(p, "idle") {
					clean := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(p, "idle", ""), "%", ""))
					if v, err := strconv.ParseFloat(clean, 64); err == nil {
						return round2(100 - v)
					}
				}
			}
		}
	}
	return -1
}

// memUsage returns total and currently-used bytes via sysctl + vm_stat.
func memUsage() (total, used uint64) {
	if t, err := unix.SysctlUint64("hw.memsize"); err == nil {
		total = t
	}

	out, err := exec.Command("vm_stat").CombinedOutput()
	if err != nil {
		return total, 0
	}

	pageSize := uint64(4096)
	freePages := uint64(0)
	for _, line := range strings.Split(string(out), "\n") {
		if idx := strings.Index(line, "page size of "); idx >= 0 {
			if v, err := strconv.ParseUint(strings.TrimSpace(line[idx+len("page size of "):]), 10, 64); err == nil {
				pageSize = v
			}
		}
		p := strings.TrimSpace(line)
		for _, label := range []string{"Pages free:", "Pages inactive:", "Pages speculative:"} {
			if !strings.HasPrefix(p, label) {
				continue
			}
			rest := strings.TrimSpace(strings.TrimPrefix(p, label))
			rest = strings.ReplaceAll(rest, ".", "")
			if v, err := strconv.ParseUint(strings.TrimSpace(rest), 10, 64); err == nil {
				freePages += v
			}
		}
	}

	avail := freePages * pageSize
	if avail > total {
		avail = total
	}
	return total, diff(total, avail)
}