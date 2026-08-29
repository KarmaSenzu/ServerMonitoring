package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/sysinfo"
)

// systemTimeout caps every system metrics request.
const systemTimeout = 8 * time.Second

// SystemHandler exposes /system/* endpoints backed by sysinfo.
type SystemHandler struct {
	App *app.App
}

// NewSystemHandler constructs a SystemHandler.
func NewSystemHandler(a *app.App) *SystemHandler {
	return &SystemHandler{App: a}
}

// Register mounts /system/* on rg. The caller is responsible for applying
// authentication middleware to rg.
func (h *SystemHandler) Register(rg *gin.RouterGroup) {
	rg.GET("/system/stats", h.stats)
	rg.GET("/system/cpu", h.cpu)
	rg.GET("/system/memory", h.memory)
	rg.GET("/system/disk", h.disk)
	rg.GET("/system/host", h.host)
	rg.GET("/system/network", h.network)
	rg.GET("/system/history", h.history)
}

func (h *SystemHandler) stats(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), systemTimeout)
	defer cancel()

	snap, err := sysinfo.Capture(ctx)
	if err != nil {
		h.systemError(c, "snapshot", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": snap})
}

func (h *SystemHandler) cpu(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), systemTimeout)
	defer cancel()

	v, err := sysinfo.CPU(ctx)
	if err != nil {
		h.systemError(c, "cpu", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": v})
}

func (h *SystemHandler) memory(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), systemTimeout)
	defer cancel()

	v, err := sysinfo.Memory(ctx)
	if err != nil {
		h.systemError(c, "memory", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": v})
}

func (h *SystemHandler) disk(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), systemTimeout)
	defer cancel()

	v, err := sysinfo.Disk(ctx, c.DefaultQuery("path", "/"))
	if err != nil {
		h.systemError(c, "disk", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": v})
}

func (h *SystemHandler) host(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), systemTimeout)
	defer cancel()

	v, err := sysinfo.Host(ctx)
	if err != nil {
		h.systemError(c, "host", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": v})
}

func (h *SystemHandler) network(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), systemTimeout)
	defer cancel()

	v, err := sysinfo.Network(ctx)
	if err != nil {
		h.systemError(c, "network", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": v})
}

func (h *SystemHandler) systemError(c *gin.Context, kind string, err error) {
	h.App.Logger.Error().
		Err(err).
		Str("kind", kind).
		Msg("system_stats_failed")
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":  "system_stats_failed",
		"detail": err.Error(),
	})
}

const (
	historyMinWindow = 1 * time.Minute
	historyMaxWindow = 12 * time.Hour
	historyDefault   = 1 * time.Hour
)

// history handles GET /system/history?window=1h. Window is parsed via
// time.ParseDuration and clamped to [1m, 12h]. If no samples have been
// recorded yet, returns 503.
func (h *SystemHandler) history(c *gin.Context) {
	window := historyDefault
	if v := c.Query("window"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_window"})
			return
		}
		if d < historyMinWindow || d > historyMaxWindow {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":  "invalid_window",
				"detail": "must be between 1m and 12h",
			})
			return
		}
		window = d
	}

	if h.App.Recorder == nil || !h.App.Recorder.HasSamples() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "history_unavailable"})
		return
	}

	snap := h.App.Recorder.Snapshot(window)
	c.JSON(http.StatusOK, gin.H{"data": snap})
}
