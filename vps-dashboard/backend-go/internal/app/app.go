package app

import (
	"database/sql"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/alerts"
	"vps-dashboard-api/internal/backup"
	"vps-dashboard-api/internal/cloud"
	"vps-dashboard-api/internal/commands"
	"vps-dashboard-api/internal/config"
	"vps-dashboard-api/internal/containers"
	"vps-dashboard-api/internal/deploy"
	"vps-dashboard-api/internal/files"
	"vps-dashboard-api/internal/healthcheck"
	"vps-dashboard-api/internal/mcp"
	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/notifier"
	"vps-dashboard-api/internal/remote"
	"vps-dashboard-api/internal/search"
	"vps-dashboard-api/internal/ssh"
	"vps-dashboard-api/internal/sysinfo"
)

// Version is the running build's semantic version, exposed via /health.
const Version = "go-1.0.0"

// App is the shared dependency container passed into the HTTP layer.
//
// The first block of fields was introduced in earlier waves and must
// not change shape (the test helpers and existing handlers all
// instantiate App with these names). The second block was added in
// Wave 3 to expose the alert/health pipeline pieces to handlers.
// The third block was added in Wave 4 for deploy/backup/env-overrides.
type App struct {
	Cfg      *config.Config
	DB       *sql.DB
	Logger   zerolog.Logger
	Recorder *sysinfo.Recorder

	Events         *models.EventRepo
	Channels       *models.ChannelRepo
	Alerts         *models.AlertRepo
	Health         *models.HealthRepo
	Settings       *models.SettingsRepo
	Notifier       *notifier.Service
	AlertEvaluator *alerts.Evaluator
	HealthEngine   *healthcheck.Engine

	Deployments   *deploy.DeploymentRepo
	Backups       *backup.Repo
	EnvOverrides  *models.EnvOverrideRepo
	DeployService *deploy.Service
	BackupService *backup.Service

	// Infrastructure Platform (Phase 1: Server Registry).
	Servers *models.ServerRepo

	// Infrastructure Platform (Phase 2: SSH engine).
	SSHKeys    *ssh.KeyStore
	SSHService *ssh.Service

	// Infrastructure Platform (Phase 3: remote monitoring).
	ServerMetrics   *models.ServerMetricRepo
	RemoteEngine    *remote.Engine

	// Infrastructure Platform (Phase 4: container fleet).
	ContainerService *containers.Service

	// Infrastructure Platform (Phase 6: multi-host commands).
	CommandService *commands.Service

	// Infrastructure Platform (Phase 7: file manager).
	FileService *files.Service

	// Infrastructure Platform (Phase 8: SSH tunnels).
	TunnelManager *ssh.TunnelManager

	// Infrastructure Platform (Phase 9: cloud discovery).
	CloudRegistry *cloud.Registry

	// Infrastructure Platform (Phase 10: infrastructure search).
	SearchService *search.Service

	// Infrastructure Platform (Phase 12: MCP/AI).
	MCP *mcp.Server
}
