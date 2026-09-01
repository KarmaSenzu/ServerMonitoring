package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/containers"
	"vps-dashboard-api/internal/httpx/middleware"
	"vps-dashboard-api/internal/models"
)

// containerTimeout caps container operations on a single server.
const containerTimeout = 15 * time.Second

// containerFleetTimeout bounds a multi-server fleet sweep.
const containerFleetTimeout = 30 * time.Second

// ContainerHandler exposes /servers/:id/containers/* and /containers
// (fleet overview) endpoints.
type ContainerHandler struct {
	App    *app.App
	Repo   *models.ServerRepo
	Svc    *containers.Service
}

// NewContainerHandler constructs a ContainerHandler.
func NewContainerHandler(a *app.App) *ContainerHandler {
	svc := containers.NewService(a.SSHService)
	return &ContainerHandler{
		App:  a,
		Repo: models.NewServerRepo(a.DB),
		Svc:  svc,
	}
}

// RegisterReads mounts read-only container routes.
func (h *ContainerHandler) RegisterReads(rg *gin.RouterGroup) {
	rg.GET("/servers/:id/containers", h.listByServer)
	rg.GET("/servers/:id/containers/:name/logs", h.logsByServer)

	// Fleet overview — all servers in one call (bounded concurrency).
	rg.GET("/containers", h.fleet)
}

// RegisterWrites mounts mutating container routes (admin-only).
func (h *ContainerHandler) RegisterWrites(rg *gin.RouterGroup) {
	rg.POST("/servers/:id/containers/:name/start", h.startByServer)
	rg.POST("/servers/:id/containers/:name/stop", h.stopByServer)
	rg.POST("/servers/:id/containers/:name/restart", h.restartByServer)
}

// serverResult is a single server's container listing in the fleet
// response.
type serverResult struct {
	ServerID   string                `json:"server_id"`
	ServerName string                `json:"server_name"`
	Status     string                `json:"status"`
	Engine     string                `json:"engine,omitempty"`
	Containers []containers.Container `json:"containers"`
	Error      string                `json:"error,omitempty"`
}

// fleet gathers container lists from every enabled server using a
// bounded worker pool (§46). Failures on individual servers are
// reported per-server, never aborting the whole sweep (§15).
func (h *ContainerHandler) fleet(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), containerFleetTimeout)
	defer cancel()

	servers, err := h.Repo.List(ctx, models.ServerFilter{EnabledOnly: true})
	if err != nil {
		h.containerError(c, "containers.fleet.list", err)
		return
	}

	// Bounded concurrency: at most 4 parallel SSH probes.
	sem := make(chan struct{}, 4)
	results := make([]serverResult, len(servers))

	for i, srv := range servers {
		sem <- struct{}{}
		go func(idx int, s models.Server) {
			defer func() { <-sem }()

			engine, containers, err := h.Svc.ListByServer(ctx, s)
			res := serverResult{
				ServerID:   s.ID,
				ServerName: s.Name,
				Status:     s.Status,
			}
			if err != nil {
				res.Error = err.Error()
			} else {
				res.Engine = engine
				res.Containers = containers
			}
			results[idx] = res
		}(i, srv)
	}

	// Drain the semaphore so all goroutines finish before returning.
	for i := 0; i < len(servers); i++ {
		<-sem
	}

	c.JSON(http.StatusOK, gin.H{"data": results})
}

// listByServer returns containers for a single registered server.
func (h *ContainerHandler) listByServer(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), containerTimeout)
	defer cancel()

	srv, err := h.lookupServer(c, ctx)
	if err != nil {
		return
	}

	engine, containers, err := h.Svc.ListByServer(ctx, srv)
	if err != nil {
		h.respondContainerError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"server_id":   srv.ID,
			"server_name": srv.Name,
			"engine":      engine,
			"containers":  containers,
		},
	})
}

// logsByServer returns recent container logs from a registered server.
func (h *ContainerHandler) logsByServer(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), containerTimeout)
	defer cancel()

	srv, err := h.lookupServer(c, ctx)
	if err != nil {
		return
	}

	name := c.Param("name")
	tail := 200
	if v := c.Query("tail"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			tail = n
		}
	}

	stdout, stderr, err := h.Svc.LogsByServer(ctx, srv, name, tail)
	if err != nil {
		h.respondContainerError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"stdout": stdout,
			"stderr": stderr,
		},
	})
}

// startByServer starts a container on a registered server.
func (h *ContainerHandler) startByServer(c *gin.Context) {
	h.mutateByServer(c, "start")
}

// stopByServer stops a container on a registered server.
func (h *ContainerHandler) stopByServer(c *gin.Context) {
	h.mutateByServer(c, "stop")
}

// restartByServer restarts a container on a registered server.
func (h *ContainerHandler) restartByServer(c *gin.Context) {
	h.mutateByServer(c, "restart")
}

func (h *ContainerHandler) mutateByServer(c *gin.Context, action string) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), containerTimeout)
	defer cancel()

	srv, err := h.lookupServer(c, ctx)
	if err != nil {
		return
	}

	name := c.Param("name")
	timeoutSec := 10
	if v := c.Query("timeout_seconds"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeoutSec = n
		}
	}

	var mutateErr error
	switch action {
	case "start":
		mutateErr = h.Svc.StartByServer(ctx, srv, name)
	case "stop":
		mutateErr = h.Svc.StopByServer(ctx, srv, name, timeoutSec)
	case "restart":
		mutateErr = h.Svc.RestartByServer(ctx, srv, name, timeoutSec)
	}

	if mutateErr != nil {
		h.respondContainerError(c, mutateErr)
		return
	}

	// Audit the container action.
	userID, _ := middleware.CurrentUserID(c)
	h.appendContainerEvent(c, "container_"+action, srv, name, userID)

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"action":  action,
		"server":  srv.Name,
		"target":  name,
	}})
}

// lookupServer resolves the :id parameter and writes the HTTP error
// response on failure.
func (h *ContainerHandler) lookupServer(c *gin.Context, ctx context.Context) (models.Server, error) {
	id := c.Param("id")
	s, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrServerNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return models.Server{}, err
		}
		h.containerError(c, "containers.lookup", err)
		return models.Server{}, err
	}
	return s, nil
}

// respondContainerError maps engine errors to HTTP responses.
func (h *ContainerHandler) respondContainerError(c *gin.Context, err error) {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "host unreachable"):
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": "ssh_host_unreachable", "detail": msg})
	case strings.Contains(msg, "authentication failed"):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ssh_auth_failed", "detail": msg})
	case strings.Contains(msg, "host key changed"):
		c.JSON(http.StatusConflict, gin.H{"error": "ssh_host_key_changed", "detail": msg})
	case errors.Is(err, containers.ErrNoContainerRuntime):
		c.JSON(http.StatusNotFound, gin.H{"error": "no_container_runtime", "detail": "Neither docker nor podman is installed on this server"})
	default:
		c.JSON(http.StatusBadGateway, gin.H{"error": "container_error", "detail": msg})
	}
}

// appendContainerEvent records an audit event for a container action.
func (h *ContainerHandler) appendContainerEvent(c *gin.Context, action string, srv models.Server, target, userID string) {
	if h.App.Events == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = h.App.Events.Append(ctx, models.Event{
		Category: models.EventCategoryDocker,
		Severity: models.SeverityInfo,
		Source:   "containers:" + srv.Name,
		Message:  fmt.Sprintf("%s %s on %s", action, target, srv.Name),
		Data: map[string]any{
			"action":      action,
			"server_id":   srv.ID,
			"server_name": srv.Name,
			"target":      target,
			"by_user_id":  userID,
		},
	})
}

func (h *ContainerHandler) containerError(c *gin.Context, op string, err error) {
	h.App.Logger.Error().Err(err).Str("op", op).Msg("container_handler_error")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
}
