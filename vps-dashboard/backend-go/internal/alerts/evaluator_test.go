package alerts_test

import (
	"context"
	"io"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/alerts"
	"vps-dashboard-api/internal/db"
	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/notifier"
)

type captureSender struct {
	mu    sync.Mutex
	calls []notifier.Message
}

func (c *captureSender) Send(_ context.Context, _ *models.Channel, m notifier.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, m)
	return nil
}

func (c *captureSender) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

type evalFixture struct {
	Eval     *alerts.Evaluator
	Rules    *models.AlertRepo
	Channels *models.ChannelRepo
	Events   *models.EventRepo
	Sender   *captureSender
	Channel  models.Channel
}

func newFixture(t *testing.T) *evalFixture {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "eval.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.Migrate(context.Background(), conn, zerolog.New(io.Discard)); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	channels := models.NewChannelRepo(conn)
	rules := models.NewAlertRepo(conn)
	events := models.NewEventRepo(conn)

	ch, err := channels.Create(context.Background(), models.Channel{
		Type:    models.ChannelTypeTelegram,
		Name:    "ops",
		Enabled: true,
		Config:  map[string]any{"bot_token": "tok", "chat_id": "1"},
	})
	if err != nil {
		t.Fatalf("Channels.Create: %v", err)
	}

	sender := &captureSender{}
	svc := notifier.NewService(zerolog.New(io.Discard), channels, map[string]notifier.Sender{
		models.ChannelTypeTelegram: sender,
	})
	eval := alerts.NewEvaluator(zerolog.New(io.Discard), rules, channels, svc, events)

	return &evalFixture{
		Eval:     eval,
		Rules:    rules,
		Channels: channels,
		Events:   events,
		Sender:   sender,
		Channel:  ch,
	}
}

func TestEvaluatorNumericSustained(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	rule := models.AlertRule{
		Name:            "cpu80",
		Enabled:         true,
		Type:            models.AlertTypeSystemCPU,
		Threshold:       80,
		Comparator:      models.ComparatorGTE,
		ForSeconds:      60,
		CooldownSeconds: 300,
		Severity:        models.SeverityWarning,
		Channels:        []string{f.Channel.ID},
	}
	if _, err := f.Rules.Create(ctx, rule); err != nil {
		t.Fatalf("Create rule: %v", err)
	}

	base := time.Now().UTC()

	// Below threshold -> no fire.
	f.Eval.Evaluate(ctx, alerts.Signal{Type: models.AlertTypeSystemCPU, Value: 70, Timestamp: base})
	if f.Sender.Count() != 0 {
		t.Fatalf("unexpected fire at value < threshold")
	}

	// First breach -> not yet sustained.
	f.Eval.Evaluate(ctx, alerts.Signal{Type: models.AlertTypeSystemCPU, Value: 92, Timestamp: base.Add(1 * time.Second)})
	if f.Sender.Count() != 0 {
		t.Fatalf("fired before ForSeconds elapsed")
	}

	// 30s later still under ForSeconds -> still no fire.
	f.Eval.Evaluate(ctx, alerts.Signal{Type: models.AlertTypeSystemCPU, Value: 92, Timestamp: base.Add(30 * time.Second)})
	if f.Sender.Count() != 0 {
		t.Fatalf("fired before ForSeconds elapsed (30s)")
	}

	// 90s later -> sustained, should fire once.
	f.Eval.Evaluate(ctx, alerts.Signal{Type: models.AlertTypeSystemCPU, Value: 92, Timestamp: base.Add(90 * time.Second)})
	if f.Sender.Count() != 1 {
		t.Fatalf("expected 1 fire, got %d", f.Sender.Count())
	}
}

func TestEvaluatorCooldown(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	rule := models.AlertRule{
		Name:            "cpu80",
		Enabled:         true,
		Type:            models.AlertTypeSystemCPU,
		Threshold:       80,
		Comparator:      models.ComparatorGTE,
		ForSeconds:      0,
		CooldownSeconds: 600,
		Severity:        models.SeverityWarning,
		Channels:        []string{f.Channel.ID},
	}
	if _, err := f.Rules.Create(ctx, rule); err != nil {
		t.Fatalf("Create: %v", err)
	}

	base := time.Now().UTC()
	f.Eval.Evaluate(ctx, alerts.Signal{Type: models.AlertTypeSystemCPU, Value: 95, Timestamp: base})
	if f.Sender.Count() != 1 {
		t.Fatalf("expected first fire, got %d", f.Sender.Count())
	}
	// Within cooldown -> still 1.
	f.Eval.Evaluate(ctx, alerts.Signal{Type: models.AlertTypeSystemCPU, Value: 95, Timestamp: base.Add(60 * time.Second)})
	if f.Sender.Count() != 1 {
		t.Fatalf("cooldown did not block, got %d", f.Sender.Count())
	}
}

func TestEvaluatorStateRule(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	rule := models.AlertRule{
		Name:            "proj-health",
		Enabled:         true,
		Type:            models.AlertTypeProjectHealth,
		Threshold:       0,
		Comparator:      models.ComparatorEQ,
		ForSeconds:      0,
		CooldownSeconds: 600,
		Severity:        models.SeverityError,
		Channels:        []string{f.Channel.ID},
		Scope:           map[string]any{"project_id": "p-1"},
	}
	if _, err := f.Rules.Create(ctx, rule); err != nil {
		t.Fatalf("Create: %v", err)
	}

	base := time.Now().UTC()

	// up -> no fire
	f.Eval.Evaluate(ctx, alerts.Signal{Type: models.AlertTypeProjectHealth, ProjectID: "p-1", State: "up", Timestamp: base})
	if f.Sender.Count() != 0 {
		t.Fatalf("up should not fire")
	}

	// down on different project -> no fire (scope mismatch)
	f.Eval.Evaluate(ctx, alerts.Signal{Type: models.AlertTypeProjectHealth, ProjectID: "other", State: "down", Timestamp: base})
	if f.Sender.Count() != 0 {
		t.Fatalf("scope mismatch should not fire")
	}

	// down on matching project -> fire
	f.Eval.Evaluate(ctx, alerts.Signal{Type: models.AlertTypeProjectHealth, ProjectID: "p-1", State: "down", Timestamp: base})
	if f.Sender.Count() != 1 {
		t.Fatalf("expected fire, got %d", f.Sender.Count())
	}
}

func TestEvaluatorForce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	rule := models.AlertRule{
		Name:            "cpu80",
		Enabled:         true,
		Type:            models.AlertTypeSystemCPU,
		Threshold:       80,
		Comparator:      models.ComparatorGTE,
		ForSeconds:      300,
		CooldownSeconds: 600,
		Severity:        models.SeverityWarning,
		Channels:        []string{f.Channel.ID},
	}
	created, err := f.Rules.Create(ctx, rule)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	delivered, errs := f.Eval.Force(ctx, created, alerts.Signal{Type: models.AlertTypeSystemCPU, Value: 99})
	if len(errs) != 0 {
		t.Errorf("force errors: %+v", errs)
	}
	if len(delivered) != 1 {
		t.Errorf("force delivered: %+v", delivered)
	}
	if f.Sender.Count() != 1 {
		t.Errorf("force should deliver one message, got %d", f.Sender.Count())
	}

	// Force should NOT update LastTriggeredAt.
	got, _ := f.Rules.Get(ctx, created.ID)
	if !got.LastTriggeredAt.IsZero() {
		t.Errorf("force unexpectedly updated LastTriggeredAt: %v", got.LastTriggeredAt)
	}
}
