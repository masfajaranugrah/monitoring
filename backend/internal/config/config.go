package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerPort     string
	DatabaseURL    string
	JWTSecret      string
	JWTExpiryHours int
	PingConcurrency int
	CheckVersion    int
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
		PingConcurrency: getEnvInt("PING_CONCURRENCY", 30),
	}
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