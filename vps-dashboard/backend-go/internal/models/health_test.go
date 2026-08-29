package models_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/db"
	"vps-dashboard-api/internal/models"
)

func newHealthRepo(t *testing.T) *models.HealthRepo {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "health_test.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.Migrate(context.Background(), conn, zerolog.New(io.Discard)); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	return models.NewHealthRepo(conn)
}

func TestHealthRepoAppendAndHistory(t *testing.T) {
	r := newHealthRepo(t)
	ctx := context.Background()

	now := time.Now().UTC()
	for i := 0; i < 4; i++ {
		ok := i%2 == 0
		if err := r.Append(ctx, models.HealthResult{
			ProjectID:  "p-1",
			OK:         ok,
			StatusCode: 200,
			LatencyMs:  10 + i,
			Timestamp:  now.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	latest, err := r.LatestByProject(ctx, "p-1")
	if err != nil {
		t.Fatalf("LatestByProject: %v", err)
	}
	if latest.LatencyMs != 13 {
		t.Errorf("expected newest latency 13, got %d", latest.LatencyMs)
	}

	hist, err := r.History(ctx, "p-1", time.Time{}, 100)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 4 {
		t.Errorf("History: got %d want 4", len(hist))
	}

	since, err := r.History(ctx, "p-1", now.Add(2*time.Second), 100)
	if err != nil {
		t.Fatalf("History since: %v", err)
	}
	if len(since) != 2 {
		t.Errorf("Since filter: got %d want 2", len(since))
	}
}

func TestHealthRepoLatestEmpty(t *testing.T) {
	r := newHealthRepo(t)
	if _, err := r.LatestByProject(context.Background(), "missing"); !errors.Is(err, models.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestHealthRepoPurge(t *testing.T) {
	r := newHealthRepo(t)
	ctx := context.Background()

	old := time.Now().UTC().Add(-2 * time.Hour)
	recent := time.Now().UTC().Add(-1 * time.Minute)

	if err := r.Append(ctx, models.HealthResult{ProjectID: "p", OK: true, Timestamp: old}); err != nil {
		t.Fatalf("Append old: %v", err)
	}
	if err := r.Append(ctx, models.HealthResult{ProjectID: "p", OK: false, Timestamp: recent}); err != nil {
		t.Fatalf("Append new: %v", err)
	}

	n, err := r.Purge(ctx, time.Now().UTC().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if n != 1 {
		t.Errorf("Purge count: %d", n)
	}
}

func TestSettingsRepo(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "settings_test.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.Migrate(context.Background(), conn, zerolog.New(io.Discard)); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	s := models.NewSettingsRepo(conn)
	ctx := context.Background()

	v, err := s.Get(ctx, "missing")
	if err != nil {
		t.Fatalf("Get missing: %v", err)
	}
	if v != "" {
		t.Errorf("expected empty for missing key, got %q", v)
	}

	if err := s.Set(ctx, "k", "v1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if v, _ := s.Get(ctx, "k"); v != "v1" {
		t.Errorf("Set/Get round-trip: %q", v)
	}
	if err := s.Set(ctx, "k", "v2"); err != nil {
		t.Fatalf("Set upsert: %v", err)
	}
	if v, _ := s.Get(ctx, "k"); v != "v2" {
		t.Errorf("upsert: %q", v)
	}
	if err := s.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if v, _ := s.Get(ctx, "k"); v != "" {
		t.Errorf("after delete: %q", v)
	}
}
