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

	// Public auth routes (login/logout) live on the root group.
	public := r.Group("")
	authH.Register(public)
	// Webhook receiver is public: HMAC over the request body is the
	// authentication mechanism.
	webhooksH.RegisterPublic(public)

	// Protected routes require a valid JWT.
	protected := r.Group("")
	protected.Use(middleware.RequireAuth([]byte(a.Cfg.JWTSecret)))
	authH.RegisterProtected(protected)
	systemH.Register(protected)
	tunnelH.Register(protected)
	dockerH.RegisterReads(protected)
	pm2H.RegisterReads(protected)
	genH.Register(protected)
	projectsH.RegisterReads(protected)
	discoveryH.RegisterReads(protected)
	eventsH.RegisterReads(protected)
	notifsH.RegisterChannelsReads(protected)
	notifsH.RegisterRulesReads(protected)
	backupsH.RegisterReads(protected)
	envH.RegisterReads(protected)

	// Mutating routes additionally require an admin role.
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

	return r
}
