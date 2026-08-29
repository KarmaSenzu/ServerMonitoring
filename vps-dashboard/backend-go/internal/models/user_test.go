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

func newUserRepo(t *testing.T) *models.UserRepo {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "users_test.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := db.Migrate(context.Background(), conn, zerolog.New(io.Discard)); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	return models.NewUserRepo(conn)
}

func TestUserRepoCreateAndList(t *testing.T) {
	r := newUserRepo(t)
	ctx := context.Background()

	if _, err := r.Create(ctx, "alpha", "hash-a", "admin"); err != nil {
		t.Fatalf("Create alpha: %v", err)
	}
	if _, err := r.Create(ctx, "bravo", "hash-b", "viewer"); err != nil {
		t.Fatalf("Create bravo: %v", err)
	}

	users, err := r.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("List: got %d want 2", len(users))
	}
	if users[0].Username != "alpha" || users[1].Username != "bravo" {
		t.Errorf("List order wrong: %+v", users)
	}
}

func TestUserRepoErrUsernameTaken(t *testing.T) {
	r := newUserRepo(t)
	ctx := context.Background()

	if _, err := r.Create(ctx, "alpha", "h1", "admin"); err != nil {
		t.Fatalf("Create #1: %v", err)
	}
	_, err := r.Create(ctx, "alpha", "h2", "viewer")
	if !errors.Is(err, models.ErrUsernameTaken) {
		t.Errorf("Create dup: got %v want ErrUsernameTaken", err)
	}
}

func TestUserRepoUpdatePassword(t *testing.T) {
	r := newUserRepo(t)
	ctx := context.Background()

	u, err := r.Create(ctx, "alpha", "h1", "admin")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := r.UpdatePassword(ctx, u.ID, "h2"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	if err := r.UpdatePassword(ctx, "missing", "h3"); !errors.Is(err, models.ErrUserNotFound) {
		t.Errorf("UpdatePassword missing: got %v want ErrUserNotFound", err)
	}

	_, hashPtr, err := r.GetByUsername(ctx, "alpha")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if hashPtr == nil || *hashPtr != "h2" {
		t.Errorf("hash not updated: %v", hashPtr)
	}
}

func TestUserRepoUpdateRole(t *testing.T) {
	r := newUserRepo(t)
	ctx := context.Background()

	u, err := r.Create(ctx, "alpha", "h1", "viewer")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := r.UpdateRole(ctx, u.ID, "admin"); err != nil {
		t.Fatalf("UpdateRole admin: %v", err)
	}
	if err := r.UpdateRole(ctx, u.ID, "hacker"); err == nil {
		t.Errorf("expected error for invalid role, got nil")
	}
	if err := r.UpdateRole(ctx, "missing", "viewer"); !errors.Is(err, models.ErrUserNotFound) {
		t.Errorf("UpdateRole missing: got %v want ErrUserNotFound", err)
	}

	got, err := r.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Role != "admin" {
		t.Errorf("role not updated: %q", got.Role)
	}
}

func TestUserRepoDelete(t *testing.T) {
	r := newUserRepo(t)
	ctx := context.Background()

	u, err := r.Create(ctx, "alpha", "h1", "viewer")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := r.Delete(ctx, u.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := r.Delete(ctx, u.ID); !errors.Is(err, models.ErrUserNotFound) {
		t.Errorf("Delete missing: got %v want ErrUserNotFound", err)
	}
	if _, err := r.GetByID(ctx, u.ID); !errors.Is(err, models.ErrUserNotFound) {
		t.Errorf("GetByID after delete: got %v want ErrUserNotFound", err)
	}
}

func TestUserRepoCountByRole(t *testing.T) {
	r := newUserRepo(t)
	ctx := context.Background()

	if _, err := r.Create(ctx, "a1", "h", "admin"); err != nil {
		t.Fatalf("Create a1: %v", err)
	}
	if _, err := r.Create(ctx, "a2", "h", "admin"); err != nil {
		t.Fatalf("Create a2: %v", err)
	}
	if _, err := r.Create(ctx, "v1", "h", "viewer"); err != nil {
		t.Fatalf("Create v1: %v", err)
	}

	if n, err := r.CountByRole(ctx, "admin"); err != nil || n != 2 {
		t.Errorf("CountByRole admin: %d %v want 2 nil", n, err)
	}
	if n, err := r.CountByRole(ctx, "viewer"); err != nil || n != 1 {
		t.Errorf("CountByRole viewer: %d %v want 1 nil", n, err)
	}
}
