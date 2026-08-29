package deploy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DeploymentRepo persists Deployment rows.
type DeploymentRepo struct {
	DB *sql.DB
}

// NewDeploymentRepo binds a DeploymentRepo to db.
func NewDeploymentRepo(db *sql.DB) *DeploymentRepo {
	return &DeploymentRepo{DB: db}
}

// Create inserts a pending deployment row and returns it.
func (r *DeploymentRepo) Create(ctx context.Context, d Deployment) (Deployment, error) {
	if d.TriggeredAt.IsZero() {
		d.TriggeredAt = time.Now().UTC()
	}
	if d.Status == "" {
		d.Status = StatusPending
	}
	if d.ExitCode == 0 {
		d.ExitCode = -1
	}
	if _, err := r.DB.ExecContext(ctx, `
		INSERT INTO deployments (
			id, project_id, triggered_by, triggered_at, finished_at,
			status, exit_code, stdout, stderr, remote_ref, error
		) VALUES (?, ?, ?, ?, '', ?, ?, '', '', ?, '')
	`,
		d.ID, d.ProjectID, d.TriggeredBy,
		d.TriggeredAt.UTC().Format(time.RFC3339Nano),
		d.Status, d.ExitCode, d.RemoteRef,
	); err != nil {
		return Deployment{}, fmt.Errorf("deploy: insert: %w", err)
	}
	return r.Get(ctx, d.ID)
}

// Get returns the deployment with id or ErrNotFound.
func (r *DeploymentRepo) Get(ctx context.Context, id string) (Deployment, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT id, project_id, triggered_by, triggered_at, finished_at,
			status, exit_code, stdout, stderr, remote_ref, error
		FROM deployments WHERE id = ?
	`, id)
	var (
		d                                                       Deployment
		triggeredAt, finishedAt                                 string
	)
	if err := row.Scan(
		&d.ID, &d.ProjectID, &d.TriggeredBy, &triggeredAt, &finishedAt,
		&d.Status, &d.ExitCode, &d.Stdout, &d.Stderr, &d.RemoteRef, &d.Error,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Deployment{}, ErrNotFound
		}
		return Deployment{}, fmt.Errorf("deploy: get: %w", err)
	}
	d.TriggeredAt = parseTime(triggeredAt)
	d.FinishedAt = parseTime(finishedAt)
	return d, nil
}

// List returns recent deployments for projectID ordered by triggered_at
// DESC. Limit is clamped to [1, 200].
func (r *DeploymentRepo) List(ctx context.Context, projectID string, limit int) ([]Deployment, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, project_id, triggered_by, triggered_at, finished_at,
			status, exit_code, stdout, stderr, remote_ref, error
		FROM deployments WHERE project_id = ?
		ORDER BY triggered_at DESC, id DESC
		LIMIT ?
	`, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("deploy: list: %w", err)
	}
	defer rows.Close()

	out := make([]Deployment, 0, limit)
	for rows.Next() {
		var (
			d                       Deployment
			triggeredAt, finishedAt string
		)
		if err := rows.Scan(
			&d.ID, &d.ProjectID, &d.TriggeredBy, &triggeredAt, &finishedAt,
			&d.Status, &d.ExitCode, &d.Stdout, &d.Stderr, &d.RemoteRef, &d.Error,
		); err != nil {
			return nil, fmt.Errorf("deploy: list scan: %w", err)
		}
		d.TriggeredAt = parseTime(triggeredAt)
		d.FinishedAt = parseTime(finishedAt)
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("deploy: list iter: %w", err)
	}
	return out, nil
}

// MarkStatus updates only the status column.
func (r *DeploymentRepo) MarkStatus(ctx context.Context, id, status string) error {
	if _, err := r.DB.ExecContext(ctx,
		`UPDATE deployments SET status = ? WHERE id = ?`,
		status, id,
	); err != nil {
		return fmt.Errorf("deploy: mark status: %w", err)
	}
	return nil
}

// AppendStdout appends to deployments.stdout up to outputCap. Once the
// cap is exceeded the column ends with "... [truncated]".
func (r *DeploymentRepo) AppendStdout(ctx context.Context, id, chunk string) error {
	return r.appendOutput(ctx, id, "stdout", chunk)
}

// AppendStderr appends to deployments.stderr up to outputCap.
func (r *DeploymentRepo) AppendStderr(ctx context.Context, id, chunk string) error {
	return r.appendOutput(ctx, id, "stderr", chunk)
}

func (r *DeploymentRepo) appendOutput(ctx context.Context, id, column, chunk string) error {
	if chunk == "" {
		return nil
	}
	var cur string
	row := r.DB.QueryRowContext(ctx, fmt.Sprintf(`SELECT %s FROM deployments WHERE id = ?`, column), id)
	if err := row.Scan(&cur); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("deploy: append %s: %w", column, err)
	}
	if strings.HasSuffix(cur, "... [truncated]") {
		return nil
	}
	remaining := outputCap - len(cur)
	if remaining <= 0 {
		return nil
	}
	add := chunk
	truncated := false
	if len(add) > remaining {
		add = add[:remaining]
		truncated = true
	}
	next := cur + add
	if truncated {
		next += "... [truncated]"
	}
	if _, err := r.DB.ExecContext(ctx,
		fmt.Sprintf(`UPDATE deployments SET %s = ? WHERE id = ?`, column),
		next, id,
	); err != nil {
		return fmt.Errorf("deploy: write %s: %w", column, err)
	}
	return nil
}

// Finish records the terminal status, exit code and (optional) error
// message. finishedAt defaults to time.Now().UTC().
func (r *DeploymentRepo) Finish(ctx context.Context, id, status string, exitCode int, errMsg string) error {
	if _, err := r.DB.ExecContext(ctx, `
		UPDATE deployments
		SET status = ?, exit_code = ?, error = ?, finished_at = ?
		WHERE id = ?
	`,
		status, exitCode, errMsg,
		time.Now().UTC().Format(time.RFC3339Nano),
		id,
	); err != nil {
		return fmt.Errorf("deploy: finish: %w", err)
	}
	return nil
}

// parseTime mirrors models.parseSQLiteTime without crossing packages so
// the deploy package can be tested in isolation.
func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
