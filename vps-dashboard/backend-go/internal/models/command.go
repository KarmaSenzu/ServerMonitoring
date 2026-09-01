package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Command snippet danger levels (§19 Blast Radius Protection).
const (
	DangerSafe      = "safe"
	DangerCaution   = "caution"
	DangerDangerous = "dangerous"
)

// Command run statuses.
const (
	RunRunning = "running"
	RunSuccess = "success"
	RunFailed  = "failed"
	RunTimeout = "timeout"
	RunError   = "error"
)

var (
	ErrSnippetNotFound      = errors.New("snippet: not found")
	ErrSnippetDuplicateName = errors.New("snippet: duplicate name")
)

// CommandSnippet is a reusable command template (§20).
type CommandSnippet struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Command     string    `json:"command"`
	Variables   []string `json:"variables"`
	DangerLevel string    `json:"danger_level"`
	CreatedBy   string    `json:"created_by"`
	UpdatedBy   string    `json:"updated_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CommandRun is a single per-host execution audit record (§17).
type CommandRun struct {
	ID         string    `json:"id"`
	SnippetID  string    `json:"snippet_id"`
	ServerID   string    `json:"server_id"`
	ServerName string    `json:"server_name"`
	UserID     string    `json:"user_id"`
	Command    string    `json:"command"`
	ExitCode   int       `json:"exit_code"`
	Stdout     string    `json:"stdout"`
	Stderr     string    `json:"stderr"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMs int64     `json:"duration_ms"`
}

// CommandSnippetRepo persists and retrieves command snippets.
type CommandSnippetRepo struct {
	DB *sql.DB
}

func NewCommandSnippetRepo(db *sql.DB) *CommandSnippetRepo {
	return &CommandSnippetRepo{DB: db}
}

// Create inserts a new snippet.
func (r *CommandSnippetRepo) Create(ctx context.Context, s CommandSnippet) (CommandSnippet, error) {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.DangerLevel == "" {
		s.DangerLevel = DangerSafe
	}
	varsJSON, _ := json.Marshal(s.Variables)
	if _, err := r.DB.ExecContext(ctx, `
		INSERT INTO command_snippets (id, name, description, command, variables_json, danger_level, created_by, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, s.ID, s.Name, s.Description, s.Command, string(varsJSON), s.DangerLevel, s.CreatedBy, s.UpdatedBy); err != nil {
		if isUniqueViolation(err) {
			return CommandSnippet{}, ErrSnippetDuplicateName
		}
		return CommandSnippet{}, fmt.Errorf("snippet: insert: %w", err)
	}
	return r.Get(ctx, s.ID)
}

// Update replaces editable fields.
func (r *CommandSnippetRepo) Update(ctx context.Context, s CommandSnippet) (CommandSnippet, error) {
	if s.DangerLevel == "" {
		s.DangerLevel = DangerSafe
	}
	varsJSON, _ := json.Marshal(s.Variables)
	res, err := r.DB.ExecContext(ctx, `
		UPDATE command_snippets SET
			name = ?, description = ?, command = ?, variables_json = ?,
			danger_level = ?, updated_by = ?, updated_at = datetime('now')
		WHERE id = ?
	`, s.Name, s.Description, s.Command, string(varsJSON), s.DangerLevel, s.UpdatedBy, s.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return CommandSnippet{}, ErrSnippetDuplicateName
		}
		return CommandSnippet{}, fmt.Errorf("snippet: update: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return CommandSnippet{}, ErrSnippetNotFound
	}
	return r.Get(ctx, s.ID)
}

// Delete removes a snippet.
func (r *CommandSnippetRepo) Delete(ctx context.Context, id string) error {
	res, err := r.DB.ExecContext(ctx, `DELETE FROM command_snippets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("snippet: delete: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSnippetNotFound
	}
	return nil
}

// Get returns a snippet by ID.
func (r *CommandSnippetRepo) Get(ctx context.Context, id string) (CommandSnippet, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT id, name, description, command, variables_json, danger_level,
			created_by, updated_by, created_at, updated_at
		FROM command_snippets WHERE id = ?
	`, id)
	s, err := scanSnippet(row)
	if errors.Is(err, sql.ErrNoRows) {
		return CommandSnippet{}, ErrSnippetNotFound
	}
	return s, err
}

// List returns all snippets ordered by name.
func (r *CommandSnippetRepo) List(ctx context.Context) ([]CommandSnippet, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, name, description, command, variables_json, danger_level,
			created_by, updated_by, created_at, updated_at
		FROM command_snippets ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("snippet: list: %w", err)
	}
	defer rows.Close()
	out := make([]CommandSnippet, 0, 16)
	for rows.Next() {
		s, err := scanSnippet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CommandRunRepo persists per-host execution records.
type CommandRunRepo struct {
	DB *sql.DB
}

func NewCommandRunRepo(db *sql.DB) *CommandRunRepo {
	return &CommandRunRepo{DB: db}
}

// Append persists a completed command run.
func (r *CommandRunRepo) Append(ctx context.Context, run CommandRun) (CommandRun, error) {
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}
	if run.FinishedAt.IsZero() {
		run.FinishedAt = time.Now().UTC()
	}
	if _, err := r.DB.ExecContext(ctx, `
		INSERT INTO command_runs (
			id, snippet_id, server_id, server_name, user_id, command,
			exit_code, stdout, stderr, status, started_at, finished_at, duration_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, run.ID, run.SnippetID, run.ServerID, run.ServerName, run.UserID, run.Command,
		run.ExitCode, run.Stdout, run.Stderr, run.Status,
		run.StartedAt.UTC().Format(time.RFC3339Nano),
		run.FinishedAt.UTC().Format(time.RFC3339Nano),
		run.DurationMs,
	); err != nil {
		return CommandRun{}, fmt.Errorf("run: insert: %w", err)
	}
	return run, nil
}

// History returns recent command runs, optionally filtered by server.
func (r *CommandRunRepo) History(ctx context.Context, serverID string, limit int) ([]CommandRun, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	q := `SELECT id, snippet_id, server_id, server_name, user_id, command,
			exit_code, stdout, stderr, status, started_at, finished_at, duration_ms
		FROM command_runs`
	var args []any
	if serverID != "" {
		q += " WHERE server_id = ?"
		args = append(args, serverID)
	}
	q += " ORDER BY started_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("run: history: %w", err)
	}
	defer rows.Close()
	out := make([]CommandRun, 0, limit)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func scanSnippet(s rowScanner) (CommandSnippet, error) {
	var (
		snippet  CommandSnippet
		varsJSON string
		createdRaw string
		updatedRaw string
	)
	err := s.Scan(&snippet.ID, &snippet.Name, &snippet.Description, &snippet.Command,
		&varsJSON, &snippet.DangerLevel, &snippet.CreatedBy, &snippet.UpdatedBy,
		&createdRaw, &updatedRaw)
	if err != nil {
		return CommandSnippet{}, err
	}
	snippet.CreatedAt = parseSQLiteTime(createdRaw)
	snippet.UpdatedAt = parseSQLiteTime(updatedRaw)
	_ = json.Unmarshal([]byte(varsJSON), &snippet.Variables)
	if snippet.Variables == nil {
		snippet.Variables = []string{}
	}
	return snippet, nil
}

func scanRun(s rowScanner) (CommandRun, error) {
	var (
		run         CommandRun
		startedRaw  string
		finishedRaw string
	)
	err := s.Scan(&run.ID, &run.SnippetID, &run.ServerID, &run.ServerName, &run.UserID,
		&run.Command, &run.ExitCode, &run.Stdout, &run.Stderr, &run.Status,
		&startedRaw, &finishedRaw, &run.DurationMs)
	if err != nil {
		return CommandRun{}, err
	}
	run.StartedAt = parseSQLiteTime(startedRaw)
	run.FinishedAt = parseSQLiteTime(finishedRaw)
	return run, nil
}

// strings guard.
var _ = strings.TrimSpace
