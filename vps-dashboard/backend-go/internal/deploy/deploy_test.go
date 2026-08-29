package deploy_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/db"
	"vps-dashboard-api/internal/deploy"
	"vps-dashboard-api/internal/models"
)

func newServiceWithProject(t *testing.T, p models.Project) (*deploy.Service, models.Project) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "deploy_test.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	logger := zerolog.New(io.Discard)
	if err := db.Migrate(context.Background(), conn, logger); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	projects := models.NewProjectRepo(conn)
	created, err := projects.Create(context.Background(), p)
	if err != nil {
		t.Fatalf("projects.Create: %v", err)
	}
	repo := deploy.NewDeploymentRepo(conn)
	events := models.NewEventRepo(conn)
	svc := deploy.NewService(logger, projects, repo, events)
	return svc, created
}

func waitTerminal(t *testing.T, svc *deploy.Service, id string, timeout time.Duration) deploy.Deployment {
	t.Helper()
	d, err := svc.WaitFor(context.Background(), id, timeout)
	if err != nil {
		t.Fatalf("WaitFor(%s): %v (status=%s)", id, err, d.Status)
	}
	return d
}

func TestServiceTriggerSuccess(t *testing.T) {
	svc, project := newServiceWithProject(t, models.Project{
		Name:                 "deploy-ok",
		Environment:          "staging",
		DeployEnabled:        true,
		DeployCommand:        "echo hello-from-deploy",
		DeployTimeoutSeconds: 30,
	})

	d, err := svc.Trigger(context.Background(), project, "manual:test", "abc")
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if d.Status != deploy.StatusPending && d.Status != deploy.StatusRunning {
		t.Fatalf("expected pending/running, got %q", d.Status)
	}

	final := waitTerminal(t, svc, d.ID, 10*time.Second)
	if final.Status != deploy.StatusSuccess {
		t.Fatalf("status: got %q want success (err=%q stderr=%q)", final.Status, final.Error, final.Stderr)
	}
	if final.ExitCode != 0 {
		t.Errorf("exit code: got %d want 0", final.ExitCode)
	}
	if !contains(final.Stdout, "hello-from-deploy") {
		t.Errorf("stdout missing greeting: %q", final.Stdout)
	}
}

func TestServiceTriggerNonZeroExit(t *testing.T) {
	svc, project := newServiceWithProject(t, models.Project{
		Name:                 "deploy-fail",
		Environment:          "staging",
		DeployEnabled:        true,
		DeployCommand:        "exit 5",
		DeployTimeoutSeconds: 30,
	})

	d, err := svc.Trigger(context.Background(), project, "manual:test", "")
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	final := waitTerminal(t, svc, d.ID, 10*time.Second)
	if final.Status != deploy.StatusFailed {
		t.Fatalf("status: got %q want failed", final.Status)
	}
	if final.ExitCode != 5 {
		t.Errorf("exit code: got %d want 5", final.ExitCode)
	}
}

func TestServiceTriggerTimeout(t *testing.T) {
	svc, project := newServiceWithProject(t, models.Project{
		Name:                 "deploy-timeout",
		Environment:          "staging",
		DeployEnabled:        true,
		DeployCommand:        "sleep 10",
		DeployTimeoutSeconds: 30, // ignored: validator min 30; we override via persisted row below
	})

	// Force a sub-second timeout by mutating the row directly.
	project.DeployTimeoutSeconds = 1
	d, err := svc.Trigger(context.Background(), project, "manual:test", "")
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	final := waitTerminal(t, svc, d.ID, 10*time.Second)
	if final.Status != deploy.StatusTimeout {
		t.Fatalf("status: got %q want timeout (err=%q)", final.Status, final.Error)
	}
}

func TestServiceTriggerAlreadyRunning(t *testing.T) {
	svc, project := newServiceWithProject(t, models.Project{
		Name:                 "deploy-busy",
		Environment:          "staging",
		DeployEnabled:        true,
		DeployCommand:        "sleep 2",
		DeployTimeoutSeconds: 30,
	})

	first, err := svc.Trigger(context.Background(), project, "manual:test", "")
	if err != nil {
		t.Fatalf("Trigger #1: %v", err)
	}

	// Give the goroutine a moment to take the project lock.
	time.Sleep(100 * time.Millisecond)

	if _, err := svc.Trigger(context.Background(), project, "manual:test", ""); !errors.Is(err, deploy.ErrAlreadyRunning) {
		t.Fatalf("Trigger #2: got %v want ErrAlreadyRunning", err)
	}

	// Drain the first deployment so subsequent tests don't leave goroutines.
	waitTerminal(t, svc, first.ID, 10*time.Second)
}

func TestServiceTriggerNotConfigured(t *testing.T) {
	svc, project := newServiceWithProject(t, models.Project{
		Name:                 "deploy-off",
		Environment:          "staging",
		DeployEnabled:        false,
		DeployCommand:        "echo hi",
		DeployTimeoutSeconds: 60,
	})

	if _, err := svc.Trigger(context.Background(), project, "manual:test", ""); !errors.Is(err, deploy.ErrNotConfigured) {
		t.Fatalf("Trigger: got %v want ErrNotConfigured", err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (sub == "" || (len(s) > 0 && stringIndex(s, sub) >= 0))
}

func stringIndex(s, sub string) int {
	// Tiny manual implementation to avoid pulling in strings just for tests.
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
