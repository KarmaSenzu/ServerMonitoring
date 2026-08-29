package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"vps-dashboard-api/internal/models"
)

func TestProjectsCRUDFlow(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	tok := loginAs(t, eng, testUsername, testPassword)

	// Create.
	body := map[string]any{
		"name":           "My App",
		"description":    "first",
		"domain":         "app.example.com",
		"port":           3000,
		"container_name": "my-app",
		"pm2_name":       "my-app",
		"health_url":     "https://app.example.com/health",
		"tags":           []string{"prod", "api"},
	}
	rec := doJSON(t, eng, http.MethodPost, "/projects", body, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST status %d body=%s", rec.Code, rec.Body.String())
	}
	created := decodeBody(t, rec)
	data, _ := created["data"].(map[string]any)
	id, _ := data["id"].(string)
	if id == "" {
		t.Fatalf("missing id in %v", created)
	}

	// List.
	rec = doJSON(t, eng, http.MethodGet, "/projects", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET list status %d body=%s", rec.Code, rec.Body.String())
	}
	list := decodeBody(t, rec)
	items, _ := list["data"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 project, got %d", len(items))
	}

	// Get.
	rec = doJSON(t, eng, http.MethodGet, "/projects/"+id, nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET one status %d body=%s", rec.Code, rec.Body.String())
	}

	// Patch.
	rec = doJSON(t, eng, http.MethodPatch, "/projects/"+id, map[string]any{
		"description": "updated",
	}, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH status %d body=%s", rec.Code, rec.Body.String())
	}
	patched := decodeBody(t, rec)
	pdata, _ := patched["data"].(map[string]any)
	if pdata["description"] != "updated" {
		t.Errorf("description not updated: %v", pdata)
	}

	// Put (replace).
	rec = doJSON(t, eng, http.MethodPut, "/projects/"+id, map[string]any{
		"name": "My App",
		"port": 3001,
	}, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status %d body=%s", rec.Code, rec.Body.String())
	}

	// Duplicate name.
	rec = doJSON(t, eng, http.MethodPost, "/projects", map[string]any{
		"name": "My App",
	}, tok)
	if rec.Code != http.StatusConflict {
		t.Fatalf("dup status %d body=%s", rec.Code, rec.Body.String())
	}

	// Delete.
	rec = doJSON(t, eng, http.MethodDelete, "/projects/"+id, nil, tok)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status %d body=%s", rec.Code, rec.Body.String())
	}

	// Get after delete -> 404.
	rec = doJSON(t, eng, http.MethodGet, "/projects/"+id, nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET after delete status %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProjectsViewerForbiddenOnWrites(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	seedUser(t, a, "viewer1", "Viewer123!", "viewer")
	tok := loginAs(t, eng, "viewer1", "Viewer123!")

	// Viewer can list (read).
	rec := doJSON(t, eng, http.MethodGet, "/projects", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer GET status %d body=%s", rec.Code, rec.Body.String())
	}

	// Viewer cannot create.
	rec = doJSON(t, eng, http.MethodPost, "/projects", map[string]any{
		"name": "Forbidden",
	}, tok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer POST status %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProjectsGetUnknown404(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	tok := loginAs(t, eng, testUsername, testPassword)

	rec := doJSON(t, eng, http.MethodGet, "/projects/does-not-exist", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProjectHealthCheckOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	tok := loginAs(t, eng, testUsername, testPassword)

	// Create a project pointing to the test server.
	rec := doJSON(t, eng, http.MethodPost, "/projects", map[string]any{
		"name":       "health-app",
		"health_url": srv.URL + "/",
	}, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d body=%s", rec.Code, rec.Body.String())
	}
	created := decodeBody(t, rec)
	data, _ := created["data"].(map[string]any)
	id, _ := data["id"].(string)
	if id == "" {
		t.Fatalf("missing id in %v", created)
	}

	rec = doJSON(t, eng, http.MethodGet, "/projects/"+id+"/health", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("health: %d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	hd, _ := body["data"].(map[string]any)
	if ok, _ := hd["ok"].(bool); !ok {
		t.Errorf("expected ok=true, got %v", hd)
	}
	if status, _ := hd["status_code"].(float64); int(status) != http.StatusOK {
		t.Errorf("expected status_code=200, got %v", hd["status_code"])
	}
}

func TestProjectHealthMissingURL(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	tok := loginAs(t, eng, testUsername, testPassword)

	repo := models.NewProjectRepo(a.DB)
	p, err := repo.Create(context.Background(), models.Project{Name: "no-health", Enabled: true})
	if err != nil {
		t.Fatalf("repo.Create: %v", err)
	}

	rec := doJSON(t, eng, http.MethodGet, "/projects/"+p.ID+"/health", nil, tok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProjectActionNoRuntime(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	tok := loginAs(t, eng, testUsername, testPassword)

	rec := doJSON(t, eng, http.MethodPost, "/projects", map[string]any{
		"name": "runtime-less",
	}, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d body=%s", rec.Code, rec.Body.String())
	}
	created := decodeBody(t, rec)
	data, _ := created["data"].(map[string]any)
	id, _ := data["id"].(string)

	rec = doJSON(t, eng, http.MethodPost, "/projects/"+id+"/restart", nil, tok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 no_runtime, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	if body["error"] != "no_runtime" {
		t.Errorf("error: got %v want no_runtime", body["error"])
	}
}

// TestProjectHealthHistoryAfterCheck calls GET /projects/:id/health (which
// should persist a row) and then GET /projects/:id/health-history to confirm
// the row is returned in the history listing.
func TestProjectHealthHistoryAfterCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := newTestApp(t)
	a.Health = models.NewHealthRepo(a.DB) // wire history persistence path
	eng := buildTestEngine(t, a)
	tok := loginAs(t, eng, testUsername, testPassword)

	// Create a project pointing to the test server.
	rec := doJSON(t, eng, http.MethodPost, "/projects", map[string]any{
		"name":       "history-app",
		"health_url": srv.URL + "/",
	}, tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d body=%s", rec.Code, rec.Body.String())
	}
	created := decodeBody(t, rec)
	data, _ := created["data"].(map[string]any)
	id, _ := data["id"].(string)

	// Hit /health twice so the history has 2 rows.
	for i := 0; i < 2; i++ {
		rec = doJSON(t, eng, http.MethodGet, "/projects/"+id+"/health", nil, tok)
		if rec.Code != http.StatusOK {
			t.Fatalf("health %d: %d body=%s", i, rec.Code, rec.Body.String())
		}
	}

	rec = doJSON(t, eng, http.MethodGet, "/projects/"+id+"/health-history", nil, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("history: %d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	items, _ := body["data"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 history rows, got %d body=%s", len(items), rec.Body.String())
	}
	first, _ := items[0].(map[string]any)
	if ok, _ := first["ok"].(bool); !ok {
		t.Errorf("history[0].ok should be true, got %v", first)
	}
}

func TestProjectHealthHistoryUnknown404(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	tok := loginAs(t, eng, testUsername, testPassword)

	rec := doJSON(t, eng, http.MethodGet, "/projects/does-not-exist/health-history", nil, tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}
