package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// HealthResult is one persisted health-probe outcome for a project.
type HealthResult struct {
	ID         int64     `json:"id"`
	ProjectID  string    `json:"project_id"`
	Timestamp  time.Time `json:"ts"`
	OK         bool      `json:"ok"`
	StatusCode int       `json:"status_code"`
	LatencyMs  int       `json:"latency_ms"`
	Error      string    `json:"error"`
}

// HealthRepo persists and retrieves health probe results.
type HealthRepo struct {
	DB *sql.DB
}

// NewHealthRepo constructs a HealthRepo bound to the given *sql.DB.
func NewHealthRepo(db *sql.DB) *HealthRepo {
	return &HealthRepo{DB: db}
}

// Append persists a single result. Timestamp defaults to time.Now().UTC()
// when zero.
func (r *HealthRepo) Append(ctx context.Context, h HealthResult) error {
	if h.ProjectID == "" {
		return fmt.Errorf("health: project_id required")
	}
	if h.Timestamp.IsZero() {
		h.Timestamp = time.Now().UTC()
	}
	okInt := 0
	if h.OK {
		okInt = 1
	}
	if _, err := r.DB.ExecContext(ctx, `
		INSERT INTO health_results (project_id, ts, ok, status_code, latency_ms, error)
		VALUES (?, ?, ?, ?, ?, ?)
	`, h.ProjectID, h.Timestamp.UTC().Format(time.RFC3339Nano),
		okInt, h.StatusCode, h.LatencyMs, h.Error,
	); err != nil {
		return fmt.Errorf("health: insert: %w", err)
	}
	return nil
}

// LatestByProject returns the most recent health row for a project, or
// ErrNotFound when none exist.
func (r *HealthRepo) LatestByProject(ctx context.Context, projectID string) (HealthResult, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT id, project_id, ts, ok, status_code, latency_ms, error
		FROM health_results WHERE project_id = ?
		ORDER BY ts DESC, id DESC LIMIT 1
	`, projectID)
	h, err := scanHealthRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return HealthResult{}, ErrNotFound
	}
	if err != nil {
		return HealthResult{}, err
	}
	return h, nil
}

// History returns up to limit rows for projectID since `since` (zero
// means no lower bound), ordered by ts DESC.
func (r *HealthRepo) History(ctx context.Context, projectID string, since time.Time, limit int) ([]HealthResult, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	args := []any{projectID}
	q := `
		SELECT id, project_id, ts, ok, status_code, latency_ms, error
		FROM health_results WHERE project_id = ?`
	if !since.IsZero() {
		q += ` AND ts >= ?`
		args = append(args, since.UTC().Format(time.RFC3339Nano))
	}
	q += ` ORDER BY ts DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("health: history: %w", err)
	}
	defer rows.Close()

	out := make([]HealthResult, 0, 32)
	for rows.Next() {
		h, err := scanHealthRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("health: history iter: %w", err)
	}
	return out, nil
}

// Purge deletes rows older than olderThan and returns the count.
func (r *HealthRepo) Purge(ctx context.Context, olderThan time.Time) (int, error) {
	res, err := r.DB.ExecContext(ctx,
		`DELETE FROM health_results WHERE ts < ?`,
		olderThan.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, fmt.Errorf("health: purge: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("health: rows affected: %w", err)
	}
	return int(n), nil
}

func scanHealthRow(row *sql.Row) (HealthResult, error) {
	var (
		h     HealthResult
		ok    int
		tsRaw string
	)
	if err := row.Scan(&h.ID, &h.ProjectID, &tsRaw, &ok, &h.StatusCode, &h.LatencyMs, &h.Error); err != nil {
		return HealthResult{}, err
	}
	h.OK = ok != 0
	h.Timestamp = parseSQLiteTime(tsRaw)
	return h, nil
}

func scanHealthRows(rows *sql.Rows) (HealthResult, error) {
	var (
		h     HealthResult
		ok    int
		tsRaw string
	)
	if err := rows.Scan(&h.ID, &h.ProjectID, &tsRaw, &ok, &h.StatusCode, &h.LatencyMs, &h.Error); err != nil {
		return HealthResult{}, fmt.Errorf("health: scan: %w", err)
	}
	h.OK = ok != 0
	h.Timestamp = parseSQLiteTime(tsRaw)
	return h, nil
}
