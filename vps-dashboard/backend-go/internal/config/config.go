package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	Env                    string
	HTTPAddr               string
	DBPath                 string
	JWTSecret              string
	JWTTTL                 time.Duration
	BootstrapAdminUsername string
	BootstrapAdminPassword string
	LogLevel               string
	CORSOrigins            []string

	// Wave 3 retention/cadence knobs. All four are positive durations
	// validated at load time; defaults are applied when the env var is
	// missing.
	EventsRetention     time.Duration
	HealthRetention     time.Duration
	HealthcheckInterval time.Duration
	SystemTickInterval  time.Duration

	// Wave 4 backup knobs.
	BackupDir       string
	BackupKeep      int
	BackupHourLocal int
}

// LoadFromEnv reads configuration from process environment.
// Required variables (currently only JWT_SECRET) cause an error if missing.
// BOOTSTRAP_ADMIN_PASSWORD is validated later, only when bootstrap is needed.
func LoadFromEnv() (*Config, error) {
	cfg := &Config{
		Env:                    getEnv("ENV", "development"),
		HTTPAddr:               getEnv("HTTP_ADDR", ":3001"),
		DBPath:                 getEnv("DB_PATH", "./data/vps-dashboard.db"),
		JWTSecret:              os.Getenv("JWT_SECRET"),
		BootstrapAdminUsername: getEnv("BOOTSTRAP_ADMIN_USERNAME", "admin"),
		BootstrapAdminPassword: os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
		LogLevel:               getEnv("LOG_LEVEL", "info"),
	}

	if strings.TrimSpace(cfg.JWTSecret) == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	ttlRaw := getEnv("JWT_TTL", "24h")
	ttl, err := time.ParseDuration(ttlRaw)
	if err != nil {
		return nil, fmt.Errorf("invalid JWT_TTL %q: %w", ttlRaw, err)
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("JWT_TTL must be positive, got %s", ttl)
	}
	cfg.JWTTTL = ttl

	originsRaw := getEnv("CORS_ORIGINS", "http://localhost:5173")
	for _, o := range strings.Split(originsRaw, ",") {
		if v := strings.TrimSpace(o); v != "" {
			cfg.CORSOrigins = append(cfg.CORSOrigins, v)
		}
	}
	if len(cfg.CORSOrigins) == 0 {
		cfg.CORSOrigins = []string{"http://localhost:5173"}
	}

	switch cfg.Env {
	case "development", "production":
	default:
		return nil, fmt.Errorf("invalid ENV %q: must be development or production", cfg.Env)
	}

	if d, err := parsePositiveDuration("EVENTS_RETENTION", "720h"); err != nil {
		return nil, err
	} else {
		cfg.EventsRetention = d
	}
	if d, err := parsePositiveDuration("HEALTH_RETENTION", "336h"); err != nil {
		return nil, err
	} else {
		cfg.HealthRetention = d
	}
	if d, err := parsePositiveDuration("HEALTHCHECK_INTERVAL", "60s"); err != nil {
		return nil, err
	} else {
		cfg.HealthcheckInterval = d
	}
	if d, err := parsePositiveDuration("SYSTEM_TICK_INTERVAL", "30s"); err != nil {
		return nil, err
	} else {
		cfg.SystemTickInterval = d
	}

	cfg.BackupDir = getEnv("BACKUP_DIR", "./data/backups")
	if n, err := parsePositiveInt("BACKUP_KEEP", "7"); err != nil {
		return nil, err
	} else {
		cfg.BackupKeep = n
	}
	if n, err := parseHourOfDay("BACKUP_HOUR_LOCAL", "3"); err != nil {
		return nil, err
	} else {
		cfg.BackupHourLocal = n
	}

	return cfg, nil
}

// parsePositiveInt reads a non-negative integer env var, defaulting to
// fallback when unset. The result must be > 0.
func parsePositiveInt(key, fallback string) (int, error) {
	raw := getEnv(key, fallback)
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", key, raw, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%s must be positive, got %d", key, n)
	}
	return n, nil
}

// parseHourOfDay accepts an integer in [0, 23].
func parseHourOfDay(key, fallback string) (int, error) {
	raw := getEnv(key, fallback)
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", key, raw, err)
	}
	if n < 0 || n > 23 {
		return 0, fmt.Errorf("%s must be between 0 and 23, got %d", key, n)
	}
	return n, nil
}

// parsePositiveDuration reads a duration env var, defaulting to fallback
// when unset. The result must be > 0; otherwise an error is returned.
func parsePositiveDuration(key, fallback string) (time.Duration, error) {
	raw := getEnv(key, fallback)
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", key, raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive, got %s", key, d)
	}
	return d, nil
}

// IsProduction reports whether the configured environment is production.
func (c *Config) IsProduction() bool {
	return c.Env == "production"
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
