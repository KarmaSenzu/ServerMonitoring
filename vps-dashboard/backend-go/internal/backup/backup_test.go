package backup_test

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/backup"
	"vps-dashboard-api/internal/db"
	"vps-dashboard-api/internal/models"
)

func newService(t *testing.T) *backup.Service {
	t.Helper()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "wave4_backup.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	logger := zerolog.New(io.Discard)
	if err := db.Migrate(context.Background(), conn, logger); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	// Seed a row so the backup has non-trivial content.
	projects := models.NewProjectRepo(conn)
	if _, err := projects.Create(context.Background(), models.Project{Name: "alpha"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	return &backup.Service{
		Logger:    logger,
		DB:        conn,
		DBPath:    dbPath,
		Dir:       filepath.Join(tmp, "backups"),
		Keep:      3,
		HourLocal: 3,
		Repo:      backup.NewRepo(conn),
		Events:    models.NewEventRepo(conn),
	}
}

func TestRunOnceCreatesFileAndRow(t *testing.T) {
	svc := newService(t)

	b, err := svc.RunOnce(context.Background(), "manual:test")
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !b.OK {
		t.Fatalf("OK=false: %s", b.Error)
	}
	if b.SizeBytes <= 0 {
		t.Errorf("SizeBytes=%d", b.SizeBytes)
	}
	if !strings.HasPrefix(b.Path, svc.Dir) {
		t.Errorf("Path %q not inside Dir %q", b.Path, svc.Dir)
	}

	rows, err := svc.Repo.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != b.ID {
		t.Fatalf("List should contain the new row, got %+v", rows)
	}
}

func TestRunOncePrunesToKeep(t *testing.T) {
	svc := newService(t)

	// Run 9 backups; only the last 3 should remain on disk + DB.
	created := make([]backup.Backup, 0, 9)
	for i := 0; i < 9; i++ {
		b, err := svc.RunOnce(context.Background(), "manual:test")
		if err != nil {
			t.Fatalf("RunOnce #%d: %v", i, err)
		}
		created = append(created, b)
		// Sleep just enough to keep filenames unique even on coarse clocks.
		time.Sleep(20 * time.Millisecond)
	}

	rows, err := svc.Repo.List(context.Background(), 100)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows after pruning, got %d", len(rows))
	}
	// The newest 3 should match the last 3 created (newest-first).
	want := []string{created[8].ID, created[7].ID, created[6].ID}
	for i, id := range want {
		if rows[i].ID != id {
			t.Errorf("row[%d]: got %s want %s", i, rows[i].ID, id)
		}
	}
}

func TestDeleteRefusesLastBackup(t *testing.T) {
	svc := newService(t)
	b, err := svc.RunOnce(context.Background(), "manual:test")
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if err := svc.Delete(context.Background(), b.ID); err != backup.ErrLastBackup {
		t.Fatalf("Delete: got %v want ErrLastBackup", err)
	}
}

func TestDeleteRemovesFile(t *testing.T) {
	svc := newService(t)
	b1, err := svc.RunOnce(context.Background(), "manual:test")
	if err != nil {
		t.Fatalf("RunOnce 1: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	b2, err := svc.RunOnce(context.Background(), "manual:test")
	if err != nil {
		t.Fatalf("RunOnce 2: %v", err)
	}
	_ = b2

	if err := svc.Delete(context.Background(), b1.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	rows, err := svc.Repo.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, r := range rows {
		if r.ID == b1.ID {
			t.Fatalf("row %s still present after delete", b1.ID)
		}
	}
}

func TestPathInsideRejectsTraversal(t *testing.T) {
	if !backup.PathInside("/srv/backups", "/srv/backups/x.db") {
		t.Errorf("expected inside")
	}
	if backup.PathInside("/srv/backups", "/etc/passwd") {
		t.Errorf("expected outside")
	}
	if backup.PathInside("/srv/backups", "/srv/backups/../../etc/passwd") {
		t.Errorf("expected outside via traversal")
	}
}
