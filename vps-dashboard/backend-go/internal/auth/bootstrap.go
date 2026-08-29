package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// ErrBootstrapPasswordRequired is returned by EnsureAdmin when the users table
// is empty and BOOTSTRAP_ADMIN_PASSWORD was not provided.
var ErrBootstrapPasswordRequired = errors.New("BOOTSTRAP_ADMIN_PASSWORD required for first run")

// EnsureAdmin creates the first admin user if no users exist yet.
// If at least one user already exists, this is a no-op.
func EnsureAdmin(ctx context.Context, db *sql.DB, username, password string, logger zerolog.Logger) error {
	username = strings.TrimSpace(username)
	if username == "" {
		username = "admin"
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return fmt.Errorf("bootstrap: count users: %w", err)
	}
	if count > 0 {
		logger.Debug().Int("users", count).Msg("admin bootstrap skipped, users already exist")
		return nil
	}

	if password == "" {
		return ErrBootstrapPasswordRequired
	}

	hash, err := Hash(password)
	if err != nil {
		return fmt.Errorf("bootstrap: hash password: %w", err)
	}

	id := uuid.NewString()
	_, err = db.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role)
		VALUES (?, ?, ?, 'admin')
	`, id, username, hash)
	if err != nil {
		return fmt.Errorf("bootstrap: insert admin: %w", err)
	}

	logger.Info().Str("username", username).Str("id", id).Msg("bootstrap admin user created")
	return nil
}
