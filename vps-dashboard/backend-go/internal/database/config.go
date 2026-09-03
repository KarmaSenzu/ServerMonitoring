package database

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vps-dashboard-api/internal/crypto"
)

// Config holds the configuration for a database connection.
type Config struct {
	// Type is the database type (sqlite, postgres, supabase, mysql).
	Type DatabaseType `json:"type"`
	
	// SQLite-specific configuration
	SQLite *SQLiteConfig `json:"sqlite,omitempty"`
	
	// PostgreSQL-specific configuration
	Postgres *PostgresConfig `json:"postgres,omitempty"`
	
	// Supabase-specific configuration (extends PostgreSQL)
	Supabase *SupabaseConfig `json:"supabase,omitempty"`
	
	// MySQL-specific configuration
	MySQL *MySQLConfig `json:"mysql,omitempty"`
	
	// Migration metadata
	MigratedFrom *MigrationMetadata `json:"migrated_from,omitempty"`
}

// SQLiteConfig holds SQLite-specific configuration.
type SQLiteConfig struct {
	// Path to the SQLite database file.
	Path string `json:"path"`
	
	// Additional SQLite pragmas (e.g., "journal_mode=WAL").
	Pragmas map[string]string `json:"pragmas,omitempty"`
}

// PostgresConfig holds PostgreSQL-specific configuration.
type PostgresConfig struct {
	// Host is the database server hostname or IP.
	Host string `json:"host"`
	
	// Port is the database server port (default 5432).
	Port int `json:"port"`
	
	// Database name.
	Database string `json:"database"`
	
	// Username for authentication.
	Username string `json:"username"`
	
	// Password for authentication. Can be:
	// - Direct password (not recommended for production)
	// - Environment variable reference: "$ENV_VAR_NAME"
	// - File reference: "file:///path/to/password"
	Password string `json:"password"`
	
	// SSLMode controls SSL/TLS encryption:
	// - disable: No SSL
	// - require: Require SSL (default)
	// - verify-ca: Require SSL and verify CA
	// - verify-full: Require SSL and verify hostname
	SSLMode string `json:"ssl_mode"`
	
	// Connection pool settings
	MaxOpenConns    int           `json:"max_open_conns,omitempty"`
	MaxIdleConns    int           `json:"max_idle_conns,omitempty"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime,omitempty"`
	ConnMaxIdleTime time.Duration `json:"conn_max_idle_time,omitempty"`
}

// SupabaseConfig holds Supabase-specific configuration.
// Supabase is PostgreSQL with additional features, so it extends PostgresConfig.
type SupabaseConfig struct {
	// Supabase project reference (e.g., "abcdefgh")
	ProjectRef string `json:"project_ref"`
	
	// Supabase project URL (e.g., "https://abcdefgh.supabase.co")
	ProjectURL string `json:"project_url"`
	
	// Supabase API keys
	AnonKey        string `json:"anon_key,omitempty"`         // Public anonymous key
	ServiceRoleKey string `json:"service_role_key,omitempty"` // Private service key (admin access)
	
	// Database configuration (underlying PostgreSQL)
	Database *PostgresConfig `json:"database"`
}

// MySQLConfig holds MySQL/MariaDB-specific configuration.
type MySQLConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	Username string `json:"username"`
	Password string `json:"password"`
	
	// Charset (default utf8mb4)
	Charset string `json:"charset,omitempty"`
	
	// Connection pool settings
	MaxOpenConns    int           `json:"max_open_conns,omitempty"`
	MaxIdleConns    int           `json:"max_idle_conns,omitempty"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime,omitempty"`
}

// MigrationMetadata holds information about a database migration.
type MigrationMetadata struct {
	SourceType    DatabaseType `json:"source_type"`
	MigratedAt    time.Time    `json:"migrated_at"`
	MigratedBy    string       `json:"migrated_by"`     // User who initiated migration
	BackupPath    string       `json:"backup_path"`     // Path to backup of source database
	TotalRecords  int64        `json:"total_records"`   // Total records migrated
	DurationMs    int64        `json:"duration_ms"`     // Migration duration
	Success       bool         `json:"success"`
	ErrorMessage  string       `json:"error_message,omitempty"`
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if !c.Type.IsValid() {
		return fmt.Errorf("invalid database type: %s", c.Type)
	}
	
	switch c.Type {
	case DatabaseTypeSQLite:
		if c.SQLite == nil {
			return fmt.Errorf("sqlite configuration is required")
		}
		return c.SQLite.Validate()
		
	case DatabaseTypePostgres:
		if c.Postgres == nil {
			return fmt.Errorf("postgres configuration is required")
		}
		return c.Postgres.Validate()
		
	case DatabaseTypeSupabase:
		if c.Supabase == nil {
			return fmt.Errorf("supabase configuration is required")
		}
		return c.Supabase.Validate()
		
	case DatabaseTypeMySQL:
		if c.MySQL == nil {
			return fmt.Errorf("mysql configuration is required")
		}
		return c.MySQL.Validate()
	}
	
	return nil
}

// Validate checks if SQLite configuration is valid.
func (c *SQLiteConfig) Validate() error {
	if c.Path == "" {
		return fmt.Errorf("sqlite path is required")
	}
	return nil
}

// Validate checks if PostgreSQL configuration is valid.
func (c *PostgresConfig) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("postgres host is required")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("postgres port must be between 1 and 65535")
	}
	if c.Database == "" {
		return fmt.Errorf("postgres database name is required")
	}
	if c.Username == "" {
		return fmt.Errorf("postgres username is required")
	}
	if c.Password == "" {
		return fmt.Errorf("postgres password is required")
	}
	return nil
}

// Validate checks if Supabase configuration is valid.
// ProjectRef/ProjectURL are optional (only needed for future Supabase
// API integrations); the database connection itself only needs the
// underlying PostgreSQL config.
func (c *SupabaseConfig) Validate() error {
	if c.Database == nil {
		return fmt.Errorf("supabase database configuration is required")
	}
	return c.Database.Validate()
}

// Validate checks if MySQL configuration is valid.
func (c *MySQLConfig) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("mysql host is required")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("mysql port must be between 1 and 65535")
	}
	if c.Database == "" {
		return fmt.Errorf("mysql database name is required")
	}
	if c.Username == "" {
		return fmt.Errorf("mysql username is required")
	}
	return nil
}

// DSN returns the database connection string (Data Source Name).
func (c *Config) DSN() (string, error) {
	switch c.Type {
	case DatabaseTypeSQLite:
		return c.SQLite.DSN()
	case DatabaseTypePostgres:
		return c.Postgres.DSN()
	case DatabaseTypeSupabase:
		return c.Supabase.DSN()
	case DatabaseTypeMySQL:
		return c.MySQL.DSN()
	default:
		return "", fmt.Errorf("unsupported database type: %s", c.Type)
	}
}

// DSN returns the SQLite connection string.
func (c *SQLiteConfig) DSN() (string, error) {
	dsn := c.Path
	
	// Add pragmas as query parameters
	if len(c.Pragmas) > 0 {
		dsn += "?"
		first := true
		for key, value := range c.Pragmas {
			if !first {
				dsn += "&"
			}
			dsn += fmt.Sprintf("_pragma=%s=%s", key, value)
			first = false
		}
	}
	
	return dsn, nil
}

// DSN returns the PostgreSQL connection string.
func (c *PostgresConfig) DSN() (string, error) {
	sslMode := c.SSLMode
	if sslMode == "" {
		sslMode = "require"
	}
	
	password := c.Password
	// Resolve password in priority order:
	// 1. Encrypted value ("enc:...") — decrypt with JWT_SECRET-derived key
	// 2. Environment variable reference ("$ENV_VAR_NAME")
	// 3. Plaintext (backward compat)
	if strings.HasPrefix(password, "enc:") {
		key := crypto.GetEncryptionKey()
		if key == nil {
			return "", fmt.Errorf("JWT_SECRET not set — cannot decrypt database password")
		}
		decrypted, err := crypto.Decrypt(password, key)
		if err != nil {
			return "", fmt.Errorf("decrypt database password: %w", err)
		}
		password = decrypted
	} else if len(password) > 0 && password[0] == '$' {
		envVar := password[1:]
		password = os.Getenv(envVar)
		if password == "" {
			return "", fmt.Errorf("environment variable %s not set", envVar)
		}
	}
	
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.Username,
		password,
		c.Host,
		c.Port,
		c.Database,
		sslMode,
	), nil
}

// DSN returns the Supabase connection string (PostgreSQL under the hood).
func (c *SupabaseConfig) DSN() (string, error) {
	if c.Database == nil {
		return "", fmt.Errorf("supabase database configuration is required")
	}
	return c.Database.DSN()
}

// DSN returns the MySQL connection string.
func (c *MySQLConfig) DSN() (string, error) {
	charset := c.Charset
	if charset == "" {
		charset = "utf8mb4"
	}
	
	password := c.Password
	// Resolve password from environment variable if reference
	if len(password) > 0 && password[0] == '$' {
		envVar := password[1:]
		password = os.Getenv(envVar)
	}
	
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=true",
		c.Username,
		password,
		c.Host,
		c.Port,
		c.Database,
		charset,
	), nil
}

// LoadConfig loads database configuration from a file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}
	
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}
	
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	
	return &config, nil
}

// SaveConfig saves database configuration to a file.
func SaveConfig(path string, config *Config) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	
	// Marshal config to JSON
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	
	// Write to file with restricted permissions (contains passwords)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	
	return nil
}

// DefaultSQLiteConfig returns the default SQLite configuration.
func DefaultSQLiteConfig(path string) *Config {
	return &Config{
		Type: DatabaseTypeSQLite,
		SQLite: &SQLiteConfig{
			Path: path,
			Pragmas: map[string]string{
				"journal_mode": "WAL",
				"synchronous":  "NORMAL",
				"foreign_keys": "ON",
			},
		},
	}
}
