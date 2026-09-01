package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/httpx/middleware"
	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/ssh"
)

// sshTunnelTimeout caps SSH tunnel CRUD operations.
const sshTunnelTimeout = 8 * time.Second

// sshTunnelConnectTimeout caps the initial SSH dial for a tunnel.
const sshTunnelConnectTimeout = 15 * time.Second

// SshTunnelHandler exposes /tunnels/* and /servers/:id/tunnels endpoints
// for SSH port forwarding (Phase 8).
type SshTunnelHandler struct {
	App     *app.App
	Repo    *models.TunnelRepo
	Servers *models.ServerRepo
	TM      *ssh.TunnelManager
}

// NewSshTunnelHandler constructs an SshTunnelHandler.
func NewSshTunnelHandler(a *app.App) *SshTunnelHandler {
	return &SshTunnelHandler{
		App:     a,
		Repo:    models.NewTunnelRepo(a.DB),
		Servers: models.NewServerRepo(a.DB),
		TM:      a.TunnelManager,
	}
}

// RegisterReads mounts read-only tunnel routes.
func (h *SshTunnelHandler) RegisterReads(rg *gin.RouterGroup) {
	rg.GET("/tunnels", h.tunnelList)
	rg.GET("/tunnels/:id", h.tunnelGet)
	rg.GET("/servers/:id/tunnels", h.tunnelListByServer)
}

// RegisterWrites mounts mutating tunnel routes (admin-only).
func (h *SshTunnelHandler) RegisterWrites(rg *gin.RouterGroup) {
	rg.POST("/tunnels", h.tunnelCreate)
	rg.PUT("/tunnels/:id", h.tunnelUpdate)
	rg.DELETE("/tunnels/:id", h.tunnelDelete)
	rg.POST("/tunnels/:id/connect", h.tunnelConnect)
	rg.POST("/tunnels/:id/disconnect", h.tunnelDisconnect)
}

// --- DTOs ---

type sshTunnelDTO struct {
	Name       string `json:"name" binding:"required"`
	ServerID   string `json:"server_id" binding:"required"`
	Type       string `json:"type" binding:"required"`
	LocalAddr  string `json:"local_addr"`
	RemoteAddr string `json:"remote_addr"`
	AutoStart  bool   `json:"auto_start"`
}

func (d sshTunnelDTO) toModel() models.Tunnel {
	return models.Tunnel{
		Name:       d.Name,
		ServerID:   d.ServerID,
		Type:       d.Type,
		LocalAddr:  d.LocalAddr,
		RemoteAddr: d.RemoteAddr,
		AutoStart:  d.AutoStart,
		Status:     models.TunnelStopped,
	}
}

// --- CRUD ---

func (h *SshTunnelHandler) tunnelList(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), sshTunnelTimeout)
	defer cancel()
	tunnels, err := h.Repo.List(ctx)
	if err != nil {
		h.sshTunnelError(c, "tunnels.list", err)
		return
	}
	for i := range tunnels {
		if h.TM != nil {
			if lt := h.TM.Get(tunnels[i].ID); lt != nil {
				tunnels[i].Status = lt.Status()
				if lt.Error() != "" {
					tunnels[i].Error = lt.Error()
				}
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": tunnels})
}

func (h *SshTunnelHandler) tunnelGet(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), sshTunnelTimeout)
	defer cancel()
	t, err := h.Repo.Get(ctx, c.Param("id"))
	if err != nil {
		if errors.Is(err, models.ErrTunnelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.sshTunnelError(c, "tunnels.get", err)
		return
	}
	if h.TM != nil {
		if lt := h.TM.Get(t.ID); lt != nil {
			t.Status = lt.Status()
			if lt.Error() != "" {
				t.Error = lt.Error()
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": t})
}

func (h *SshTunnelHandler) tunnelListByServer(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), sshTunnelTimeout)
	defer cancel()
	tunnels, err := h.Repo.ListByServer(ctx, c.Param("id"))
	if err != nil {
		h.sshTunnelError(c, "tunnels.list_by_server", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": tunnels})
}

func (h *SshTunnelHandler) tunnelCreate(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), sshTunnelTimeout)
	defer cancel()
	var dto sshTunnelDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}
	switch dto.Type {
	case models.TunnelTypeLocal, models.TunnelTypeRemote, models.TunnelTypeSocks:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_type"})
		return
	}
	if dto.Type != models.TunnelTypeSocks && dto.RemoteAddr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "remote_addr_required"})
		return
	}
	if dto.LocalAddr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "local_addr_required"})
		return
	}

	t, err := h.Repo.Create(ctx, dto.toModel())
	if err != nil {
		if errors.Is(err, models.ErrTunnelDuplicateName) {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate_name"})
			return
		}
		h.sshTunnelError(c, "tunnels.create", err)
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	h.auditSshTunnel(c, "tunnel_create", t, userID)
	c.JSON(http.StatusCreated, gin.H{"data": t})
}

func (h *SshTunnelHandler) tunnelUpdate(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), sshTunnelTimeout)
	defer cancel()
	id := c.Param("id")
	existing, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrTunnelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.sshTunnelError(c, "tunnels.update_lookup", err)
		return
	}
	var dto sshTunnelDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}
	existing.Name = dto.Name
	existing.ServerID = dto.ServerID
	existing.Type = dto.Type
	existing.LocalAddr = dto.LocalAddr
	existing.RemoteAddr = dto.RemoteAddr
	existing.AutoStart = dto.AutoStart

	updated, err := h.Repo.Update(ctx, existing)
	if err != nil {
		if errors.Is(err, models.ErrTunnelDuplicateName) {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate_name"})
			return
		}
		h.sshTunnelError(c, "tunnels.update", err)
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	h.auditSshTunnel(c, "tunnel_update", updated, userID)
	c.JSON(http.StatusOK, gin.H{"data": updated})
}

func (h *SshTunnelHandler) tunnelDelete(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), sshTunnelTimeout)
	defer cancel()
	id := c.Param("id")
	t, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrTunnelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.sshTunnelError(c, "tunnels.delete_lookup", err)
		return
	}
	if h.TM != nil {
		_ = h.TM.Disconnect(id)
	}
	if err := h.Repo.Delete(ctx, id); err != nil {
		h.sshTunnelError(c, "tunnels.delete", err)
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	h.auditSshTunnel(c, "tunnel_delete", t, userID)
	c.Status(http.StatusNoContent)
}

// --- Connect/Disconnect ---

func (h *SshTunnelHandler) tunnelConnect(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), sshTunnelConnectTimeout)
	defer cancel()
	id := c.Param("id")
	t, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrTunnelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.sshTunnelError(c, "tunnels.connect_lookup", err)
		return
	}

	server, err := h.Servers.Get(ctx, t.ServerID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "server_not_found"})
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	_ = h.Repo.SetStatus(ctx, id, models.TunnelConnecting, userID, "", time.Now().UTC())

	if h.TM == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "tunnel_manager_unavailable"})
		return
	}

	lt, err := h.TM.ConnectByServer(ctx, id, server, t.Type, t.LocalAddr, t.RemoteAddr)
	if err != nil {
		_ = h.Repo.SetStatus(ctx, id, models.TunnelError, userID, err.Error(), time.Time{})
		c.JSON(http.StatusBadGateway, gin.H{"error": "tunnel_connect_failed", "detail": err.Error()})
		return
	}

	_ = h.Repo.SetStatus(ctx, id, models.TunnelActive, userID, "", time.Now().UTC())

	h.auditSshTunnel(c, "tunnel_connect", t, userID)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"id":         id,
		"name":       t.Name,
		"status":     lt.Status(),
		"type":       t.Type,
		"local_addr": t.LocalAddr,
	}})
}

func (h *SshTunnelHandler) tunnelDisconnect(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), sshTunnelTimeout)
	defer cancel()
	id := c.Param("id")
	t, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrTunnelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.sshTunnelError(c, "tunnels.disconnect_lookup", err)
		return
	}

	if h.TM != nil {
		_ = h.TM.Disconnect(id)
	}

	userID, _ := middleware.CurrentUserID(c)
	_ = h.Repo.SetStatus(ctx, id, models.TunnelStopped, userID, "", time.Time{})

	h.auditSshTunnel(c, "tunnel_disconnect", t, userID)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": id, "status": "stopped"}})
}

// --- Helpers ---

func (h *SshTunnelHandler) auditSshTunnel(c *gin.Context, action string, t models.Tunnel, userID string) {
	if h.App.Events == nil {
		return
	}
	evCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = h.App.Events.Append(evCtx, models.Event{
		Category: models.EventCategoryTunnel,
		Severity: models.SeverityInfo,
		Source:   "ssh-tunnels:" + t.Name,
		Message:  fmt.Sprintf("%s %s (%s)", action, t.Name, t.Type),
		Data: map[string]any{
			"action":      action,
			"tunnel_id":   t.ID,
			"tunnel_name": t.Name,
			"tunnel_type": t.Type,
			"by_user_id":  userID,
		},
	})
}

func (h *SshTunnelHandler) sshTunnelError(c *gin.Context, op string, err error) {
	h.App.Logger.Error().Err(err).Str("op", op).Msg("ssh_tunnel_handler_error")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
}
