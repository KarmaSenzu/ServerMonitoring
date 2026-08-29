package models_test

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/db"
	"vps-dashboard-api/internal/models"
)

func newEventRepo(t *testing.T) *models.EventRepo {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "events_test.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.Migrate(context.Background(), conn, zerolog.New(io.Discard)); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	return models.NewEventRepo(conn)
}

func TestEventRepoAppendList(t *testing.T) {
	r := newEventRepo(t)
	ctx := context.Background()

	now := time.Now().UTC()
	for i, ev := range []models.Event{
		{Category: models.EventCategoryHealth, Severity: models.SeverityInfo, Source: "system", Message: "first", Timestamp: now.Add(-3 * time.Minute), Data: map[string]any{"k": "v1"}},
		{Category: models.EventCategoryHealth, Severity: models.SeverityWarning, Source: "system", ProjectID: "p-1", Message: "down", Timestamp: now.Add(-2 * time.Minute)},
		{Category: models.EventCategorySystem, Severity: models.SeverityCritical, Source: "system", Message: "cpu spike", Timestamp: now.Add(-1 * time.Minute)},
	} {
		if _, err := r.Append(ctx, ev); err != nil {
			t.Fatalf("Append #%d: %v", i, err)
		}
	}

	all, err := r.List(ctx, models.EventFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("List: got %d want 3", len(all))
	}
	if all[0].Message != "cpu spike" {
		t.Errorf("expected newest first, got %q", all[0].Message)
	}

	healthOnly, err := r.List(ctx, models.EventFilter{Category: models.EventCategoryHealth})
	if err != nil {
		t.Fatalf("List health: %v", err)
	}
	if len(healthOnly) != 2 {
		t.Fatalf("category filter: got %d want 2", len(healthOnly))
	}

	warn, err := r.List(ctx, models.EventFilter{Severity: models.SeverityWarning})
	if err != nil {
		t.Fatalf("List warn: %v", err)
	}
	if len(warn) != 1 || warn[0].Message != "down" {
		t.Errorf("severity filter wrong: %+v", warn)
	}

	proj, err := r.List(ctx, models.EventFilter{ProjectID: "p-1"})
	if err != nil {
		t.Fatalf("List proj: %v", err)
	}
	if len(proj) != 1 {
		t.Errorf("project filter wrong: %+v", proj)
	}

	search, err := r.List(ctx, models.EventFilter{Search: "cpu"})
	if err != nil {
		t.Fatalf("List search: %v", err)
	}
	if len(search) != 1 || search[0].Message != "cpu spike" {
		t.Errorf("search filter wrong: %+v", search)
	}

	since, err := r.List(ctx, models.EventFilter{Since: now.Add(-90 * time.Second)})
	if err != nil {
		t.Fatalf("List since: %v", err)
	}
	if len(since) != 1 {
		t.Errorf("since filter wrong: %+v", since)
	}

	count, err := r.Count(ctx, models.EventFilter{})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 3 {
		t.Errorf("Count: got %d want 3", count)
	}
}

func TestEventRepoLimitOffset(t *testing.T) {
	r := newEventRepo(t)
	ctx := context.Background()

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		_, err := r.Append(ctx, models.Event{
			Category:  models.EventCategorySystem,
			Severity:  models.SeverityInfo,
			Message:   "msg",
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
		if err != nil {
			t.Fatalf("Append #%d: %v", i, err)
		}
	}
	page, err := r.List(ctx, models.EventFilter{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("List paged: %v", err)
	}
	if len(page) != 2 {
		t.Errorf("Limit: got %d want 2", len(page))
	}
}

func TestEventRepoPurge(t *testing.T) {
	r := newEventRepo(t)
	ctx := context.Background()

	old := time.Now().UTC().Add(-2 * time.Hour)
	recent := time.Now().UTC().Add(-1 * time.Minute)

	if _, err := r.Append(ctx, models.Event{Category: models.EventCategoryAuth, Severity: models.SeverityInfo, Message: "old", Timestamp: old}); err != nil {
		t.Fatalf("Append old: %v", err)
	}
	if _, err := r.Append(ctx, models.Event{Category: models.EventCategoryAuth, Severity: models.SeverityInfo, Message: "new", Timestamp: recent}); err != nil {
		t.Fatalf("Append new: %v", err)
	}

	n, err := r.Purge(ctx, time.Now().UTC().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if n != 1 {
		t.Errorf("Purge count: got %d want 1", n)
	}
	all, _ := r.List(ctx, models.EventFilter{})
	if len(all) != 1 || all[0].Message != "new" {
		t.Errorf("after purge: %+v", all)
	}
}

func TestEventRepoInvalidSeverity(t *testing.T) {
	r := newEventRepo(t)
	if _, err := r.Append(context.Background(), models.Event{Category: "x", Severity: "bogus", Message: "m"}); err == nil {
		t.Fatal("expected validation error for invalid severity")
	}
}
