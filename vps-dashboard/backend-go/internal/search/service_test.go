package search_test

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/db"
	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/search"
)

func newSearchService(t *testing.T) *search.Service {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "search_test.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := db.Migrate(context.Background(), conn, zerolog.New(io.Discard)); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	serversRepo := models.NewServerRepo(conn)
	snippetsRepo := models.NewCommandSnippetRepo(conn)
	tunnelsRepo := models.NewTunnelRepo(conn)

	// Seed data.
	s1, err := serversRepo.Create(context.Background(), models.Server{
		Name:        "web-prod-01",
		Hostname:    "web-prod-01.example.com",
		IPAddress:   "10.0.1.10",
		SSHUsername: "deploy",
		Environment: "production",
		Tags:        []string{"production", "web", "critical"},
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("seed server 1: %v", err)
	}
	t.Logf("server 1: id=%s tags=%v", s1.ID, s1.Tags)

	s2, err := serversRepo.Create(context.Background(), models.Server{
		Name:        "db-staging-01",
		Hostname:    "db-staging-01.example.com",
		IPAddress:   "10.0.2.20",
		SSHUsername: "deploy",
		Environment: "staging",
		Tags:        []string{"staging", "database"},
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("seed server 2: %v", err)
	}
	t.Logf("server 2: id=%s tags=%v", s2.ID, s2.Tags)

	if _, err := snippetsRepo.Create(context.Background(), models.CommandSnippet{
		Name:        "check-docker",
		Command:     "docker ps -a",
		DangerLevel: models.DangerSafe,
	}); err != nil {
		t.Fatalf("seed snippet 1: %v", err)
	}
	if _, err := snippetsRepo.Create(context.Background(), models.CommandSnippet{
		Name:        "restart-nginx",
		Command:     "systemctl restart nginx",
		DangerLevel: models.DangerCaution,
	}); err != nil {
		t.Fatalf("seed snippet 2: %v", err)
	}

	if _, err := tunnelsRepo.Create(context.Background(), models.Tunnel{
		Name:       "db-tunnel",
		ServerID:   s1.ID,
		Type:       "local",
		LocalAddr:  "127.0.0.1:5432",
		RemoteAddr: "db.internal:5432",
	}); err != nil {
		t.Fatalf("seed tunnel: %v", err)
	}

	return search.NewService(serversRepo, snippetsRepo, tunnelsRepo)
}

func TestSearchServers(t *testing.T) {
	svc := newSearchService(t)
	results := svc.Search(context.Background(), "web-prod")
	if len(results.Servers) != 1 {
		t.Fatalf("servers: got %d want 1", len(results.Servers))
	}
	if results.Servers[0].Name != "web-prod-01" {
		t.Errorf("name: %q", results.Servers[0].Name)
	}
}

func TestSearchCommands(t *testing.T) {
	svc := newSearchService(t)
	results := svc.Search(context.Background(), "docker")
	if len(results.Commands) != 1 {
		t.Fatalf("commands: got %d want 1", len(results.Commands))
	}
	if results.Commands[0].Name != "check-docker" {
		t.Errorf("name: %q", results.Commands[0].Name)
	}
}

func TestSearchTunnels(t *testing.T) {
	svc := newSearchService(t)
	results := svc.Search(context.Background(), "db-tunnel")
	if len(results.Tunnels) != 1 {
		t.Fatalf("tunnels: got %d want 1", len(results.Tunnels))
	}
	if results.Tunnels[0].Name != "db-tunnel" {
		t.Errorf("name: %q", results.Tunnels[0].Name)
	}
}

func TestSearchTags(t *testing.T) {
	svc := newSearchService(t)
	results := svc.Search(context.Background(), "production")
	if len(results.Tags) != 1 {
		t.Fatalf("tags: got %d want 1", len(results.Tags))
	}
	if results.Tags[0].Name != "production" {
		t.Errorf("tag: %q", results.Tags[0].Name)
	}
}

func TestSearchEmpty(t *testing.T) {
	svc := newSearchService(t)
	results := svc.Search(context.Background(), "")
	if results.Total != 0 {
		t.Errorf("total: %d", results.Total)
	}
}

func TestSearchNoMatch(t *testing.T) {
	svc := newSearchService(t)
	results := svc.Search(context.Background(), "zzz-not-found")
	if results.Total != 0 {
		t.Errorf("total: %d", results.Total)
	}
}

func TestSearchAcrossKinds(t *testing.T) {
	svc := newSearchService(t)
	// "staging" matches server name (db-staging-01 has tag "staging")
	// and tag "staging".
	results := svc.Search(context.Background(), "staging")
	t.Logf("results: total=%d servers=%d commands=%d tunnels=%d tags=%d",
		results.Total, len(results.Servers), len(results.Commands), len(results.Tunnels), len(results.Tags))
	for i, tag := range results.Tags {
		t.Logf("  tag[%d]: name=%q", i, tag.Name)
	}
	if results.Total == 0 {
		t.Fatal("expected results for 'staging'")
	}
	// Should find the staging tag.
	found := false
	for _, tag := range results.Tags {
		if tag.Name == "staging" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'staging' tag in tags")
	}
}
