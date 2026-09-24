//go:build linux

package handlers

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
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

// Sistem berkas virtual/temporary yang tidak mewakili disk sungguhan; disk
// ini dikecualikan pada pass pertama supaya total kapasitas tidak meleset.
var virtualFS = map[string]bool{
	"proc": true, "sysfs": true, "devtmpfs": true, "tmpfs": true,
	"devpts": true, "cgroup": true, "cgroup2": true, "mqueue": true,
	"shm": true, "debugfs": true, "tracefs": true,
	"configfs": true, "securityfs": true, "pstore": true, "bpf": true,
	"rpc_pipefs": true, "hugetlbfs": true, "autofs": true, "fuse": true,
	"fusectl": true, "nsfs": true, "binfmt_misc": true, "fuse.portal": true,
}

// unescapeMountPath mengubah escape oktal (mis. "\040" untuk spasi) pada
// /proc/self/mounts menjadi karakter asli.
func unescapeMountPath(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
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

// sumMounts menjumlahkan kapasitas mount point dari /proc/self/mounts dengan
// deduplikasi per pasangan device (major:minor). Ketika allowVirtual true,
// filesystem seperti overlay/squashfs/ramfs ikut dihitung (jaring pengaman
// bila sistem berjalan di container/chroot yang semua mount-nya overlay).
func sumMounts(sc *bufio.Scanner, allowVirtual bool) (total, used uint64) {
	seen := map[[2]uint64]bool{}
	var free uint64
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 {
			continue
		}
		if !allowVirtual && virtualFS[fields[2]] {
			continue
		}
		majmin := fields[3]
		var maj, min uint64
		if _, err := fmt.Sscanf(majmin, "%d:%d", &maj, &min); err != nil {
			continue
		}
		if maj == 0 && min == 0 {
			continue
		}
		dev := [2]uint64{maj, min}
		if seen[dev] {
			continue
		}
		seen[dev] = true

		var fs unix.Statfs_t
		if err := unix.Statfs(unescapeMountPath(fields[1]), &fs); err != nil {
			continue
		}
		bsize := uint64(fs.Bsize)
		if bsize == 0 {
			bsize = 512
		}
		total += uint64(fs.Blocks) * bsize
		free += uint64(fs.Bfree) * bsize
	}
	if total > free {
		used = total - free
	}
	return total, used
}

// diskUsageAll menjumlahkan kapasitas seluruh disk sungguhan di mesin
// (direktori root + disk data/attached seperti /data atau /mnt) sehingga
// dashboard menampilkan kapasitas total server, bukan hanya partisi root.
// Berlapis: pass normal, lalu pasokan hasil pas jika nol, lalu fallback
// statfs langsung pada "/".
func diskUsageAll() (total, used uint64) {
	f, err := os.Open("/proc/self/mounts")
	if err != nil {
		return diskUsage("/")
	}
	defer f.Close()

	// Pass 1: hanya filesystem block sungguhan.
	_, _ = f.Seek(0, 0)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	total, used = sumMounts(sc, false)

	// Pass 2: kalau 0 (mis. seluruh root overlay/squashfs), hitung semua.
	if total == 0 {
		_, _ = f.Seek(0, 0)
		total, used = sumMounts(bufio.NewScanner(f), true)
	}

	// Pass 3: jaring pengaman terakhir.
	if total == 0 {
		return diskUsage("/")
	}
	return total, used
}
