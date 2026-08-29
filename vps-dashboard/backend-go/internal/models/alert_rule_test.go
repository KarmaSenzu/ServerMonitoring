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

func newAlertRepo(t *testing.T) *models.AlertRepo {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "alerts_test.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.Migrate(context.Background(), conn, zerolog.New(io.Discard)); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	return models.NewAlertRepo(conn)
}

func validAlertRule(name string) models.AlertRule {
	return models.AlertRule{
		Name:            name,
		Enabled:         true,
		Type:            models.AlertTypeSystemCPU,
		Threshold:       80,
		Comparator:      models.ComparatorGTE,
		ForSeconds:      60,
		CooldownSeconds: 600,
		Severity:        models.SeverityWarning,
		Channels:        []string{"chan-a"},
		Scope:           map[string]any{},
	}
}

func TestAlertRepoCRUD(t *testing.T) {
	r := newAlertRepo(t)
	ctx := context.Background()

	rule := validAlertRule("cpu-warn")
	created, err := r.Create(ctx, rule)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("Create: empty id")
	}

	got, err := r.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "cpu-warn" {
		t.Errorf("Get name: %q", got.Name)
	}

	got.Threshold = 90
	updated, err := r.Update(ctx, got)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Threshold != 90 {
		t.Errorf("Update threshold: %v", updated.Threshold)
	}

	now := time.Now().UTC()
	if err := r.UpdateLastTriggered(ctx, updated.ID, now); err != nil {
		t.Fatalf("UpdateLastTriggered: %v", err)
	}
	got, _ = r.Get(ctx, updated.ID)
	if got.LastTriggeredAt.IsZero() {
		t.Errorf("LastTriggeredAt not persisted")
	}

	all, err := r.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("List len: %d", len(all))
	}

	if err := r.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.Get(ctx, created.ID); !errors.Is(err, models.ErrAlertRuleNotFound) {
		t.Errorf("Get after delete: %v", err)
	}
}

func TestAlertRepoDuplicate(t *testing.T) {
	r := newAlertRepo(t)
	ctx := context.Background()

	if _, err := r.Create(ctx, validAlertRule("dupe")); err != nil {
		t.Fatalf("Create #1: %v", err)
	}
	_, err := r.Create(ctx, validAlertRule("dupe"))
	if !errors.Is(err, models.ErrDuplicateAlertRuleName) {
		t.Errorf("expected ErrDuplicateAlertRuleName, got %v", err)
	}
}

func TestAlertRuleValidate(t *testing.T) {
	cases := []struct {
		name    string
		mut     func(*models.AlertRule)
		wantErr bool
	}{
		{"ok", func(*models.AlertRule) {}, false},
		{"empty name", func(a *models.AlertRule) { a.Name = "" }, true},
		{"bad type", func(a *models.AlertRule) { a.Type = "nope" }, true},
		{"bad comparator", func(a *models.AlertRule) { a.Comparator = "bogus" }, true},
		{"bad severity", func(a *models.AlertRule) { a.Severity = "bogus" }, true},
		{"threshold too high", func(a *models.AlertRule) { a.Threshold = 150 }, true},
		{"negative for", func(a *models.AlertRule) { a.ForSeconds = -1 }, true},
		{"too long for", func(a *models.AlertRule) { a.ForSeconds = 1000000 }, true},
		{"empty channels", func(a *models.AlertRule) { a.Channels = nil }, true},
		{"blank channel", func(a *models.AlertRule) { a.Channels = []string{""} }, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			a := validAlertRule("rule")
			tc.mut(&a)
			err := a.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
