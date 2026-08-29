package handlers_test

import (
	"net/http"
	"testing"
)

func TestPM2ListReachable(t *testing.T) {
	f := newTestFixture(t)
	tok := loginToken(t, f)
	rec := f.do(t, http.MethodGet, "/pm2/processes", nil, map[string]string{
		"Authorization": "Bearer " + tok,
	})
	// Accept either 200 (pm2 installed locally) or 503 (pm2 not on PATH).
	if rec.Code != http.StatusOK && rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d want 200 or 503 body=%s", rec.Code, rec.Body.String())
	}
}

func TestPM2StartUnauthorized(t *testing.T) {
	f := newTestFixture(t)
	rec := f.do(t, http.MethodPost, "/pm2/processes/foo/start", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestPM2StartViewerForbidden(t *testing.T) {
	f := newTestFixture(t)
	seedUser(t, f.app, "viewer1", "Viewer123!", "viewer")
	tok := loginAs(t, f.engine, "viewer1", "Viewer123!")
	rec := f.do(t, http.MethodPost, "/pm2/processes/foo/start", nil, map[string]string{
		"Authorization": "Bearer " + tok,
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer POST: got %d want %d body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestPM2StartAdminInvalidName(t *testing.T) {
	f := newTestFixture(t)
	tok := loginToken(t, f)
	rec := f.do(t, http.MethodPost, "/pm2/processes/-bad/start", nil, map[string]string{
		"Authorization": "Bearer " + tok,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	body := parseJSON(t, rec)
	if body["error"] != "invalid_process_name" {
		t.Errorf("error: got %v want invalid_process_name", body["error"])
	}
}

func TestPM2StartAdminWhenPM2Missing(t *testing.T) {
	// When pm2 isn't installed on PATH we expect 503 pm2_unavailable.
	// On dev machines that DO have pm2, we accept 204 (success), 5xx
	// (pm2 errors), or 503 - we just want to confirm the request reached
	// the handler with a valid name.
	f := newTestFixture(t)
	tok := loginToken(t, f)
	rec := f.do(t, http.MethodPost, "/pm2/processes/never-exists-vpsd/start", nil, map[string]string{
		"Authorization": "Bearer " + tok,
	})
	switch rec.Code {
	case http.StatusNoContent, http.StatusServiceUnavailable, http.StatusInternalServerError:
		// ok
	default:
		t.Fatalf("status: got %d want 204/503/500 body=%s", rec.Code, rec.Body.String())
	}
}

func TestPM2LogsInvalidLines(t *testing.T) {
	f := newTestFixture(t)
	tok := loginToken(t, f)
	rec := f.do(t, http.MethodGet, "/pm2/processes/myproc/logs?lines=abc", nil, map[string]string{
		"Authorization": "Bearer " + tok,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
