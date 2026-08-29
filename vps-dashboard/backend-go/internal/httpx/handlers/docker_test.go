package handlers_test

import (
	"net/http"
	"testing"
)

func TestDockerStartInvalidNameReturns400(t *testing.T) {
	f := newTestFixture(t)
	tok := loginToken(t, f)

	rec := f.do(t, http.MethodPost, "/docker/start", map[string]string{
		"name": "; rm -rf /",
	}, map[string]string{
		"Authorization": "Bearer " + tok,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	body := parseJSON(t, rec)
	if body["error"] != "invalid_container_name" {
		t.Errorf("error: got %v want invalid_container_name", body["error"])
	}
}

func TestDockerStartByNameInvalidReturns400(t *testing.T) {
	f := newTestFixture(t)
	tok := loginToken(t, f)

	// URL path can't include a slash, but a leading dash still violates Docker rules.
	rec := f.do(t, http.MethodPost, "/docker/containers/-bad/start", nil, map[string]string{
		"Authorization": "Bearer " + tok,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	body := parseJSON(t, rec)
	if body["error"] != "invalid_container_name" {
		t.Errorf("error: got %v want invalid_container_name", body["error"])
	}
}

func TestDockerStartUnauthorized(t *testing.T) {
	f := newTestFixture(t)
	rec := f.do(t, http.MethodPost, "/docker/start", map[string]string{
		"name": "valid-name",
	}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestDockerListReachable(t *testing.T) {
	// We can't assert that docker is installed on the dev machine, but the
	// route MUST respond either 200 (docker available) or 503
	// (docker_unavailable). Anything else means the wiring is wrong.
	f := newTestFixture(t)
	tok := loginToken(t, f)
	rec := f.do(t, http.MethodGet, "/docker/containers", nil, map[string]string{
		"Authorization": "Bearer " + tok,
	})
	if rec.Code != http.StatusOK && rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d want 200 or 503 body=%s", rec.Code, rec.Body.String())
	}
}

func TestDockerLogsInvalidName(t *testing.T) {
	f := newTestFixture(t)
	tok := loginToken(t, f)
	rec := f.do(t, http.MethodGet, "/docker/containers/-bad/logs?tail=10", nil, map[string]string{
		"Authorization": "Bearer " + tok,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestDockerLogsValidNameNoDocker(t *testing.T) {
	// On a dev machine without docker, /logs must surface the same
	// error envelope as other docker reads (503). On a host with docker
	// installed but no such container we accept 500.
	f := newTestFixture(t)
	tok := loginToken(t, f)
	rec := f.do(t, http.MethodGet, "/docker/containers/probably-not-real-vpsd/logs?tail=10", nil, map[string]string{
		"Authorization": "Bearer " + tok,
	})
	switch rec.Code {
	case http.StatusOK, http.StatusServiceUnavailable, http.StatusInternalServerError:
		// ok
	default:
		t.Fatalf("status: got %d want 200/503/500 body=%s", rec.Code, rec.Body.String())
	}
}

func TestDockerLogsInvalidTail(t *testing.T) {
	f := newTestFixture(t)
	tok := loginToken(t, f)
	rec := f.do(t, http.MethodGet, "/docker/containers/foo/logs?tail=99999", nil, map[string]string{
		"Authorization": "Bearer " + tok,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
