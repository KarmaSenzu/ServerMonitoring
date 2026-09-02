package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// DeriveKey derives a 32-byte AES-256 key from a secret string (typically
// JWT_SECRET) using SHA-256. This is intentionally simple — for production
// with high-value secrets, consider using HKDF or Argon2.
func DeriveKey(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// Encrypt encrypts plaintext using AES-256-GCM with the given key.
// Returns base64-encoded ciphertext (nonce prepended).
// Returns empty string if plaintext is empty (no encryption needed).
func Encrypt(plaintext string, key []byte) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "enc:" + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a base64-encoded AES-256-GCM ciphertext.
// Returns empty string if ciphertext is empty or not encrypted (no "enc:" prefix).
// If the value doesn't start with "enc:", it's treated as plaintext (backward compat).
func Decrypt(ciphertext string, key []byte) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	// Backward compatibility: values without "enc:" prefix are plaintext
	// (from before encryption was implemented). Return as-is.
	if len(ciphertext) < 4 || ciphertext[:4] != "enc:" {
		return ciphertext, nil
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext[4:])
	if err != nil {
		return "", fmt.Errorf("crypto: decode base64: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("crypto: ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("crypto: decrypt: %w", err)
	}

	return string(plaintext), nil
}

// GetEncryptionKey derives the encryption key from JWT_SECRET env var.
// If JWT_SECRET is not set, returns nil (encryption disabled — plaintext fallback).
func GetEncryptionKey() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return nil
	}
	return DeriveKey(secret)
}

// FingerprintKey returns a hex-encoded short fingerprint of the key
// for logging purposes (never log the key itself).
func FingerprintKey(key []byte) string {
	if key == nil {
		return "none"
	}
	h := sha256.Sum256(key)
	return hex.EncodeToString(h[:8])
}
