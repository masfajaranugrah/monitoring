package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerPort      string
	DatabaseURL     string
	JWTSecret       string
	JWTExpiryHours  int
	PingConcurrency int
	PingIntervalSec int
	PingTimeoutMs   int
	PingJitterSec   int
	CheckVersion    int
	// Backup database via pg_dump (.sql).
	BackupEnabled    bool
	BackupIntervalH  int
	BackupRetention  int
	BackupDir        string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	return &Config{
		ServerPort:      getEnv("SERVER_PORT", "8080"),
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://monitor:monitor123@localhost:5432/monitoring?sslmode=disable"),
		JWTSecret:       getEnv("JWT_SECRET", "change-me-in-production"),
		JWTExpiryHours:  getEnvInt("JWT_EXPIRY_HOURS", 24),
		PingConcurrency: getEnvInt("PING_CONCURRENCY", 20),
		// Interval siklus ping global (detik). Isi 0 agar memakai interval
		// per-pelanggan (kolom ping_interval). Default 300 = 5 menit.
		PingIntervalSec: getEnvInt("PING_INTERVAL_SEC", 300),
		// Timeout per ping (ms). Isi 0 agar memakai default per-pelanggan
		// (kolom timeout_ms). Default 1000 = 1 detik.
		PingTimeoutMs: getEnvInt("PING_TIMEOUT_MS", 1000),
		// Jitter acak setelah tiap siklus (detik). 0 = otomatis interval/5.
		PingJitterSec: getEnvInt("PING_JITTER_SEC", 0),
		// Backup otomatis database via pg_dump. PING_HISTORY_RETENTION_DAYS
		// tidak dipakai untuk backup — file backup disimpan apa adanya.
		BackupEnabled:   getEnvBool("BACKUP_ENABLED", true),
		BackupIntervalH: getEnvInt("BACKUP_INTERVAL_HOURS", 24),
		BackupRetention: getEnvInt("BACKUP_RETENTION", 14),
		BackupDir:       getEnv("BACKUP_DIR", ".data/backups"),
	}
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return fallback
}