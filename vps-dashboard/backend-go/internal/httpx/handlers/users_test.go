package handlers_test

import (
	"net/http"
	"testing"
)

func TestUsersListAsAdmin(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	tok := loginAs(t, eng, testUsername, testPassword)

	rec := doJSON(t, eng, http.MethodGet, "/users", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status %d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	items, _ := body["data"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 user, got %d", len(items))
	}
}

func TestUsersListAsViewerForbidden(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	seedUser(t, a, "viewer1", "Viewer123!", "viewer")
	tok := loginAs(t, eng, "viewer1", "Viewer123!")

	rec := doJSON(t, eng, http.MethodGet, "/users", nil, tok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUsersCreateAndPatch(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	tok := loginAs(t, eng, testUsername, testPassword)

	rec := doJSON(t, eng, http.MethodPost, "/users", map[string]any{
		"username": "viewer1",
		"password": "Viewer123!",
		"role":     "viewer",
	}, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST status %d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	data, _ := body["data"].(map[string]any)
	id, _ := data["id"].(string)
	if id == "" {
		t.Fatalf("missing id: %v", body)
	}

	// Duplicate username -> 409.
	rec = doJSON(t, eng, http.MethodPost, "/users", map[string]any{
		"username": "viewer1",
		"password": "Viewer123!",
		"role":     "viewer",
	}, tok)
	if rec.Code != http.StatusConflict {
		t.Fatalf("dup status %d body=%s", rec.Code, rec.Body.String())
	}

	// Patch role to admin.
	rec = doJSON(t, eng, http.MethodPatch, "/users/"+id, map[string]any{
		"role": "admin",
	}, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH status %d body=%s", rec.Code, rec.Body.String())
	}

	// Patch with no fields -> 400.
	rec = doJSON(t, eng, http.MethodPatch, "/users/"+id, map[string]any{}, tok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty PATCH status %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUsersDeleteSelfRefused(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	tok := loginAs(t, eng, testUsername, testPassword)

	// Discover own id via /auth/me.
	rec := doJSON(t, eng, http.MethodGet, "/auth/me", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /auth/me status %d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	user, _ := body["user"].(map[string]any)
	id, _ := user["id"].(string)
	if id == "" {
		t.Fatalf("missing id: %v", body)
	}

	rec = doJSON(t, eng, http.MethodDelete, "/users/"+id, nil, tok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("self-delete status %d body=%s", rec.Code, rec.Body.String())
	}
	out := decodeBody(t, rec)
	if out["error"] != "cannot_delete_self" {
		t.Errorf("error = %v want cannot_delete_self", out["error"])
	}
}

func TestUsersDeleteLastAdminRefused(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)

	// Create a second admin and a viewer; log in as the second admin.
	seedUser(t, a, "admin2", "Admin222!", "admin")
	tok := loginAs(t, eng, "admin2", "Admin222!")

	// Delete the bootstrap admin (testUsername). That should succeed
	// because admin2 still exists.
	rec := doJSON(t, eng, http.MethodGet, "/users", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status %d body=%s", rec.Code, rec.Body.String())
	}
	list := decodeBody(t, rec)
	items, _ := list["data"].([]any)

	var firstAdminID string
	for _, raw := range items {
		u, _ := raw.(map[string]any)
		if u["username"] == testUsername {
			firstAdminID, _ = u["id"].(string)
			break
		}
	}
	if firstAdminID == "" {
		t.Fatalf("could not find bootstrap admin in %v", items)
	}

	rec = doJSON(t, eng, http.MethodDelete, "/users/"+firstAdminID, nil, tok)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete first admin status %d body=%s", rec.Code, rec.Body.String())
	}

	// Now deleting admin2 (the only remaining admin) should be refused.
	rec = doJSON(t, eng, http.MethodGet, "/auth/me", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET me status %d", rec.Code)
	}
	body := decodeBody(t, rec)
	user, _ := body["user"].(map[string]any)
	myID, _ := user["id"].(string)

	// Deleting yourself returns cannot_delete_self before last_admin —
	// but if we delete admin2 via someone else it's last_admin. We
	// simulate that by attempting to demote admin2 instead.
	rec = doJSON(t, eng, http.MethodPatch, "/users/"+myID, map[string]any{
		"role": "viewer",
	}, tok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("demote last admin status %d body=%s", rec.Code, rec.Body.String())
	}
	out := decodeBody(t, rec)
	if out["error"] != "last_admin" {
		t.Errorf("error = %v want last_admin", out["error"])
	}
}
