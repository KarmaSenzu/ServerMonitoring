package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/pm2"
)

// pm2ListTimeout caps a pm2 jlist invocation.
const pm2ListTimeout = 8 * time.Second

// pm2MutateTimeout caps pm2 start/stop/restart/reload/delete. PM2 itself
// can take a moment to spawn its daemon and then act on a process.
const pm2MutateTimeout = 30 * time.Second

// pm2LogsTimeout caps `pm2 logs --nostream --lines N`.
const pm2LogsTimeout = 8 * time.Second

// PM2Handler exposes /pm2/* endpoints.
type PM2Handler struct {
	App *app.App
	Svc *pm2.Service
}

// NewPM2Handler constructs a PM2Handler with a service that uses the
// app's logger.
func NewPM2Handler(a *app.App) *PM2Handler {
	return &PM2Handler{App: a, Svc: pm2.NewService(a.Logger)}
}

// RegisterReads mounts read-only routes. Caller supplies auth middleware.
func (h *PM2Handler) RegisterReads(rg *gin.RouterGroup) {
	rg.GET("/pm2/processes", h.list)
	rg.GET("/pm2/processes/:name/logs", h.logs)
}

// RegisterWrites mounts mutating routes. Caller supplies auth + admin
// middleware.
func (h *PM2Handler) RegisterWrites(rg *gin.RouterGroup) {
	rg.POST("/pm2/processes/:name/start", h.start)
	rg.POST("/pm2/processes/:name/stop", h.stop)
	rg.POST("/pm2/processes/:name/restart", h.restart)
	rg.POST("/pm2/processes/:name/reload", h.reload)
	rg.DELETE("/pm2/processes/:name", h.delete)
}

func (h *PM2Handler) list(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), pm2ListTimeout)
	defer cancel()

	procs, err := h.Svc.List(ctx)
	if err != nil {
		h.respondPM2Error(c, "pm2.list", err, "")
		return
	}
	if procs == nil {
		procs = []pm2.Process{}
	}
	c.JSON(http.StatusOK, gin.H{"data": procs})
}

func (h *PM2Handler) logs(c *gin.Context) {
	name := c.Param("name")
	if err := pm2.ValidateProcessName(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_process_name"})
		return
	}

	lines := 200
	if v := c.Query("lines"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_lines"})
			return
		}
		if n > 2000 {
			n = 2000
		}
		lines = n
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), pm2LogsTimeout)
	defer cancel()

	stdout, _, err := h.Svc.Logs(ctx, name, lines)
	if err != nil {
		h.respondPM2Error(c, "pm2.logs", err, name)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"name":   name,
		"lines":  lines,
		"stdout": stdout,
	}})
}

func (h *PM2Handler) start(c *gin.Context)   { h.runAction(c, "start") }
func (h *PM2Handler) stop(c *gin.Context)    { h.runAction(c, "stop") }
func (h *PM2Handler) restart(c *gin.Context) { h.runAction(c, "restart") }
func (h *PM2Handler) reload(c *gin.Context)  { h.runAction(c, "reload") }
func (h *PM2Handler) delete(c *gin.Context)  { h.runAction(c, "delete") }

func (h *PM2Handler) runAction(c *gin.Context, action string) {
	name := c.Param("name")
	if err := pm2.ValidateProcessName(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_process_name"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), pm2MutateTimeout)
	defer cancel()

	var err error
	switch action {
	case "start":
		err = h.Svc.Start(ctx, name)
	case "stop":
		err = h.Svc.Stop(ctx, name)
	case "restart":
		err = h.Svc.Restart(ctx, name)
	case "reload":
		err = h.Svc.Reload(ctx, name)
	case "delete":
		err = h.Svc.Delete(ctx, name)
	default:
		// Unreachable; static call sites only.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "pm2_unknown_action"})
		return
	}
	if err != nil {
		h.respondPM2Error(c, "pm2."+action, err, name)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *PM2Handler) respondPM2Error(c *gin.Context, op string, err error, name string) {
	if errors.Is(err, pm2.ErrPM2Unavailable) {
		h.App.Logger.Warn().Str("op", op).Str("name", name).Msg("pm2_unavailable")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "pm2_unavailable"})
		return
	}
	h.App.Logger.Error().Err(err).Str("op", op).Str("name", name).Msg("pm2_command_failed")
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":  "pm2_command_failed",
		"detail": err.Error(),
	})
}
