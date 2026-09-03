package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/crypto"
	"vps-dashboard-api/internal/database"
	"vps-dashboard-api/internal/models"
)

// DatabaseHandler exposes /database/* endpoints for managing the
// database backend (SQLite / PostgreSQL / Supabase) from the UI.
// Read endpoints are protected; write endpoints (test/configure) are
// admin-only.
type DatabaseHandler struct {
	App *app.App
}

// NewDatabaseHandler constructs a DatabaseHandler.
func NewDatabaseHandler(a *app.App) *DatabaseHandler {
	return &DatabaseHandler{App: a}
}

// RegisterReads mounts the read-only database routes (JWT-protected).
func (h *DatabaseHandler) RegisterReads(rg *gin.RouterGroup) {
	rg.GET("/database/status", h.status)
	rg.GET("/database/config", h.getConfig)
}

// RegisterWrites mounts the mutating database routes (admin-only).
func (h *DatabaseHandler) RegisterWrites(rg *gin.RouterGroup) {
	rg.POST("/database/test", h.testConnection)
	rg.POST("/database/configure", h.configure)
	rg.POST("/database/migrate", h.migrate)
}

// dbStatus is the safe API shape returned to the frontend. It never
// includes the password (raw or encrypted).
type dbStatus struct {
	Type          string `json:"type"`
	Connection    string `json:"connection"`    // sanitized, no password
	Configured    bool   `json:"configured"`
	RestartNeeded bool   `json:"restart_needed"`
}

// status returns the current database configuration status.
func (h *DatabaseHandler) status(c *gin.Context) {
	path := database.GetDefaultConfigPath()
	cfg, err := database.LoadConfig(path)
	if err != nil {
		// No config or invalid — report unconfigured
		c.JSON(http.StatusOK, gin.H{"data": dbStatus{
			Type:       "sqlite",
			Connection: "sqlite://./data/vpsdash.db (default)",
			Configured: false,
		}})
		return
	}

	// Build sanitized connection string (no password)
	var connStr string
	switch cfg.Type {
	case database.DatabaseTypeSQLite:
		connStr = "sqlite://" + cfg.SQLite.Path
	case database.DatabaseTypePostgres:
		connStr = "postgres://" + cfg.Postgres.Host + ":" + itoa(cfg.Postgres.Port) + "/" + cfg.Postgres.Database
	case database.DatabaseTypeSupabase:
		connStr = "supabase://" + cfg.Supabase.Database.Host + ":" + itoa(cfg.Supabase.Database.Port) + "/" + cfg.Supabase.Database.Database
	case database.DatabaseTypeMySQL:
		connStr = "mysql://" + cfg.MySQL.Host + ":" + itoa(cfg.MySQL.Port) + "/" + cfg.MySQL.Database
	}

	c.JSON(http.StatusOK, gin.H{"data": dbStatus{
		Type:       string(cfg.Type),
		Connection: connStr,
		Configured: true,
	}})
}

// getConfig returns the current config WITHOUT the password field.
func (h *DatabaseHandler) getConfig(c *gin.Context) {
	path := database.GetDefaultConfigPath()
	cfg, err := database.LoadConfig(path)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"data": nil})
		return
	}

	// Deep-copy and strip all passwords
	safe := sanitizeConfig(cfg)
	c.JSON(http.StatusOK, gin.H{"data": safe})
}

// testConnection tests a database config without saving it.
func (h *DatabaseHandler) testConnection(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	var req dbConfigDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	cfg, err := req.toConfig()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	start := time.Now()
	err = database.TestConnection(ctx, cfg)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"ok":      false,
			"error":   err.Error(),
			"latency": latency,
		}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"ok":      true,
		"latency": latency,
	}})
}

// configure saves a database config to disk. The password is encrypted
// at rest (AES-256-GCM keyed by JWT_SECRET). A restart is required to
// apply the new connection.
func (h *DatabaseHandler) configure(c *gin.Context) {
	var req dbConfigDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	cfg, err := req.toConfig()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	// Encrypt password before saving
	if err := encryptConfigPassword(cfg); err != nil {
		h.dbError(c, "db.configure.encrypt", err)
		return
	}

	path := database.GetDefaultConfigPath()
	if err := database.SaveConfig(path, cfg); err != nil {
		h.dbError(c, "db.configure.save", err)
		return
	}

	// Audit the change
	h.auditDB(c, "database_configure", string(cfg.Type))

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"type":             string(cfg.Type),
		"saved":            true,
		"restart_required": true,
		"message":          "Configuration saved. Restart the server to apply.",
	}})
}

// dbConfigDTO is the JSON shape for database config from the UI.
type dbConfigDTO struct {
	Type      string `json:"type"`      // sqlite | postgres | supabase
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Database  string `json:"database"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	SSLMode   string `json:"ssl_mode"`
	ProjectRef string `json:"project_ref"`
	ProjectURL string `json:"project_url"`
}

// toConfig converts the DTO into a database.Config.
func (d *dbConfigDTO) toConfig() (*database.Config, error) {
	switch d.Type {
	case "sqlite":
		return database.DefaultSQLiteConfig("./data/vpsdash.db"), nil

	case "postgres":
		port := d.Port
		if port == 0 {
			port = 5432
		}
		return &database.Config{
			Type: database.DatabaseTypePostgres,
			Postgres: &database.PostgresConfig{
				Host:     d.Host,
				Port:     port,
				Database: d.Database,
				Username: d.Username,
				Password: d.Password,
				SSLMode:  d.SSLMode,
			},
		}, nil

	case "supabase":
		port := d.Port
		if port == 0 {
			port = 5432
		}
		return &database.Config{
			Type: database.DatabaseTypeSupabase,
			Supabase: &database.SupabaseConfig{
				ProjectRef: d.ProjectRef,
				ProjectURL: d.ProjectURL,
				Database: &database.PostgresConfig{
					Host:     d.Host,
					Port:     port,
					Database: d.Database,
					Username: d.Username,
					Password: d.Password,
					SSLMode:  d.SSLMode,
				},
			},
		}, nil

	default:
		return nil, fmt.Errorf("unsupported database type: %s", d.Type)
	}
}

// encryptConfigPassword encrypts the password in the config (in-place).
func encryptConfigPassword(cfg *database.Config) error {
	key := crypto.GetEncryptionKey()
	if key == nil {
		// No JWT_SECRET — can't encrypt. Leave as-is (will be plaintext,
		// but the system won't boot without JWT_SECRET anyway).
		return nil
	}

	switch cfg.Type {
	case database.DatabaseTypePostgres:
		if cfg.Postgres != nil && cfg.Postgres.Password != "" && !isEncrypted(cfg.Postgres.Password) {
			enc, err := crypto.Encrypt(cfg.Postgres.Password, key)
			if err != nil {
				return err
			}
			cfg.Postgres.Password = enc
		}
	case database.DatabaseTypeSupabase:
		if cfg.Supabase != nil && cfg.Supabase.Database != nil && cfg.Supabase.Database.Password != "" && !isEncrypted(cfg.Supabase.Database.Password) {
			enc, err := crypto.Encrypt(cfg.Supabase.Database.Password, key)
			if err != nil {
				return err
			}
			cfg.Supabase.Database.Password = enc
		}
	}
	return nil
}

func isEncrypted(s string) bool {
	return len(s) >= 4 && s[:4] == "enc:"
}

// sanitizeConfig returns a copy of the config with all passwords removed.
func sanitizeConfig(cfg *database.Config) map[string]any {
	out := map[string]any{
		"type": string(cfg.Type),
	}
	switch cfg.Type {
	case database.DatabaseTypePostgres:
		out["postgres"] = map[string]any{
			"host":     cfg.Postgres.Host,
			"port":     cfg.Postgres.Port,
			"database": cfg.Postgres.Database,
			"username": cfg.Postgres.Username,
			"ssl_mode": cfg.Postgres.SSLMode,
			// password intentionally omitted
		}
	case database.DatabaseTypeSupabase:
		out["supabase"] = map[string]any{
			"project_ref": cfg.Supabase.ProjectRef,
			"project_url": cfg.Supabase.ProjectURL,
			"database": map[string]any{
				"host":     cfg.Supabase.Database.Host,
				"port":     cfg.Supabase.Database.Port,
				"database": cfg.Supabase.Database.Database,
				"username": cfg.Supabase.Database.Username,
				"ssl_mode": cfg.Supabase.Database.SSLMode,
			},
		}
	case database.DatabaseTypeSQLite:
		out["sqlite"] = map[string]any{
			"path": cfg.SQLite.Path,
		}
	}
	return out
}

// auditDB records a database configuration change in the event log.
func (h *DatabaseHandler) auditDB(c *gin.Context, action, dbType string) {
	if h.App.Events == nil {
		return
	}
	evCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = h.App.Events.Append(evCtx, models.Event{
		Category: models.EventCategorySystem,
		Severity: models.SeverityInfo,
		Source:   "database:" + action,
		Message:  "Database " + action + " to " + dbType,
		Data: map[string]any{
			"action":   action,
			"db_type":  dbType,
			"user_id":  c.GetString("user_id"),
			"username": c.GetString("username"),
		},
	})
}

// dbError logs and returns a 500 server error.
func (h *DatabaseHandler) dbError(c *gin.Context, op string, err error) {
	h.App.Logger.Error().Err(err).Str("op", op).Msg("database_handler_error")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
}

// migrate copies data from the current database (SQLite) into a target
// PostgreSQL/Supabase database. The target schema is created first via
// migrations, then all tables are copied. The source is read-only and
// untouched throughout; on failure the target is rolled back.
func (h *DatabaseHandler) migrate(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()

	var req dbConfigDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	cfg, err := req.toConfig()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	// Migration only makes sense FROM SQLite TO PostgreSQL/Supabase.
	if cfg.Type == database.DatabaseTypeSQLite {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_target", "detail": "target must be PostgreSQL or Supabase"})
		return
	}

	// Source = the running app's live SQLite connection.
	source := database.WrapSQLite(h.App.DB)

	// Target = the requested PostgreSQL/Supabase.
	mgr, err := database.NewManager(cfg)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_target", "detail": err.Error()})
		return
	}
	if err := mgr.Connect(ctx); err != nil {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"ok": false, "error": "connect target: " + err.Error()}})
		return
	}
	defer func() { _ = mgr.Close() }()
	target, err := mgr.DB()
	if err != nil {
		h.dbError(c, "db.migrate.target", err)
		return
	}

	// Create schema on target (dialect-aware migrations).
	if err := target.RunMigrations(ctx); err != nil {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"ok": false, "error": "target migrations: " + err.Error()}})
		return
	}

	// Copy data.
	migrator := database.NewMigrator(source, target)
	stats, err := migrator.GetStats(ctx)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"ok": false, "error": "stats: " + err.Error()}})
		return
	}

	start := time.Now()
	if err := migrator.Migrate(ctx, nil); err != nil {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"ok":       false,
			"error":    err.Error(),
			"duration_ms": time.Since(start).Milliseconds(),
		}})
		return
	}

	h.auditDB(c, "database_migrate", string(cfg.Type))

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"ok":           true,
		"total_rows":   stats.TotalRows,
		"tables":       len(stats.Tables),
		"duration_ms":  time.Since(start).Milliseconds(),
		"restart_required": true,
		"message":      "Data migrated. Save configuration and restart to switch.",
	}})
}

func itoa(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%d", n)
}
