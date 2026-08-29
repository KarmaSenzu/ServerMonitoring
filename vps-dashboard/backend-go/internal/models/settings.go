package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SettingsRepo is a tiny key/value store backed by the settings table.
type SettingsRepo struct {
	DB *sql.DB
}

// NewSettingsRepo constructs a SettingsRepo bound to the given *sql.DB.
func NewSettingsRepo(db *sql.DB) *SettingsRepo {
	return &SettingsRepo{DB: db}
}

// Get returns the value for key. A missing key returns "" without error.
func (r *SettingsRepo) Get(ctx context.Context, key string) (string, error) {
	var v string
	err := r.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("settings: get %q: %w", key, err)
	}
	return v, nil
}

// Set upserts the value for key.
func (r *SettingsRepo) Set(ctx context.Context, key, value string) error {
	if _, err := r.DB.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = datetime('now')
	`, key, value); err != nil {
		return fmt.Errorf("settings: set %q: %w", key, err)
	}
	return nil
}

// Delete removes the setting if present. Missing keys return nil.
func (r *SettingsRepo) Delete(ctx context.Context, key string) error {
	if _, err := r.DB.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key); err != nil {
		return fmt.Errorf("settings: delete %q: %w", key, err)
	}
	return nil
}
