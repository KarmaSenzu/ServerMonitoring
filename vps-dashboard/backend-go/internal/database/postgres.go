package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/db"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgresDB implements the Database interface for PostgreSQL and Supabase.
// Supabase is PostgreSQL under the hood, so this implementation handles both.
type PostgresDB struct {
	config *Config
	conn   *sql.DB
}

// NewPostgresDB creates a new PostgreSQL database instance.
func NewPostgresDB(config *Config) (*PostgresDB, error) {
	if config.Type != DatabaseTypePostgres && config.Type != DatabaseTypeSupabase {
		return nil, fmt.Errorf("expected postgres or supabase config, got %s", config.Type)
	}
	
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	
	return &PostgresDB{
		config: config,
	}, nil
}

// Open opens the PostgreSQL database connection.
func (db *PostgresDB) Open(ctx context.Context) error {
	// Get DSN
	dsn, err := db.config.DSN()
	if err != nil {
		return fmt.Errorf("build DSN: %w", err)
	}
	
	// Open connection
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	
	// Get config for connection pool settings
	var pgConfig *PostgresConfig
	if db.config.Type == DatabaseTypePostgres {
		pgConfig = db.config.Postgres
	} else {
		pgConfig = db.config.Supabase.Database
	}
	
	// Set connection pool settings
	if pgConfig.MaxOpenConns > 0 {
		conn.SetMaxOpenConns(pgConfig.MaxOpenConns)
	} else {
		conn.SetMaxOpenConns(25) // Default
	}
	
	if pgConfig.MaxIdleConns > 0 {
		conn.SetMaxIdleConns(pgConfig.MaxIdleConns)
	} else {
		conn.SetMaxIdleConns(5) // Default
	}
	
	if pgConfig.ConnMaxLifetime > 0 {
		conn.SetConnMaxLifetime(pgConfig.ConnMaxLifetime)
	}
	
	if pgConfig.ConnMaxIdleTime > 0 {
		conn.SetConnMaxIdleTime(pgConfig.ConnMaxIdleTime)
	}
	
	// Test connection
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return fmt.Errorf("ping database: %w", err)
	}
	
	db.conn = conn
	return nil
}

// Close closes the database connection.
func (db *PostgresDB) Close() error {
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}

// Ping checks if the database connection is alive.
func (db *PostgresDB) Ping(ctx context.Context) error {
	if db.conn == nil {
		return fmt.Errorf("database not connected")
	}
	return db.conn.PingContext(ctx)
}

// Type returns the database type.
func (db *PostgresDB) Type() DatabaseType {
	return db.config.Type
}

// ConnectionString returns a sanitized connection string (without password).
func (db *PostgresDB) ConnectionString() string {
	var host string
	var port int
	var database string
	
	if db.config.Type == DatabaseTypePostgres {
		host = db.config.Postgres.Host
		port = db.config.Postgres.Port
		database = db.config.Postgres.Database
	} else {
		host = db.config.Supabase.Database.Host
		port = db.config.Supabase.Database.Port
		database = db.config.Supabase.Database.Database
	}
	
	return fmt.Sprintf("postgres://%s:%d/%s", host, port, database)
}

// BeginTx starts a new transaction.
func (db *PostgresDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	if db.conn == nil {
		return nil, fmt.Errorf("database not connected")
	}
	return db.conn.BeginTx(ctx, opts)
}

// Exec executes a query that doesn't return rows.
func (db *PostgresDB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	if db.conn == nil {
		return nil, fmt.Errorf("database not connected")
	}
	return db.conn.ExecContext(ctx, query, args...)
}

// Query executes a query that returns rows.
func (db *PostgresDB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	if db.conn == nil {
		return nil, fmt.Errorf("database not connected")
	}
	return db.conn.QueryContext(ctx, query, args...)
}

// QueryRow executes a query that returns at most one row.
func (db *PostgresDB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return db.conn.QueryRowContext(ctx, query, args...)
}

// Prepare creates a prepared statement.
func (db *PostgresDB) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	if db.conn == nil {
		return nil, fmt.Errorf("database not connected")
	}
	return db.conn.PrepareContext(ctx, query)
}

// RunMigrations runs database migrations for PostgreSQL.
// Uses the PostgreSQL-specific migration files in migrations/postgres/.
func (d *PostgresDB) RunMigrations(ctx context.Context) error {
	if d.conn == nil {
		return fmt.Errorf("database not connected")
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	return db.Migrate(ctx, d.conn, logger, db.DialectPostgres)
}

// GetSchemaVersion returns the current schema version.
func (db *PostgresDB) GetSchemaVersion(ctx context.Context) (int, error) {
	if db.conn == nil {
		return 0, fmt.Errorf("database not connected")
	}
	
	var version int
	err := db.conn.QueryRowContext(ctx, 
		`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`,
	).Scan(&version)
	
	if err != nil {
		return 0, err
	}
	
	return version, nil
}

// Stats returns database statistics.
func (db *PostgresDB) Stats() sql.DBStats {
	if db.conn == nil {
		return sql.DBStats{}
	}
	return db.conn.Stats()
}

// Size returns the size of the database in bytes.
func (db *PostgresDB) Size(ctx context.Context) (int64, error) {
	if db.conn == nil {
		return 0, fmt.Errorf("database not connected")
	}
	
	var size int64
	err := db.conn.QueryRowContext(ctx, 
		`SELECT pg_database_size(current_database())`,
	).Scan(&size)
	
	if err != nil {
		return 0, fmt.Errorf("query database size: %w", err)
	}
	
	return size, nil
}

// Underlying returns the underlying *sql.DB connection.
func (db *PostgresDB) Underlying() *sql.DB {
	return db.conn
}

// PostgresQueryAdapter adapts queries for PostgreSQL.
type PostgresQueryAdapter struct{}

// NewPostgresQueryAdapter creates a new PostgreSQL query adapter.
func NewPostgresQueryAdapter() *PostgresQueryAdapter {
	return &PostgresQueryAdapter{}
}

// AdaptQuery adapts a query from SQLite syntax to PostgreSQL syntax.
func (a *PostgresQueryAdapter) AdaptQuery(query string) string {
	// Convert SQLite datetime functions to PostgreSQL
	query = strings.ReplaceAll(query, "datetime('now')", "NOW()")
	query = strings.ReplaceAll(query, "AUTOINCREMENT", "")
	
	// Convert TEXT PRIMARY KEY to UUID PRIMARY KEY (optional)
	// This is a simple example - real adapter would be more sophisticated
	
	return query
}

// Placeholder returns the placeholder syntax for PostgreSQL.
// PostgreSQL uses $1, $2, $3, etc. for placeholders.
func (a *PostgresQueryAdapter) Placeholder(index int) string {
	return fmt.Sprintf("$%d", index)
}

// QuoteIdentifier quotes an identifier for PostgreSQL.
func (a *PostgresQueryAdapter) QuoteIdentifier(name string) string {
	return fmt.Sprintf(`"%s"`, name)
}

// PostgresBackupManager handles PostgreSQL backups using pg_dump.
type PostgresBackupManager struct {
	db *PostgresDB
}

// NewPostgresBackupManager creates a new PostgreSQL backup manager.
func NewPostgresBackupManager(db *PostgresDB) *PostgresBackupManager {
	return &PostgresBackupManager{db: db}
}

// Backup creates a backup of the PostgreSQL database using pg_dump.
func (m *PostgresBackupManager) Backup(ctx context.Context) (string, error) {
	// For PostgreSQL, backups typically use pg_dump which needs to be
	// executed as an external command. This is a placeholder.
	return "", fmt.Errorf("postgres backup not yet implemented - use pg_dump manually")
}

// Restore restores from a backup using psql.
func (m *PostgresBackupManager) Restore(ctx context.Context, backupPath string) error {
	return fmt.Errorf("postgres restore not yet implemented - use psql manually")
}

// ListBackups returns all available backups.
func (m *PostgresBackupManager) ListBackups(ctx context.Context) ([]BackupInfo, error) {
	return nil, fmt.Errorf("postgres backup listing not yet implemented")
}

// DeleteBackup removes a backup.
func (m *PostgresBackupManager) DeleteBackup(ctx context.Context, backupPath string) error {
	return fmt.Errorf("postgres backup deletion not yet implemented")
}
