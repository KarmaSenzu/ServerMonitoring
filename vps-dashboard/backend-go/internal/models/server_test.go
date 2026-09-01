package models_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/db"
	"vps-dashboard-api/internal/models"
)

func newServerRepo(t *testing.T) *models.ServerRepo {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "servers_test.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := db.Migrate(context.Background(), conn, zerolog.New(io.Discard)); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	return models.NewServerRepo(conn)
}

func testServer(name string) models.Server {
	return models.Server{
		Name:           name,
		Hostname:       name + ".example.com",
		IPAddress:      "203.0.113.10",
		SSHPort:        22,
		SSHUsername:    "deploy",
		CredentialType: models.ServerCredentialSSHKey,
		CredentialRef:  "production-key",
		Environment:    models.ServerEnvProduction,
		Tags:           []string{"production", "web"},
		Enabled:        true,
	}
}

func TestServerRepoCreateGet(t *testing.T) {
	r := newServerRepo(t)
	ctx := context.Background()

	created, err := r.Create(ctx, testServer("alpha"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected generated id")
	}
	if created.SSHPort != 22 {
		t.Errorf("SSHPort default: got %d", created.SSHPort)
	}
	if len(created.Tags) != 2 {
		t.Errorf("Tags round-trip: got %v", created.Tags)
	}
	if created.Status != models.ServerStatusUnknown {
		t.Errorf("default status: got %q", created.Status)
	}
	if created.CreatedAt.IsZero() {
		t.Errorf("CreatedAt zero")
	}

	got, err := r.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "alpha" {
		t.Errorf("Get.Name: %q", got.Name)
	}
	if len(got.Tags) != 2 {
		t.Errorf("Get.Tags: %v", got.Tags)
	}

	byName, err := r.GetByName(ctx, "alpha")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if byName.ID != created.ID {
		t.Errorf("GetByName id mismatch")
	}
}

func TestServerRepoDuplicateName(t *testing.T) {
	r := newServerRepo(t)
	ctx := context.Background()

	if _, err := r.Create(ctx, testServer("alpha")); err != nil {
		t.Fatalf("Create alpha: %v", err)
	}
	_, err := r.Create(ctx, testServer("alpha"))
	if !errors.Is(err, models.ErrServerDuplicateName) {
		t.Fatalf("expected ErrServerDuplicateName, got %v", err)
	}
}

func TestServerRepoUpdateReplacesTags(t *testing.T) {
	r := newServerRepo(t)
	ctx := context.Background()

	created, err := r.Create(ctx, testServer("alpha"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	created.Tags = []string{"database", "critical"}
	created.Notes = "updated"
	updated, err := r.Update(ctx, created)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(updated.Tags) != 2 {
		t.Fatalf("Update tags: %v", updated.Tags)
	}
	if updated.Tags[0] != "critical" || updated.Tags[1] != "database" {
		t.Errorf("tags sorted: %v", updated.Tags)
	}
	if updated.Notes != "updated" {
		t.Errorf("notes not persisted")
	}

	got, _ := r.Get(ctx, created.ID)
	if len(got.Tags) != 2 {
		t.Errorf("Get after update tags: %v", got.Tags)
	}
}

func TestServerRepoUpdatePreservesStatus(t *testing.T) {
	r := newServerRepo(t)
	ctx := context.Background()

	created, err := r.Create(ctx, testServer("alpha"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := r.SetStatus(ctx, created.ID, models.ServerStatusOnline, "ssh ok", time.Now().UTC()); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	// A subsequent full Update must not clobber monitoring-owned fields.
	created.Notes = "refreshed"
	updated, err := r.Update(ctx, created)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Status != models.ServerStatusOnline {
		t.Errorf("status clobbered: %q", updated.Status)
	}
	if updated.LastSeenAt.IsZero() {
		t.Errorf("last_seen clobbered")
	}
}

func TestServerRepoListFilter(t *testing.T) {
	r := newServerRepo(t)
	ctx := context.Background()

	a := testServer("alpha")
	a.Environment = models.ServerEnvProduction
	a.Tags = []string{"web"}
	a.Status = models.ServerStatusOnline
	if _, err := r.Create(ctx, a); err != nil {
		t.Fatalf("Create alpha: %v", err)
	}

	b := testServer("bravo")
	b.Environment = models.ServerEnvStaging
	b.Tags = []string{"db"}
	b.Enabled = false
	if _, err := r.Create(ctx, b); err != nil {
		t.Fatalf("Create bravo: %v", err)
	}

	all, err := r.List(ctx, models.ServerFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List: got %d want 2", len(all))
	}

	staging, err := r.List(ctx, models.ServerFilter{Environment: models.ServerEnvStaging})
	if err != nil {
		t.Fatalf("List staging: %v", err)
	}
	if len(staging) != 1 || staging[0].Name != "bravo" {
		t.Errorf("staging filter: %+v", staging)
	}

	enabledOnly, err := r.List(ctx, models.ServerFilter{EnabledOnly: true})
	if err != nil {
		t.Fatalf("List enabled: %v", err)
	}
	if len(enabledOnly) != 1 || enabledOnly[0].Name != "alpha" {
		t.Errorf("enabled filter: %+v", enabledOnly)
	}

	byTag, err := r.List(ctx, models.ServerFilter{Tag: "web"})
	if err != nil {
		t.Fatalf("List tag: %v", err)
	}
	if len(byTag) != 1 || byTag[0].Name != "alpha" {
		t.Errorf("tag filter: %+v", byTag)
	}

	byStatus, err := r.List(ctx, models.ServerFilter{Status: models.ServerStatusOnline})
	if err != nil {
		t.Fatalf("List status: %v", err)
	}
	if len(byStatus) != 1 || byStatus[0].Name != "alpha" {
		t.Errorf("status filter: %+v", byStatus)
	}

	searched, err := r.List(ctx, models.ServerFilter{Search: "bravo"})
	if err != nil {
		t.Fatalf("List search: %v", err)
	}
	if len(searched) != 1 {
		t.Errorf("search filter: %+v", searched)
	}
}

func TestServerRepoDelete(t *testing.T) {
	r := newServerRepo(t)
	ctx := context.Background()

	created, err := r.Create(ctx, testServer("alpha"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := r.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.Get(ctx, created.ID); !errors.Is(err, models.ErrServerNotFound) {
		t.Fatalf("expected ErrServerNotFound, got %v", err)
	}
	if err := r.Delete(ctx, created.ID); !errors.Is(err, models.ErrServerNotFound) {
		t.Fatalf("double delete: expected ErrServerNotFound, got %v", err)
	}
}

func TestServerRepoListTags(t *testing.T) {
	r := newServerRepo(t)
	ctx := context.Background()

	if _, err := r.Create(ctx, testServer("alpha")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	b := testServer("bravo")
	b.Tags = []string{"database"}
	if _, err := r.Create(ctx, b); err != nil {
		t.Fatalf("Create: %v", err)
	}

	tags, err := r.ListTags(ctx)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	// production + web + database
	if len(names) != 3 {
		t.Fatalf("tag catalogue: %v", names)
	}
}

func TestServerRepoTagDetachOnUpdate(t *testing.T) {
	r := newServerRepo(t)
	ctx := context.Background()

	created, err := r.Create(ctx, testServer("alpha"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Remove all tags.
	created.Tags = nil
	updated, err := r.Update(ctx, created)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(updated.Tags) != 0 {
		t.Errorf("tags not detached: %v", updated.Tags)
	}
}

func TestServerValidate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*models.Server)
		wantErr string
	}{
		{"valid", func(s *models.Server) {}, ""},
		{"missing name", func(s *models.Server) { s.Name = "" }, "name"},
		{"missing hostname", func(s *models.Server) { s.Hostname = "" }, "hostname"},
		{
			"bad ip",
			func(s *models.Server) { s.IPAddress = "999.999.999.999" },
			"ip_address",
		},
		{
			"bad port",
			func(s *models.Server) { s.SSHPort = 70000 },
			"ssh_port",
		},
		{
			"missing ssh username",
			func(s *models.Server) { s.SSHUsername = "" },
			"ssh_username",
		},
		{
			"bad credential type",
			func(s *models.Server) { s.CredentialType = "magic" },
			"credential_type",
		},
		{
			"bad environment",
			func(s *models.Server) { s.Environment = "qa" },
			"environment",
		},
		{
			"bad tag",
			func(s *models.Server) { s.Tags = []string{"!!"} },
			"tags",
		},
		{
			"ipv6 hostname ok",
			func(s *models.Server) { s.Hostname = "2001:db8::1" },
			"",
		},
		{
			"ipv4 hostname ok",
			func(s *models.Server) { s.Hostname = "10.0.0.5" },
			"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := testServer("alpha")
			tc.mutate(&s)
			err := s.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not mention %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
