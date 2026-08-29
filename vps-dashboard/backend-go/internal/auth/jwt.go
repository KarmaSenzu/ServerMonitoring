package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the JWT payload used by this service.
type Claims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// Issue creates a signed HS256 JWT for the given user.
func Issue(userID, username, role string, secret []byte, ttl time.Duration) (string, error) {
	if len(secret) == 0 {
		return "", fmt.Errorf("auth: jwt secret is empty")
	}
	if ttl <= 0 {
		return "", fmt.Errorf("auth: jwt ttl must be positive")
	}

	now := time.Now()
	claims := &Claims{
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("auth: sign jwt: %w", err)
	}
	return signed, nil
}

// Parse verifies a JWT signature and expiry, returning the parsed claims.
func Parse(token string, secret []byte) (*Claims, error) {
	if token == "" {
		return nil, fmt.Errorf("auth: empty token")
	}

	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("auth: parse jwt: %w", err)
	}

	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, fmt.Errorf("auth: invalid jwt claims")
	}
	return claims, nil
}
