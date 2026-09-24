package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"

	"monitoring/internal/config"
	"monitoring/internal/database"
)

func main() {
	username := flag.String("u", "", "username (wajib)")
	password := flag.String("p", "", "password (wajib, minimal 8 karakter)")
	fullName := flag.String("n", "", "nama lengkap")
	role := flag.String("r", "OPERATOR", "role: ADMIN atau OPERATOR")
	flag.Parse()

	if *username == "" || *password == "" {
		fmt.Println(`Usage: mkuser -u <username> -p <password> [-n "Nama Lengkap"] [-r ADMIN|OPERATOR]`)
		os.Exit(1)
	}
	if len(*password) < 8 {
		log.Fatal("password minimal 8 karakter")
	}
	if *role != "ADMIN" && *role != "OPERATOR" {
		log.Fatal("role harus ADMIN atau OPERATOR")
	}

	if err := database.Connect(config.Load().DatabaseURL); err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer database.Close()

	hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("failed to hash password: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = database.Pool.Exec(ctx, `
		INSERT INTO users (username, password_hash, full_name, role, is_active)
		VALUES ($1, $2, $3, $4, true)
		ON CONFLICT (username) DO UPDATE SET
		  password_hash = EXCLUDED.password_hash,
		  full_name = EXCLUDED.full_name,
		  role = EXCLUDED.role,
		  is_active = true,
		  updated_at = now()`,
		*username, string(hash), *fullName, *role)
	if err != nil {
		log.Fatalf("failed to save user: %v", err)
	}

	fmt.Printf("OK: user %q (%s) dibuat/diupdate.\n", *username, *role)
}
