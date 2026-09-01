package mcp_test

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/db"
	"vps-dashboard-api/internal/mcp"
	"vps-dashboard-api/internal/models"
)

func newMCPServer(t *testing.T) *mcp.Server {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "mcp_test.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := db.Migrate(context.Background(), conn, zerolog.New(io.Discard)); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	serversRepo := models.NewServerRepo(conn)
	metricsRepo := models.NewServerMetricRepo(conn)
	eventsRepo := models.NewEventRepo(conn)
	snippetsRepo := models.NewCommandSnippetRepo(conn)
	tunnelsRepo := models.NewTunnelRepo(conn)

	// Seed data.
	serversRepo.Create(context.Background(), models.Server{
		Name: "web-prod-01", Hostname: "web-prod-01.example.com",
		IPAddress: "10.0.1.10", SSHUsername: "deploy", Environment: "production",
		Tags: []string{"production", "web"}, Enabled: true,
	})
	serversRepo.Create(context.Background(), models.Server{
		Name: "db-staging-01", Hostname: "db-staging-01.example.com",
		IPAddress: "10.0.2.20", SSHUsername: "deploy", Environment: "staging",
		Tags: []string{"staging", "database"}, Enabled: true,
	})

	auditPath := filepath.Join(t.TempDir(), "mcp-audit.jsonl")
	return mcp.NewServer(
		zerolog.New(io.Discard),
		serversRepo, metricsRepo, eventsRepo, snippetsRepo, tunnelsRepo,
		"test-api-key-12345",
		auditPath,
	)
}

func TestListTools(t *testing.T) {
	s := newMCPServer(t)
	tools := s.ListTools()
	if len(tools) < 5 {
		t.Fatalf("expected at least 5 tools, got %d", len(tools))
	}
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	expected := []string{"list_servers", "get_server", "list_events", "search_infrastructure", "list_tunnels", "list_snippets"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestListServers(t *testing.T) {
	s := newMCPServer(t)
	result, err := s.CallTool(context.Background(), "list_servers", nil)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("empty content")
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].Text)
	}
	// Should contain both server names.
	text := result.Content[0].Text
	if !contains(text, "web-prod-01") {
		t.Error("missing web-prod-01")
	}
	if !contains(text, "db-staging-01") {
		t.Error("missing db-staging-01")
	}
}

func TestGetServer(t *testing.T) {
	s := newMCPServer(t)
	// List servers to get an ID.
	listResult, _ := s.CallTool(context.Background(), "list_servers", nil)
	text := listResult.Content[0].Text
	// Find a server ID (UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx).
	// MarshalIndent uses ": " after key, so look for `"id": "`.
	idx := indexOf(text, `"id": "`)
	if idx < 0 {
		t.Fatalf("no id found in list output: %s", text[:min(len(text), 200)])
	}
	// Extract ID after the value start.
	idStart := idx + len(`"id": "`)
	idEnd := idStart
	for idEnd < len(text) && text[idEnd] != '"' {
		idEnd++
	}
	serverID := text[idStart:idEnd]
	if serverID == "" {
		t.Fatal("empty server ID")
	}

	result, err := s.CallTool(context.Background(), "get_server", map[string]any{
		"server_id": serverID,
	})
	if err != nil {
		t.Fatalf("CallTool get_server: %v", err)
	}
	if result.IsError {
		t.Fatalf("error: %s", result.Content[0].Text)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestSearchInfrastructure(t *testing.T) {
	s := newMCPServer(t)
	result, err := s.CallTool(context.Background(), "search_infrastructure", map[string]any{
		"query": "web-prod",
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("error: %s", result.Content[0].Text)
	}
	if !contains(result.Content[0].Text, "web-prod-01") {
		t.Error("expected web-prod-01 in search results")
	}
}

func TestUnknownTool(t *testing.T) {
	s := newMCPServer(t)
	_, err := s.CallTool(context.Background(), "nonexistent_tool", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestAPIKey(t *testing.T) {
	s := newMCPServer(t)
	if s.APIKey() != "test-api-key-12345" {
		t.Errorf("APIKey: %q", s.APIKey())
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
