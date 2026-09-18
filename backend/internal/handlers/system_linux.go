//go:build linux

package handlers

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"
	"time"
)

// cpuPercent samples /proc/stat across a short interval.
func cpuPercent(ctx context.Context) float64 {
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

// memUsage returns total and used bytes from /proc/meminfo.
func memUsage() (total, used uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	var avail uint64
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
	return total, diff(total, avail)
}