package models

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ServerDiscovery holds auto-discovered services for a remote server.
// Populated by the remote monitoring engine after each successful SSH poll.
type ServerDiscovery struct {
	ID           string `json:"id"`
	ServerID     string `json:"server_id"`
	PM2JSON      string `json:"pm2_json"`
	DockerJSON   string `json:"docker_json"`
	TunnelsJSON  string `json:"tunnels_json"`
	SystemdJSON  string `json:"systemd_json"`
	PortsJSON    string `json:"ports_json"`
	Hostname     string `json:"hostname"`
	Kernel       string `json:"kernel"`
	OSName       string `json:"os_name"`
	DiscoveredAt string `json:"discovered_at"`
}

// DiscoveryRepo provides access to the server_discoveries table.
type DiscoveryRepo struct {
	DB *sql.DB
}

// NewDiscoveryRepo creates a new discovery repository.
func NewDiscoveryRepo(db *sql.DB) *DiscoveryRepo {
	return &DiscoveryRepo{DB: db}
}

// Upsert inserts or replaces the discovery data for a server.
// There is at most one row per server (upsert by server_id).
func (r *DiscoveryRepo) Upsert(ctx context.Context, d ServerDiscovery) error {
	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	if d.DiscoveredAt == "" {
		d.DiscoveredAt = time.Now().UTC().Format(time.RFC3339)
	}

	// Delete existing row for this server, then insert new one.
	// This is simpler than UPDATE ... WHERE and works on both SQLite and Postgres.
	if _, err := r.DB.ExecContext(ctx,
		`DELETE FROM server_discoveries WHERE server_id = ?`, d.ServerID); err != nil {
		return fmt.Errorf("discovery: delete old: %w", err)
	}

	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO server_discoveries (
			id, server_id, pm2_json, docker_json, tunnels_json,
			systemd_json, ports_json, hostname, kernel, os_name, discovered_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		d.ID, d.ServerID, d.PM2JSON, d.DockerJSON, d.TunnelsJSON,
		d.SystemdJSON, d.PortsJSON, d.Hostname, d.Kernel, d.OSName, d.DiscoveredAt,
	)
	if err != nil {
		return fmt.Errorf("discovery: insert: %w", err)
	}
	return nil
}

// Get retrieves the latest discovery data for a server.
func (r *DiscoveryRepo) Get(ctx context.Context, serverID string) (*ServerDiscovery, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT id, server_id, pm2_json, docker_json, tunnels_json,
		       systemd_json, ports_json, hostname, kernel, os_name, discovered_at
		FROM server_discoveries
		WHERE server_id = ?
	`, serverID)

	var d ServerDiscovery
	err := row.Scan(
		&d.ID, &d.ServerID, &d.PM2JSON, &d.DockerJSON, &d.TunnelsJSON,
		&d.SystemdJSON, &d.PortsJSON, &d.Hostname, &d.Kernel, &d.OSName, &d.DiscoveredAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No discovery data yet
		}
		return nil, fmt.Errorf("discovery: get: %w", err)
	}
	return &d, nil
}

// DeleteAll removes all discovery data for a server (on server delete).
func (r *DiscoveryRepo) DeleteAll(ctx context.Context, serverID string) error {
	_, err := r.DB.ExecContext(ctx,
		`DELETE FROM server_discoveries WHERE server_id = ?`, serverID)
	if err != nil {
		return fmt.Errorf("discovery: delete: %w", err)
	}
	return nil
}
