package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"monitoring/internal/config"
	"monitoring/internal/database"
)

var backupMu = make(chan struct{}, 1)

// BackupEntry mewakili satu file backup .sql di direktori backup.
type BackupEntry struct {
	File      string    `json:"file"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// BackupInfo ringkasan konfigurasi & status backup untuk tampilan frontend.
type BackupInfo struct {
	Enabled          bool   `json:"enabled"`
	IntervalHours    int    `json:"interval_hours"`
	Retention        int    `json:"retention"`
	Dir              string `json:"dir"`
	PgDumpAvailable  bool   `json:"pg_dump_available"`
	LastBackup       string `json:"last_backup,omitempty"`
	NextScheduled    string `json:"next_scheduled,omitempty"`
	Count            int    `json:"count"`
	TotalSize        int64  `json:"total_size"`
}

// findPgDump mencari binary pg_dump yang versinya kompatibel dengan server
// database (major version >= server). pg_dump lama menolak database yang lebih
// baru, jadi pilih yang cocok daripada asal PATH.
func findPgDump() string {
	serverMajor := serverMajorVersion()
	candidates := pgDumpCandidates()
	best := ""
	for _, c := range candidates {
		info, err := os.Stat(c)
		if err != nil || info.IsDir() {
			continue
		}
		major := pgDumpMajorVersion(c)
		if major == 0 {
			continue
		}
		if serverMajor == 0 || major >= serverMajor {
			return c
		}
		if best == "" {
			best = c
		}
	}
	return best
}

func pgDumpCandidates() []string {
	cands := []string{}
	if p, err := exec.LookPath("pg_dump"); err == nil {
		cands = append(cands, p)
	}
	cands = append(cands,
		"/usr/bin/pg_dump",
		"/usr/local/bin/pg_dump",
		"/opt/homebrew/bin/pg_dump",
		"/opt/homebrew/opt/postgresql@17/bin/pg_dump",
		"/opt/homebrew/opt/postgresql@16/bin/pg_dump",
		"/opt/homebrew/opt/postgresql@18/bin/pg_dump",
		"/usr/lib/postgresql/16/bin/pg_dump",
		"/usr/lib/postgresql/17/bin/pg_dump",
		"/usr/lib/postgresql/18/bin/pg_dump",
	)
	return cands
}

// serverMajorVersion membaca versi server database lewat pool yang sudah ada.
func serverMajorVersion() int {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var s string
	if err := database.Pool.QueryRow(ctx, "SHOW server_version").Scan(&s); err != nil {
		return 0
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			n = n*10 + int(s[i]-'0')
		} else {
			break
		}
	}
	return n
}

// pgDumpMajorVersion membaca major version dari "pg_dump --version".
func pgDumpMajorVersion(path string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return 0
	}
	// Output: "pg_dump (PostgreSQL) 16.15 (Homebrew)"
	s := strings.TrimSpace(string(out))
	idx := strings.LastIndex(s, ") ")
	if idx < 0 {
		return 0
	}
	ver := strings.TrimSpace(s[idx+2:])
	dot := strings.Index(ver, ".")
	if dot < 0 {
		dot = len(ver)
	}
	major := 0
	for i := 0; i < dot && i < len(ver); i++ {
		if ver[i] >= '0' && ver[i] <= '9' {
			major = major*10 + int(ver[i]-'0')
		}
	}
	return major
}

// backupDir menjamin direktori backup ada dan mengembalikan path absolutnya.
func backupDir(dir string) (string, error) {
	if dir == "" {
		dir = ".data/backups"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return "", err
	}
	return abs, nil
}

// backupFileList mendaftar semua file *.sql di direktori backup, terbaru dulu.
func backupFileList(dir string) ([]BackupEntry, error) {
	abs, err := backupDir(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	list := make([]BackupEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".sql") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		list = append(list, BackupEntry{
			File:      e.Name(),
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
		})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list, nil
}

// runBackup menjalankan pg_dump ke direktori backup. Mengembalikan nama file
// yang dibuat. Mutex sederhana mencegah dua backup berjalan bersamaan.
func runBackup(cfg *config.Config) (string, error) {
	if len(backupMu) == 1 {
		return "", fmt.Errorf("backup sedang berjalan, tunggu sebentar")
	}
	backupMu <- struct{}{}
	defer func() { <-backupMu }()

	pgDump := findPgDump()
	if pgDump == "" {
		return "", fmt.Errorf("pg_dump tidak ditemukan. Install PostgreSQL client di server")
	}

	abs, err := backupDir(cfg.BackupDir)
	if err != nil {
		return "", err
	}

	name := "monitoring_" + time.Now().Format("20060102_150405") + ".sql"
	dest := filepath.Join(abs, name)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, pgDump, "--dbname="+cfg.DatabaseURL, "--file="+dest, "--format=plain")
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(dest)
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("backup timeout (10 menit)")
		}
		msg := strings.TrimSpace(string(out))
		if len(msg) > 300 {
			msg = msg[:300] + "..."
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("pg_dump gagal: %s", msg)
	}

	// File kosong dianggap gagal.
	info, err := os.Stat(dest)
	if err != nil || info.Size() == 0 {
		_ = os.Remove(dest)
		return "", fmt.Errorf("pg_dump menghasilkan file kosong")
	}

	return name, nil
}

// pruneBackups menghapus backup tertua bila jumlahnya melebihi retention.
func pruneBackups(cfg *config.Config) {
	list, err := backupFileList(cfg.BackupDir)
	if err != nil {
		return
	}
	if cfg.BackupRetention <= 0 || len(list) <= cfg.BackupRetention {
		return
	}
	abs, _ := backupDir(cfg.BackupDir)
	for _, b := range list[cfg.BackupRetention:] {
		_ = os.Remove(filepath.Join(abs, b.File))
	}
}

// backupSummary membangun info status terbaru untuk GET /api/backups.
func backupSummary(cfg *config.Config) BackupInfo {
	list, _ := backupFileList(cfg.BackupDir)
	info := BackupInfo{
		Enabled:         cfg.BackupEnabled,
		IntervalHours:   cfg.BackupIntervalH,
		Retention:       cfg.BackupRetention,
		Dir:             cfg.BackupDir,
		PgDumpAvailable: findPgDump() != "",
		Count:           len(list),
	}
	for _, b := range list {
		info.TotalSize += b.Size
	}
	if len(list) > 0 {
		info.LastBackup = list[0].CreatedAt.UTC().Format(time.RFC3339)
		interval := time.Duration(cfg.BackupIntervalH) * time.Hour
		next := list[0].CreatedAt.Add(interval)
		if next.Before(time.Now()) {
			next = time.Now().Add(interval)
		}
		info.NextScheduled = next.UTC().Format(time.RFC3339)
	} else if cfg.BackupEnabled {
		info.NextScheduled = time.Now().Add(time.Duration(cfg.BackupIntervalH) * time.Hour).UTC().Format(time.RFC3339)
	}
	return info
}

// GetBackups menampilkan daftar backup + ringkasan konfigurasi.
func GetBackups(c *gin.Context) {
	cfg := config.Load()
	list, err := backupFileList(cfg.BackupDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal membaca direktori backup"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":  list,
		"info":  backupSummary(cfg),
	})
}

// CreateBackup memicu backup sekarang (admin only).
func CreateBackup(c *gin.Context) {
	cfg := config.Load()
	name, err := runBackup(cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	abs, _ := backupDir(cfg.BackupDir)
	info, err := os.Stat(filepath.Join(abs, name))
	size := int64(0)
	if err == nil {
		size = info.Size()
	}
	pruneBackups(cfg)
	c.JSON(http.StatusCreated, gin.H{
		"file":       name,
		"size":       size,
		"created_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// DownloadBackup mengirim file backup sebagai unduhan.
func DownloadBackup(c *gin.Context) {
	cfg := config.Load()
	name := c.Param("name")
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nama file tidak valid"})
		return
	}
	abs, err := backupDir(cfg.BackupDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Direktori backup tidak tersedia"})
		return
	}
	path := filepath.Join(abs, name)
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "Backup tidak ditemukan"})
		return
	}
	c.FileAttachment(path, name)
}

// DeleteBackup menghapus file backup (admin only).
func DeleteBackup(c *gin.Context) {
	cfg := config.Load()
	name := c.Param("name")
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nama file tidak valid"})
		return
	}
	abs, err := backupDir(cfg.BackupDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Direktori backup tidak tersedia"})
		return
	}
	path := filepath.Join(abs, name)
	if err := os.Remove(path); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Backup tidak ditemukan"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Backup dihapus"})
}

// BackupWorker menjalankan backup otomatis berkala. Dipanggil sebagai goroutine
// dari main; berjalan setiap interval & menunggu konteks dibatalkan (shutdown).
func BackupWorker(ctx context.Context) {
	cfg := config.Load()
	if !cfg.BackupEnabled {
		log.Println("[backup] otomatis dinonaktifkan (BACKUP_ENABLED=false)")
		return
	}
	if cfg.BackupIntervalH <= 0 {
		cfg.BackupIntervalH = 24
	}

	// Jalankan segera bila belum ada backup sama sekali, supaya begitu server
	// pertama kalinya menyala sudah punya baseline.
	if list, _ := backupFileList(cfg.BackupDir); len(list) == 0 {
		log.Printf("[backup] belum ada backup, membuat backup awal...")
		if name, err := runBackup(cfg); err != nil {
			log.Printf("[backup] backup awal gagal: %v", err)
		} else {
			log.Printf("[backup] backup awal dibuat: %s", name)
		}
	}

	interval := time.Duration(cfg.BackupIntervalH) * time.Hour
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if name, err := runBackup(cfg); err != nil {
				log.Printf("[backup] gagal: %v", err)
			} else {
				log.Printf("[backup] backup otomatis: %s", name)
				pruneBackups(cfg)
			}
		}
	}
}