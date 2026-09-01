package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"vps-dashboard-api/internal/models"
)

func serverPayload(name string) map[string]any {
	return map[string]any{
		"name":            name,
		"hostname":        name + ".example.com",
		"ip_address":      "203.0.113.10",
		"ssh_port":        22,
		"ssh_username":    "deploy",
		"credential_type": "ssh_key",
		"credential_ref":  "production-key",
		"environment":     "production",
		"tags":            []string{"production", "web"},
		"notes":           "",
		"enabled":         true,
	}
}

func TestServersHandlerCRUD(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	token := loginAs(t, eng, testUsername, testPassword)

	// Create.
	rec := doJSON(t, eng, http.MethodPost, "/servers", serverPayload("alpha"), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status %d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("create data missing: %v", body)
	}
	id, _ := data["id"].(string)
	if id == "" {
		t.Fatal("created id missing")
	}
	if data["status"] != models.ServerStatusUnknown {
		t.Errorf("default status: %v", data["status"])
	}
	if data["credential_present"] != true {
		t.Errorf("credential_present: %v", data["credential_present"])
	}
	// No raw secret material may leak into responses.
	raw, _ := json.Marshal(data)
	if containsStr(string(raw), "-----BEGIN") {
		t.Errorf("secret material in response: %s", raw)
	}

	// List.
	rec = doJSON(t, eng, http.MethodGet, "/servers", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status %d", rec.Code)
	}
	body = decodeBody(t, rec)
	items, _ := body["data"].([]any)
	if len(items) != 1 {
		t.Fatalf("list: got %d items", len(items))
	}

	// Get.
	rec = doJSON(t, eng, http.MethodGet, "/servers/"+id, nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: status %d", rec.Code)
	}

	// PATCH: change environment only.
	rec = doJSON(t, eng, http.MethodPatch, "/servers/"+id, map[string]any{"environment": "staging"}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: status %d body=%s", rec.Code, rec.Body.String())
	}
	body = decodeBody(t, rec)
	data, _ = body["data"].(map[string]any)
	if data["environment"] != "staging" {
		t.Errorf("patch environment: %v", data["environment"])
	}
	if data["name"] != "alpha" {
		t.Errorf("patch clobbered name: %v", data["name"])
	}

	// Tags catalogue endpoint.
	rec = doJSON(t, eng, http.MethodGet, "/servers/tags", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("tags: status %d", rec.Code)
	}

	// Delete.
	rec = doJSON(t, eng, http.MethodDelete, "/servers/"+id, nil, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status %d body=%s", rec.Code, rec.Body.String())
	}

	// Get after delete → 404.
	rec = doJSON(t, eng, http.MethodGet, "/servers/"+id, nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: status %d", rec.Code)
	}
}

func TestServersHandlerValidation(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	token := loginAs(t, eng, testUsername, testPassword)

	cases := []struct {
		name   string
		mutate func(p map[string]any)
	}{
		{"missing name", func(p map[string]any) { p["name"] = "" }},
		{"missing hostname", func(p map[string]any) { p["hostname"] = "" }},
		{"bad port", func(p map[string]any) { p["ssh_port"] = 70000 }},
		{"bad ip", func(p map[string]any) { p["ip_address"] = "999.1.1.1" }},
		{"missing ssh username", func(p map[string]any) { p["ssh_username"] = "" }},
		{"bad env", func(p map[string]any) { p["environment"] = "qa" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := serverPayload("bad")
			tc.mutate(p)
			rec := doJSON(t, eng, http.MethodPost, "/servers", p, token)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestServersHandlerDuplicateName(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	token := loginAs(t, eng, testUsername, testPassword)

	rec := doJSON(t, eng, http.MethodPost, "/servers", serverPayload("alpha"), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create 1: status %d", rec.Code)
	}
	rec = doJSON(t, eng, http.MethodPost, "/servers", serverPayload("alpha"), token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("create 2: expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestServersHandlerRequiresAuth(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)

	rec := doJSON(t, eng, http.MethodGet, "/servers", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list: expected 401, got %d", rec.Code)
	}
	rec = doJSON(t, eng, http.MethodPost, "/servers", serverPayload("alpha"), "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated create: expected 401, got %d", rec.Code)
	}
}

func TestServersHandlerViewerCannotWrite(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	_ = seedUser(t, a, "viewer1", "viewer-pass-123", "viewer")
	viewerToken := loginAs(t, eng, "viewer1", "viewer-pass-123")

	rec := doJSON(t, eng, http.MethodPost, "/servers", serverPayload("alpha"), viewerToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer create: expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestServersHandlerFiltering(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	token := loginAs(t, eng, testUsername, testPassword)

	p1 := serverPayload("alpha")
	p1["environment"] = "production"
	p1["tags"] = []string{"web"}
	rec := doJSON(t, eng, http.MethodPost, "/servers", p1, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create alpha: %d", rec.Code)
	}

	p2 := serverPayload("bravo")
	p2["environment"] = "staging"
	p2["tags"] = []string{"database"}
	rec = doJSON(t, eng, http.MethodPost, "/servers", p2, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bravo: %d", rec.Code)
	}

	// environment filter
	rec = doJSON(t, eng, http.MethodGet, "/servers?environment=staging", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list staging: %d", rec.Code)
	}
	body := decodeBody(t, rec)
	items, _ := body["data"].([]any)
	if len(items) != 1 {
		t.Fatalf("staging filter: got %d", len(items))
	}
	got := items[0].(map[string]any)
	if got["name"] != "bravo" {
		t.Errorf("staging filter name: %v", got["name"])
	}

	// tag filter
	rec = doJSON(t, eng, http.MethodGet, "/servers?tag=web", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list tag: %d", rec.Code)
	}
	body = decodeBody(t, rec)
	items, _ = body["data"].([]any)
	if len(items) != 1 {
		t.Fatalf("tag filter: got %d", len(items))
	}

	// search filter
	rec = doJSON(t, eng, http.MethodGet, "/servers?q=bravo", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list search: %d", rec.Code)
	}
	body = decodeBody(t, rec)
	items, _ = body["data"].([]any)
	if len(items) != 1 {
		t.Fatalf("search filter: got %d", len(items))
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
