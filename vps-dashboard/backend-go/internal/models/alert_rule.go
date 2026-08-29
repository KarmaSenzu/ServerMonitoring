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

// ErrAlertRuleNotFound is returned when no rule matches a lookup.
var ErrAlertRuleNotFound = errors.New("alert_rule: not found")

// ErrDuplicateAlertRuleName is returned when name UNIQUE is violated.
var ErrDuplicateAlertRuleName = errors.New("alert_rule: duplicate name")

// Alert rule type identifiers.
const (
	AlertTypeSystemCPU      = "system_cpu"
	AlertTypeSystemMemory   = "system_memory"
	AlertTypeSystemDisk     = "system_disk"
	AlertTypeProjectHealth  = "project_health"
	AlertTypeContainerState = "container_state"
	AlertTypeTunnelState    = "tunnel_state"
)

// Alert rule comparators.
const (
	ComparatorGTE = "gte"
	ComparatorLTE = "lte"
	ComparatorEQ  = "eq"
	ComparatorNEQ = "neq"
)

// AlertRule is a configurable threshold/state-change rule.
type AlertRule struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Enabled         bool           `json:"enabled"`
	Type            string         `json:"type"`
	Threshold       float64        `json:"threshold"`
	Comparator      string         `json:"comparator"`
	ForSeconds      int            `json:"for_seconds"`
	CooldownSeconds int            `json:"cooldown_seconds"`
	Severity        string         `json:"severity"`
	Channels        []string       `json:"channels"`
	Scope           map[string]any `json:"scope"`
	LastTriggeredAt time.Time      `json:"last_triggered_at"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// Validate enforces the data invariants documented for AlertRule.
func (a *AlertRule) Validate() error {
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("alert_rule: name required")
	}
	switch a.Type {
	case AlertTypeSystemCPU, AlertTypeSystemMemory, AlertTypeSystemDisk,
		AlertTypeProjectHealth, AlertTypeContainerState, AlertTypeTunnelState:
	default:
		return fmt.Errorf("alert_rule: invalid type %q", a.Type)
	}
	switch a.Comparator {
	case ComparatorGTE, ComparatorLTE, ComparatorEQ, ComparatorNEQ:
	default:
		return fmt.Errorf("alert_rule: invalid comparator %q", a.Comparator)
	}
	if !validSeverity(a.Severity) {
		return fmt.Errorf("alert_rule: invalid severity %q", a.Severity)
	}
	switch a.Type {
	case AlertTypeSystemCPU, AlertTypeSystemMemory, AlertTypeSystemDisk:
		if a.Threshold < 0 || a.Threshold > 100 {
			return fmt.Errorf("alert_rule: threshold must be 0..100 for %s", a.Type)
		}
	}
	if a.ForSeconds < 0 || a.ForSeconds > 86400 {
		return fmt.Errorf("alert_rule: for_seconds out of range")
	}
	if a.CooldownSeconds < 0 || a.CooldownSeconds > 86400 {
		return fmt.Errorf("alert_rule: cooldown_seconds out of range")
	}
	if len(a.Channels) == 0 {
		return fmt.Errorf("alert_rule: at least one channel required")
	}
	for _, c := range a.Channels {
		if strings.TrimSpace(c) == "" {
			return fmt.Errorf("alert_rule: channel id is empty")
		}
	}
	return nil
}

// AlertRepo persists alert rule rows.
type AlertRepo struct {
	DB *sql.DB
}

// NewAlertRepo constructs an AlertRepo bound to the given *sql.DB.
func NewAlertRepo(db *sql.DB) *AlertRepo {
	return &AlertRepo{DB: db}
}

// Create inserts a rule. ID is generated when empty.
func (r *AlertRepo) Create(ctx context.Context, a AlertRule) (AlertRule, error) {
	if err := a.Validate(); err != nil {
		return AlertRule{}, err
	}
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	channelsRaw, err := json.Marshal(a.Channels)
	if err != nil {
		return AlertRule{}, fmt.Errorf("alert_rule: marshal channels: %w", err)
	}
	scope := a.Scope
	if scope == nil {
		scope = map[string]any{}
	}
	scopeRaw, err := json.Marshal(scope)
	if err != nil {
		return AlertRule{}, fmt.Errorf("alert_rule: marshal scope: %w", err)
	}
	if _, err := r.DB.ExecContext(ctx, `
		INSERT INTO alert_rules (
			id, name, enabled, type, threshold, comparator,
			for_seconds, cooldown_seconds, severity, channels_json, scope_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, a.ID, a.Name, enabledInt(a.Enabled), a.Type, a.Threshold, a.Comparator,
		a.ForSeconds, a.CooldownSeconds, a.Severity, string(channelsRaw), string(scopeRaw),
	); err != nil {
		if isUniqueViolation(err) {
			return AlertRule{}, ErrDuplicateAlertRuleName
		}
		return AlertRule{}, fmt.Errorf("alert_rule: insert: %w", err)
	}
	return r.Get(ctx, a.ID)
}

// Get returns a rule by id or ErrAlertRuleNotFound.
func (r *AlertRepo) Get(ctx context.Context, id string) (AlertRule, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT id, name, enabled, type, threshold, comparator,
			for_seconds, cooldown_seconds, severity, channels_json, scope_json,
			last_triggered_at, created_at, updated_at
		FROM alert_rules WHERE id = ?
	`, id)
	a, err := scanAlertRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AlertRule{}, ErrAlertRuleNotFound
	}
	if err != nil {
		return AlertRule{}, err
	}
	return a, nil
}

// List returns rules ordered by name ASC.
func (r *AlertRepo) List(ctx context.Context) ([]AlertRule, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, name, enabled, type, threshold, comparator,
			for_seconds, cooldown_seconds, severity, channels_json, scope_json,
			last_triggered_at, created_at, updated_at
		FROM alert_rules ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("alert_rule: list: %w", err)
	}
	defer rows.Close()

	out := make([]AlertRule, 0, 8)
	for rows.Next() {
		a, err := scanAlertRuleRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("alert_rule: list iter: %w", err)
	}
	return out, nil
}

// Update replaces every column except last_triggered_at and timestamps.
func (r *AlertRepo) Update(ctx context.Context, a AlertRule) (AlertRule, error) {
	if err := a.Validate(); err != nil {
		return AlertRule{}, err
	}
	channelsRaw, err := json.Marshal(a.Channels)
	if err != nil {
		return AlertRule{}, fmt.Errorf("alert_rule: marshal channels: %w", err)
	}
	scope := a.Scope
	if scope == nil {
		scope = map[string]any{}
	}
	scopeRaw, err := json.Marshal(scope)
	if err != nil {
		return AlertRule{}, fmt.Errorf("alert_rule: marshal scope: %w", err)
	}
	res, err := r.DB.ExecContext(ctx, `
		UPDATE alert_rules SET
			name = ?, enabled = ?, type = ?, threshold = ?, comparator = ?,
			for_seconds = ?, cooldown_seconds = ?, severity = ?,
			channels_json = ?, scope_json = ?, updated_at = datetime('now')
		WHERE id = ?
	`, a.Name, enabledInt(a.Enabled), a.Type, a.Threshold, a.Comparator,
		a.ForSeconds, a.CooldownSeconds, a.Severity, string(channelsRaw), string(scopeRaw),
		a.ID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return AlertRule{}, ErrDuplicateAlertRuleName
		}
		return AlertRule{}, fmt.Errorf("alert_rule: update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return AlertRule{}, fmt.Errorf("alert_rule: rows affected: %w", err)
	}
	if n == 0 {
		return AlertRule{}, ErrAlertRuleNotFound
	}
	return r.Get(ctx, a.ID)
}

// Delete removes a rule by id.
func (r *AlertRepo) Delete(ctx context.Context, id string) error {
	res, err := r.DB.ExecContext(ctx, `DELETE FROM alert_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("alert_rule: delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("alert_rule: rows affected: %w", err)
	}
	if n == 0 {
		return ErrAlertRuleNotFound
	}
	return nil
}

// UpdateLastTriggered records a successful firing time.
func (r *AlertRepo) UpdateLastTriggered(ctx context.Context, id string, t time.Time) error {
	res, err := r.DB.ExecContext(ctx, `
		UPDATE alert_rules SET last_triggered_at = ?, updated_at = datetime('now') WHERE id = ?
	`, t.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("alert_rule: update last_triggered: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("alert_rule: rows affected: %w", err)
	}
	if n == 0 {
		return ErrAlertRuleNotFound
	}
	return nil
}

func scanAlertRule(row *sql.Row) (AlertRule, error) {
	var (
		a            AlertRule
		enabled      int
		channelsRaw  string
		scopeRaw     string
		lastRaw      string
		createdRaw   string
		updatedRaw   string
	)
	if err := row.Scan(
		&a.ID, &a.Name, &enabled, &a.Type, &a.Threshold, &a.Comparator,
		&a.ForSeconds, &a.CooldownSeconds, &a.Severity, &channelsRaw, &scopeRaw,
		&lastRaw, &createdRaw, &updatedRaw,
	); err != nil {
		return AlertRule{}, err
	}
	hydrateAlertRule(&a, enabled, channelsRaw, scopeRaw, lastRaw, createdRaw, updatedRaw)
	return a, nil
}

func scanAlertRuleRow(rows *sql.Rows) (AlertRule, error) {
	var (
		a            AlertRule
		enabled      int
		channelsRaw  string
		scopeRaw     string
		lastRaw      string
		createdRaw   string
		updatedRaw   string
	)
	if err := rows.Scan(
		&a.ID, &a.Name, &enabled, &a.Type, &a.Threshold, &a.Comparator,
		&a.ForSeconds, &a.CooldownSeconds, &a.Severity, &channelsRaw, &scopeRaw,
		&lastRaw, &createdRaw, &updatedRaw,
	); err != nil {
		return AlertRule{}, fmt.Errorf("alert_rule: scan: %w", err)
	}
	hydrateAlertRule(&a, enabled, channelsRaw, scopeRaw, lastRaw, createdRaw, updatedRaw)
	return a, nil
}

func hydrateAlertRule(a *AlertRule, enabled int, channelsRaw, scopeRaw, lastRaw, createdRaw, updatedRaw string) {
	a.Enabled = boolFromInt(enabled)
	a.Channels = decodeStringList(channelsRaw)
	a.Scope = decodeJSONObject(scopeRaw)
	a.LastTriggeredAt = parseSQLiteTime(lastRaw)
	a.CreatedAt = parseSQLiteTime(createdRaw)
	a.UpdatedAt = parseSQLiteTime(updatedRaw)
}

func decodeStringList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func decodeJSONObject(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	return out
}
