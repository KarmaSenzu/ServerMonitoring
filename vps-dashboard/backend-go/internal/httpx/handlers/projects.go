package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/deploy"
	"vps-dashboard-api/internal/docker"
	"vps-dashboard-api/internal/httpx/middleware"
	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/pm2"
	"vps-dashboard-api/internal/tunnel"
)

// projectTimeout caps every project CRUD request.
const projectTimeout = 8 * time.Second

// projectHealthTimeout caps an outbound HTTP probe of project.health_url.
const projectHealthTimeout = 5 * time.Second

// projectActionTimeout caps a project-aware start/stop/restart, which
// proxies to the underlying pm2/docker/tunnel command.
const projectActionTimeout = 30 * time.Second

// ProjectHandler exposes /projects/* endpoints. Read routes are mounted
// under a group that already requires auth; write routes additionally
// require RequireRole("admin") at the caller.
type ProjectHandler struct {
	App    *app.App
	Repo   *models.ProjectRepo
	Health *models.HealthRepo
	PM2    *pm2.Service
	Docker *docker.Service
	Tunnel *tunnel.Service
	Deploy *deploy.Service
}

// NewProjectHandler constructs a ProjectHandler with a repo bound to
// the app's DB connection plus the underlying PM2/Docker/Tunnel
// services used by project-aware actions. The health repo is reused
// from a.Health when wired by main; otherwise a fresh repo is built.
func NewProjectHandler(a *app.App) *ProjectHandler {
	healthRepo := a.Health
	if healthRepo == nil {
		healthRepo = models.NewHealthRepo(a.DB)
	}
	return &ProjectHandler{
		App:    a,
		Repo:   models.NewProjectRepo(a.DB),
		Health: healthRepo,
		PM2:    pm2.NewService(a.Logger),
		Docker: docker.NewService(a.Logger),
		Tunnel: tunnel.NewService(a.Logger),
		Deploy: a.DeployService,
	}
}

// RegisterReads mounts the read-only project routes.
func (h *ProjectHandler) RegisterReads(rg *gin.RouterGroup) {
	rg.GET("/projects", h.list)
	rg.GET("/projects/:id", h.get)
	rg.GET("/projects/by-name/:name", h.getByName)
	rg.GET("/projects/:id/health", h.health)
	rg.GET("/projects/:id/health-history", h.healthHistory)
	rg.GET("/projects/:id/deployments", h.listDeployments)
	rg.GET("/projects/:id/deployments/:deployment_id", h.getDeployment)
}

// RegisterWrites mounts the mutating project routes. Caller is
// responsible for adding admin-role middleware.
func (h *ProjectHandler) RegisterWrites(rg *gin.RouterGroup) {
	rg.POST("/projects", h.create)
	rg.PUT("/projects/:id", h.put)
	rg.PATCH("/projects/:id", h.patch)
	rg.DELETE("/projects/:id", h.delete)

	rg.POST("/projects/:id/restart", h.restartAction)
	rg.POST("/projects/:id/stop", h.stopAction)
	rg.POST("/projects/:id/start", h.startAction)

	rg.POST("/projects/:id/deploy", h.triggerDeploy)
	rg.POST("/projects/:id/webhook-secret/regenerate", h.regenerateWebhookSecret)
}

// projectDTO is the JSON shape accepted on POST/PUT.
type projectDTO struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Domain        string   `json:"domain"`
	Port          int      `json:"port"`
	ContainerName string   `json:"container_name"`
	PM2Name       string   `json:"pm2_name"`
	TunnelService string   `json:"tunnel_service"`
	HealthURL     string   `json:"health_url"`
	Enabled       *bool    `json:"enabled"`
	Tags          []string `json:"tags"`
	Notes         string   `json:"notes"`

	Environment          string `json:"environment"`
	DeployCommand        string `json:"deploy_command"`
	DeployTimeoutSeconds int    `json:"deploy_timeout_seconds"`
	DeployWorkingDir     string `json:"deploy_working_dir"`
	DeployEnabled        *bool  `json:"deploy_enabled"`
}

func (d projectDTO) toProject() models.Project {
	enabled := true
	if d.Enabled != nil {
		enabled = *d.Enabled
	}
	deployEnabled := false
	if d.DeployEnabled != nil {
		deployEnabled = *d.DeployEnabled
	}
	tags := d.Tags
	if tags == nil {
		tags = []string{}
	}
	env := d.Environment
	if env == "" {
		env = models.ProjectEnvProduction
	}
	timeout := d.DeployTimeoutSeconds
	if timeout == 0 {
		timeout = 300
	}
	return models.Project{
		Name:                 d.Name,
		Description:          d.Description,
		Domain:               d.Domain,
		Port:                 d.Port,
		ContainerName:        d.ContainerName,
		PM2Name:              d.PM2Name,
		TunnelService:        d.TunnelService,
		HealthURL:            d.HealthURL,
		Enabled:              enabled,
		Tags:                 tags,
		Notes:                d.Notes,
		Environment:          env,
		DeployCommand:        d.DeployCommand,
		DeployTimeoutSeconds: timeout,
		DeployWorkingDir:     d.DeployWorkingDir,
		DeployEnabled:        deployEnabled,
	}
}

// projectPatchDTO uses pointers so the handler can distinguish "field
// not present" from "field set to empty".
type projectPatchDTO struct {
	Name          *string   `json:"name"`
	Description   *string   `json:"description"`
	Domain        *string   `json:"domain"`
	Port          *int      `json:"port"`
	ContainerName *string   `json:"container_name"`
	PM2Name       *string   `json:"pm2_name"`
	TunnelService *string   `json:"tunnel_service"`
	HealthURL     *string   `json:"health_url"`
	Enabled       *bool     `json:"enabled"`
	Tags          *[]string `json:"tags"`
	Notes         *string   `json:"notes"`

	Environment          *string `json:"environment"`
	DeployCommand        *string `json:"deploy_command"`
	DeployTimeoutSeconds *int    `json:"deploy_timeout_seconds"`
	DeployWorkingDir     *string `json:"deploy_working_dir"`
	DeployEnabled        *bool   `json:"deploy_enabled"`
}

func (d projectPatchDTO) apply(p *models.Project) {
	if d.Name != nil {
		p.Name = *d.Name
	}
	if d.Description != nil {
		p.Description = *d.Description
	}
	if d.Domain != nil {
		p.Domain = *d.Domain
	}
	if d.Port != nil {
		p.Port = *d.Port
	}
	if d.ContainerName != nil {
		p.ContainerName = *d.ContainerName
	}
	if d.PM2Name != nil {
		p.PM2Name = *d.PM2Name
	}
	if d.TunnelService != nil {
		p.TunnelService = *d.TunnelService
	}
	if d.HealthURL != nil {
		p.HealthURL = *d.HealthURL
	}
	if d.Enabled != nil {
		p.Enabled = *d.Enabled
	}
	if d.Tags != nil {
		p.Tags = *d.Tags
	}
	if d.Notes != nil {
		p.Notes = *d.Notes
	}
	if d.Environment != nil {
		p.Environment = *d.Environment
	}
	if d.DeployCommand != nil {
		p.DeployCommand = *d.DeployCommand
	}
	if d.DeployTimeoutSeconds != nil {
		p.DeployTimeoutSeconds = *d.DeployTimeoutSeconds
	}
	if d.DeployWorkingDir != nil {
		p.DeployWorkingDir = *d.DeployWorkingDir
	}
	if d.DeployEnabled != nil {
		p.DeployEnabled = *d.DeployEnabled
	}
}

// projectResponse is the safe API shape: WebhookSecret is replaced
// with a presence flag so secret values never leak via GET.
type projectResponse struct {
	models.Project
	WebhookSecretPresent bool `json:"webhook_secret_present"`
}

// sanitizeProject returns a projectResponse suitable for JSON
// encoding. Project.WebhookSecret has json:"-" so it is dropped
// automatically; we surface presence so the UI can distinguish
// "no secret yet" from "configured".
func sanitizeProject(p models.Project) projectResponse {
	return projectResponse{
		Project:              p,
		WebhookSecretPresent: strings.TrimSpace(p.WebhookSecret) != "",
	}
}

// sanitizeProjects is the slice helper for list responses.
func sanitizeProjects(in []models.Project) []projectResponse {
	out := make([]projectResponse, 0, len(in))
	for _, p := range in {
		out = append(out, sanitizeProject(p))
	}
	return out
}

func (h *ProjectHandler) list(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), projectTimeout)
	defer cancel()

	filter := models.ProjectFilter{
		Search:      strings.TrimSpace(c.Query("q")),
		Tag:         strings.TrimSpace(c.Query("tag")),
		Environment: strings.TrimSpace(c.Query("environment")),
	}
	if v := strings.TrimSpace(c.Query("enabled")); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_query", "detail": "enabled must be a boolean"})
			return
		}
		filter.EnabledOnly = b
	}

	rows, err := h.Repo.List(ctx, filter)
	if err != nil {
		h.serverError(c, "project.list", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sanitizeProjects(rows)})
}

func (h *ProjectHandler) get(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), projectTimeout)
	defer cancel()

	id := c.Param("id")
	p, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "project.get", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sanitizeProject(p)})
}

func (h *ProjectHandler) getByName(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), projectTimeout)
	defer cancel()

	name := c.Param("name")
	p, err := h.Repo.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "project.get_by_name", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sanitizeProject(p)})
}

func (h *ProjectHandler) create(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), projectTimeout)
	defer cancel()

	var dto projectDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	p := dto.toProject()
	if err := p.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	created, err := h.Repo.Create(ctx, p)
	if err != nil {
		if errors.Is(err, models.ErrDuplicateName) {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate_name"})
			return
		}
		h.serverError(c, "project.create", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": sanitizeProject(created)})
}

func (h *ProjectHandler) put(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), projectTimeout)
	defer cancel()

	id := c.Param("id")

	existing, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "project.put_lookup", err)
		return
	}

	var dto projectDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	updated := dto.toProject()
	updated.ID = existing.ID
	// Preserve the existing webhook secret across PUTs (the regenerate
	// endpoint is the only place it can be set).
	updated.WebhookSecret = existing.WebhookSecret

	if err := updated.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	res, err := h.Repo.Update(ctx, updated)
	if err != nil {
		if errors.Is(err, models.ErrDuplicateName) {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate_name"})
			return
		}
		if errors.Is(err, models.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "project.put", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sanitizeProject(res)})
}

func (h *ProjectHandler) patch(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), projectTimeout)
	defer cancel()

	id := c.Param("id")

	existing, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "project.patch_lookup", err)
		return
	}

	var dto projectPatchDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	dto.apply(&existing)

	if err := existing.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	res, err := h.Repo.Update(ctx, existing)
	if err != nil {
		if errors.Is(err, models.ErrDuplicateName) {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate_name"})
			return
		}
		if errors.Is(err, models.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "project.patch", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sanitizeProject(res)})
}

func (h *ProjectHandler) delete(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), projectTimeout)
	defer cancel()

	id := c.Param("id")
	if err := h.Repo.Delete(ctx, id); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "project.delete", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ProjectHandler) serverError(c *gin.Context, op string, err error) {
	h.App.Logger.Error().Err(err).Str("op", op).Msg("project_handler_error")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
}

// healthResult is the response shape for GET /projects/:id/health.
type healthResult struct {
	OK         bool      `json:"ok"`
	StatusCode int       `json:"status_code"`
	LatencyMs  int64     `json:"latency_ms"`
	Error      string    `json:"error,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

// health probes the project's health_url with a 5s timeout. Up to 3
// redirects are followed; 2xx/3xx counts as ok. The result is also
// persisted via HealthRepo.Append after the response is computed so
// /projects/:id/health-history reflects ad-hoc UI checks. We persist
// synchronously because SQLite WAL is fast enough that the extra ms
// is not worth the complexity of a background goroutine.
func (h *ProjectHandler) health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), projectTimeout)
	defer cancel()

	id := c.Param("id")
	p, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "project.health.lookup", err)
		return
	}
	if strings.TrimSpace(p.HealthURL) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no_health_url"})
		return
	}
	if u, err := url.Parse(p.HealthURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_health_url"})
		return
	}

	probeCtx, probeCancel := context.WithTimeout(c.Request.Context(), projectHealthTimeout)
	defer probeCancel()

	res := probeHealth(probeCtx, p.HealthURL)

	// Persist the probe outcome so /health-history surfaces ad-hoc
	// UI checks alongside the engine's scheduled probes. Failures here
	// are logged but never block the HTTP response.
	if h.Health != nil {
		errMsg := res.Error
		if err := h.Health.Append(c.Request.Context(), models.HealthResult{
			ProjectID:  p.ID,
			Timestamp:  res.CheckedAt,
			OK:         res.OK,
			StatusCode: res.StatusCode,
			LatencyMs:  int(res.LatencyMs),
			Error:      errMsg,
		}); err != nil {
			h.App.Logger.Warn().Err(err).Str("project_id", p.ID).Msg("project.health.persist_failed")
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": res})
}

// healthHistory returns the persisted probe history for a project.
// since/until are RFC3339 timestamps; only since is used as a lower
// bound at the moment (HealthRepo.History does not support an upper
// bound). limit defaults to 200 and is capped at 1000.
func (h *ProjectHandler) healthHistory(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), projectTimeout)
	defer cancel()

	id := c.Param("id")
	if _, err := h.Repo.Get(ctx, id); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "project.health_history.lookup", err)
		return
	}

	since := time.Now().UTC().Add(-24 * time.Hour)
	if v := strings.TrimSpace(c.Query("since")); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			since = t
		}
	}

	limit := 200
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	rows, err := h.Health.History(ctx, id, since, limit)
	if err != nil {
		h.serverError(c, "project.health_history", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// probeHealth performs a single GET against rawURL with a 3-redirect
// ceiling. The result populates StatusCode/LatencyMs/Error/CheckedAt
// fields; OK is set when the final response is 2xx or 3xx.
func probeHealth(ctx context.Context, rawURL string) healthResult {
	out := healthResult{CheckedAt: time.Now().UTC()}

	client := &http.Client{
		Timeout: projectHealthTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	start := time.Now()
	resp, err := client.Do(req)
	out.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		out.Error = err.Error()
		return out
	}
	defer func() { _ = resp.Body.Close() }()
	out.StatusCode = resp.StatusCode
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		out.OK = true
	}
	return out
}

// projectActionResult is the response shape for start/stop/restart.
type projectActionResult struct {
	Action string `json:"action"`
	Target string `json:"target"`
}

// restartAction maps a project to its primitive (pm2 -> docker -> tunnel)
// and restarts whichever is configured.
func (h *ProjectHandler) restartAction(c *gin.Context) {
	h.runProjectAction(c, "restart")
}

// stopAction stops the configured pm2 process or container. Tunnels are
// not stoppable here.
func (h *ProjectHandler) stopAction(c *gin.Context) {
	h.runProjectAction(c, "stop")
}

// startAction starts the configured pm2 process or container.
func (h *ProjectHandler) startAction(c *gin.Context) {
	h.runProjectAction(c, "start")
}

func (h *ProjectHandler) runProjectAction(c *gin.Context, action string) {
	lookupCtx, lookupCancel := context.WithTimeout(c.Request.Context(), projectTimeout)
	defer lookupCancel()

	id := c.Param("id")
	p, err := h.Repo.Get(lookupCtx, id)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "project.action.lookup", err)
		return
	}

	actionCtx, cancel := context.WithTimeout(c.Request.Context(), projectActionTimeout)
	defer cancel()

	pm2Name := strings.TrimSpace(p.PM2Name)
	containerName := strings.TrimSpace(p.ContainerName)
	tunnelService := strings.TrimSpace(p.TunnelService)

	switch action {
	case "restart":
		switch {
		case pm2Name != "":
			if err := h.PM2.Restart(actionCtx, pm2Name); err != nil {
				h.respondPrimitiveError(c, "pm2", err)
				return
			}
			c.JSON(http.StatusOK, gin.H{"data": projectActionResult{Action: "pm2", Target: pm2Name}})
		case containerName != "":
			if err := h.Docker.Restart(actionCtx, containerName, 0); err != nil {
				h.respondPrimitiveError(c, "docker", err)
				return
			}
			c.JSON(http.StatusOK, gin.H{"data": projectActionResult{Action: "docker", Target: containerName}})
		case tunnelService != "":
			if err := h.Tunnel.Restart(actionCtx, tunnelService); err != nil {
				h.respondPrimitiveError(c, "tunnel", err)
				return
			}
			c.JSON(http.StatusOK, gin.H{"data": projectActionResult{Action: "tunnel", Target: tunnelService}})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "no_runtime"})
		}
	case "start":
		switch {
		case pm2Name != "":
			if err := h.PM2.Start(actionCtx, pm2Name); err != nil {
				h.respondPrimitiveError(c, "pm2", err)
				return
			}
			c.JSON(http.StatusOK, gin.H{"data": projectActionResult{Action: "pm2", Target: pm2Name}})
		case containerName != "":
			if err := h.Docker.Start(actionCtx, containerName); err != nil {
				h.respondPrimitiveError(c, "docker", err)
				return
			}
			c.JSON(http.StatusOK, gin.H{"data": projectActionResult{Action: "docker", Target: containerName}})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "no_runtime"})
		}
	case "stop":
		switch {
		case pm2Name != "":
			if err := h.PM2.Stop(actionCtx, pm2Name); err != nil {
				h.respondPrimitiveError(c, "pm2", err)
				return
			}
			c.JSON(http.StatusOK, gin.H{"data": projectActionResult{Action: "pm2", Target: pm2Name}})
		case containerName != "":
			if err := h.Docker.Stop(actionCtx, containerName, 0); err != nil {
				h.respondPrimitiveError(c, "docker", err)
				return
			}
			c.JSON(http.StatusOK, gin.H{"data": projectActionResult{Action: "docker", Target: containerName}})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "no_runtime"})
		}
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("unknown_action_%s", action)})
	}
}

// respondPrimitiveError translates a pm2/docker/tunnel error into the
// standard envelope used by those underlying handlers.
func (h *ProjectHandler) respondPrimitiveError(c *gin.Context, kind string, err error) {
	switch kind {
	case "pm2":
		if errors.Is(err, pm2.ErrPM2Unavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "pm2_unavailable"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "pm2_command_failed", "detail": err.Error()})
	case "docker":
		if errors.Is(err, docker.ErrDockerUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker_unavailable"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "docker_command_failed", "detail": err.Error()})
	case "tunnel":
		if errors.Is(err, tunnel.ErrInvalidTunnelService) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_tunnel_service"})
			return
		}
		msg := err.Error()
		if strings.Contains(msg, "permission denied") || strings.Contains(msg, "Failed to restart") {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "tunnel_restart_unauthorized", "detail": msg})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tunnel_restart_failed", "detail": msg})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "unknown_primitive_error", "detail": err.Error()})
	}
}

// triggerDeployBody is the optional JSON body for POST /projects/:id/deploy.
type triggerDeployBody struct {
	Wait      bool   `json:"wait"`
	RemoteRef string `json:"remote_ref"`
}

// deployTriggerTimeout caps the synchronous portion of the manual
// deploy trigger handler. WaitFor uses its own deadline.
const deployTriggerTimeout = 10 * time.Second

// triggerDeploy is the admin manual-deploy endpoint.
func (h *ProjectHandler) triggerDeploy(c *gin.Context) {
	if h.Deploy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deploy_unavailable"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), deployTriggerTimeout)
	defer cancel()

	id := c.Param("id")
	project, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "project.deploy.lookup", err)
		return
	}
	if !project.DeployEnabled || strings.TrimSpace(project.DeployCommand) == "" {
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": "precondition_failed", "detail": "deploy_not_configured"})
		return
	}

	body := triggerDeployBody{}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
			return
		}
	}
	wait := false
	if v := strings.TrimSpace(c.Query("wait")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			wait = b
		}
	}
	if body.Wait {
		wait = true
	}

	userID, _ := middleware.CurrentUserID(c)
	if userID == "" {
		userID = "unknown"
	}
	triggeredBy := "manual:" + userID

	d, err := h.Deploy.Trigger(ctx, project, triggeredBy, strings.TrimSpace(body.RemoteRef))
	if err != nil {
		if errors.Is(err, deploy.ErrAlreadyRunning) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "already_running"})
			return
		}
		if errors.Is(err, deploy.ErrNotConfigured) {
			c.JSON(http.StatusPreconditionFailed, gin.H{"error": "precondition_failed", "detail": "deploy_not_configured"})
			return
		}
		h.serverError(c, "project.deploy.trigger", err)
		return
	}

	if wait {
		waitTimeout := time.Duration(project.DeployTimeoutSeconds+30) * time.Second
		// Use a fresh background context — the trigger context is short.
		final, werr := h.Deploy.WaitFor(c.Request.Context(), d.ID, waitTimeout)
		if werr != nil {
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": "wait_timeout", "detail": werr.Error(), "data": final})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": final})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"data": d})
}

// listDeployments returns recent deployments for a project (auth, both roles).
func (h *ProjectHandler) listDeployments(c *gin.Context) {
	if h.Deploy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deploy_unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), projectTimeout)
	defer cancel()

	id := c.Param("id")
	if _, err := h.Repo.Get(ctx, id); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "project.deployments.lookup", err)
		return
	}
	limit := 20
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	rows, err := h.Deploy.Repo.List(ctx, id, limit)
	if err != nil {
		h.serverError(c, "project.deployments.list", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// getDeployment returns a single deployment with stdout/stderr.
func (h *ProjectHandler) getDeployment(c *gin.Context) {
	if h.Deploy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "deploy_unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), projectTimeout)
	defer cancel()

	projectID := c.Param("id")
	deploymentID := c.Param("deployment_id")
	d, err := h.Deploy.Repo.Get(ctx, deploymentID)
	if err != nil {
		if errors.Is(err, deploy.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "project.deployment.get", err)
		return
	}
	if d.ProjectID != projectID {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": d})
}

// regenerateWebhookSecret writes a new 32-byte hex secret to the
// project and returns the plaintext exactly once. Subsequent GETs on
// the project will only show webhook_secret_present=true.
func (h *ProjectHandler) regenerateWebhookSecret(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), projectTimeout)
	defer cancel()

	id := c.Param("id")
	project, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "project.webhook_secret.lookup", err)
		return
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		h.serverError(c, "project.webhook_secret.rand", err)
		return
	}
	secret := hex.EncodeToString(buf)
	project.WebhookSecret = secret

	if _, err := h.Repo.Update(ctx, project); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "project.webhook_secret.update", err)
		return
	}

	if h.App.Events != nil {
		userID, _ := middleware.CurrentUserID(c)
		_, _ = h.App.Events.Append(ctx, models.Event{
			Category:  "deploy",
			Severity:  models.SeverityInfo,
			Source:    "deploy:" + project.ID,
			ProjectID: project.ID,
			Message:   fmt.Sprintf("Webhook secret regenerated for %s", project.Name),
			Data:      map[string]any{"by_user_id": userID},
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"project_id":     project.ID,
		"webhook_secret": secret,
	}})
}
