package database

import (
	"context"
	"fmt"
	"time"
)

// ConnectWithRetry attempts to connect to the database with exponential
// backoff retry. This is important for cloud databases (Supabase, Neon)
// where transient network failures shouldn't crash the application on boot.
//
// Retry schedule (5 attempts total):
//   Attempt 1: immediate
//   Attempt 2: after 1s
//   Attempt 3: after 2s
//   Attempt 4: after 4s
//   Attempt 5: after 8s (last try)
//
// If all attempts fail, the last error is returned.
func (m *Manager) ConnectWithRetry(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.db != nil {
		return fmt.Errorf("already connected")
	}

	const maxAttempts = 5
	backoff := 1 * time.Second

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Create database instance
		db, err := m.createDatabase()
		if err != nil {
			lastErr = fmt.Errorf("create database: %w", err)
		} else {
			// Try to open connection
			if err := db.Open(ctx); err != nil {
				lastErr = fmt.Errorf("open connection (attempt %d/%d): %w", attempt, maxAttempts, err)
				_ = db.Close() // Cleanup failed connection
			} else {
				// Success!
				m.adapter = m.createQueryAdapter()
				m.backup = m.createBackupManager(db)
				m.db = db
				return nil
			}
		}

		// Check context before retrying
		if ctx.Err() != nil {
			return fmt.Errorf("context cancelled: %w (last error: %v)", ctx.Err(), lastErr)
		}

		// Don't sleep after the last attempt
		if attempt < maxAttempts {
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff: 1s, 2s, 4s, 8s
		}
	}

	return fmt.Errorf("failed after %d attempts: %w", maxAttempts, lastErr)
}
