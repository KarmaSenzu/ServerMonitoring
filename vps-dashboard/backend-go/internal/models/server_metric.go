package models

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ServerMetric is a single timestamped sample collected from a remote
// server via the SSH engine. It mirrors the shape of sysinfo.Sample but
// is persisted (remote servers don't share the in-memory ring buffer).
type ServerMetric struct {
	ID           string  `json:"id"`
	ServerID     string  `json:"server_id"`
	Timestamp    time.Time `json:"ts"`
	CPUUsage     float64 `json:"cpu_usage"`
	CPULoad1     float64 `json:"cpu_load1"`
	CPULoad5     float64 `json:"cpu_load5"`
	CPULoad15    float64 `json:"cpu_load15"`
	MemTotal     float64 `json:"mem_total"`
	MemUsed      float64 `json:"mem_used"`
	MemPercent   float64 `json:"mem_percent"`
	DiskTotal    float64 `json:"disk_total"`
	DiskUsed     float64 `json:"disk_used"`
	DiskPercent  float64 `json:"disk_percent"`
	NetBytesSent float64 `json:"net_bytes_sent"`
	NetBytesRecv float64 `json:"net_bytes_recv"`
	Uptime       float64 `json:"uptime"`
	Error        string  `json:"error,omitempty"`
	// RawStdout holds the raw SSH command output. Not persisted — used
	// by the engine to extract system info (OS, architecture) after
	// collection so it can auto-populate the server registry.
	RawStdout    string  `json:"-"`
}

// ServerMetricRepo persists and retrieves ServerMetric rows.
type ServerMetricRepo struct {
	DB *sql.DB
}

// NewServerMetricRepo constructs a ServerMetricRepo.
func NewServerMetricRepo(db *sql.DB) *ServerMetricRepo {
	return &ServerMetricRepo{DB: db}
}

// Append persists a metric sample. ID and Timestamp are defaulted when
// empty/zero.
func (r *ServerMetricRepo) Append(ctx context.Context, m ServerMetric) (ServerMetric, error) {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	if m.Timestamp.IsZero() {
		m.Timestamp = time.Now().UTC()
	}
	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO server_metrics (
			id, server_id, ts,
			cpu_usage, cpu_load1, cpu_load5, cpu_load15,
			mem_total, mem_used, mem_percent,
			disk_total, disk_used, disk_percent,
			net_bytes_sent, net_bytes_recv, uptime, error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		m.ID, m.ServerID, m.Timestamp.UTC().Format(time.RFC3339Nano),
		m.CPUUsage, m.CPULoad1, m.CPULoad5, m.CPULoad15,
		m.MemTotal, m.MemUsed, m.MemPercent,
		m.DiskTotal, m.DiskUsed, m.DiskPercent,
		m.NetBytesSent, m.NetBytesRecv, m.Uptime, m.Error,
	)
	if err != nil {
		return ServerMetric{}, fmt.Errorf("metric: insert: %w", err)
	}
	return m, nil
}

// History returns the most recent N metrics for a server, ordered
// oldest-to-newest for charting.
func (r *ServerMetricRepo) History(ctx context.Context, serverID string, limit int) ([]ServerMetric, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, server_id, ts,
			cpu_usage, cpu_load1, cpu_load5, cpu_load15,
			mem_total, mem_used, mem_percent,
			disk_total, disk_used, disk_percent,
			net_bytes_sent, net_bytes_recv, uptime, error
		FROM server_metrics
		WHERE server_id = ?
		ORDER BY ts DESC
		LIMIT ?
	`, serverID, limit)
	if err != nil {
		return nil, fmt.Errorf("metric: history: %w", err)
	}
	defer rows.Close()

	out := make([]ServerMetric, 0, limit)
	for rows.Next() {
		m, err := scanMetricRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("metric: history iter: %w", err)
	}

	// Reverse to oldest→newest for charting.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// Latest returns the most recent metric sample for a server, or
// sql.ErrNoRows when none exist.
func (r *ServerMetricRepo) Latest(ctx context.Context, serverID string) (ServerMetric, error) {
	row := r.DB.QueryRowContext(ctx, `
		SELECT id, server_id, ts,
			cpu_usage, cpu_load1, cpu_load5, cpu_load15,
			mem_total, mem_used, mem_percent,
			disk_total, disk_used, disk_percent,
			net_bytes_sent, net_bytes_recv, uptime, error
		FROM server_metrics
		WHERE server_id = ?
		ORDER BY ts DESC
		LIMIT 1
	`, serverID)
	return scanMetric(row)
}

// Purge deletes metrics older than the given threshold.
func (r *ServerMetricRepo) Purge(ctx context.Context, olderThan time.Time) (int, error) {
	res, err := r.DB.ExecContext(ctx, `DELETE FROM server_metrics WHERE ts < ?`,
		olderThan.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("metric: purge: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("metric: rows affected: %w", err)
	}
	return int(n), nil
}

// CountByServer returns the number of stored metrics for a server.
func (r *ServerMetricRepo) CountByServer(ctx context.Context, serverID string) (int, error) {
	var n int
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM server_metrics WHERE server_id = ?`, serverID).Scan(&n); err != nil {
		return 0, fmt.Errorf("metric: count: %w", err)
	}
	return n, nil
}

func scanMetric(row *sql.Row) (ServerMetric, error) {
	m, err := scanMetricRow(row)
	if err != nil {
		return ServerMetric{}, err
	}
	return m, nil
}

func scanMetricRow(s rowScanner) (ServerMetric, error) {
	var (
		m   ServerMetric
		tsRaw string
	)
	err := s.Scan(
		&m.ID, &m.ServerID, &tsRaw,
		&m.CPUUsage, &m.CPULoad1, &m.CPULoad5, &m.CPULoad15,
		&m.MemTotal, &m.MemUsed, &m.MemPercent,
		&m.DiskTotal, &m.DiskUsed, &m.DiskPercent,
		&m.NetBytesSent, &m.NetBytesRecv, &m.Uptime, &m.Error,
	)
	if err != nil {
		return ServerMetric{}, err
	}
	m.Timestamp = parseSQLiteTime(tsRaw)
	return m, nil
}

// strings import guard — used by callers in this file.
var _ = strings.TrimSpace
