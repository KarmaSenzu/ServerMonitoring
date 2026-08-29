package notifier_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/db"
	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/notifier"
)

type recordingSender struct {
	mu    sync.Mutex
	calls []notifier.Message
	err   error
}

func (r *recordingSender) Send(_ context.Context, _ *models.Channel, m notifier.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, m)
	return r.err
}

func newChannelRepo(t *testing.T) *models.ChannelRepo {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "notif_test.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.Migrate(context.Background(), conn, zerolog.New(io.Discard)); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return models.NewChannelRepo(conn)
}

func TestServiceNotify(t *testing.T) {
	repo := newChannelRepo(t)
	ctx := context.Background()

	enabled, err := repo.Create(ctx, models.Channel{Type: models.ChannelTypeTelegram, Name: "ops", Enabled: true, Config: map[string]any{"bot_token": "tok", "chat_id": "1"}})
	if err != nil {
		t.Fatalf("Create enabled: %v", err)
	}
	disabled, err := repo.Create(ctx, models.Channel{Type: models.ChannelTypeTelegram, Name: "off", Enabled: false, Config: map[string]any{"bot_token": "tok", "chat_id": "2"}})
	if err != nil {
		t.Fatalf("Create disabled: %v", err)
	}

	rec := &recordingSender{}
	svc := notifier.NewService(zerolog.New(io.Discard), repo, map[string]notifier.Sender{
		models.ChannelTypeTelegram: rec,
	})

	delivered, errs := svc.Notify(ctx, []string{enabled.ID, disabled.ID, "missing"}, notifier.Message{
		Title: "hi", Text: "world", Severity: models.SeverityWarning,
	})

	if len(delivered) != 1 || delivered[0] != enabled.ID {
		t.Errorf("delivered: %+v", delivered)
	}
	if len(errs) != 2 {
		t.Errorf("errs: %+v", errs)
	}
	if len(rec.calls) != 1 || rec.calls[0].Title != "hi" {
		t.Errorf("recording calls: %+v", rec.calls)
	}
}

func TestTelegramSenderSuccess(t *testing.T) {
	var (
		gotPath string
		gotBody map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	sender := &notifier.TelegramSender{
		HTTP:    &http.Client{Timeout: 2 * time.Second},
		APIBase: srv.URL,
	}

	ch := &models.Channel{
		ID:      "c1",
		Type:    models.ChannelTypeTelegram,
		Name:    "ops",
		Enabled: true,
		Config:  map[string]any{"bot_token": "abc", "chat_id": "42"},
	}

	err := sender.Send(context.Background(), ch, notifier.Message{
		Title: "Down", Text: "site offline", Severity: models.SeverityError,
		ProjectID: "p-1",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.HasPrefix(gotPath, "/botabc/sendMessage") {
		t.Errorf("path: %q", gotPath)
	}
	if gotBody["chat_id"] != "42" {
		t.Errorf("chat_id: %v", gotBody["chat_id"])
	}
	if mode, _ := gotBody["parse_mode"].(string); mode != "HTML" {
		t.Errorf("parse_mode: %q", mode)
	}
	text, _ := gotBody["text"].(string)
	if !strings.Contains(text, "[ERROR]") || !strings.Contains(text, "Down") {
		t.Errorf("text: %q", text)
	}
	if !strings.Contains(text, "p-1") {
		t.Errorf("text missing project_id: %q", text)
	}
}

func TestTelegramSenderNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"description":"bad chat_id"}`))
	}))
	t.Cleanup(srv.Close)

	sender := &notifier.TelegramSender{
		HTTP:    &http.Client{Timeout: 2 * time.Second},
		APIBase: srv.URL,
	}
	ch := &models.Channel{
		Type:   models.ChannelTypeTelegram,
		Name:   "ops",
		Config: map[string]any{"bot_token": "abc", "chat_id": "42"},
	}
	err := sender.Send(context.Background(), ch, notifier.Message{Title: "x"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should include status: %v", err)
	}
	if !strings.Contains(err.Error(), "bad chat_id") {
		t.Errorf("error should include body snippet: %v", err)
	}
}

func TestTelegramSenderMissingConfig(t *testing.T) {
	sender := notifier.NewTelegramSender()
	if err := sender.Send(context.Background(), &models.Channel{Type: models.ChannelTypeTelegram, Config: map[string]any{}}, notifier.Message{}); err == nil {
		t.Fatal("expected validation error")
	}
}
