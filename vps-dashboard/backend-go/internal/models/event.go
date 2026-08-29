package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Severity values accepted by the events.severity CHECK constraint.
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityError    = "error"
	SeverityCritical = "critical"
)

// Event categories used across the codebase.
const (
	EventCategoryHealth = "health"
	EventCategorySystem = "system"
	EventCategoryDocker = "docker"
	EventCategoryPM2    = "pm2"
	EventCategoryTunnel = "tunnel"
	EventCategoryAuth   = "auth"
	EventCategoryAlert  = "alert"
)

// Event is a persisted application event row.
type Event struct {
	ID        string         `json:"id"`
	Category  string         `json:"category"`
	Severity  string         `json:"severity"`
	Source    string         `json:"source"`
	ProjectID string         `json:"project_id"`
	Message   string         `json:"message"`
	Timestamp time.Time      `json:"ts"`
	Data      map[string]any `json:"data"`
}

// EventFilter narrows event listings. All fields are optional.
type EventFilter struct {
	Since     time.Time
	Until     time.Time
	Category  string
	Severity  string
	ProjectID string
	Search    string
	Limit     int
	Offset    int
}

// EventRepo persists and retrieves Event rows.
type EventRepo struct {
	DB *sql.DB
}

// NewEventRepo constructs an EventRepo bound to the given *sql.DB.
func NewEventRepo(db *sql.DB) *EventRepo {
	return &EventRepo{DB: db}
}

// Append persists ev. ID is generated when empty; Timestamp is set to
// time.Now().UTC() when zero. Data is JSON-encoded; nil maps are stored
// as the empty string.
func (r *EventRepo) Append(ctx context.Context, ev Event) (Event, error) {
	if ev.ID == "" {
		ev.ID = uuid.NewString()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	if ev.Severity == "" {
		ev.Severity = SeverityInfo
	}
	if !validSeverity(ev.Severity) {
		return Event{}, fmt.Errorf("event: invalid severity %q", ev.Severity)
	}
	if strings.TrimSpace(ev.Category) == "" {
		return Event{}, fmt.Errorf("event: category is required")
	}

	dataRaw := ""
	if len(ev.Data) > 0 {
		buf, err := json.Marshal(ev.Data)
		if err != nil {
			return Event{}, fmt.Errorf("event: marshal data: %w", err)
		}
		dataRaw = string(buf)
	}

	if _, err := r.DB.ExecContext(ctx, `
		INSERT INTO events (id, ts, category, severity, source, project_id, message, data_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		ev.ID, ev.Timestamp.UTC().Format(time.RFC3339Nano),
		ev.Category, ev.Severity, ev.Source, ev.ProjectID,
		ev.Message, dataRaw,
	); err != nil {
		return Event{}, fmt.Errorf("event: insert: %w", err)
	}
	return ev, nil
}

// List returns events ordered by ts DESC subject to the filter.
func (r *EventRepo) List(ctx context.Context, filter EventFilter) ([]Event, error) {
	q, args := buildEventQuery("SELECT id, ts, category, severity, source, project_id, message, data_json FROM events", filter, true)

	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("event: list: %w", err)
	}
	defer rows.Close()

	out := make([]Event, 0, 32)
	for rows.Next() {
		ev, err := scanEventRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("event: list iter: %w", err)
	}
	return out, nil
}

// Count returns the number of rows matching filter (limit/offset
// ignored).
func (r *EventRepo) Count(ctx context.Context, filter EventFilter) (int, error) {
	q, args := buildEventQuery("SELECT COUNT(*) FROM events", filter, false)
	var n int
	if err := r.DB.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("event: count: %w", err)
	}
	return n, nil
}

// Purge deletes events older than olderThan and returns the affected
// row count.
func (r *EventRepo) Purge(ctx context.Context, olderThan time.Time) (int, error) {
	res, err := r.DB.ExecContext(ctx,
		`DELETE FROM events WHERE ts < ?`,
		olderThan.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, fmt.Errorf("event: purge: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("event: rows affected: %w", err)
	}
	return int(n), nil
}

// buildEventQuery composes the WHERE/ORDER BY/LIMIT clauses shared by
// List and Count. When withOrderLimit is false the LIMIT/OFFSET/ORDER
// BY are omitted (suitable for COUNT).
func buildEventQuery(base string, filter EventFilter, withOrderLimit bool) (string, []any) {
	var (
		clauses []string
		args    []any
	)

	if !filter.Since.IsZero() {
		clauses = append(clauses, "ts >= ?")
		args = append(args, filter.Since.UTC().Format(time.RFC3339Nano))
	}
	if !filter.Until.IsZero() {
		clauses = append(clauses, "ts < ?")
		args = append(args, filter.Until.UTC().Format(time.RFC3339Nano))
	}
	if c := strings.TrimSpace(filter.Category); c != "" {
		clauses = append(clauses, "category = ?")
		args = append(args, c)
	}
	if s := strings.TrimSpace(filter.Severity); s != "" {
		clauses = append(clauses, "severity = ?")
		args = append(args, s)
	}
	if p := strings.TrimSpace(filter.ProjectID); p != "" {
		clauses = append(clauses, "project_id = ?")
		args = append(args, p)
	}
	if s := strings.TrimSpace(filter.Search); s != "" {
		like := "%" + s + "%"
		clauses = append(clauses, "(message LIKE ? OR source LIKE ? OR data_json LIKE ?)")
		args = append(args, like, like, like)
	}

	q := base
	if len(clauses) > 0 {
		q += " WHERE " + strings.Join(clauses, " AND ")
	}
	if withOrderLimit {
		q += " ORDER BY ts DESC"

		limit := filter.Limit
		if limit <= 0 {
			limit = 100
		}
		if limit > 1000 {
			limit = 1000
		}
		offset := filter.Offset
		if offset < 0 {
			offset = 0
		}
		q += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}
	return q, args
}

func scanEventRow(rows *sql.Rows) (Event, error) {
	var (
		ev      Event
		tsRaw   string
		dataRaw string
	)
	if err := rows.Scan(&ev.ID, &tsRaw, &ev.Category, &ev.Severity, &ev.Source, &ev.ProjectID, &ev.Message, &dataRaw); err != nil {
		return Event{}, fmt.Errorf("event: scan: %w", err)
	}
	ev.Timestamp = parseSQLiteTime(tsRaw)
	if strings.TrimSpace(dataRaw) != "" {
		if err := json.Unmarshal([]byte(dataRaw), &ev.Data); err != nil {
			// Surface invalid JSON as an empty map; do not fail the read.
			ev.Data = map[string]any{"_raw": dataRaw, "_parse_error": err.Error()}
		}
	}
	if ev.Data == nil {
		ev.Data = map[string]any{}
	}
	return ev, nil
}

func validSeverity(s string) bool {
	switch s {
	case SeverityInfo, SeverityWarning, SeverityError, SeverityCritical:
		return true
	}
	return false
}
