package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/db"

	_ "modernc.org/sqlite" // SQLite driver
)

// SQLiteDB implements the Database interface for SQLite.
type SQLiteDB struct {
	config *Config
	conn   *sql.DB
}

// NewSQLiteDB creates a new SQLite database instance.
func NewSQLiteDB(config *Config) (*SQLiteDB, error) {
	if config.Type != DatabaseTypeSQLite {
		return nil, fmt.Errorf("expected sqlite config, got %s", config.Type)
	}
	
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	
	return &SQLiteDB{
		config: config,
	}, nil
}

// Open opens the SQLite database connection.
func (db *SQLiteDB) Open(ctx context.Context) error {
	// Ensure directory exists
	dir := filepath.Dir(db.config.SQLite.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	
	// Get DSN
	dsn, err := db.config.DSN()
	if err != nil {
		return fmt.Errorf("build DSN: %w", err)
	}
	
	// Open connection
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	
	// Set connection pool settings (SQLite is single-writer)
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	
	// Test connection
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return fmt.Errorf("ping database: %w", err)
	}
	
	db.conn = conn
	return nil
}

// Close closes the database connection.
func (db *SQLiteDB) Close() error {
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}

// Ping checks if the database connection is alive.
func (db *SQLiteDB) Ping(ctx context.Context) error {
	if db.conn == nil {
		return fmt.Errorf("database not connected")
	}
	return db.conn.PingContext(ctx)
}

// Type returns the database type.
func (db *SQLiteDB) Type() DatabaseType {
	return DatabaseTypeSQLite
}

// ConnectionString returns a sanitized connection string (without password).
func (db *SQLiteDB) ConnectionString() string {
	return fmt.Sprintf("sqlite://%s", db.config.SQLite.Path)
}

// BeginTx starts a new transaction.
func (db *SQLiteDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	if db.conn == nil {
		return nil, fmt.Errorf("database not connected")
	}
	return db.conn.BeginTx(ctx, opts)
}

// Exec executes a query that doesn't return rows.
func (db *SQLiteDB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	if db.conn == nil {
		return nil, fmt.Errorf("database not connected")
	}
	return db.conn.ExecContext(ctx, query, args...)
}

// Query executes a query that returns rows.
func (db *SQLiteDB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	if db.conn == nil {
		return nil, fmt.Errorf("database not connected")
	}
	return db.conn.QueryContext(ctx, query, args...)
}

// QueryRow executes a query that returns at most one row.
func (db *SQLiteDB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return db.conn.QueryRowContext(ctx, query, args...)
}

// Prepare creates a prepared statement.
func (db *SQLiteDB) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	if db.conn == nil {
		return nil, fmt.Errorf("database not connected")
	}
	return db.conn.PrepareContext(ctx, query)
}

// RunMigrations runs database migrations for SQLite.
// This delegates to the existing migration system in internal/db.
func (d *SQLiteDB) RunMigrations(ctx context.Context) error {
	if d.conn == nil {
		return fmt.Errorf("database not connected")
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	return db.Migrate(ctx, d.conn, logger, db.DialectSQLite)
}

// GetSchemaVersion returns the current schema version.
func (db *SQLiteDB) GetSchemaVersion(ctx context.Context) (int, error) {
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
func (db *SQLiteDB) Stats() sql.DBStats {
	if db.conn == nil {
		return sql.DBStats{}
	}
	return db.conn.Stats()
}

// Size returns the size of the database in bytes.
func (db *SQLiteDB) Size(ctx context.Context) (int64, error) {
	info, err := os.Stat(db.config.SQLite.Path)
	if err != nil {
		return 0, fmt.Errorf("stat database file: %w", err)
	}
	return info.Size(), nil
}

// Underlying returns the underlying *sql.DB connection.
func (db *SQLiteDB) Underlying() *sql.DB {
	return db.conn
}

// SQLiteQueryAdapter adapts queries for SQLite.
type SQLiteQueryAdapter struct{}

// NewSQLiteQueryAdapter creates a new SQLite query adapter.
func NewSQLiteQueryAdapter() *SQLiteQueryAdapter {
	return &SQLiteQueryAdapter{}
}

// AdaptQuery adapts a query for SQLite (usually no changes needed).
func (a *SQLiteQueryAdapter) AdaptQuery(query string) string {
	return query
}

// Placeholder returns the placeholder syntax for SQLite.
// SQLite uses ? for placeholders.
func (a *SQLiteQueryAdapter) Placeholder(index int) string {
	return "?"
}

// QuoteIdentifier quotes an identifier for SQLite.
func (a *SQLiteQueryAdapter) QuoteIdentifier(name string) string {
	return fmt.Sprintf(`"%s"`, name)
}

// SQLiteBackupManager handles SQLite backups.
type SQLiteBackupManager struct {
	db *SQLiteDB
}

// NewSQLiteBackupManager creates a new SQLite backup manager.
func NewSQLiteBackupManager(db *SQLiteDB) *SQLiteBackupManager {
	return &SQLiteBackupManager{db: db}
}

// Backup creates a backup of the SQLite database.
func (m *SQLiteBackupManager) Backup(ctx context.Context) (string, error) {
	sourcePath := m.db.config.SQLite.Path
	
	// Generate backup filename
	timestamp := ctx.Value("backup_timestamp")
	if timestamp == nil {
		return "", fmt.Errorf("backup timestamp not provided in context")
	}
	
	backupPath := fmt.Sprintf("%s.backup-%s", sourcePath, timestamp)
	
	// Copy file
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", fmt.Errorf("read source database: %w", err)
	}
	
	if err := os.WriteFile(backupPath, source, 0600); err != nil {
		return "", fmt.Errorf("write backup: %w", err)
	}
	
	return backupPath, nil
}

// Restore restores from a backup.
func (m *SQLiteBackupManager) Restore(ctx context.Context, backupPath string) error {
	targetPath := m.db.config.SQLite.Path
	
	// Close current connection
	if err := m.db.Close(); err != nil {
		return fmt.Errorf("close current database: %w", err)
	}
	
	// Copy backup to target
	backup, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("read backup: %w", err)
	}
	
	if err := os.WriteFile(targetPath, backup, 0600); err != nil {
		return fmt.Errorf("write restored database: %w", err)
	}
	
	// Reopen connection
	return m.db.Open(ctx)
}

// ListBackups returns all available backups.
func (m *SQLiteBackupManager) ListBackups(ctx context.Context) ([]BackupInfo, error) {
	sourcePath := m.db.config.SQLite.Path
	dir := filepath.Dir(sourcePath)
	base := filepath.Base(sourcePath)
	
	pattern := fmt.Sprintf("%s.backup-*", base)
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return nil, fmt.Errorf("glob backups: %w", err)
	}
	
	backups := make([]BackupInfo, 0, len(matches))
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		
		backups = append(backups, BackupInfo{
			Path:      path,
			CreatedAt: info.ModTime(),
			Size:      info.Size(),
			Type:      DatabaseTypeSQLite,
		})
	}
	
	return backups, nil
}

// DeleteBackup removes a backup.
func (m *SQLiteBackupManager) DeleteBackup(ctx context.Context, backupPath string) error {
	return os.Remove(backupPath)
}
