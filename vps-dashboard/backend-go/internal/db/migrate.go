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

//go:embed all:migrations
var migrationsFS embed.FS

// Dialect represents the SQL dialect for migrations.
type Dialect string

const (
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
)

// Migrate applies all embedded migrations in lexicographic order for the
// given dialect. Each migration runs inside its own transaction.
// Already-applied versions are skipped. Versions are derived from the
// leading numeric prefix of the filename (e.g. "001_init.sql" -> 1).
//
// For backwards compatibility, when dialect is empty DialectSQLite is used.
func Migrate(ctx context.Context, conn *sql.DB, logger zerolog.Logger, dialect ...Dialect) error {
	d := DialectSQLite
	if len(dialect) > 0 && dialect[0] != "" {
		d = dialect[0]
	}

	// Create schema_migrations table with dialect-appropriate timestamp.
	var createSQL string
	switch d {
	case DialectPostgres:
		createSQL = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT to_char(NOW(), 'YYYY-MM-DD"T"HH24:MI:SS')
		);`
	default: // sqlite
		createSQL = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		);`
	}
	if _, err := conn.ExecContext(ctx, createSQL); err != nil {
		return fmt.Errorf("migrate: ensure schema_migrations: %w", err)
	}

	applied, err := loadAppliedVersions(ctx, conn)
	if err != nil {
		return err
	}

	files, err := listMigrationFiles(d)
	if err != nil {
		return err
	}

	for _, f := range files {
		if _, ok := applied[f.version]; ok {
			logger.Debug().Int("version", f.version).Str("file", f.name).Msg("migration already applied, skipping")
			continue
		}

		body, err := migrationsFS.ReadFile(f.fullPath)
		if err != nil {
			return fmt.Errorf("migrate: read %s: %w", f.name, err)
		}

		if err := applyMigration(ctx, conn, f.version, string(body), d); err != nil {
			return fmt.Errorf("migrate: apply %s: %w", f.name, err)
		}

		logger.Info().Int("version", f.version).Str("file", f.name).Str("dialect", string(d)).Msg("migration applied")
	}

	return nil
}

type migrationFile struct {
	version  int
	name     string
	fullPath string // path within embed FS, e.g. "migrations/001_init.sql"
}

// listMigrationFiles lists migration files for the given dialect.
// SQLite dialect uses files in migrations/ root (excluding subdirs).
// PostgreSQL dialect uses files in migrations/postgres/.
func listMigrationFiles(dialect Dialect) ([]migrationFile, error) {
	var dir string
	switch dialect {
	case DialectPostgres:
		dir = "migrations/postgres"
	default: // sqlite
		dir = "migrations"
	}

	entries, err := fs.ReadDir(migrationsFS, dir)
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
		out = append(out, migrationFile{
			version:  v,
			name:     name,
			fullPath: dir + "/" + name,
		})
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

func applyMigration(ctx context.Context, conn *sql.DB, version int, body string, dialect Dialect) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, body); err != nil {
		return fmt.Errorf("exec body: %w", err)
	}

	// Record version with dialect-appropriate timestamp syntax.
	var recordSQL string
	var args []interface{}
	switch dialect {
	case DialectPostgres:
		recordSQL = `INSERT INTO schema_migrations(version, applied_at) VALUES ($1, to_char(NOW(), 'YYYY-MM-DD"T"HH24:MI:SS'))`
		args = []interface{}{version}
	default: // sqlite
		recordSQL = `INSERT INTO schema_migrations(version, applied_at) VALUES (?, datetime('now'))`
		args = []interface{}{version}
	}

	if _, err := tx.ExecContext(ctx, recordSQL, args...); err != nil {
		return fmt.Errorf("record version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
