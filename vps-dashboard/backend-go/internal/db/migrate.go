package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies all embedded migrations in lexicographic order.
// Each migration runs inside its own transaction. Already-applied versions
// are skipped. Versions are derived from the leading numeric prefix of the
// filename (e.g. "001_init.sql" -> 1).
func Migrate(ctx context.Context, conn *sql.DB, logger zerolog.Logger) error {
	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
	`); err != nil {
		return fmt.Errorf("migrate: ensure schema_migrations: %w", err)
	}

	applied, err := loadAppliedVersions(ctx, conn)
	if err != nil {
		return err
	}

	files, err := listMigrationFiles()
	if err != nil {
		return err
	}

	for _, f := range files {
		if _, ok := applied[f.version]; ok {
			logger.Debug().Int("version", f.version).Str("file", f.name).Msg("migration already applied, skipping")
			continue
		}

		body, err := migrationsFS.ReadFile("migrations/" + f.name)
		if err != nil {
			return fmt.Errorf("migrate: read %s: %w", f.name, err)
		}

		if err := applyMigration(ctx, conn, f.version, string(body)); err != nil {
			return fmt.Errorf("migrate: apply %s: %w", f.name, err)
		}

		logger.Info().Int("version", f.version).Str("file", f.name).Msg("migration applied")
	}

	return nil
}

type migrationFile struct {
	version int
	name    string
}

func listMigrationFiles() ([]migrationFile, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("migrate: list embed: %w", err)
	}

	out := make([]migrationFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		v, err := parseVersion(name)
		if err != nil {
			return nil, err
		}
		out = append(out, migrationFile{version: v, name: name})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

func parseVersion(name string) (int, error) {
	// Expect a leading run of digits, e.g. "001_init.sql" or "12_foo.sql".
	end := 0
	for end < len(name) && name[end] >= '0' && name[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, fmt.Errorf("migrate: file %q has no numeric prefix", name)
	}
	v, err := strconv.Atoi(name[:end])
	if err != nil {
		return 0, fmt.Errorf("migrate: parse version of %q: %w", name, err)
	}
	return v, nil
}

func loadAppliedVersions(ctx context.Context, conn *sql.DB) (map[int]struct{}, error) {
	rows, err := conn.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("migrate: query applied: %w", err)
	}
	defer rows.Close()

	out := make(map[int]struct{})
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("migrate: scan applied: %w", err)
		}
		out[v] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrate: iterate applied: %w", err)
	}
	return out, nil
}

func applyMigration(ctx context.Context, conn *sql.DB, version int, body string) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, body); err != nil {
		return fmt.Errorf("exec body: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations(version, applied_at) VALUES (?, datetime('now'))`,
		version,
	); err != nil {
		return fmt.Errorf("record version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
