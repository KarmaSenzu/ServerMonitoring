package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrDuplicateName is returned when an INSERT/UPDATE would violate the
// projects.name UNIQUE constraint.
var ErrDuplicateName = errors.New("project: duplicate name")

// ErrNotFound is returned when an id (or name) lookup yields no rows
// or when an UPDATE/DELETE affects zero rows.
var ErrNotFound = errors.New("project: not found")

// Project is the registry shape exposed over the API.
//
// WebhookSecret is persisted but MUST NOT be returned in GET responses;
// handlers replace it with a webhook_secret_present boolean before
// serialising. The json tag uses "-" so accidental encoding paths still
// drop it.
type Project struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Domain        string    `json:"domain"`
	Port          int       `json:"port"`
	ContainerName string    `json:"container_name"`
	PM2Name       string    `json:"pm2_name"`
	TunnelService string    `json:"tunnel_service"`
	HealthURL     string    `json:"health_url"`
	Enabled       bool      `json:"enabled"`
	Tags          []string  `json:"tags"`
	Notes         string    `json:"notes"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// Wave 4 additions.
	Environment          string `json:"environment"`
	WebhookSecret        string `json:"-"`
	DeployCommand        string `json:"deploy_command"`
	DeployTimeoutSeconds int    `json:"deploy_timeout_seconds"`
	DeployWorkingDir     string `json:"deploy_working_dir"`
	DeployEnabled        bool   `json:"deploy_enabled"`
}

// ProjectEnvDevelopment, ProjectEnvStaging, ProjectEnvProduction are
// the only values accepted by Project.Environment.
const (
	ProjectEnvDevelopment = "development"
	ProjectEnvStaging     = "staging"
	ProjectEnvProduction  = "production"
)

// ProjectFilter narrows the result of List.
// Empty fields mean "no filter on this field".
type ProjectFilter struct {
	Search      string
	EnabledOnly bool
	Tag         string
	Environment string
}

// ProjectRepo persists and retrieves Project rows.
type ProjectRepo struct {
	DB *sql.DB
}

// NewProjectRepo constructs a ProjectRepo bound to the given *sql.DB.
func NewProjectRepo(db *sql.DB) *ProjectRepo {
	return &ProjectRepo{DB: db}
}

// sqliteTSLayout is the layout SQLite's datetime('now') produces:
// "2006-01-02 15:04:05".
const sqliteTSLayout = "2006-01-02 15:04:05"

// projectTimestamps maps a SQLite TEXT timestamp into a time.Time. The
// migrations write "datetime('now')" which lands in this layout. We try
// RFC3339 too, in case a row was inserted by a future code path.
func parseSQLiteTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{sqliteTSLayout, time.RFC3339, time.RFC3339Nano} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// encodeTags produces the comma-separated form persisted in the
// projects.tags column. It trims whitespace, drops empty entries, and
// removes duplicates while preserving the first-seen order.
func encodeTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return strings.Join(out, ",")
}

// decodeTags parses the comma-separated form back into a slice. Empty
// input yields an empty slice (never nil) so JSON encodes as "[]".
func decodeTags(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func enabledInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func boolFromInt(n int) bool { return n != 0 }

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "constraint failed: UNIQUE")
}

// Create inserts a new project. If p.ID is empty a UUID is generated.
// CreatedAt and UpdatedAt are set by the database. Tags are normalized
// before persistence.
func (r *ProjectRepo) Create(ctx context.Context, p Project) (Project, error) {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if strings.TrimSpace(p.Environment) == "" {
		p.Environment = ProjectEnvProduction
	}
	if p.DeployTimeoutSeconds <= 0 {
		p.DeployTimeoutSeconds = 300
	}
	tagsRaw := encodeTags(p.Tags)
	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO projects (
			id, name, description, domain, port,
			container_name, pm2_name, tunnel_service, health_url,
			enabled, tags, notes,
			environment, webhook_secret, deploy_command,
			deploy_timeout_seconds, deploy_working_dir, deploy_enabled
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		p.ID, p.Name, p.Description, p.Domain, p.Port,
		p.ContainerName, p.PM2Name, p.TunnelService, p.HealthURL,
		enabledInt(p.Enabled), tagsRaw, p.Notes,
		p.Environment, p.WebhookSecret, p.DeployCommand,
		p.DeployTimeoutSeconds, p.DeployWorkingDir, enabledInt(p.DeployEnabled),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Project{}, ErrDuplicateName
		}
		return Project{}, fmt.Errorf("project: insert: %w", err)
	}
	return r.Get(ctx, p.ID)
}

// Update replaces every persisted column on the row with id == p.ID
// (except created_at). updated_at is bumped to now. Returns ErrNotFound
// when no row matches.
func (r *ProjectRepo) Update(ctx context.Context, p Project) (Project, error) {
	if strings.TrimSpace(p.Environment) == "" {
		p.Environment = ProjectEnvProduction
	}
	if p.DeployTimeoutSeconds <= 0 {
		p.DeployTimeoutSeconds = 300
	}
	tagsRaw := encodeTags(p.Tags)
	res, err := r.DB.ExecContext(ctx, `
		UPDATE projects SET
			name = ?, description = ?, domain = ?, port = ?,
			container_name = ?, pm2_name = ?, tunnel_service = ?, health_url = ?,
			enabled = ?, tags = ?, notes = ?,
			environment = ?, webhook_secret = ?, deploy_command = ?,
			deploy_timeout_seconds = ?, deploy_working_dir = ?, deploy_enabled = ?,
			updated_at = datetime('now')
		WHERE id = ?
	`,
		p.Name, p.Description, p.Domain, p.Port,
		p.ContainerName, p.PM2Name, p.TunnelService, p.HealthURL,
		enabledInt(p.Enabled), tagsRaw, p.Notes,
		p.Environment, p.WebhookSecret, p.DeployCommand,
		p.DeployTimeoutSeconds, p.DeployWorkingDir, enabledInt(p.DeployEnabled),
		p.ID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Project{}, ErrDuplicateName
		}
		return Project{}, fmt.Errorf("project: update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Project{}, fmt.Errorf("project: rows affected: %w", err)
	}
	if n == 0 {
		return Project{}, ErrNotFound
	}
	return r.Get(ctx, p.ID)
}

// Delete removes the project with the given id.
// Returns ErrNotFound when no row matches.
func (r *ProjectRepo) Delete(ctx context.Context, id string) error {
	res, err := r.DB.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("project: delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("project: rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Get returns the project with the given id or ErrNotFound.
func (r *ProjectRepo) Get(ctx context.Context, id string) (Project, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT id, name, description, domain, port,
			container_name, pm2_name, tunnel_service, health_url,
			enabled, tags, notes, created_at, updated_at,
			environment, webhook_secret, deploy_command,
			deploy_timeout_seconds, deploy_working_dir, deploy_enabled
		FROM projects WHERE id = ?
	`, id)
	return scanProject(row)
}

// GetByName returns the project with a matching name or ErrNotFound.
func (r *ProjectRepo) GetByName(ctx context.Context, name string) (Project, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT id, name, description, domain, port,
			container_name, pm2_name, tunnel_service, health_url,
			enabled, tags, notes, created_at, updated_at,
			environment, webhook_secret, deploy_command,
			deploy_timeout_seconds, deploy_working_dir, deploy_enabled
		FROM projects WHERE name = ?
	`, name)
	return scanProject(row)
}

// Count returns the total number of projects.
func (r *ProjectRepo) Count(ctx context.Context) (int, error) {
	var n int
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects`).Scan(&n); err != nil {
		return 0, fmt.Errorf("project: count: %w", err)
	}
	return n, nil
}

// List returns projects matching the supplied filter, sorted by name ASC.
// Tag filtering is performed in memory after a SQL LIKE pre-filter so the
// comma-separated representation is interpreted token-wise.
func (r *ProjectRepo) List(ctx context.Context, filter ProjectFilter) ([]Project, error) {
	var (
		clauses []string
		args    []any
	)

	if s := strings.TrimSpace(filter.Search); s != "" {
		like := "%" + s + "%"
		clauses = append(clauses, "(name LIKE ? OR domain LIKE ? OR description LIKE ?)")
		args = append(args, like, like, like)
	}

	if filter.EnabledOnly {
		clauses = append(clauses, "enabled = 1")
	}

	if t := strings.TrimSpace(filter.Tag); t != "" {
		// Cheap pre-filter; final equality check happens in Go below.
		clauses = append(clauses, "tags LIKE ?")
		args = append(args, "%"+t+"%")
	}

	if e := strings.TrimSpace(filter.Environment); e != "" {
		clauses = append(clauses, "environment = ?")
		args = append(args, e)
	}

	q := `SELECT id, name, description, domain, port,
		container_name, pm2_name, tunnel_service, health_url,
		enabled, tags, notes, created_at, updated_at,
		environment, webhook_secret, deploy_command,
		deploy_timeout_seconds, deploy_working_dir, deploy_enabled
		FROM projects`
	if len(clauses) > 0 {
		q += " WHERE " + strings.Join(clauses, " AND ")
	}
	q += " ORDER BY name ASC"

	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("project: list: %w", err)
	}
	defer rows.Close()

	out := make([]Project, 0, 16)
	for rows.Next() {
		p, err := scanProjectRow(rows)
		if err != nil {
			return nil, err
		}
		if t := strings.TrimSpace(filter.Tag); t != "" {
			if !hasTag(p.Tags, t) {
				continue
			}
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("project: list iter: %w", err)
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanProject(row *sql.Row) (Project, error) {
	p, err := scanProjectRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, err
	}
	return p, nil
}

func scanProjectRow(s rowScanner) (Project, error) {
	var (
		p              Project
		enabled        int
		tagsRaw        string
		createdRaw     string
		updatedRaw     string
		environment    string
		webhookSecret  string
		deployCommand  string
		deployTimeout  int
		deployCwd      string
		deployEnabled  int
	)
	err := s.Scan(
		&p.ID, &p.Name, &p.Description, &p.Domain, &p.Port,
		&p.ContainerName, &p.PM2Name, &p.TunnelService, &p.HealthURL,
		&enabled, &tagsRaw, &p.Notes, &createdRaw, &updatedRaw,
		&environment, &webhookSecret, &deployCommand,
		&deployTimeout, &deployCwd, &deployEnabled,
	)
	if err != nil {
		return Project{}, err
	}
	p.Enabled = boolFromInt(enabled)
	p.Tags = decodeTags(tagsRaw)
	p.CreatedAt = parseSQLiteTime(createdRaw)
	p.UpdatedAt = parseSQLiteTime(updatedRaw)
	p.Environment = environment
	p.WebhookSecret = webhookSecret
	p.DeployCommand = deployCommand
	p.DeployTimeoutSeconds = deployTimeout
	p.DeployWorkingDir = deployCwd
	p.DeployEnabled = boolFromInt(deployEnabled)
	return p, nil
}
