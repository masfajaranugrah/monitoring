package handlers

import (
	"context"
	"math"
	"net/http"
	"os"
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
}

// GetSystemStats returns live CPU, memory and disk usage of the host server.
func GetSystemStats(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	sw := &SystemUsage{}
	sw.Hostname, _ = os.Hostname()

	sw.CPUPercent = cpuPercent(ctx)
	sw.MemTotal, sw.MemUsed = memUsage()
	sw.MemPercent = percent(sw.MemUsed, sw.MemTotal)
	sw.DiskTotal, sw.DiskUsed = diskUsageAll()
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

// diskUsage returns total and used bytes for the given mount point.
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
