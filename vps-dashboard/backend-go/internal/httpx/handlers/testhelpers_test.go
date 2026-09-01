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

// newTestApp opens a temp DB, runs migrations, and seeds a default admin
// user. The DB is closed automatically when the test finishes.
func newTestApp(t *testing.T) *app.App {
	t.Helper()

	gin.SetMode(gin.TestMode)

	dbPath := filepath.Join(t.TempDir(), "wave14_test.db")
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
		SSHKeysDir:  filepath.Join(t.TempDir(), "ssh-keys"),
	}

	return &app.App{
		Cfg:             cfg,
		DB:              conn,
		Logger:          logger,
		ServerMetrics:   models.NewServerMetricRepo(conn),
		ContainerService: nil, // handler falls back to a.SSHService
	}
}

// buildTestEngine returns the production gin.Engine wired against a.
func buildTestEngine(t *testing.T, a *app.App) *gin.Engine {
	t.Helper()
	return httpx.BuildEngine(a)
}

// loginAs performs an /auth/login request and returns the JWT.
func loginAs(t *testing.T, eng *gin.Engine, username, password string) string {
	t.Helper()

	body, err := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})
	if err != nil {
		t.Fatalf("marshal login: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	eng.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %q: status %d body=%s", username, rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("login decode: %v", err)
	}
	tok, ok := out["token"].(string)
	if !ok || tok == "" {
		t.Fatalf("login token missing in %v", out)
	}
	return tok
}

// seedUser inserts a user directly via the repo (bypassing the API).
// Useful for setting up viewers without going through admin routes.
func seedUser(t *testing.T, a *app.App, username, password, role string) *models.User {
	t.Helper()

	hash, err := auth.Hash(password)
	if err != nil {
		t.Fatalf("auth.Hash: %v", err)
	}
	repo := models.NewUserRepo(a.DB)
	u, err := repo.Create(context.Background(), username, hash, role)
	if err != nil {
		t.Fatalf("seedUser: %v", err)
	}
	return u
}

// doJSON performs an HTTP request against the engine with optional JSON
// body and bearer token, returning the recorder.
func doJSON(t *testing.T, eng *gin.Engine, method, path string, body any, token string) *httptest.ResponseRecorder {
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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	eng.ServeHTTP(rec, req)
	return rec
}

// decodeBody is a small wrapper for tests that just need the JSON map.
func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return out
}
