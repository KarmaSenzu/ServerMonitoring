package models_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/db"
	"vps-dashboard-api/internal/models"
)

func newProjectRepo(t *testing.T) *models.ProjectRepo {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "projects_test.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := db.Migrate(context.Background(), conn, zerolog.New(io.Discard)); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	return models.NewProjectRepo(conn)
}

func TestProjectRepoCreateGet(t *testing.T) {
	r := newProjectRepo(t)
	ctx := context.Background()

	p := models.Project{
		Name:          "alpha",
		Description:   "first",
		Domain:        "alpha.example.com",
		Port:          3000,
		ContainerName: "alpha",
		PM2Name:       "alpha",
		TunnelService: "cloudflared-alpha",
		HealthURL:     "https://alpha.example.com/health",
		Enabled:       true,
		Tags:          []string{"prod", "api", "prod"},
		Notes:         "hello",
	}

	created, err := r.Create(ctx, p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected generated id")
	}
	if len(created.Tags) != 2 {
		t.Errorf("Tags should dedupe: got %v", created.Tags)
	}
	if !created.Enabled {
		t.Errorf("Enabled lost in round-trip")
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

	byName, err := r.GetByName(ctx, "alpha")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if byName.ID != created.ID {
		t.Errorf("GetByName id mismatch")
	}
}

func TestProjectRepoListAndCount(t *testing.T) {
	r := newProjectRepo(t)
	ctx := context.Background()

	if _, err := r.Create(ctx, models.Project{Name: "alpha", Domain: "alpha.example.com", Enabled: true, Tags: []string{"prod"}}); err != nil {
		t.Fatalf("Create alpha: %v", err)
	}
	if _, err := r.Create(ctx, models.Project{Name: "bravo", Domain: "bravo.example.com", Enabled: false, Tags: []string{"staging"}}); err != nil {
		t.Fatalf("Create bravo: %v", err)
	}

	all, err := r.List(ctx, models.ProjectFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List: got %d want 2", len(all))
	}
	if all[0].Name != "alpha" {
		t.Errorf("expected alpha first, got %q", all[0].Name)
	}

	n, err := r.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Errorf("Count: got %d want 2", n)
	}

	enabledOnly, err := r.List(ctx, models.ProjectFilter{EnabledOnly: true})
	if err != nil {
		t.Fatalf("List enabled: %v", err)
	}
	if len(enabledOnly) != 1 || enabledOnly[0].Name != "alpha" {
		t.Errorf("EnabledOnly filter wrong: %+v", enabledOnly)
	}

	prod, err := r.List(ctx, models.ProjectFilter{Tag: "prod"})
	if err != nil {
		t.Fatalf("List tag: %v", err)
	}
	if len(prod) != 1 || prod[0].Name != "alpha" {
		t.Errorf("tag filter wrong: %+v", prod)
	}

	search, err := r.List(ctx, models.ProjectFilter{Search: "bravo"})
	if err != nil {
		t.Fatalf("List search: %v", err)
	}
	if len(search) != 1 || search[0].Name != "bravo" {
		t.Errorf("search wrong: %+v", search)
	}
}

func TestProjectRepoUpdateDelete(t *testing.T) {
	r := newProjectRepo(t)
	ctx := context.Background()

	a, err := r.Create(ctx, models.Project{Name: "alpha"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	a.Description = "updated"
	a.Tags = []string{"prod"}
	updated, err := r.Update(ctx, a)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Description != "updated" {
		t.Errorf("Description not persisted: %q", updated.Description)
	}
	if len(updated.Tags) != 1 || updated.Tags[0] != "prod" {
		t.Errorf("Tags not persisted: %v", updated.Tags)
	}

	if err := r.Delete(ctx, a.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.Get(ctx, a.ID); !errors.Is(err, models.ErrNotFound) {
		t.Errorf("Get after delete: got %v want ErrNotFound", err)
	}
	if err := r.Delete(ctx, a.ID); !errors.Is(err, models.ErrNotFound) {
		t.Errorf("Delete missing: got %v want ErrNotFound", err)
	}
	if _, err := r.Update(ctx, a); !errors.Is(err, models.ErrNotFound) {
		t.Errorf("Update missing: got %v want ErrNotFound", err)
	}
}

func TestProjectRepoDuplicateName(t *testing.T) {
	r := newProjectRepo(t)
	ctx := context.Background()

	if _, err := r.Create(ctx, models.Project{Name: "alpha"}); err != nil {
		t.Fatalf("Create #1: %v", err)
	}
	_, err := r.Create(ctx, models.Project{Name: "alpha"})
	if !errors.Is(err, models.ErrDuplicateName) {
		t.Errorf("expected ErrDuplicateName, got %v", err)
	}
}

func TestProjectValidate(t *testing.T) {
	cases := []struct {
		name    string
		p       models.Project
		wantErr bool
	}{
		{"ok minimal", models.Project{Name: "Hello"}, false},
		{"empty name", models.Project{}, true},
		{"bad name slash", models.Project{Name: "bad/name"}, true},
		{"bad domain", models.Project{Name: "ok", Domain: "::not_a_host"}, true},
		{"port too high", models.Project{Name: "ok", Port: 70000}, true},
		{"bad container", models.Project{Name: "ok", ContainerName: "-bad"}, true},
		{"bad pm2", models.Project{Name: "ok", PM2Name: "-bad"}, true},
		{"bad tunnel", models.Project{Name: "ok", TunnelService: "nginx"}, true},
		{"bad health url", models.Project{Name: "ok", HealthURL: "ftp://x"}, true},
		{"too many tags", models.Project{Name: "ok", Tags: makeTags(21)}, true},
		{"bad env", models.Project{Name: "ok", Environment: "qa"}, true},
		{"deploy enabled no cmd", models.Project{Name: "ok", DeployEnabled: true}, true},
		{"deploy cmd inject ;", models.Project{Name: "ok", DeployCommand: "echo a;ls"}, true},
		{"deploy cmd inject &&", models.Project{Name: "ok", DeployCommand: "echo a && echo b"}, true},
		{"deploy cmd inject |", models.Project{Name: "ok", DeployCommand: "echo a | wc -l"}, true},
		{"deploy cmd inject backtick", models.Project{Name: "ok", DeployCommand: "echo `whoami`"}, true},
		{"deploy cmd inject $(", models.Project{Name: "ok", DeployCommand: "echo $(whoami)"}, true},
		{"deploy cmd trailing & ok", models.Project{Name: "ok", DeployCommand: "make build &"}, false},
		{"deploy timeout too small", models.Project{Name: "ok", DeployTimeoutSeconds: 5}, true},
		{"deploy timeout too big", models.Project{Name: "ok", DeployTimeoutSeconds: 99999}, true},
		{"deploy cwd relative", models.Project{Name: "ok", DeployWorkingDir: "relative/path"}, true},
		{"deploy cwd traversal", models.Project{Name: "ok", DeployWorkingDir: "/srv/app/../etc"}, true},
		{"deploy cwd ok", models.Project{Name: "ok", DeployWorkingDir: "/srv/app-1"}, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.p.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("expected err, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected err: %v", err)
			}
		})
	}
}

func TestProjectRepoEnvironmentFilter(t *testing.T) {
	r := newProjectRepo(t)
	ctx := context.Background()

	if _, err := r.Create(ctx, models.Project{Name: "alpha", Environment: "production"}); err != nil {
		t.Fatalf("Create alpha: %v", err)
	}
	if _, err := r.Create(ctx, models.Project{Name: "bravo", Environment: "staging"}); err != nil {
		t.Fatalf("Create bravo: %v", err)
	}

	got, err := r.List(ctx, models.ProjectFilter{Environment: "staging"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Name != "bravo" {
		t.Fatalf("Environment filter wrong: %+v", got)
	}
}

func TestProjectRepoDeployRoundTrip(t *testing.T) {
	r := newProjectRepo(t)
	ctx := context.Background()

	p := models.Project{
		Name:                 "deploy-me",
		Environment:          "staging",
		DeployEnabled:        true,
		DeployCommand:        "echo hello",
		DeployTimeoutSeconds: 60,
		DeployWorkingDir:     "/srv/deploy-me",
		WebhookSecret:        "abcdef",
	}
	created, err := r.Create(ctx, p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Environment != "staging" {
		t.Errorf("Environment lost: %q", created.Environment)
	}
	if !created.DeployEnabled {
		t.Errorf("DeployEnabled lost")
	}
	if created.DeployCommand != "echo hello" {
		t.Errorf("DeployCommand lost: %q", created.DeployCommand)
	}
	if created.DeployTimeoutSeconds != 60 {
		t.Errorf("DeployTimeoutSeconds lost: %d", created.DeployTimeoutSeconds)
	}
	if created.DeployWorkingDir != "/srv/deploy-me" {
		t.Errorf("DeployWorkingDir lost: %q", created.DeployWorkingDir)
	}
	if created.WebhookSecret != "abcdef" {
		t.Errorf("WebhookSecret lost: %q", created.WebhookSecret)
	}
}

func makeTags(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "t" + string(rune('a'+i%26))
	}
	return out
}
