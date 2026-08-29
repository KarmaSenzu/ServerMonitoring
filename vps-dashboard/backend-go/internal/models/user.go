package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ErrUserNotFound is returned when a lookup yields no rows.
var ErrUserNotFound = errors.New("user not found")

// ErrUsernameTaken is returned when a username UNIQUE constraint
// is violated on insert.
var ErrUsernameTaken = errors.New("user: username taken")

// User is the safe, externally exposed user shape (no password fields).
type User struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// UserRepo provides persistence operations for users.
type UserRepo struct {
	db *sql.DB
}

// NewUserRepo constructs a UserRepo bound to the given *sql.DB.
func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

// Create inserts a new user with a generated UUID and returns the persisted row.
func (r *UserRepo) Create(ctx context.Context, username, hash, role string) (*User, error) {
	if role != "admin" && role != "viewer" {
		return nil, fmt.Errorf("user: invalid role %q", role)
	}
	id := uuid.NewString()
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role)
		VALUES (?, ?, ?, ?)
	`, id, username, hash, role); err != nil {
		if isUserUniqueViolation(err) {
			return nil, ErrUsernameTaken
		}
		return nil, fmt.Errorf("user: insert: %w", err)
	}
	return r.GetByID(ctx, id)
}

// GetByUsername returns the user and its stored password hash, or ErrUserNotFound.
func (r *UserRepo) GetByUsername(ctx context.Context, username string) (*User, *string, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, role, created_at, updated_at
		FROM users
		WHERE username = ?
	`, username)

	var u User
	var hash string
	if err := row.Scan(&u.ID, &u.Username, &hash, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrUserNotFound
		}
		return nil, nil, fmt.Errorf("user: scan by username: %w", err)
	}
	return &u, &hash, nil
}

// GetByID returns the user by id, or ErrUserNotFound.
func (r *UserRepo) GetByID(ctx context.Context, id string) (*User, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, username, role, created_at, updated_at
		FROM users
		WHERE id = ?
	`, id)

	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("user: scan by id: %w", err)
	}
	return &u, nil
}

// Count returns the total number of users in the table.
func (r *UserRepo) Count(ctx context.Context) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("user: count: %w", err)
	}
	return n, nil
}

// CountByRole returns the number of users whose role matches role.
func (r *UserRepo) CountByRole(ctx context.Context, role string) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE role = ?`, role).Scan(&n); err != nil {
		return 0, fmt.Errorf("user: count by role: %w", err)
	}
	return n, nil
}

// List returns all users ordered by username ASC. The password hash is
// never returned.
func (r *UserRepo) List(ctx context.Context) ([]User, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, username, role, created_at, updated_at
		FROM users
		ORDER BY username ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("user: list: %w", err)
	}
	defer rows.Close()

	out := make([]User, 0, 8)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("user: scan: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("user: list iter: %w", err)
	}
	return out, nil
}

// UpdatePassword sets a new bcrypt hash for the user. Returns
// ErrUserNotFound if no row matches.
func (r *UserRepo) UpdatePassword(ctx context.Context, id, hash string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE users SET password_hash = ?, updated_at = datetime('now')
		WHERE id = ?
	`, hash, id)
	if err != nil {
		return fmt.Errorf("user: update password: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("user: rows affected: %w", err)
	}
	if n == 0 {
		return ErrUserNotFound
	}
	return nil
}

// UpdateRole sets the user's role. Role must be in {admin, viewer}.
// Returns ErrUserNotFound if no row matches.
func (r *UserRepo) UpdateRole(ctx context.Context, id, role string) error {
	if role != "admin" && role != "viewer" {
		return fmt.Errorf("user: invalid role %q", role)
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE users SET role = ?, updated_at = datetime('now')
		WHERE id = ?
	`, role, id)
	if err != nil {
		return fmt.Errorf("user: update role: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("user: rows affected: %w", err)
	}
	if n == 0 {
		return ErrUserNotFound
	}
	return nil
}

// Delete removes the user with the given id. Returns ErrUserNotFound when
// no row matches.
func (r *UserRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("user: delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("user: rows affected: %w", err)
	}
	if n == 0 {
		return ErrUserNotFound
	}
	return nil
}

func isUserUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if !strings.Contains(msg, "UNIQUE constraint failed") &&
		!strings.Contains(msg, "constraint failed: UNIQUE") {
		return false
	}
	return strings.Contains(msg, "users.username") || strings.Contains(msg, "username")
}
