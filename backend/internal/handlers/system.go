package handlers

import (
	"bufio"
	"context"
	"math"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/sys/unix"
)

// SystemUsage aggregates host resource usage for the sidebar panel.
type SystemUsage struct {
	CPUPercent  float64 `json:"cpu_percent"`
	MemTotal    uint64  `json:"mem_total"`
	MemUsed     uint64  `json:"mem_used"`
	MemPercent  float64 `json:"mem_percent"`
	DiskTotal   uint64  `json:"disk_total"`
	DiskUsed    uint64  `json:"disk_used"`
	DiskPercent float64 `json:"disk_percent"`
	Hostname    string  `json:"hostname"`
	Platform    string  `json:"platform"`
}

// GetSystemStats returns live CPU, memory and disk usage of the host server.
func GetSystemStats(c *gin.Context) {
	sw := &SystemUsage{
		Platform: runtime.GOOS,
	}
	sw.Hostname, _ = os.Hostname()

	ctx := c.Request.Context()

	switch runtime.GOOS {
	case "linux":
		sw.CPUPercent = linuxCPUPercent(ctx)
		total, avail := linuxMem()
		sw.MemTotal = total
		sw.MemUsed = diff(total, avail)
	case "darwin":
		sw.CPUPercent = darwinCPUPercent(ctx)
		total, avail := darwinMem()
		sw.MemTotal = total
		sw.MemUsed = diff(total, avail)
	default:
		sw.CPUPercent = -1
	}

	sw.MemPercent = percent(sw.MemUsed, sw.MemTotal)
	sw.DiskTotal, sw.DiskUsed = diskUsage("/")
	sw.DiskPercent = percent(sw.DiskUsed, sw.DiskTotal)

	c.JSON(http.StatusOK, sw)
}

func percent(part, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return round2(float64(part) * 100 / float64(total))
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func diff(total, avail uint64) uint64 {
	if total > avail {
		return total - avail
	}
	return 0
}

// ---- CPU ----

func readCPUTimes() (busy, total uint64, ok bool) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)[1:]
		var totalT, idle uint64
		for i, v := range fields {
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				continue
			}
			totalT += n
			if i == 3 || i == 4 { // idle + iowait
				idle += n
			}
		}
		if totalT == 0 {
			return 0, 0, false
		}
		return totalT - idle, totalT, true
	}
	return 0, 0, false
}

func linuxCPUPercent(ctx context.Context) float64 {
	b1, t1, ok := readCPUTimes()
	if !ok {
		return -1
	}
	select {
	case <-ctx.Done():
		return -1
	case <-time.After(150 * time.Millisecond):
	}
	b2, t2, ok := readCPUTimes()
	if !ok || t2 <= t1 {
		return -1
	}
	return round2(float64(b2-b1) * 100 / float64(t2-t1))
}

func darwinCPUPercent(ctx context.Context) float64 {
	out, err := exec.CommandContext(ctx, "top", "-l", "2", "-n", "0", "-s", "1").CombinedOutput()
	if err != nil {
		return -1
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "CPU usage:") {
			// e.g. "CPU usage: 12.34% user, 5.67% sys, 81.99% idle"
			parts := strings.Split(line, ",")
			used := 0.0
			found := false
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if strings.HasSuffix(p, "idle") {
					clean := strings.ReplaceAll(strings.ReplaceAll(p, "idle", ""), "%", "")
					if v, err := strconv.ParseFloat(strings.TrimSpace(clean), 64); err == nil {
						used = 100 - v
						found = true
					}
					break
				}
			}
			if found {
				return round2(used)
			}
		}
	}
	return -1
}

// ---- Memory ----

func linuxMem() (total, avail uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		n, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = n * 1024
		case "MemAvailable:":
			avail = n * 1024
		}
		if total > 0 && avail > 0 {
			break
		}
	}
	return total, avail
}

func darwinMem() (total, avail uint64) {
	if t, err := unix.SysctlUint64("hw.memsize"); err == nil {
		total = t
	}

	out, err := exec.Command("vm_stat").CombinedOutput()
	if err != nil {
		return total, 0
	}

	pageSize := uint64(4096)
	freePages := uint64(0)
	parse := func(label string) uint64 {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, label) {
				rest := strings.TrimSpace(strings.TrimPrefix(line, label))
				rest = strings.ReplaceAll(rest, ".", "")
				if v, err := strconv.ParseUint(strings.TrimSpace(rest), 10, 64); err == nil {
					return v
				}
			}
		}
		return 0
	}
	if ps, err := strconv.ParseUint(strings.TrimSpace(rawPageSize(string(out))), 10, 64); err == nil {
		pageSize = ps
	}
	freePages = parse("Pages free:") + parse("Pages inactive:") + parse("Pages speculative:")
	avail = freePages * pageSize
	if avail > total {
		avail = total
	}
	return total, avail
}

func rawPageSize(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Mach Virtual Memory Statistics:") {
			continue
		}
		idx := strings.Index(line, "page size of ")
		if idx >= 0 {
			return strings.TrimSpace(line[idx+len("page size of "):])
		}
	}
	return "4096"
}

// ---- Disk ----

func diskUsage(path string) (total, used uint64) {
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		return 0, 0
	}
	bsize := uint64(fs.Bsize)
	if bsize == 0 {
		bsize = 512
	}
	total = uint64(fs.Blocks) * bsize
	free := uint64(fs.Bfree) * bsize
	if total > free {
		used = total - free
	}
	return total, used
}