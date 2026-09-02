package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"vps-dashboard-api/internal/alerts"
	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/auth"
	"vps-dashboard-api/internal/backup"
	"vps-dashboard-api/internal/cloud"
	"vps-dashboard-api/internal/commands"
	"vps-dashboard-api/internal/config"
	"vps-dashboard-api/internal/containers"
	"vps-dashboard-api/internal/database"
	"vps-dashboard-api/internal/db"
	"vps-dashboard-api/internal/deploy"
	"vps-dashboard-api/internal/discovery"
	"vps-dashboard-api/internal/docker"
	"vps-dashboard-api/internal/files"
	"vps-dashboard-api/internal/healthcheck"
	"vps-dashboard-api/internal/httpx"
	"vps-dashboard-api/internal/maintenance"
	"vps-dashboard-api/internal/mcp"
	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/notifier"
	"vps-dashboard-api/internal/pm2"
	"vps-dashboard-api/internal/remote"
	"vps-dashboard-api/internal/search"
	"vps-dashboard-api/internal/ssh"
	"vps-dashboard-api/internal/sysinfo"
	"vps-dashboard-api/internal/tunnel"
)

func main() {
	// Parse CLI flags
	versionFlag := flag.Bool("version", false, "Show version information")
	flag.BoolVar(versionFlag, "v", false, "Show version information (shorthand)")
	flag.Parse()

	// Handle --version flag
	if *versionFlag {
		fmt.Printf("VPS Dashboard %s\n", app.Version)
		fmt.Printf("Build Commit: %s\n", app.BuildCommit)
		fmt.Printf("Build Time:   %s\n", app.BuildTime)
		if httpx.HasEmbeddedFrontend() {
			fmt.Println("Frontend:     embedded")
		} else {
			fmt.Println("Frontend:     not embedded")
		}
		os.Exit(0)
	}

	if err := run(); err != nil {
		log.Error().Err(err).Msg("fatal")
		os.Exit(1)
	}
}

func run() error {
	// 1. Load .env if present (no error if missing).
	_ = godotenv.Load()

	// 2. Load config from env.
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	// 3. Configure zerolog.
	logger := buildLogger(cfg)
	zerolog.DefaultContextLogger = &logger
	log.Logger = logger

	// 3.5. Detect deployment mode and configure gopsutil paths.
	//      This must happen BEFORE any sysinfo/gopsutil calls so the
	//      correct HOST_PROC path is set for metrics collection.
	mode := config.DetectDeploymentMode()
	if err := config.SetGopsutilEnv(mode); err != nil {
		logger.Warn().Err(err).Msg("failed to set gopsutil env")
	}
	features := config.DetectFeatures(mode)
	logger.Info().
		Str("mode", string(mode)).
		Bool("docker_fleet", features.DockerFleet).
		Bool("pm2_monitor", features.PM2Monitor).
		Str("proc_path", features.ProcPath).
		Msg("deployment mode detected")

	// 4. Open DB — via database.Manager abstraction layer.
	//    Supports SQLite (default), PostgreSQL, and Supabase.
	//    Config file: ./data/database.json (falls back to SQLite default).
	//    Env vars still override for backwards compatibility:
	//      - VPSDASH_DB_CONFIG (path to config file)
	//      - DB_PATH (SQLite path, if config file missing)

	// Bind a context that is cancelled on SIGINT/SIGTERM so background
	// workers (e.g. the Recorder ticker) stop cleanly when the server is
	// shutting down.
	rootCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	dbConfigPath := database.GetDefaultConfigPath()
	if _, err := os.Stat(dbConfigPath); err != nil {
		// Config file doesn't exist — create default SQLite config.
		sqlitePath := cfg.DBPath
		defaultConfig := database.DefaultSQLiteConfig(sqlitePath)
		if err := database.SaveConfig(dbConfigPath, defaultConfig); err != nil {
			logger.Warn().Err(err).Msg("failed to create default database config, using direct SQLite open")
		} else {
			logger.Info().Str("path", dbConfigPath).Msg("created default database config (SQLite)")
		}
	}

	dbManager, err := database.NewManagerFromFile(dbConfigPath)
	var conn *sql.DB
	if err != nil {
		// Fallback: open SQLite directly (backwards compat with v1.x)
		logger.Warn().Err(err).Msg("database config load failed, falling back to direct SQLite open")
		conn, err = db.Open(cfg.DBPath)
		if err != nil {
			return err
		}
	} else {
		if err := dbManager.ConnectWithRetry(rootCtx); err != nil {
			return err
		}
		dbInst, _ := dbManager.DB()
		conn = dbInst.Underlying()
		logger.Info().
			Str("type", string(dbManager.Type())).
			Str("conn", dbManager.ConnectionString()).
			Msg("database connected via abstraction layer")
	}
	defer func() { _ = conn.Close() }()

	// 5. Run migrations.
	//    Select dialect based on database type (SQLite vs PostgreSQL).
	var dialect db.Dialect
	if dbManager != nil {
		switch dbManager.Type() {
		case database.DatabaseTypePostgres, database.DatabaseTypeSupabase:
			dialect = db.DialectPostgres
		default:
			dialect = db.DialectSQLite
		}
	} else {
		dialect = db.DialectSQLite
	}
	if err := db.Migrate(rootCtx, conn, logger, dialect); err != nil {
		return err
	}

	// 6. Bootstrap admin if needed.
	if err := auth.EnsureAdmin(rootCtx, conn, cfg.BootstrapAdminUsername, cfg.BootstrapAdminPassword, logger); err != nil {
		return err
	}

	// 7. Build app + router.
	recorder := sysinfo.NewRecorder(720) // 12h at 60s tick

	// Repos for the alert/health/notification pipeline.
	eventsRepo := models.NewEventRepo(conn)
	channelsRepo := models.NewChannelRepo(conn)
	alertsRepo := models.NewAlertRepo(conn)
	healthRepo := models.NewHealthRepo(conn)
	settingsRepo := models.NewSettingsRepo(conn)
	projectsRepo := models.NewProjectRepo(conn)
	envOverridesRepo := models.NewEnvOverrideRepo(conn)
	serversRepo := models.NewServerRepo(conn)

	// Discovery service correlates docker/pm2/tunnel state. We keep a
	// single instance for the auto-seed step below; HTTP handlers build
	// their own as part of NewDiscoveryHandler.
	discoverySvc := discovery.NewService(
		logger,
		docker.NewService(logger),
		tunnel.NewService(logger),
		pm2.NewService(logger),
	)

	// 6.5. Auto-seed projects on first boot. Failures are non-fatal:
	// the dashboard should still come up so the operator can fix
	// whatever blocked discovery (missing docker socket, broken pm2,
	// etc.) from inside the UI.
	if err := autoSeedProjects(rootCtx, projectsRepo, discoverySvc, logger); err != nil {
		logger.Warn().Err(err).Msg("auto-seed.failed")
	}

	// Notifier with the only sender we ship today.
	notif := notifier.NewService(logger, channelsRepo, map[string]notifier.Sender{
		models.ChannelTypeTelegram: notifier.NewTelegramSender(),
	})

	// Alert evaluator wires rules + channels + notifier + events.
	evaluator := alerts.NewEvaluator(logger, alertsRepo, channelsRepo, notif, eventsRepo)
	evaluator.EnvFloor = alerts.NewEnvFloor(logger, projectsRepo, envOverridesRepo)

	// Health-check engine.
	healthEngine := healthcheck.NewEngine(logger, projectsRepo, healthRepo, eventsRepo, evaluator, cfg.HealthcheckInterval)
	healthEngine.EnvOverrides = envOverridesRepo

	// Wave 4 services.
	deploymentRepo := deploy.NewDeploymentRepo(conn)
	deployService := deploy.NewService(logger, projectsRepo, deploymentRepo, eventsRepo)
	backupRepo := backup.NewRepo(conn)

	// Infrastructure Platform (Phase 2): SSH key store + engine. A
	// failure to open the key store is fatal — the platform refuses
	// to run SSH operations against an unreadable credential store.
	sshKeys, err := ssh.NewKeyStore(cfg.SSHKeysDir)
	if err != nil {
		return err
	}
	sshService := ssh.NewService(sshKeys)

	// Infrastructure Platform (Phase 3): remote monitoring engine.
	metricsRepo := models.NewServerMetricRepo(conn)
	remoteCollector := remote.NewCollector(sshService)
	remoteEngine := remote.NewEngine(
		logger,
		serversRepo,
		metricsRepo,
		eventsRepo,
		remoteCollector,
		remote.EngineConfig{
			Interval:       cfg.RemotePollInterval,
			MaxParallel:    cfg.RemoteMaxParallel,
			CommandTimeout: cfg.RemoteCommandTimeout,
			Retention:      cfg.RemoteRetention,
		},
	)

	// Infrastructure Platform (Phase 4): container fleet management.
	containerSvc := containers.NewService(sshService)

	// Infrastructure Platform (Phase 6): multi-host command engine.
	commandRunsRepo := models.NewCommandRunRepo(conn)
	commandSvc := commands.NewService(logger, sshService, commandRunsRepo)

	// Infrastructure Platform (Phase 7): file manager (SFTP).
	fileSvc := files.NewService(sshService)

	// Infrastructure Platform (Phase 8): SSH tunnel manager.
	tunnelMgr := ssh.NewTunnelManager(sshService)

	// Infrastructure Platform (Phase 9): cloud discovery.
	cloudReg := cloud.NewRegistry()
	cloudReg.Register(cloud.NewManualProvider(nil))

	// Infrastructure Platform (Phase 10): infrastructure search.
	tunnelRepo := models.NewTunnelRepo(conn)
	snippetRepo := models.NewCommandSnippetRepo(conn)
	searchSvc := search.NewService(serversRepo, snippetRepo, tunnelRepo)

	// Infrastructure Platform (Phase 12): MCP/AI server (read-only).
	var mcpServer *mcp.Server
	if cfg.MCPAPIKey != "" {
		mcpServer = mcp.NewServer(
			logger,
			serversRepo,
			metricsRepo,
			eventsRepo,
			snippetRepo,
			tunnelRepo,
			cfg.MCPAPIKey,
			cfg.MCPAuditPath,
		)
		logger.Info().Str("audit_path", cfg.MCPAuditPath).Msg("mcp.server_enabled")
	}

	backupService := &backup.Service{
		Logger:    logger,
		DB:        conn,
		DBPath:    cfg.DBPath,
		Dir:       cfg.BackupDir,
		Keep:      cfg.BackupKeep,
		HourLocal: cfg.BackupHourLocal,
		Repo:      backupRepo,
		Events:    eventsRepo,
	}

	a := &app.App{
		Cfg:            cfg,
		DB:             conn,
		Logger:         logger,
		Recorder:       recorder,
		Events:         eventsRepo,
		Channels:       channelsRepo,
		Alerts:         alertsRepo,
		Health:         healthRepo,
		Settings:       settingsRepo,
		Notifier:       notif,
		AlertEvaluator: evaluator,
		HealthEngine:   healthEngine,
		Deployments:    deploymentRepo,
		Backups:        backupRepo,
		EnvOverrides:   envOverridesRepo,
		DeployService:  deployService,
		BackupService:  backupService,
		Servers:        serversRepo,
		SSHKeys:        sshKeys,
		SSHService:     sshService,
		ServerMetrics:  metricsRepo,
		RemoteEngine:   remoteEngine,
		ContainerService: containerSvc,
		CommandService:  commandSvc,
		FileService:     fileSvc,
		TunnelManager:   tunnelMgr,
		CloudRegistry:   cloudReg,
		SearchService:  searchSvc,
		MCP:            mcpServer,
	}
	router := httpx.NewRouter(a)

	// Background recorder loop. We capture immediately so /system/history
	// has at least one sample before the first tick interval elapses.
	go runRecorder(rootCtx, recorder, logger)

	// Health probe engine for projects with a health_url.
	go healthEngine.Run(rootCtx)

	// System tick: feeds CPU/memory/disk into the alert evaluator.
	go alerts.RunSystemTick(rootCtx, logger, recorder, evaluator, alerts.SystemTickConfig{
		Interval: cfg.SystemTickInterval,
	})

	// Maintenance purger trims old events and health rows.
	purger := &maintenance.Purger{
		Logger:         logger,
		Events:         eventsRepo,
		Health:         healthRepo,
		Metrics:        metricsRepo,
		KeepEventsFor:  cfg.EventsRetention,
		KeepHealthFor:  cfg.HealthRetention,
		KeepMetricsFor: cfg.RemoteRetention,
	}
	go purger.Run(rootCtx)

	// Daily backup scheduler.
	go backupService.Run(rootCtx)

	// Remote monitoring loop: polls registered servers for metrics.
	go remoteEngine.Run(rootCtx)

	// 8. http.Server with sane timeouts.
	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 10. Startup banner.
	logger.Info().
		Str("addr", cfg.HTTPAddr).
		Str("env", cfg.Env).
		Str("db", cfg.DBPath).
		Str("version", app.Version).
		Str("commit", app.BuildCommit).
		Str("build_time", app.BuildTime).
		Bool("frontend_embedded", httpx.HasEmbeddedFrontend()).
		Dur("healthcheck_interval", cfg.HealthcheckInterval).
		Dur("system_tick_interval", cfg.SystemTickInterval).
		Dur("events_retention", cfg.EventsRetention).
		Dur("health_retention", cfg.HealthRetention).
		Str("backup_dir", cfg.BackupDir).
		Int("backup_keep", cfg.BackupKeep).
		Int("backup_hour_local", cfg.BackupHourLocal).
		Msg("vps-dashboard-api starting")

	// 9. Graceful shutdown on SIGINT/SIGTERM.
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-rootCtx.Done():
		logger.Info().Str("signal", "shutdown").Msg("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			return err
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("shutdown error")
		return err
	}
	logger.Info().Msg("server stopped cleanly")
	return nil
}

func buildLogger(cfg *config.Config) zerolog.Logger {
	level, err := zerolog.ParseLevel(strings.ToLower(cfg.LogLevel))
	if err != nil || level == zerolog.NoLevel {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)
	zerolog.TimeFieldFormat = time.RFC3339

	if cfg.IsProduction() {
		return zerolog.New(os.Stdout).With().Timestamp().Logger()
	}
	return zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).
		With().Timestamp().Logger()
}

// runRecorder ticks the in-memory metrics recorder once a minute. It
// returns when ctx is cancelled. The first tick is performed eagerly so
// the /system/history endpoint has at least one sample available
// shortly after boot.
func runRecorder(ctx context.Context, recorder *sysinfo.Recorder, logger zerolog.Logger) {
	const tickInterval = 60 * time.Second

	// First tick is bounded to a few seconds so a slow gopsutil call does
	// not block the server boot sequence indefinitely.
	bootCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	recorder.Tick(bootCtx)
	cancel()

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info().Msg("recorder.stopped")
			return
		case <-ticker.C:
			tickCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			recorder.Tick(tickCtx)
			cancel()
		}
	}
}

// autoSeedProjects runs once on first boot: when the projects table is
// empty, every discovery candidate (docker container, pm2 app, tunnel
// ingress) is adopted into the registry so the dashboard ships with a
// populated project list. The check is idempotent across restarts —
// once any project exists, this is a no-op.
//
// All failures are non-fatal. Discovery is best-effort (a missing
// docker socket should not stop the dashboard from starting), and a
// per-candidate insert error simply skips that candidate and continues.
// The combined report is logged at the end so the operator can see
// what happened during the seed.
func autoSeedProjects(
	ctx context.Context,
	repo *models.ProjectRepo,
	svc *discovery.Service,
	logger zerolog.Logger,
) error {
	count, err := repo.Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		logger.Info().Int("existing", count).Msg("auto-seed.skipped")
		return nil
	}

	// Bound the seed to a generous-but-finite budget so a slow docker /
	// pm2 / tunnel call cannot wedge boot.
	seedCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	report, err := svc.AdoptAll(seedCtx, repo)
	if err != nil {
		logger.Warn().Err(err).Msg("auto-seed.snapshot_failed")
		return nil
	}

	logger.Info().
		Int("adopted", len(report.Adopted)).
		Int("skipped", len(report.Skipped)).
		Int("errors", len(report.Errors)).
		Msg("auto-seed.done")

	for _, r := range report.Errors {
		logger.Warn().Str("name", r.Name).Str("err", r.Error).Msg("auto-seed.candidate_error")
	}
	return nil
}
