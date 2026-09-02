package database

import (
	"context"
	"fmt"
	"os"
	"sync"
)

// Manager manages database connections and provides a factory
// for creating the appropriate database implementation.
type Manager struct {
	config  *Config
	db      Database
	mu      sync.RWMutex
	adapter QueryAdapter
	backup  BackupManager
}

// NewManager creates a new database manager from configuration.
func NewManager(config *Config) (*Manager, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	
	return &Manager{
		config: config,
	}, nil
}

// NewManagerFromFile creates a new database manager from a config file.
func NewManagerFromFile(path string) (*Manager, error) {
	config, err := LoadConfig(path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	
	return NewManager(config)
}

// Connect opens the database connection based on configuration.
func (m *Manager) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.db != nil {
		return fmt.Errorf("already connected")
	}
	
	// Create database instance based on type
	db, err := m.createDatabase()
	if err != nil {
		return fmt.Errorf("create database: %w", err)
	}
	
	// Open connection
	if err := db.Open(ctx); err != nil {
		return fmt.Errorf("open connection: %w", err)
	}
	
	// Create query adapter
	m.adapter = m.createQueryAdapter()
	
	// Create backup manager
	m.backup = m.createBackupManager(db)
	
	m.db = db
	return nil
}

// createDatabase creates the appropriate database implementation.
func (m *Manager) createDatabase() (Database, error) {
	switch m.config.Type {
	case DatabaseTypeSQLite:
		return NewSQLiteDB(m.config)
		
	case DatabaseTypePostgres, DatabaseTypeSupabase:
		return NewPostgresDB(m.config)
		
	default:
		return nil, fmt.Errorf("unsupported database type: %s", m.config.Type)
	}
}

// createQueryAdapter creates the appropriate query adapter.
func (m *Manager) createQueryAdapter() QueryAdapter {
	switch m.config.Type {
	case DatabaseTypeSQLite:
		return NewSQLiteQueryAdapter()
		
	case DatabaseTypePostgres, DatabaseTypeSupabase:
		return NewPostgresQueryAdapter()
		
	default:
		return nil
	}
}

// createBackupManager creates the appropriate backup manager.
func (m *Manager) createBackupManager(db Database) BackupManager {
	switch m.config.Type {
	case DatabaseTypeSQLite:
		return NewSQLiteBackupManager(db.(*SQLiteDB))
		
	case DatabaseTypePostgres, DatabaseTypeSupabase:
		return NewPostgresBackupManager(db.(*PostgresDB))
		
	default:
		return nil
	}
}

// Close closes the database connection.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.db == nil {
		return nil
	}
	
	err := m.db.Close()
	m.db = nil
	m.adapter = nil
	m.backup = nil
	
	return err
}

// DB returns the database instance. Must call Connect first.
func (m *Manager) DB() (Database, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if m.db == nil {
		return nil, fmt.Errorf("not connected - call Connect first")
	}
	
	return m.db, nil
}

// Adapter returns the query adapter. Must call Connect first.
func (m *Manager) Adapter() (QueryAdapter, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if m.adapter == nil {
		return nil, fmt.Errorf("not connected - call Connect first")
	}
	
	return m.adapter, nil
}

// Backup returns the backup manager. Must call Connect first.
func (m *Manager) Backup() (BackupManager, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if m.backup == nil {
		return nil, fmt.Errorf("not connected - call Connect first")
	}
	
	return m.backup, nil
}

// Config returns a copy of the current configuration.
func (m *Manager) Config() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return *m.config
}

// Type returns the database type.
func (m *Manager) Type() DatabaseType {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.config.Type
}

// ConnectionString returns a sanitized connection string (no password)
// from the underlying database instance. Must call Connect first.
func (m *Manager) ConnectionString() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.db == nil {
		return ""
	}
	return m.db.ConnectionString()
}

// TestConnection tests a database connection without actually connecting.
func TestConnection(ctx context.Context, config *Config) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	
	// Create temporary database instance
	var db Database
	var err error
	
	switch config.Type {
	case DatabaseTypeSQLite:
		db, err = NewSQLiteDB(config)
	case DatabaseTypePostgres, DatabaseTypeSupabase:
		db, err = NewPostgresDB(config)
	default:
		return fmt.Errorf("unsupported database type: %s", config.Type)
	}
	
	if err != nil {
		return fmt.Errorf("create database: %w", err)
	}
	
	// Try to open and ping
	if err := db.Open(ctx); err != nil {
		return fmt.Errorf("open connection: %w", err)
	}
	defer db.Close()
	
	if err := db.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	
	return nil
}

// GetDefaultConfigPath returns the default database config file path.
func GetDefaultConfigPath() string {
	// Check for environment variable override
	if path := os.Getenv("VPSDASH_DB_CONFIG"); path != "" {
		return path
	}
	
	// Default to ./data/database.json
	return "./data/database.json"
}

// EnsureDefaultConfig ensures a default SQLite config exists if no config file present.
func EnsureDefaultConfig(configPath string) error {
	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		return nil // Config exists, nothing to do
	}
	
	// Create default SQLite config
	defaultConfig := DefaultSQLiteConfig("./data/vpsdash.db")
	
	// Save to file
	if err := SaveConfig(configPath, defaultConfig); err != nil {
		return fmt.Errorf("save default config: %w", err)
	}
	
	return nil
}

// LoadOrDefault loads config from file, or creates default if not exists.
func LoadOrDefault(configPath string) (*Config, error) {
	// Ensure default config exists
	if err := EnsureDefaultConfig(configPath); err != nil {
		return nil, fmt.Errorf("ensure default config: %w", err)
	}
	
	// Load config
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	
	return config, nil
}
