package handlers_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"vps-dashboard-api/internal/ssh"
)

// newSSHKeyStore builds a scratch key store bound to the test app's
// configured dir.
func newSSHKeyStore(t *testing.T, a *sshTestApp) *ssh.KeyStore {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "ssh-keys")
	ks, err := ssh.NewKeyStore(dir)
	if err != nil {
		t.Fatalf("NewKeyStore: %v", err)
	}
	a.sshDir = dir
	return ks
}

type sshTestApp struct {
	sshDir string
}

func TestSSHKeysHandlerCRUD(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	token := loginAs(t, eng, testUsername, testPassword)

	// Generate a key.
	rec := doJSON(t, eng, http.MethodPost, "/ssh/keys/generate", map[string]any{
		"name":    "production-key",
		"comment": "dashboard",
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("generate: status %d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	data, _ := body["data"].(map[string]any)
	meta, _ := data["meta"].(map[string]any)
	if meta == nil || meta["name"] != "production-key" {
		t.Fatalf("generate meta: %v", data)
	}
	if pub, _ := data["public_key"].(string); pub == "" {
		t.Fatal("public_key missing")
	}

	// List keys.
	rec = doJSON(t, eng, http.MethodGet, "/ssh/keys", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status %d", rec.Code)
	}
	body = decodeBody(t, rec)
	items, _ := body["data"].([]any)
	if len(items) != 1 {
		t.Fatalf("list: got %d items", len(items))
	}
	item := items[0].(map[string]any)
	// No private material may leak.
	raw, _ := json.Marshal(item)
	if containsStr(string(raw), "PRIVATE KEY") {
		t.Fatalf("private key material leaked: %s", raw)
	}
	if item["fingerprint"] == "" || item["fingerprint"].(string)[:7] != "SHA256:" {
		t.Fatalf("fingerprint: %v", item["fingerprint"])
	}

	// Get public line.
	rec = doJSON(t, eng, http.MethodGet, "/ssh/keys/production-key/public", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("public: status %d", rec.Code)
	}

	// Duplicate name is rejected.
	rec = doJSON(t, eng, http.MethodPost, "/ssh/keys/generate", map[string]any{
		"name": "production-key",
	}, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate generate: expected 409, got %d", rec.Code)
	}

	// Add an invalid key.
	rec = doJSON(t, eng, http.MethodPost, "/ssh/keys", map[string]any{
		"name":        "bad-key",
		"private_key": "not a pem",
	}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid add: expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}

	// Delete.
	rec = doJSON(t, eng, http.MethodDelete, "/ssh/keys/production-key", nil, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status %d", rec.Code)
	}

	// List is empty again.
	rec = doJSON(t, eng, http.MethodGet, "/ssh/keys", nil, token)
	body = decodeBody(t, rec)
	items, _ = body["data"].([]any)
	if len(items) != 0 {
		t.Fatalf("list after delete: %d", len(items))
	}
}

func TestSSHKeysHandlerAuth(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)

	// Unauthenticated.
	rec := doJSON(t, eng, http.MethodGet, "/ssh/keys", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list: expected 401, got %d", rec.Code)
	}
	rec = doJSON(t, eng, http.MethodPost, "/ssh/keys/generate", map[string]any{"name": "x"}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated generate: expected 401, got %d", rec.Code)
	}

	// Viewer cannot manage keys.
	_ = seedUser(t, a, "viewer1", "viewer-pass-123", "viewer")
	viewerToken := loginAs(t, eng, "viewer1", "viewer-pass-123")
	rec = doJSON(t, eng, http.MethodPost, "/ssh/keys/generate", map[string]any{"name": "x"}, viewerToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer generate: expected 403, got %d", rec.Code)
	}
	rec = doJSON(t, eng, http.MethodDelete, "/ssh/keys/x", nil, viewerToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer delete: expected 403, got %d", rec.Code)
	}
}

func TestSSHCommandRequiresServer(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	token := loginAs(t, eng, testUsername, testPassword)

	// Unknown server → 404.
	rec := doJSON(t, eng, http.MethodPost, "/ssh/command/nope", map[string]any{
		"command": "uptime",
	}, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("command unknown server: expected 404, got %d", rec.Code)
	}

	// Unknown server → 404 for test too.
	rec = doJSON(t, eng, http.MethodPost, "/ssh/test/nope", nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("test unknown server: expected 404, got %d", rec.Code)
	}
}

func TestSSHCommandValidation(t *testing.T) {
	a := newTestApp(t)
	eng := buildTestEngine(t, a)
	token := loginAs(t, eng, testUsername, testPassword)

	// Create a server row first.
	rec := doJSON(t, eng, http.MethodPost, "/servers", serverPayload("alpha"), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create server: %d", rec.Code)
	}
	body := decodeBody(t, rec)
	data := body["data"].(map[string]any)
	id := data["id"].(string)

	// Missing command → 400.
	rec = doJSON(t, eng, http.MethodPost, "/ssh/command/"+id, map[string]any{}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty command: expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}

	// Credential not configured (no key in store) → 400.
	rec = doJSON(t, eng, http.MethodPost, "/ssh/command/"+id, map[string]any{
		"command": "uptime",
	}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing credential: expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if body := decodeBody(t, rec); body["error"] != "ssh_credential_not_configured" {
		t.Fatalf("error code: %v", body["error"])
	}

	// Test endpoint with the same missing credential.
	rec = doJSON(t, eng, http.MethodPost, "/ssh/test/"+id, nil, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("test missing credential: expected 400, got %d", rec.Code)
	}
}

// TestSSHKeyStorePermission verifies the on-disk key store directory
// exists with restrictive permissions after generate.
func TestSSHKeyStorePermission(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ssh-keys")
	ks, err := ssh.NewKeyStore(dir)
	if err != nil {
		t.Fatalf("NewKeyStore: %v", err)
	}
	if _, _, err := ks.Generate("alpha", ""); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "alpha"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Fatalf("key file permission: %o, want 600", perm)
	}
}
