package httpx

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/httpx/handlers"
	"vps-dashboard-api/internal/httpx/middleware"
)

// NewRouter wires the gin engine with middleware and routes.
func NewRouter(a *app.App) *gin.Engine {
	return BuildEngine(a)
}

// BuildEngine constructs the application's *gin.Engine. It is exported so
// tests can wire the same handlers/middleware that production uses.
func BuildEngine(a *app.App) *gin.Engine {
	if a.Cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	r.Use(middleware.RequestID())
	r.Use(middleware.Logger(a.Logger))
	r.Use(middleware.Recover(a.Logger))
	r.Use(cors.New(cors.Config{
		AllowOrigins:     a.Cfg.CORSOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type", "X-Request-ID"},
		ExposeHeaders:    []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	r.GET("/health", handlers.Health(a))

	authH := handlers.NewAuthHandler(a)
	systemH := handlers.NewSystemHandler(a)
	tunnelH := handlers.NewTunnelHandler(a)
	dockerH := handlers.NewDockerHandler(a)
	pm2H := handlers.NewPM2Handler(a)
	genH := handlers.NewGeneratorHandler(a)
	projectsH := handlers.NewProjectHandler(a)
	discoveryH := handlers.NewDiscoveryHandler(a)
	usersH := handlers.NewUsersHandler(a)
	eventsH := handlers.NewEventsHandler(a)
	notifsH := handlers.NewNotificationsHandler(a)
	webhooksH := handlers.NewWebhooksHandler(a)
	backupsH := handlers.NewBackupsHandler(a)
	envH := handlers.NewEnvHandler(a)
	serversH := handlers.NewServerHandler(a)
	sshH := handlers.NewSSHHandler(a)
	containersH := handlers.NewContainerHandler(a)
	terminalH := handlers.NewTerminalHandler(a)
	commandsH := handlers.NewCommandHandler(a)
	filesH := handlers.NewFileHandler(a)
	sshTunnelsH := handlers.NewSshTunnelHandler(a)
	cloudH := handlers.NewCloudHandler(a)
	searchH := handlers.NewSearchHandler(a)
	mcpH := handlers.NewMCPHandler(a)
	dbH := handlers.NewDatabaseHandler(a)

	// Public auth routes (login/logout) live on the root group.
	public := r.Group("")
	authH.Register(public)
	// Webhook receiver is public: HMAC over the request body is the
	// authentication mechanism.
	webhooksH.RegisterPublic(public)
	// MCP endpoints are public (API key auth is inline, not JWT).
	mcpH.Register(public)

	// Protected routes require a valid JWT.
	protected := r.Group("")
	protected.Use(middleware.RequireAuth([]byte(a.Cfg.JWTSecret)))
	authH.RegisterProtected(protected)
	systemH.Register(protected)
	tunnelH.Register(protected)
	dockerH.RegisterReads(protected)
	pm2H.RegisterReads(protected)
	projectsH.RegisterReads(protected)
	discoveryH.RegisterReads(protected)
	eventsH.RegisterReads(protected)
	notifsH.RegisterChannelsReads(protected)
	notifsH.RegisterRulesReads(protected)
	backupsH.RegisterReads(protected)
	envH.RegisterReads(protected)
	serversH.RegisterReads(protected)
	sshH.RegisterReads(protected)
	containersH.RegisterReads(protected)
	commandsH.RegisterReads(protected)
	filesH.RegisterReads(protected)
	sshTunnelsH.RegisterReads(protected)
	cloudH.RegisterReads(protected)
	searchH.RegisterReads(protected)
	dbH.RegisterReads(protected)

	// Mutating routes — operator-level (admin + operator can access).
	// Per §32: OPERATOR can restart containers, run commands, deploy,
	// backup, and use SSH terminal/files. ADMIN can do everything
	// OPERATOR can plus manage users/credentials/providers.
	operatorOnly := r.Group("")
	operatorOnly.Use(middleware.RequireAuth([]byte(a.Cfg.JWTSecret)))
	operatorOnly.Use(middleware.RequireRole("admin", "operator"))

	// Container operations: restart/stop/start (operator-level).
	containersH.RegisterWrites(operatorOnly)

	// Terminal WebSocket (operator-level interactive access).
	terminalH.Register(operatorOnly)

	// File operations: upload/mkdir/rename/delete (operator-level).
	filesH.RegisterWrites(operatorOnly)

	// SSH operations: test + command execution (operator-level).
	// Key management stays admin-only.
	sshH.RegisterOperatorWrites(operatorOnly)

	// Command execution + preview (operator-level).
	// Snippet CRUD stays admin-only.
	commandsH.RegisterOperatorWrites(operatorOnly)

	// Generator (operator-level — renders command strings, no execution).
	genH.Register(operatorOnly)

	// Admin-only routes (manage users, credentials, providers, config).
	adminOnly := r.Group("")
	adminOnly.Use(middleware.RequireAuth([]byte(a.Cfg.JWTSecret)))
	adminOnly.Use(middleware.RequireRole("admin"))
	dockerH.RegisterWrites(adminOnly)
	pm2H.RegisterWrites(adminOnly)
	tunnelH.RegisterWrites(adminOnly)
	projectsH.RegisterWrites(adminOnly)
	discoveryH.RegisterWrites(adminOnly)
	usersH.RegisterWrites(adminOnly)
	notifsH.RegisterChannelsWrites(adminOnly)
	notifsH.RegisterRulesWrites(adminOnly)
	backupsH.RegisterWrites(adminOnly)
	envH.RegisterWrites(adminOnly)
	serversH.RegisterWrites(adminOnly)
	sshH.RegisterWrites(adminOnly)
	commandsH.RegisterWrites(adminOnly)
	sshTunnelsH.RegisterWrites(adminOnly)
	cloudH.RegisterWrites(adminOnly)
	dbH.RegisterWrites(adminOnly)

	// Serve embedded frontend (SPA fallback). This must come last so API
	// routes take precedence. If the frontend wasn't embedded at build time,
	// this handler will still work but serve 404s for all frontend routes.
	r.Use(ServeFrontend())

	return r
}
