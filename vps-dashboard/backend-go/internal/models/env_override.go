package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// EnvOverride is a per-environment configuration override.
//
// The Config map is opaque JSON; service code interprets specific keys
// (e.g. "healthcheck_multiplier", "alert_severity_floor"). Unknown keys
// are preserved on round-trip so the UI can configure forward-compatible
// settings without a schema migration.
type EnvOverride struct {
	Environment string         `json:"environment"`
	Config      map[string]any `json:"config"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// EnvOverrideRepo persists environment_overrides rows.
type EnvOverrideRepo struct {
	DB *sql.DB
}

// NewEnvOverrideRepo binds an EnvOverrideRepo to db.
func NewEnvOverrideRepo(db *sql.DB) *EnvOverrideRepo {
	return &EnvOverrideRepo{DB: db}
}

// DefaultEnvOverrides are the hard-coded defaults seeded by migration
// 006. They are also returned by EnvOverrideRepo.Get when a row is
// missing, so callers always observe a usable config.
func DefaultEnvOverrides() []EnvOverride {
	return []EnvOverride{
		{
			Environment: ProjectEnvDevelopment,
			Config: map[string]any{
				"healthcheck_multiplier": 3.0,
				"alert_severity_floor":   SeverityInfo,
			},
		},
		{
			Environment: ProjectEnvStaging,
			Config: map[string]any{
				"healthcheck_multiplier": 1.5,
				"alert_severity_floor":   SeverityWarning,
			},
		},
		{
			Environment: ProjectEnvProduction,
			Config: map[string]any{
				"healthcheck_multiplier": 1.0,
				"alert_severity_floor":   SeverityInfo,
			},
		},
	}
}

// DefaultEnvOverrideFor returns the canonical default for env, or an
// empty config when env is unknown.
func DefaultEnvOverrideFor(env string) EnvOverride {
	for _, d := range DefaultEnvOverrides() {
		if d.Environment == env {
			return d
		}
	}
	return EnvOverride{Environment: env, Config: map[string]any{}}
}

// Get returns the override for env, falling back to the hard-coded
// defaults when no row exists.
func (r *EnvOverrideRepo) Get(ctx context.Context, env string) (EnvOverride, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT environment, config_json, updated_at
		FROM environment_overrides WHERE environment = ?
	`, env)
	var (
		o          EnvOverride
		raw        string
		updatedRaw string
	)
	err := row.Scan(&o.Environment, &raw, &updatedRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultEnvOverrideFor(env), nil
	}
	if err != nil {
		return EnvOverride{}, fmt.Errorf("env_override: get: %w", err)
	}
	o.Config = decodeJSONObject(raw)
	o.UpdatedAt = parseSQLiteTime(updatedRaw)
	return o, nil
}

// List returns every persisted override ordered by environment ASC.
// Missing canonical environments are NOT synthesised — call DefaultEnvOverrides
// for that.
func (r *EnvOverrideRepo) List(ctx context.Context) ([]EnvOverride, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT environment, config_json, updated_at
		FROM environment_overrides ORDER BY environment ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("env_override: list: %w", err)
	}
	defer rows.Close()

	out := make([]EnvOverride, 0, 4)
	for rows.Next() {
		var (
			o          EnvOverride
			raw        string
			updatedRaw string
		)
		if err := rows.Scan(&o.Environment, &raw, &updatedRaw); err != nil {
			return nil, fmt.Errorf("env_override: scan: %w", err)
		}
		o.Config = decodeJSONObject(raw)
		o.UpdatedAt = parseSQLiteTime(updatedRaw)
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("env_override: list iter: %w", err)
	}
	return out, nil
}

// Upsert writes the supplied config for env. The environment value is
// validated against the canonical set so callers cannot accidentally
// add new environments.
func (r *EnvOverrideRepo) Upsert(ctx context.Context, env string, config map[string]any) (EnvOverride, error) {
	env = strings.TrimSpace(strings.ToLower(env))
	switch env {
	case ProjectEnvDevelopment, ProjectEnvStaging, ProjectEnvProduction:
	default:
		return EnvOverride{}, fmt.Errorf("env_override: invalid environment %q", env)
	}
	if config == nil {
		config = map[string]any{}
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return EnvOverride{}, fmt.Errorf("env_override: marshal: %w", err)
	}
	if _, err := r.DB.ExecContext(ctx, `
		INSERT INTO environment_overrides (environment, config_json, updated_at)
		VALUES (?, ?, datetime('now'))
		ON CONFLICT(environment) DO UPDATE SET
			config_json = excluded.config_json,
			updated_at  = datetime('now')
	`, env, string(raw)); err != nil {
		return EnvOverride{}, fmt.Errorf("env_override: upsert: %w", err)
	}
	return r.Get(ctx, env)
}
