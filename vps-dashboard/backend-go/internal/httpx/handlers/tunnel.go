package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/tunnel"
)

const (
	tunnelTimeout        = 8 * time.Second
	tunnelRestartTimeout = 15 * time.Second
)

// TunnelHandler exposes /system/tunnels (and the legacy /system/tunnel
// single-tunnel shim for the existing frontend).
type TunnelHandler struct {
	App *app.App
	Svc *tunnel.Service
}

// NewTunnelHandler builds a TunnelHandler with default discovery paths.
func NewTunnelHandler(a *app.App) *TunnelHandler {
	return &TunnelHandler{App: a, Svc: tunnel.NewService(a.Logger)}
}

// Register mounts the tunnel routes. The caller supplies auth middleware.
func (h *TunnelHandler) Register(rg *gin.RouterGroup) {
	rg.GET("/system/tunnels", h.list)
	rg.GET("/system/tunnel", h.first)
}

// RegisterWrites mounts mutating tunnel routes. Caller wraps with admin.
func (h *TunnelHandler) RegisterWrites(rg *gin.RouterGroup) {
	rg.POST("/system/tunnels/:service/restart", h.restart)
}

func (h *TunnelHandler) list(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), tunnelTimeout)
	defer cancel()

	tunnels, err := h.Svc.List(ctx)
	if err != nil {
		h.App.Logger.Error().Err(err).Msg("tunnel.list_failed")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "tunnel_list_failed",
			"detail": err.Error(),
		})
		return
	}
	if tunnels == nil {
		tunnels = []tunnel.Tunnel{}
	}
	c.JSON(http.StatusOK, gin.H{"data": tunnels})
}

func (h *TunnelHandler) first(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), tunnelTimeout)
	defer cancel()

	tunnels, err := h.Svc.List(ctx)
	if err != nil {
		h.App.Logger.Error().Err(err).Msg("tunnel.first_failed")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "tunnel_list_failed",
			"detail": err.Error(),
		})
		return
	}
	if len(tunnels) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no_tunnels", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": tunnels[0]})
}

// restart handles POST /system/tunnels/:service/restart. It validates
// the service name, calls systemctl, and maps known failure modes to
// stable error envelopes.
func (h *TunnelHandler) restart(c *gin.Context) {
	service := c.Param("service")
	ctx, cancel := context.WithTimeout(c.Request.Context(), tunnelRestartTimeout)
	defer cancel()

	if err := h.Svc.Restart(ctx, service); err != nil {
		if errors.Is(err, tunnel.ErrInvalidTunnelService) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_tunnel_service"})
			return
		}
		msg := err.Error()
		if strings.Contains(msg, "permission denied") || strings.Contains(msg, "Failed to restart") {
			h.App.Logger.Warn().Err(err).Str("service", service).Msg("tunnel_restart_unauthorized")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":  "tunnel_restart_unauthorized",
				"detail": msg,
			})
			return
		}
		h.App.Logger.Error().Err(err).Str("service", service).Msg("tunnel_restart_failed")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "tunnel_restart_failed",
			"detail": msg,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"service":   service,
		"restarted": true,
	}})
}
