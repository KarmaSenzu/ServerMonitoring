package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/auth"
	"vps-dashboard-api/internal/config"
	"vps-dashboard-api/internal/db"
	"vps-dashboard-api/internal/httpx"
	"vps-dashboard-api/internal/models"
)

const (
	testUsername = "testadmin"
	testPassword = "Test1234!"
)

type testFixture struct {
	engine *gin.Engine
	app    *app.App
}

func newTestFixture(t *testing.T) *testFixture {
	t.Helper()

	gin.SetMode(gin.TestMode)

	dbPath := filepath.Join(t.TempDir(), "auth_test.db")

	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	logger := zerolog.New(io.Discard)

	if err := db.Migrate(context.Background(), conn, logger); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	hash, err := auth.Hash(testPassword)
	if err != nil {
		t.Fatalf("auth.Hash: %v", err)
	}
	repo := models.NewUserRepo(conn)
	if _, err := repo.Create(context.Background(), testUsername, hash, "admin"); err != nil {
		t.Fatalf("repo.Create: %v", err)
	}

	cfg := &config.Config{
		Env:         "development",
		HTTPAddr:    ":0",
		DBPath:      dbPath,
		JWTSecret:   "test-secret-handlers",
		JWTTTL:      time.Hour,
		LogLevel:    "info",
		CORSOrigins: []string{"http://localhost"},
	}

	a := &app.App{Cfg: cfg, DB: conn, Logger: logger}
	return &testFixture{engine: httpx.BuildEngine(a), app: a}
}

func (f *testFixture) do(t *testing.T, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(buf)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	f.engine.ServeHTTP(rec, req)
	return rec
}

func parseJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal body %q: %v", rec.Body.String(), err)
	}
	return out
}

func TestLoginWrongPassword(t *testing.T) {
	f := newTestFixture(t)

	rec := f.do(t, http.MethodPost, "/auth/login", map[string]string{
		"username": testUsername,
		"password": "wrong",
	}, nil)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	body := parseJSON(t, rec)
	if body["error"] != "invalid_credentials" {
		t.Errorf("error: got %v want invalid_credentials", body["error"])
	}
}

func TestLoginUnknownUserSameError(t *testing.T) {
	f := newTestFixture(t)

	rec := f.do(t, http.MethodPost, "/auth/login", map[string]string{
		"username": "ghost",
		"password": "whatever",
	}, nil)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusUnauthorized)
	}
	body := parseJSON(t, rec)
	if body["error"] != "invalid_credentials" {
		t.Errorf("error: got %v want invalid_credentials", body["error"])
	}
}

func TestLoginSuccess(t *testing.T) {
	f := newTestFixture(t)

	rec := f.do(t, http.MethodPost, "/auth/login", map[string]string{
		"username": testUsername,
		"password": testPassword,
	}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := parseJSON(t, rec)
	tok, ok := body["token"].(string)
	if !ok || tok == "" {
		t.Fatalf("token missing or not string in %v", body)
	}

	user, ok := body["user"].(map[string]any)
	if !ok {
		t.Fatalf("user missing in %v", body)
	}
	if user["username"] != testUsername {
		t.Errorf("user.username: got %v want %s", user["username"], testUsername)
	}
	if user["role"] != "admin" {
		t.Errorf("user.role: got %v want admin", user["role"])
	}

	cookies := rec.Result().Cookies()
	var found bool
	for _, ck := range cookies {
		if ck.Name == "vpsd_token" && ck.Value != "" {
			found = true
			if !ck.HttpOnly {
				t.Errorf("expected vpsd_token cookie to be HttpOnly")
			}
		}
	}
	if !found {
		t.Errorf("expected vpsd_token cookie to be set, got %+v", cookies)
	}
}

func TestMeUnauthorized(t *testing.T) {
	f := newTestFixture(t)
	rec := f.do(t, http.MethodGet, "/auth/me", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func loginToken(t *testing.T, f *testFixture) string {
	t.Helper()
	rec := f.do(t, http.MethodPost, "/auth/login", map[string]string{
		"username": testUsername,
		"password": testPassword,
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status %d body=%s", rec.Code, rec.Body.String())
	}
	body := parseJSON(t, rec)
	return body["token"].(string)
}

func TestMeAuthorized(t *testing.T) {
	f := newTestFixture(t)
	tok := loginToken(t, f)

	rec := f.do(t, http.MethodGet, "/auth/me", nil, map[string]string{
		"Authorization": "Bearer " + tok,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := parseJSON(t, rec)
	user, ok := body["user"].(map[string]any)
	if !ok {
		t.Fatalf("user missing in %v", body)
	}
	if user["username"] != testUsername {
		t.Errorf("user.username: got %v want %s", user["username"], testUsername)
	}
}

func TestRefreshIssuesNewToken(t *testing.T) {
	f := newTestFixture(t)
	original := loginToken(t, f)

	// JWT NumericDate has 1-second granularity, so wait long enough to ensure
	// IssuedAt/ExpiresAt differ between the original and refreshed tokens.
	time.Sleep(1100 * time.Millisecond)

	rec := f.do(t, http.MethodPost, "/auth/refresh", nil, map[string]string{
		"Authorization": "Bearer " + original,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := parseJSON(t, rec)
	next, ok := body["token"].(string)
	if !ok || next == "" {
		t.Fatalf("refresh token missing in %v", body)
	}
	if next == original {
		t.Fatal("refreshed token equals original")
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	f := newTestFixture(t)
	rec := f.do(t, http.MethodPost, "/auth/logout", nil, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want %d", rec.Code, http.StatusNoContent)
	}
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "vpsd_token" && ck.MaxAge >= 0 && ck.Value != "" {
			t.Errorf("expected logout to clear vpsd_token, got %+v", ck)
		}
	}
}
