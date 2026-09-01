package models_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/db"
	"vps-dashboard-api/internal/models"
)

func newMetricRepo(t *testing.T) (*models.ServerMetricRepo, *models.ServerRepo) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "metrics_test.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := db.Migrate(context.Background(), conn, zerolog.New(io.Discard)); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	return models.NewServerMetricRepo(conn), models.NewServerRepo(conn)
}

func seedServer(t *testing.T, repo *models.ServerRepo, name string) string {
	t.Helper()
	s, err := repo.Create(context.Background(), models.Server{
		Name:        name,
		Hostname:    name + ".example.com",
		SSHUsername: "deploy",
		Environment: models.ServerEnvProduction,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("seedServer: %v", err)
	}
	return s.ID
}

func TestServerMetricAppendAndHistory(t *testing.T) {
	mr, sr := newMetricRepo(t)
	ctx := context.Background()
	serverID := seedServer(t, sr, "alpha")

	m1 := models.ServerMetric{
		ServerID:     serverID,
		Timestamp:    time.Now().UTC().Add(-2 * time.Minute),
		CPUUsage:     42.5,
		MemTotal:     8e9,
		MemUsed:      4e9,
		MemPercent:   50,
		DiskPercent:  72,
		NetBytesSent: 1024,
		NetBytesRecv: 2048,
		Uptime:       86400,
	}
	if _, err := mr.Append(ctx, m1); err != nil {
		t.Fatalf("Append: %v", err)
	}

	m2 := m1
	m2.Timestamp = time.Now().UTC().Add(-1 * time.Minute)
	m2.CPUUsage = 55
	if _, err := mr.Append(ctx, m2); err != nil {
		t.Fatalf("Append 2: %v", err)
	}

	// Latest should be m2.
	latest, err := mr.Latest(ctx, serverID)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if latest.CPUUsage != 55 {
		t.Errorf("Latest CPUUsage: %f", latest.CPUUsage)
	}

	// History returns oldest→newest.
	hist, err := mr.History(ctx, serverID, 10)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("History: got %d want 2", len(hist))
	}
	if hist[0].CPUUsage != 42.5 {
		t.Errorf("History[0] CPUUsage: %f", hist[0].CPUUsage)
	}
	if hist[1].CPUUsage != 55 {
		t.Errorf("History[1] CPUUsage: %f", hist[1].CPUUsage)
	}

	// CountByServer.
	n, err := mr.CountByServer(ctx, serverID)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Errorf("Count: %d", n)
	}
}

func TestServerMetricLatestEmpty(t *testing.T) {
	mr, sr := newMetricRepo(t)
	serverID := seedServer(t, sr, "alpha")
	_, err := mr.Latest(context.Background(), serverID)
	if !errors.Is(err, sql.ErrNoRows) && err != nil {
		t.Fatalf("expected sql.ErrNoRows or nil, got %v", err)
	}
}

func TestServerMetricPurge(t *testing.T) {
	mr, sr := newMetricRepo(t)
	ctx := context.Background()
	serverID := seedServer(t, sr, "alpha")

	// Insert an old metric.
	old := models.ServerMetric{
		ServerID:  serverID,
		Timestamp: time.Now().UTC().Add(-48 * time.Hour),
		CPUUsage:  10,
	}
	if _, err := mr.Append(ctx, old); err != nil {
		t.Fatalf("Append old: %v", err)
	}

	// Insert a recent metric.
	recent := models.ServerMetric{
		ServerID:  serverID,
		Timestamp: time.Now().UTC().Add(-5 * time.Minute),
		CPUUsage:  20,
	}
	if _, err := mr.Append(ctx, recent); err != nil {
		t.Fatalf("Append recent: %v", err)
	}

	// Purge everything older than 24h.
	n, err := mr.Purge(ctx, time.Now().UTC().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if n != 1 {
		t.Fatalf("Purge: deleted %d want 1", n)
	}

	// Only the recent sample remains.
	hist, _ := mr.History(ctx, serverID, 10)
	if len(hist) != 1 {
		t.Fatalf("History after purge: %d", len(hist))
	}
	if hist[0].CPUUsage != 20 {
		t.Errorf("remaining CPUUsage: %f", hist[0].CPUUsage)
	}
}
