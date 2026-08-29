package handlers_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/alerts"
	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/notifier"
)

// wireNotifier fills a.Channels/Alerts/Notifier/AlertEvaluator/Events
// with repos bound to a.DB. The supplied telegramAPI is used as the
// httptest base for the TelegramSender so handler tests can observe
// outbound calls without actually contacting api.telegram.org.
func wireNotifier(t *testing.T, a *app.App, telegramAPI string) {
	t.Helper()

	a.Events = models.NewEventRepo(a.DB)
	a.Channels = models.NewChannelRepo(a.DB)
	a.Alerts = models.NewAlertRepo(a.DB)
	a.Health = models.NewHealthRepo(a.DB)
	a.Settings = models.NewSettingsRepo(a.DB)

	tg := &notifier.TelegramSender{
		HTTP:    http.DefaultClient,
		APIBase: telegramAPI,
	}
	svc := notifier.NewService(zerolog.New(io.Discard), a.Channels, map[string]notifier.Sender{
		models.ChannelTypeTelegram: tg,
	})
	a.Notifier = svc
	a.AlertEvaluator = alerts.NewEvaluator(zerolog.New(io.Discard), a.Alerts, a.Channels, svc, a.Events)
}

func TestNotificationsChannelCRUDStripsToken(t *testing.T) {
	a := newTestApp(t)

	tgsrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(tgsrv.Close)

	wireNotifier(t, a, tgsrv.URL)
	eng := buildTestEngine(t, a)
	tok := loginAs(t, eng, testUsername, testPassword)

	// Create.
	body := map[string]any{
		"name": "ops",
		"type": "telegram",
		"config": map[string]any{
			"bot_token": "fake-token",
			"chat_id":   "42",
		},
	}
	rec := doJSON(t, eng, http.MethodPost, "/notifications/channels", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status %d body=%s", rec.Code, rec.Body.String())
	}
	created := decodeBody(t, rec)
	data, _ := created["data"].(map[string]any)
	id, _ := data["id"].(string)
	if id == "" {
		t.Fatalf("missing id in %v", created)
	}
	cfg, _ := data["config"].(map[string]any)
	if _, leak := cfg["bot_token"]; leak {
		t.Errorf("create response leaked bot_token: %v", cfg)
	}
	if got, _ := data["bot_token_present"].(bool); !got {
		t.Errorf("expected bot_token_present=true, got %v", data["bot_token_present"])
	}
	if cfg["chat_id"] != "42" {
		t.Errorf("chat_id missing/changed in config: %v", cfg)
	}

	// List: same stripping.
	rec = doJSON(t, eng, http.MethodGet, "/notifications/channels", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status %d body=%s", rec.Code, rec.Body.String())
	}
	listBody := decodeBody(t, rec)
	items, _ := listBody["data"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(items))
	}
	first, _ := items[0].(map[string]any)
	if firstCfg, _ := first["config"].(map[string]any); firstCfg["bot_token"] != nil {
		t.Errorf("list leaked bot_token: %v", firstCfg)
	}
	if pres, _ := first["bot_token_present"].(bool); !pres {
		t.Errorf("list bot_token_present should be true")
	}

	// Patch without bot_token: existing token must be retained.
	rec = doJSON(t, eng, http.MethodPatch, "/notifications/channels/"+id, map[string]any{
		"config": map[string]any{
			"chat_id": "99",
		},
	}, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status %d body=%s", rec.Code, rec.Body.String())
	}
	// Read the raw row via the repo to confirm token survived.
	stored, err := a.Channels.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("repo.Get: %v", err)
	}
	if got, _ := stored.Config["bot_token"].(string); got != "fake-token" {
		t.Errorf("token not preserved on patch: %v", stored.Config)
	}
	if got, _ := stored.Config["chat_id"].(string); got != "99" {
		t.Errorf("chat_id not updated on patch: %v", stored.Config)
	}

	// Patch with explicit empty bot_token: documented to be ignored
	// (only JSON null clears it).
	rec = doJSON(t, eng, http.MethodPatch, "/notifications/channels/"+id, map[string]any{
		"config": map[string]any{
			"bot_token": "",
		},
	}, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch empty status %d body=%s", rec.Code, rec.Body.String())
	}
	stored, err = a.Channels.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("repo.Get post-empty: %v", err)
	}
	if got, _ := stored.Config["bot_token"].(string); got != "fake-token" {
		t.Errorf("empty-string patch wrongly cleared token: %v", stored.Config)
	}

	// Delete.
	rec = doJSON(t, eng, http.MethodDelete, "/notifications/channels/"+id, nil, tok)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status %d body=%s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, eng, http.MethodGet, "/notifications/channels/"+id, nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestNotificationsChannelTestEndpoint(t *testing.T) {
	a := newTestApp(t)

	var calls int
	tgsrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(tgsrv.Close)

	wireNotifier(t, a, tgsrv.URL)
	eng := buildTestEngine(t, a)
	tok := loginAs(t, eng, testUsername, testPassword)

	rec := doJSON(t, eng, http.MethodPost, "/notifications/channels", map[string]any{
		"name": "ops",
		"type": "telegram",
		"config": map[string]any{
			"bot_token": "fake",
			"chat_id":   "1",
		},
	}, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status %d body=%s", rec.Code, rec.Body.String())
	}
	created := decodeBody(t, rec)
	data, _ := created["data"].(map[string]any)
	id, _ := data["id"].(string)

	rec = doJSON(t, eng, http.MethodPost, "/notifications/channels/"+id+"/test", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("test status %d body=%s", rec.Code, rec.Body.String())
	}
	if calls != 1 {
		t.Errorf("expected 1 telegram call, got %d", calls)
	}
	body := decodeBody(t, rec)
	d, _ := body["data"].(map[string]any)
	if delivered, _ := d["delivered"].(bool); !delivered {
		t.Errorf("expected delivered=true, got %v", d)
	}
}

func TestNotificationsRuleCRUDAndTest(t *testing.T) {
	a := newTestApp(t)

	var calls int
	tgsrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(tgsrv.Close)

	wireNotifier(t, a, tgsrv.URL)
	eng := buildTestEngine(t, a)
	tok := loginAs(t, eng, testUsername, testPassword)

	// Create a channel so the rule has a real receiver.
	rec := doJSON(t, eng, http.MethodPost, "/notifications/channels", map[string]any{
		"name": "ops",
		"type": "telegram",
		"config": map[string]any{
			"bot_token": "fake",
			"chat_id":   "1",
		},
	}, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create channel status %d body=%s", rec.Code, rec.Body.String())
	}
	chBody := decodeBody(t, rec)
	chData, _ := chBody["data"].(map[string]any)
	chID, _ := chData["id"].(string)

	// Create a rule that always breaches: system_cpu >= 0.
	rec = doJSON(t, eng, http.MethodPost, "/alerts/rules", map[string]any{
		"name":             "always-cpu",
		"type":             "system_cpu",
		"comparator":       "gte",
		"threshold":        0,
		"for_seconds":      0,
		"cooldown_seconds": 0,
		"severity":         "warning",
		"channels":         []string{chID},
	}, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create rule status %d body=%s", rec.Code, rec.Body.String())
	}
	rBody := decodeBody(t, rec)
	rData, _ := rBody["data"].(map[string]any)
	ruleID, _ := rData["id"].(string)

	// List rules.
	rec = doJSON(t, eng, http.MethodGet, "/alerts/rules", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list rules status %d body=%s", rec.Code, rec.Body.String())
	}
	listBody := decodeBody(t, rec)
	if items, _ := listBody["data"].([]any); len(items) != 1 {
		t.Errorf("expected 1 rule, got %d", len(items))
	}

	// Patch: disable.
	disabled := false
	rec = doJSON(t, eng, http.MethodPatch, "/alerts/rules/"+ruleID, map[string]any{
		"enabled": disabled,
	}, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch rule status %d body=%s", rec.Code, rec.Body.String())
	}
	pBody := decodeBody(t, rec)
	pData, _ := pBody["data"].(map[string]any)
	if got, _ := pData["enabled"].(bool); got {
		t.Errorf("patch should have disabled rule, got %v", pData)
	}

	// Test endpoint hits the fake telegram server even when disabled
	// (Force bypasses gates).
	rec = doJSON(t, eng, http.MethodPost, "/alerts/rules/"+ruleID+"/test", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("test rule status %d body=%s", rec.Code, rec.Body.String())
	}
	if calls != 1 {
		t.Errorf("expected 1 telegram call from rule test, got %d", calls)
	}
	tBody := decodeBody(t, rec)
	tData, _ := tBody["data"].(map[string]any)
	delivered, _ := tData["delivered"].([]any)
	if len(delivered) != 1 {
		t.Errorf("expected 1 delivered channel, got %v", tData)
	}

	// Delete.
	rec = doJSON(t, eng, http.MethodDelete, "/alerts/rules/"+ruleID, nil, tok)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete rule status %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestNotificationsViewerForbiddenOnWrites(t *testing.T) {
	a := newTestApp(t)

	tgsrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(tgsrv.Close)

	wireNotifier(t, a, tgsrv.URL)
	seedUser(t, a, "viewer1", "Viewer123!", "viewer")

	eng := buildTestEngine(t, a)
	tok := loginAs(t, eng, "viewer1", "Viewer123!")

	// Viewer can list (read).
	rec := doJSON(t, eng, http.MethodGet, "/notifications/channels", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer GET channels status %d body=%s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, eng, http.MethodGet, "/alerts/rules", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer GET rules status %d body=%s", rec.Code, rec.Body.String())
	}

	// Viewer cannot create channels.
	rec = doJSON(t, eng, http.MethodPost, "/notifications/channels", map[string]any{
		"name": "x", "type": "telegram",
		"config": map[string]any{"bot_token": "t", "chat_id": "1"},
	}, tok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer POST channel status %d body=%s", rec.Code, rec.Body.String())
	}
	// Viewer cannot create rules.
	rec = doJSON(t, eng, http.MethodPost, "/alerts/rules", map[string]any{
		"name": "r", "type": "system_cpu", "comparator": "gte",
		"threshold": 50, "severity": "warning", "channels": []string{"x"},
	}, tok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer POST rule status %d body=%s", rec.Code, rec.Body.String())
	}
}
