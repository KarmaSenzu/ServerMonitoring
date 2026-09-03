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
	PM2JSON          string `json:"pm2_json"`
	DockerJSON       string `json:"docker_json"`
	TunnelsJSON      string `json:"tunnels_json"`
	CloudflareJSON   string `json:"cloudflare_json"`
	SystemdJSON      string `json:"systemd_json"`
	PortsJSON        string `json:"ports_json"`
	Hostname     string `json:"hostname"`
	Kernel       string `json:"kernel"`
	OSName       string `json:"os_name"`
	DiscoveredAt string `json:"discovered_at"`
	FirstSeen    string `json:"first_seen"`    // when this server was first discovered
	LastStatus    string `json:"last_status"`   // "active" or "missing"
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
// Preserves first_seen from existing row to track when the server was
// first discovered. Sets last_status to "active" on success.
func (r *DiscoveryRepo) Upsert(ctx context.Context, d ServerDiscovery) error {
	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	if d.DiscoveredAt == "" {
		d.DiscoveredAt = time.Now().UTC().Format(time.RFC3339)
	}
	d.LastStatus = "active"

	// Preserve first_seen from existing row
	existing, _ := r.Get(ctx, d.ServerID)
	if existing != nil && existing.FirstSeen != "" {
		d.FirstSeen = existing.FirstSeen
	} else if d.FirstSeen == "" {
		d.FirstSeen = d.DiscoveredAt
	}

	// Delete existing row for this server, then insert new one.
	if _, err := r.DB.ExecContext(ctx,
		`DELETE FROM server_discoveries WHERE server_id = ?`, d.ServerID); err != nil {
		return fmt.Errorf("discovery: delete old: %w", err)
	}

	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO server_discoveries (
			id, server_id, pm2_json, docker_json, tunnels_json, cloudflare_json,
			systemd_json, ports_json, hostname, kernel, os_name,
			discovered_at, first_seen, last_status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		d.ID, d.ServerID, d.PM2JSON, d.DockerJSON, d.TunnelsJSON, d.CloudflareJSON,
		d.SystemdJSON, d.PortsJSON, d.Hostname, d.Kernel, d.OSName,
		d.DiscoveredAt, d.FirstSeen, d.LastStatus,
	)
	if err != nil {
		return fmt.Errorf("discovery: insert: %w", err)
	}
	return nil
}

// Get retrieves the latest discovery data for a server.
func (r *DiscoveryRepo) Get(ctx context.Context, serverID string) (*ServerDiscovery, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT id, server_id, pm2_json, docker_json, tunnels_json, cloudflare_json,
		       systemd_json, ports_json, hostname, kernel, os_name,
		       discovered_at, first_seen, last_status
		FROM server_discoveries
		WHERE server_id = ?
	`, serverID)

	var d ServerDiscovery
	err := row.Scan(
		&d.ID, &d.ServerID, &d.PM2JSON, &d.DockerJSON, &d.TunnelsJSON, &d.CloudflareJSON,
		&d.SystemdJSON, &d.PortsJSON, &d.Hostname, &d.Kernel, &d.OSName,
		&d.DiscoveredAt, &d.FirstSeen, &d.LastStatus,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
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
