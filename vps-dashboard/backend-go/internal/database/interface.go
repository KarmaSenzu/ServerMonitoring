package database

import (
	"context"
	"database/sql"
	"time"
)

// Database defines the interface for database operations that abstracts
// away the specific database implementation (SQLite, PostgreSQL, etc.).
// All database access should go through this interface to maintain
// compatibility across different backends.
type Database interface {
	// Connection lifecycle
	Open(ctx context.Context) error
	Close() error
	Ping(ctx context.Context) error
	
	// Connection information
	Type() DatabaseType
	ConnectionString() string // Sanitized (no password)
	
	// Transaction support
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	
	// Query execution
	Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row
	
	// Prepared statements
	Prepare(ctx context.Context, query string) (*sql.Stmt, error)
	
	// Schema management
	RunMigrations(ctx context.Context) error
	GetSchemaVersion(ctx context.Context) (int, error)
	
	// Database info
	Stats() sql.DBStats
	Size(ctx context.Context) (int64, error) // Size in bytes
	
	// Underlying connection (for compatibility with existing code)
	// This returns the raw *sql.DB for legacy code that needs it.
	// New code should use the interface methods instead.
	Underlying() *sql.DB
}

// DatabaseType represents the type of database backend.
type DatabaseType string

const (
	DatabaseTypeSQLite   DatabaseType = "sqlite"
	DatabaseTypePostgres DatabaseType = "postgres"
	DatabaseTypeSupabase DatabaseType = "supabase" // PostgreSQL with Supabase extensions
	DatabaseTypeMySQL    DatabaseType = "mysql"
)

// String returns the string representation of DatabaseType.
func (t DatabaseType) String() string {
	return string(t)
}

// IsValid checks if the database type is supported.
func (t DatabaseType) IsValid() bool {
	switch t {
	case DatabaseTypeSQLite, DatabaseTypePostgres, DatabaseTypeSupabase, DatabaseTypeMySQL:
		return true
	default:
		return false
	}
}

// Migrator defines the interface for database migration operations.
// This is used to transfer data from one database to another.
type Migrator interface {
	// Migrate transfers all data from source to target database.
	// Returns progress updates via the callback function.
	Migrate(ctx context.Context, progress ProgressCallback) error
	
	// Validate checks if the migration is possible without actually
	// performing it. Returns any issues that would prevent migration.
	Validate(ctx context.Context) error
	
	// EstimateTime returns an estimate of how long the migration will take.
	EstimateTime(ctx context.Context) (time.Duration, error)
	
	// GetStats returns information about the data to be migrated.
	GetStats(ctx context.Context) (*MigrationStats, error)
}

// MigrationStats contains information about the data to be migrated.
type MigrationStats struct {
	Tables        []TableStats
	TotalRows     int64
	EstimatedSize int64 // Bytes
}

// TableStats contains information about a single table.
type TableStats struct {
	Name          string
	RowCount      int64
	EstimatedSize int64
}

// ProgressCallback is called during migration to report progress.
// current is the number of rows migrated so far, total is the total
// number of rows to migrate, table is the current table being migrated.
type ProgressCallback func(current, total int64, table string)

// BackupManager handles database backups and rollbacks.
type BackupManager interface {
	// Backup creates a backup of the current database.
	// For SQLite, this copies the file. For PostgreSQL, this uses pg_dump.
	Backup(ctx context.Context) (string, error) // Returns backup path
	
	// Restore restores from a backup.
	Restore(ctx context.Context, backupPath string) error
	
	// ListBackups returns all available backups.
	ListBackups(ctx context.Context) ([]BackupInfo, error)
	
	// DeleteBackup removes a backup.
	DeleteBackup(ctx context.Context, backupPath string) error
}

// BackupInfo contains information about a backup.
type BackupInfo struct {
	Path      string
	CreatedAt time.Time
	Size      int64
	Type      DatabaseType
}

// ConnectionTester tests database connections before actually connecting.
type ConnectionTester interface {
	// TestConnection attempts to connect to the database and run a simple
	// query to verify the connection is working. Returns an error if the
	// connection fails or if the database is not accessible.
	TestConnection(ctx context.Context, config Config) error
}

// QueryAdapter adapts queries for different database dialects.
// Different databases have slightly different SQL syntax, so this
// interface allows queries to be adapted as needed.
type QueryAdapter interface {
	// AdaptQuery adapts a query from generic SQL to database-specific SQL.
	AdaptQuery(query string) string
	
	// Placeholder returns the placeholder syntax for the database.
	// SQLite and PostgreSQL use $1, $2, etc. MySQL uses ?.
	Placeholder(index int) string
	
	// QuoteIdentifier quotes a table or column name for the database.
	// PostgreSQL uses "name", MySQL uses `name`.
	QuoteIdentifier(name string) string
}

// HealthChecker checks database health and connectivity.
type HealthChecker interface {
	// Check performs a health check on the database.
	Check(ctx context.Context) error
	
	// GetMetrics returns database metrics for monitoring.
	GetMetrics(ctx context.Context) (*DatabaseMetrics, error)
}

// DatabaseMetrics contains metrics about the database.
type DatabaseMetrics struct {
	// Connection pool metrics
	OpenConnections int
	InUse           int
	Idle            int
	
	// Query metrics (if supported)
	SlowQueries     int64
	AverageQueryMs  float64
	
	// Storage metrics
	Size            int64
	TableCount      int
	
	// Replication metrics (if applicable)
	ReplicationLag  *time.Duration
}

// Error types for database operations.
var (
	ErrDatabaseNotConfigured = &DatabaseError{Message: "database not configured"}
	ErrConnectionFailed      = &DatabaseError{Message: "connection failed"}
	ErrMigrationInProgress   = &DatabaseError{Message: "migration already in progress"}
	ErrInvalidConfig         = &DatabaseError{Message: "invalid configuration"}
	ErrUnsupportedDatabase   = &DatabaseError{Message: "unsupported database type"}
)

// DatabaseError represents a database-specific error.
type DatabaseError struct {
	Message string
	Cause   error
}

func (e *DatabaseError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *DatabaseError) Unwrap() error {
	return e.Cause
}
