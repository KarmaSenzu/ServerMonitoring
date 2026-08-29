package handlers_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"vps-dashboard-api/internal/models"
)

// seedEvents writes n events with deterministic categories/severities
// directly via the repo, returning the seeded rows.
func seedEvents(t *testing.T, repo *models.EventRepo) {
	t.Helper()

	now := time.Now().UTC()
	rows := []models.Event{
		{
			Category:  models.EventCategoryHealth,
			Severity:  models.SeverityInfo,
			Source:    "project:p-1",
			ProjectID: "p-1",
			Message:   "p1 went UP",
			Timestamp: now.Add(-3 * time.Minute),
			Data:      map[string]any{"k": 1},
		},
		{
			Category:  models.EventCategoryHealth,
			Severity:  models.SeverityWarning,
			Source:    "project:p-2",
			ProjectID: "p-2",
			Message:   "p2 went DOWN",
			Timestamp: now.Add(-2 * time.Minute),
		},
		{
			Category:  models.EventCategorySystem,
			Severity:  models.SeverityCritical,
			Source:    "system:cpu",
			ProjectID: "",
			Message:   "cpu critical",
			Timestamp: now.Add(-1 * time.Minute),
		},
	}
	for _, ev := range rows {
		if _, err := repo.Append(context.Background(), ev); err != nil {
			t.Fatalf("seedEvents append: %v", err)
		}
	}
}

func TestEventsListReturnsAllAndTotal(t *testing.T) {
	a := newTestApp(t)
	repo := models.NewEventRepo(a.DB)
	a.Events = repo
	seedEvents(t, repo)

	eng := buildTestEngine(t, a)
	tok := loginAs(t, eng, testUsername, testPassword)

	rec := doJSON(t, eng, http.MethodGet, "/events", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	items, _ := body["data"].([]any)
	if len(items) != 3 {
		t.Errorf("expected 3 events, got %d", len(items))
	}
	if total, _ := body["total"].(float64); int(total) != 3 {
		t.Errorf("expected total=3, got %v", body["total"])
	}
}

func TestEventsListFiltersAndLimit(t *testing.T) {
	a := newTestApp(t)
	repo := models.NewEventRepo(a.DB)
	a.Events = repo
	seedEvents(t, repo)

	eng := buildTestEngine(t, a)
	tok := loginAs(t, eng, testUsername, testPassword)

	// Filter by category=health: 2 rows.
	rec := doJSON(t, eng, http.MethodGet, "/events?category=health", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	items, _ := body["data"].([]any)
	if len(items) != 2 {
		t.Errorf("expected 2 health events, got %d", len(items))
	}
	if total, _ := body["total"].(float64); int(total) != 2 {
		t.Errorf("expected total=2, got %v", body["total"])
	}

	// Filter by severity=critical: 1 row.
	rec = doJSON(t, eng, http.MethodGet, "/events?severity=critical", nil, tok)
	body = decodeBody(t, rec)
	items, _ = body["data"].([]any)
	if len(items) != 1 {
		t.Errorf("expected 1 critical event, got %d", len(items))
	}

	// Limit=1 returns 1 item; total still reflects unbounded count (3).
	rec = doJSON(t, eng, http.MethodGet, "/events?limit=1", nil, tok)
	body = decodeBody(t, rec)
	items, _ = body["data"].([]any)
	if len(items) != 1 {
		t.Errorf("limit=1: got %d items", len(items))
	}
	if total, _ := body["total"].(float64); int(total) != 3 {
		t.Errorf("limit=1 total: got %v want 3", body["total"])
	}

	// Offset=2 with limit=10 yields 1 row (the oldest).
	rec = doJSON(t, eng, http.MethodGet, "/events?limit=10&offset=2", nil, tok)
	body = decodeBody(t, rec)
	items, _ = body["data"].([]any)
	if len(items) != 1 {
		t.Errorf("offset=2: got %d items", len(items))
	}
}

func TestEventsListInvalidSinceIgnored(t *testing.T) {
	a := newTestApp(t)
	repo := models.NewEventRepo(a.DB)
	a.Events = repo
	seedEvents(t, repo)

	eng := buildTestEngine(t, a)
	tok := loginAs(t, eng, testUsername, testPassword)

	// Garbage `since` is silently dropped per the handler contract.
	rec := doJSON(t, eng, http.MethodGet, "/events?since=not-a-time", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	items, _ := body["data"].([]any)
	if len(items) != 3 {
		t.Errorf("expected all 3 events, got %d", len(items))
	}
}
