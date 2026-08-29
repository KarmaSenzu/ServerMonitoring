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

// ErrChannelNotFound is returned by ChannelRepo when no row matches.
var ErrChannelNotFound = errors.New("channel: not found")

// ErrDuplicateChannelName is returned when a UNIQUE(name) violation occurs.
var ErrDuplicateChannelName = errors.New("channel: duplicate name")

// ChannelTypeTelegram is the only supported channel type today.
const ChannelTypeTelegram = "telegram"

// Channel is a notification destination configuration row.
type Channel struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Name      string         `json:"name"`
	Enabled   bool           `json:"enabled"`
	Config    map[string]any `json:"config"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// ChannelRepo persists and retrieves Channel rows.
type ChannelRepo struct {
	DB *sql.DB
}

// NewChannelRepo constructs a ChannelRepo bound to the given *sql.DB.
func NewChannelRepo(db *sql.DB) *ChannelRepo {
	return &ChannelRepo{DB: db}
}

// Create inserts a new channel. ID is generated when empty. Config is
// JSON-encoded.
func (r *ChannelRepo) Create(ctx context.Context, ch Channel) (Channel, error) {
	if err := validateChannel(ch); err != nil {
		return Channel{}, err
	}
	if ch.ID == "" {
		ch.ID = uuid.NewString()
	}
	cfgRaw, err := encodeChannelConfig(ch.Config)
	if err != nil {
		return Channel{}, err
	}
	if _, err := r.DB.ExecContext(ctx, `
		INSERT INTO notification_channels (id, type, name, enabled, config_json)
		VALUES (?, ?, ?, ?, ?)
	`, ch.ID, ch.Type, ch.Name, enabledInt(ch.Enabled), cfgRaw); err != nil {
		if isUniqueViolation(err) {
			return Channel{}, ErrDuplicateChannelName
		}
		return Channel{}, fmt.Errorf("channel: insert: %w", err)
	}
	return r.Get(ctx, ch.ID)
}

// Get returns the channel with the given id or ErrChannelNotFound.
func (r *ChannelRepo) Get(ctx context.Context, id string) (Channel, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT id, type, name, enabled, config_json, created_at, updated_at
		FROM notification_channels WHERE id = ?
	`, id)
	ch, err := scanChannel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Channel{}, ErrChannelNotFound
	}
	if err != nil {
		return Channel{}, err
	}
	return ch, nil
}

// List returns all channels ordered by name ASC.
func (r *ChannelRepo) List(ctx context.Context) ([]Channel, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, type, name, enabled, config_json, created_at, updated_at
		FROM notification_channels ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("channel: list: %w", err)
	}
	defer rows.Close()

	out := make([]Channel, 0, 8)
	for rows.Next() {
		ch, err := scanChannelRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("channel: list iter: %w", err)
	}
	return out, nil
}

// Update writes name/type/enabled/config back to the row identified by
// ch.ID. Returns ErrChannelNotFound when no row matches.
func (r *ChannelRepo) Update(ctx context.Context, ch Channel) (Channel, error) {
	if err := validateChannel(ch); err != nil {
		return Channel{}, err
	}
	cfgRaw, err := encodeChannelConfig(ch.Config)
	if err != nil {
		return Channel{}, err
	}
	res, err := r.DB.ExecContext(ctx, `
		UPDATE notification_channels SET
			type = ?, name = ?, enabled = ?, config_json = ?,
			updated_at = datetime('now')
		WHERE id = ?
	`, ch.Type, ch.Name, enabledInt(ch.Enabled), cfgRaw, ch.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return Channel{}, ErrDuplicateChannelName
		}
		return Channel{}, fmt.Errorf("channel: update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Channel{}, fmt.Errorf("channel: rows affected: %w", err)
	}
	if n == 0 {
		return Channel{}, ErrChannelNotFound
	}
	return r.Get(ctx, ch.ID)
}

// Delete removes a channel by id. Returns ErrChannelNotFound when no row
// matches.
func (r *ChannelRepo) Delete(ctx context.Context, id string) error {
	res, err := r.DB.ExecContext(ctx, `DELETE FROM notification_channels WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("channel: delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("channel: rows affected: %w", err)
	}
	if n == 0 {
		return ErrChannelNotFound
	}
	return nil
}

// EnableDisable toggles the enabled flag without touching other fields.
func (r *ChannelRepo) EnableDisable(ctx context.Context, id string, enabled bool) error {
	res, err := r.DB.ExecContext(ctx, `
		UPDATE notification_channels SET enabled = ?, updated_at = datetime('now') WHERE id = ?
	`, enabledInt(enabled), id)
	if err != nil {
		return fmt.Errorf("channel: enable_disable: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("channel: rows affected: %w", err)
	}
	if n == 0 {
		return ErrChannelNotFound
	}
	return nil
}

func validateChannel(ch Channel) error {
	if strings.TrimSpace(ch.Name) == "" {
		return fmt.Errorf("channel: name required")
	}
	switch ch.Type {
	case ChannelTypeTelegram:
	default:
		return fmt.Errorf("channel: unsupported type %q", ch.Type)
	}
	if ch.Type == ChannelTypeTelegram {
		token, _ := ch.Config["bot_token"].(string)
		chatID, _ := ch.Config["chat_id"].(string)
		if strings.TrimSpace(token) == "" {
			return fmt.Errorf("channel: telegram bot_token required")
		}
		if strings.TrimSpace(chatID) == "" {
			return fmt.Errorf("channel: telegram chat_id required")
		}
		if pm, ok := ch.Config["parse_mode"]; ok {
			s, _ := pm.(string)
			if s != "" && s != "HTML" && s != "Markdown" {
				return fmt.Errorf("channel: telegram parse_mode must be HTML or Markdown")
			}
		}
	}
	return nil
}

func encodeChannelConfig(cfg map[string]any) (string, error) {
	if cfg == nil {
		return "{}", nil
	}
	buf, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("channel: marshal config: %w", err)
	}
	return string(buf), nil
}

func decodeChannelConfig(raw string) map[string]any {
	out := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{"_raw": raw, "_parse_error": err.Error()}
	}
	return out
}

func scanChannel(row *sql.Row) (Channel, error) {
	var (
		ch         Channel
		enabled    int
		cfgRaw     string
		createdRaw string
		updatedRaw string
	)
	if err := row.Scan(&ch.ID, &ch.Type, &ch.Name, &enabled, &cfgRaw, &createdRaw, &updatedRaw); err != nil {
		return Channel{}, err
	}
	ch.Enabled = boolFromInt(enabled)
	ch.Config = decodeChannelConfig(cfgRaw)
	ch.CreatedAt = parseSQLiteTime(createdRaw)
	ch.UpdatedAt = parseSQLiteTime(updatedRaw)
	return ch, nil
}

func scanChannelRow(rows *sql.Rows) (Channel, error) {
	var (
		ch         Channel
		enabled    int
		cfgRaw     string
		createdRaw string
		updatedRaw string
	)
	if err := rows.Scan(&ch.ID, &ch.Type, &ch.Name, &enabled, &cfgRaw, &createdRaw, &updatedRaw); err != nil {
		return Channel{}, fmt.Errorf("channel: scan: %w", err)
	}
	ch.Enabled = boolFromInt(enabled)
	ch.Config = decodeChannelConfig(cfgRaw)
	ch.CreatedAt = parseSQLiteTime(createdRaw)
	ch.UpdatedAt = parseSQLiteTime(updatedRaw)
	return ch, nil
}
