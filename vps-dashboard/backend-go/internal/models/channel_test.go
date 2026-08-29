package models_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/db"
	"vps-dashboard-api/internal/models"
)

func newChannelRepo(t *testing.T) *models.ChannelRepo {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "channels_test.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.Migrate(context.Background(), conn, zerolog.New(io.Discard)); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	return models.NewChannelRepo(conn)
}

func TestChannelRepoCRUD(t *testing.T) {
	r := newChannelRepo(t)
	ctx := context.Background()

	created, err := r.Create(ctx, models.Channel{
		Type:    models.ChannelTypeTelegram,
		Name:    "ops",
		Enabled: true,
		Config:  map[string]any{"bot_token": "abc", "chat_id": "123"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("Create: empty id")
	}
	if created.Config["bot_token"] != "abc" {
		t.Errorf("Config not persisted: %+v", created.Config)
	}

	got, err := r.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "ops" {
		t.Errorf("Get name: %q", got.Name)
	}

	created.Config["chat_id"] = "456"
	updated, err := r.Update(ctx, created)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Config["chat_id"] != "456" {
		t.Errorf("Update did not persist: %+v", updated.Config)
	}

	if err := r.EnableDisable(ctx, created.ID, false); err != nil {
		t.Fatalf("EnableDisable: %v", err)
	}
	got, _ = r.Get(ctx, created.ID)
	if got.Enabled {
		t.Errorf("disable did not stick")
	}

	all, err := r.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("List count: %d", len(all))
	}

	if err := r.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.Get(ctx, created.ID); !errors.Is(err, models.ErrChannelNotFound) {
		t.Errorf("Get after delete: %v", err)
	}
}

func TestChannelRepoDuplicate(t *testing.T) {
	r := newChannelRepo(t)
	ctx := context.Background()

	cfg := map[string]any{"bot_token": "x", "chat_id": "y"}

	if _, err := r.Create(ctx, models.Channel{Type: models.ChannelTypeTelegram, Name: "ops", Config: cfg}); err != nil {
		t.Fatalf("Create #1: %v", err)
	}
	_, err := r.Create(ctx, models.Channel{Type: models.ChannelTypeTelegram, Name: "ops", Config: cfg})
	if !errors.Is(err, models.ErrDuplicateChannelName) {
		t.Errorf("expected ErrDuplicateChannelName, got %v", err)
	}
}

func TestChannelRepoValidation(t *testing.T) {
	r := newChannelRepo(t)
	ctx := context.Background()

	if _, err := r.Create(ctx, models.Channel{Type: "slack", Name: "x", Config: map[string]any{"a": "b"}}); err == nil {
		t.Errorf("expected validation error for unsupported type")
	}
	if _, err := r.Create(ctx, models.Channel{Type: models.ChannelTypeTelegram, Name: "", Config: map[string]any{"bot_token": "a", "chat_id": "b"}}); err == nil {
		t.Errorf("expected validation error for empty name")
	}
	if _, err := r.Create(ctx, models.Channel{Type: models.ChannelTypeTelegram, Name: "n", Config: map[string]any{"chat_id": "b"}}); err == nil {
		t.Errorf("expected validation error for missing bot_token")
	}
}
