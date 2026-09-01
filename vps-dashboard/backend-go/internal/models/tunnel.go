package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Tunnel types (§22).
const (
	TunnelTypeLocal  = "local"
	TunnelTypeRemote = "remote"
	TunnelTypeSocks  = "socks"
)

// Tunnel statuses.
const (
	TunnelStopped     = "stopped"
	TunnelConnecting  = "connecting"
	TunnelActive      = "active"
	TunnelError       = "error"
)

var (
	ErrTunnelNotFound      = errors.New("tunnel: not found")
	ErrTunnelDuplicateName = errors.New("tunnel: duplicate name")
)

// Tunnel is a persistent port-forwarding definition (§22).
type Tunnel struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	ServerID   string    `json:"server_id"`
	Type       string    `json:"type"`
	LocalAddr  string    `json:"local_addr"`
	RemoteAddr string    `json:"remote_addr"`
	AutoStart  bool      `json:"auto_start"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	StartedBy  string    `json:"started_by"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TunnelRepo persists and retrieves Tunnel rows.
type TunnelRepo struct {
	DB *sql.DB
}

func NewTunnelRepo(db *sql.DB) *TunnelRepo {
	return &TunnelRepo{DB: db}
}

const tunnelColumns = `id, name, server_id, type, local_addr, remote_addr,
	auto_start, status, started_at, started_by, error, created_at, updated_at`

func (r *TunnelRepo) Create(ctx context.Context, t Tunnel) (Tunnel, error) {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	if t.Status == "" {
		t.Status = TunnelStopped
	}
	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO tunnels (id, name, server_id, type, local_addr, remote_addr,
			auto_start, status, started_at, started_by, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.Name, t.ServerID, t.Type, t.LocalAddr, t.RemoteAddr,
		enabledInt(t.AutoStart), t.Status,
		lastSeenRaw(t.StartedAt), t.StartedBy, t.Error)
	if err != nil {
		if isUniqueViolation(err) {
			return Tunnel{}, ErrTunnelDuplicateName
		}
		return Tunnel{}, fmt.Errorf("tunnel: insert: %w", err)
	}
	return r.Get(ctx, t.ID)
}

func (r *TunnelRepo) Update(ctx context.Context, t Tunnel) (Tunnel, error) {
	res, err := r.DB.ExecContext(ctx, `
		UPDATE tunnels SET
			name = ?, server_id = ?, type = ?, local_addr = ?, remote_addr = ?,
			auto_start = ?, updated_at = datetime('now')
		WHERE id = ?
	`, t.Name, t.ServerID, t.Type, t.LocalAddr, t.RemoteAddr,
		enabledInt(t.AutoStart), t.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return Tunnel{}, ErrTunnelDuplicateName
		}
		return Tunnel{}, fmt.Errorf("tunnel: update: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Tunnel{}, ErrTunnelNotFound
	}
	return r.Get(ctx, t.ID)
}

func (r *TunnelRepo) Delete(ctx context.Context, id string) error {
	res, err := r.DB.ExecContext(ctx, `DELETE FROM tunnels WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("tunnel: delete: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrTunnelNotFound
	}
	return nil
}

func (r *TunnelRepo) Get(ctx context.Context, id string) (Tunnel, error) {
	row := r.DB.QueryRowContext(ctx, `SELECT `+tunnelColumns+` FROM tunnels WHERE id = ?`, id)
	t, err := scanTunnel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Tunnel{}, ErrTunnelNotFound
	}
	return t, err
}

func (r *TunnelRepo) List(ctx context.Context) ([]Tunnel, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT `+tunnelColumns+` FROM tunnels ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("tunnel: list: %w", err)
	}
	defer rows.Close()
	out := make([]Tunnel, 0, 16)
	for rows.Next() {
		t, err := scanTunnel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *TunnelRepo) ListByServer(ctx context.Context, serverID string) ([]Tunnel, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT `+tunnelColumns+` FROM tunnels WHERE server_id = ? ORDER BY name ASC`, serverID)
	if err != nil {
		return nil, fmt.Errorf("tunnel: list by server: %w", err)
	}
	defer rows.Close()
	out := make([]Tunnel, 0, 8)
	for rows.Next() {
		t, err := scanTunnel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetStatus updates the status/error/started_at/started_by columns.
func (r *TunnelRepo) SetStatus(ctx context.Context, id, status, startedBy, errMsg string, startedAt time.Time) error {
	res, err := r.DB.ExecContext(ctx, `
		UPDATE tunnels SET
			status = ?, started_by = ?, error = ?, started_at = ?, updated_at = datetime('now')
		WHERE id = ?
	`, status, startedBy, errMsg, lastSeenRaw(startedAt), id)
	if err != nil {
		return fmt.Errorf("tunnel: set status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrTunnelNotFound
	}
	return nil
}

// ListAutoStart returns tunnels with auto_start enabled.
func (r *TunnelRepo) ListAutoStart(ctx context.Context) ([]Tunnel, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT `+tunnelColumns+` FROM tunnels WHERE auto_start = 1 ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("tunnel: list autostart: %w", err)
	}
	defer rows.Close()
	out := make([]Tunnel, 0, 8)
	for rows.Next() {
		t, err := scanTunnel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func scanTunnel(s rowScanner) (Tunnel, error) {
	var (
		t          Tunnel
		autoStart  int
		startedRaw string
		createdRaw string
		updatedRaw string
	)
	err := s.Scan(&t.ID, &t.Name, &t.ServerID, &t.Type, &t.LocalAddr, &t.RemoteAddr,
		&autoStart, &t.Status, &startedRaw, &t.StartedBy, &t.Error,
		&createdRaw, &updatedRaw)
	if err != nil {
		return Tunnel{}, err
	}
	t.AutoStart = boolFromInt(autoStart)
	t.StartedAt = parseSQLiteTime(startedRaw)
	t.CreatedAt = parseSQLiteTime(createdRaw)
	t.UpdatedAt = parseSQLiteTime(updatedRaw)
	return t, nil
}
