package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
)

var secretKey []byte

// Init derives the AES-256 encryption key from the ENCRYPTION_KEY env var.
// The raw value is SHA-256 hashed so any length/format works; in production
// use a long random string (e.g. openssl rand -base64 48).
func Init() error {
	raw := os.Getenv("ENCRYPTION_KEY")
	if raw == "" {
		raw = "dev-encryption-key-change-me-in-production"
	}
	hash := sha256.Sum256([]byte(raw))
	secretKey = hash[:]
	return nil
}

// Encrypt encrypts a plaintext string with AES-GCM and returns base64 output.
func Encrypt(plaintext string) (string, error) {
	if secretKey == nil {
		if err := Init(); err != nil {
			return "", err
		}
	}
	if secretKey == nil {
		return "", errors.New("encryption key not initialized")
	}
	block, err := aes.NewCipher(secretKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt reverses Encrypt.
func Decrypt(encoded string) (string, error) {
	if secretKey == nil {
		if err := Init(); err != nil {
			return "", err
		}
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(secretKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}