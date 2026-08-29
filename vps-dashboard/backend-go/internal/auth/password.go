package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is fixed at 12 to match the project's security baseline.
const bcryptCost = 12

// Hash returns a bcrypt hash of the given plaintext password.
func Hash(plain string) (string, error) {
	if plain == "" {
		return "", fmt.Errorf("auth: cannot hash empty password")
	}
	out, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("auth: bcrypt hash: %w", err)
	}
	return string(out), nil
}

// Verify reports whether plain matches the bcrypt hash.
// It returns false on any comparison error (including malformed hashes).
func Verify(hash, plain string) bool {
	if hash == "" || plain == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
