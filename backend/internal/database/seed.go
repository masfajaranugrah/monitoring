package database

import (
	"context"
	"log"
	"os"

	"golang.org/x/crypto/bcrypt"
)

// SeedAdmin creates the default admin user if it does not exist.
// Credentials come from ADMIN_INITIAL_USERNAME / ADMIN_INITIAL_PASSWORD env vars.
func SeedAdmin(ctx context.Context) error {
	username := os.Getenv("ADMIN_INITIAL_USERNAME")
	if username == "" {
		username = "admin"
	}
	password := os.Getenv("ADMIN_INITIAL_PASSWORD")
	if password == "" {
		password = "admin123"
	}

	var exists bool
	err := Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)`, username).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = Pool.Exec(ctx,
		`INSERT INTO users (username, password_hash, full_name, role)
		 VALUES ($1, $2, 'System Administrator', 'ADMIN')
		 ON CONFLICT (username) DO NOTHING`,
		username, string(hash))
	if err != nil {
		return err
	}

	log.Printf("Default admin user '%s' created. Change the password after first login.", username)
	return nil
}