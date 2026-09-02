package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/httpx/middleware"
	"vps-dashboard-api/internal/models"
)

// serverTimeout caps every server CRUD request.
const serverTimeout = 8 * time.Second

// ServerHandler exposes /servers/* endpoints. Read routes are mounted
// under a group that already requires auth; write routes additionally
// require RequireRole("admin") at the caller.
type ServerHandler struct {
	App     *app.App
	Repo    *models.ServerRepo
	Metrics *models.ServerMetricRepo
	Discoveries *models.DiscoveryRepo
}

// NewServerHandler constructs a ServerHandler with a repo bound to
// the app's DB connection.
func NewServerHandler(a *app.App) *ServerHandler {
	metricsRepo := a.ServerMetrics
	if metricsRepo == nil {
		metricsRepo = models.NewServerMetricRepo(a.DB)
	}
	return &ServerHandler{
		App:         a,
		Repo:        models.NewServerRepo(a.DB),
		Metrics:     metricsRepo,
		Discoveries: models.NewDiscoveryRepo(a.DB),
	}
}

// RegisterReads mounts the read-only server routes.
func (h *ServerHandler) RegisterReads(rg *gin.RouterGroup) {
	rg.GET("/servers", h.list)
	rg.GET("/servers/:id", h.get)
	rg.GET("/servers/tags", h.listTags)
	rg.GET("/servers/:id/metrics", h.latestMetric)
	rg.GET("/servers/:id/history", h.metricHistory)
	rg.GET("/servers/:id/discovery", h.getDiscovery)
}

// RegisterWrites mounts the mutating server routes. Caller is
// responsible for adding admin-role middleware.
func (h *ServerHandler) RegisterWrites(rg *gin.RouterGroup) {
	rg.POST("/servers", h.create)
	rg.PUT("/servers/:id", h.put)
	rg.PATCH("/servers/:id", h.patch)
	rg.DELETE("/servers/:id", h.delete)
}

// serverDTO is the JSON shape accepted on POST/PUT.
type serverDTO struct {
	Name               string   `json:"name"`
	Hostname           string   `json:"hostname"`
	IPAddress          string   `json:"ip_address"`
	SSHPort            int      `json:"ssh_port"`
	SSHUsername        string   `json:"ssh_username"`
	CredentialType     string   `json:"credential_type"`
	CredentialRef      string   `json:"credential_ref"`
	CredentialPassword string   `json:"credential_password"`
	OperatingSystem    string   `json:"operating_system"`
	Architecture       string   `json:"architecture"`
	Provider           string   `json:"provider"`
	ProviderInstanceID string   `json:"provider_instance_id"`
	Environment        string   `json:"environment"`
	Notes              string   `json:"notes"`
	Enabled            *bool    `json:"enabled"`
	Tags               []string `json:"tags"`
}

func (d serverDTO) toServer() models.Server {
	enabled := true
	if d.Enabled != nil {
		enabled = *d.Enabled
	}
	tags := d.Tags
	if tags == nil {
		tags = []string{}
	}
	return models.Server{
		Name:               d.Name,
		Hostname:           d.Hostname,
		IPAddress:          d.IPAddress,
		SSHPort:            d.SSHPort,
		SSHUsername:        d.SSHUsername,
		CredentialType:     d.CredentialType,
		CredentialRef:      d.CredentialRef,
		CredentialPassword: d.CredentialPassword,
		OperatingSystem:    d.OperatingSystem,
		Architecture:       d.Architecture,
		Provider:           d.Provider,
		ProviderInstanceID: d.ProviderInstanceID,
		Environment:        d.Environment,
		Notes:              d.Notes,
		Enabled:            enabled,
		Tags:               tags,
	}
}

// serverPatchDTO uses pointers so the handler can distinguish "field
// not present" from "field set to empty".
type serverPatchDTO struct {
	Name               *string   `json:"name"`
	Hostname           *string   `json:"hostname"`
	IPAddress          *string   `json:"ip_address"`
	SSHPort            *int      `json:"ssh_port"`
	SSHUsername        *string   `json:"ssh_username"`
	CredentialType     *string   `json:"credential_type"`
	CredentialRef      *string   `json:"credential_ref"`
	OperatingSystem    *string   `json:"operating_system"`
	Architecture       *string   `json:"architecture"`
	Provider           *string   `json:"provider"`
	ProviderInstanceID *string   `json:"provider_instance_id"`
	Environment        *string   `json:"environment"`
	Notes              *string   `json:"notes"`
	Enabled            *bool     `json:"enabled"`
	Tags               *[]string `json:"tags"`
}

func (d serverPatchDTO) apply(s *models.Server) {
	if d.Name != nil {
		s.Name = *d.Name
	}
	if d.Hostname != nil {
		s.Hostname = *d.Hostname
	}
	if d.IPAddress != nil {
		s.IPAddress = *d.IPAddress
	}
	if d.SSHPort != nil {
		s.SSHPort = *d.SSHPort
	}
	if d.SSHUsername != nil {
		s.SSHUsername = *d.SSHUsername
	}
	if d.CredentialType != nil {
		s.CredentialType = *d.CredentialType
	}
	if d.CredentialRef != nil {
		s.CredentialRef = *d.CredentialRef
	}
	if d.OperatingSystem != nil {
		s.OperatingSystem = *d.OperatingSystem
	}
	if d.Architecture != nil {
		s.Architecture = *d.Architecture
	}
	if d.Provider != nil {
		s.Provider = *d.Provider
	}
	if d.ProviderInstanceID != nil {
		s.ProviderInstanceID = *d.ProviderInstanceID
	}
	if d.Environment != nil {
		s.Environment = *d.Environment
	}
	if d.Notes != nil {
		s.Notes = *d.Notes
	}
	if d.Enabled != nil {
		s.Enabled = *d.Enabled
	}
	if d.Tags != nil {
		s.Tags = *d.Tags
	}
}

// serverResponse is the safe API shape: credential_ref is surfaced as
// metadata only (name/fingerprint semantics will arrive with the
// Phase 2 credential store). No secret material ever leaves the
// backend (PROJECT ARCHITECTURE.md §11).
type serverResponse struct {
	models.Server
	CredentialPresent bool `json:"credential_present"`
}

func sanitizeServer(s models.Server) serverResponse {
	// Never return the password in the API response
	s.CredentialPassword = ""
	return serverResponse{
		Server:            s,
		CredentialPresent: strings.TrimSpace(s.CredentialRef) != "" || s.CredentialPassword != "",
	}
}

func sanitizeServers(in []models.Server) []serverResponse {
	out := make([]serverResponse, 0, len(in))
	for _, s := range in {
		out = append(out, sanitizeServer(s))
	}
	return out
}

func (h *ServerHandler) list(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), serverTimeout)
	defer cancel()

	filter := models.ServerFilter{
		Search:      strings.TrimSpace(c.Query("q")),
		Environment: strings.TrimSpace(c.Query("environment")),
		Tag:         strings.TrimSpace(c.Query("tag")),
		Status:      strings.TrimSpace(c.Query("status")),
		Provider:    strings.TrimSpace(c.Query("provider")),
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
		h.serverError(c, "server.list", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sanitizeServers(rows)})
}

func (h *ServerHandler) get(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), serverTimeout)
	defer cancel()

	id := c.Param("id")
	s, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrServerNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "server.get", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sanitizeServer(s)})
}

// listTags returns the tag catalogue so the UI can offer tag filters
// and reuse existing tag names.
func (h *ServerHandler) listTags(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), serverTimeout)
	defer cancel()

	tags, err := h.Repo.ListTags(ctx)
	if err != nil {
		h.serverError(c, "server.list_tags", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": tags})
}

func (h *ServerHandler) create(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), serverTimeout)
	defer cancel()

	var dto serverDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	s := dto.toServer()
	if err := s.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	created, err := h.Repo.Create(ctx, s)
	if err != nil {
		if errors.Is(err, models.ErrServerDuplicateName) {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate_name"})
			return
		}
		h.serverError(c, "server.create", err)
		return
	}

	h.auditServerChange(c, "server_create", created.ID, created.Name)
	c.JSON(http.StatusCreated, gin.H{"data": sanitizeServer(created)})
}

func (h *ServerHandler) put(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), serverTimeout)
	defer cancel()

	id := c.Param("id")

	existing, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrServerNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "server.put_lookup", err)
		return
	}

	var dto serverDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	// PUT replaces the editable fields; status/last_seen are owned by
	// the monitoring pipeline and survive the replace.
	updated := dto.toServer()
	updated.ID = existing.ID
	updated.Status = existing.Status
	updated.StatusDetail = existing.StatusDetail
	updated.LastSeenAt = existing.LastSeenAt

	if err := updated.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	res, err := h.Repo.Update(ctx, updated)
	if err != nil {
		if errors.Is(err, models.ErrServerDuplicateName) {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate_name"})
			return
		}
		if errors.Is(err, models.ErrServerNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "server.put", err)
		return
	}

	h.auditServerChange(c, "server_update", res.ID, res.Name)
	c.JSON(http.StatusOK, gin.H{"data": sanitizeServer(res)})
}

func (h *ServerHandler) patch(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), serverTimeout)
	defer cancel()

	id := c.Param("id")

	existing, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrServerNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "server.patch_lookup", err)
		return
	}

	var dto serverPatchDTO
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
		if errors.Is(err, models.ErrServerDuplicateName) {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate_name"})
			return
		}
		if errors.Is(err, models.ErrServerNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "server.patch", err)
		return
	}

	h.auditServerChange(c, "server_update", res.ID, res.Name)
	c.JSON(http.StatusOK, gin.H{"data": sanitizeServer(res)})
}

func (h *ServerHandler) delete(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), serverTimeout)
	defer cancel()

	id := c.Param("id")
	// Look up first so the audit event can carry the server name.
	s, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrServerNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "server.delete_lookup", err)
		return
	}
	if err := h.Repo.Delete(ctx, id); err != nil {
		if errors.Is(err, models.ErrServerNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "server.delete", err)
		return
	}

	h.auditServerChange(c, "server_delete", s.ID, s.Name)
	c.Status(http.StatusNoContent)
}

// auditServerChange appends an infrastructure event for registry
// mutations. Failures are logged but never block the HTTP response.
func (h *ServerHandler) auditServerChange(c *gin.Context, action, serverID, serverName string) {
	if h.App.Events == nil {
		return
	}
	userID, _ := middleware.CurrentUserID(c)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = h.App.Events.Append(ctx, models.Event{
		Category: models.EventCategorySystem,
		Severity: models.SeverityInfo,
		Source:   "server-registry",
		Message:  fmt.Sprintf("%s %s (registry)", action, serverName),
		Data: map[string]any{
			"action":      action,
			"server_id":   serverID,
			"server_name": serverName,
			"by_user_id":  userID,
		},
	})
}

func (h *ServerHandler) serverError(c *gin.Context, op string, err error) {
	h.App.Logger.Error().Err(err).Str("op", op).Msg("server_handler_error")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
}

// latestMetric returns the most recent metric sample for a server.
func (h *ServerHandler) latestMetric(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), serverTimeout)
	defer cancel()

	id := c.Param("id")
	if _, err := h.Repo.Get(ctx, id); err != nil {
		if errors.Is(err, models.ErrServerNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "server.metrics.lookup", err)
		return
	}

	metric, err := h.Metrics.Latest(ctx, id)
	if err != nil {
		// No metrics yet is not an error from the caller's perspective.
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusOK, gin.H{"data": nil})
			return
		}
		h.serverError(c, "server.metrics.latest", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": metric})
}

// metricHistory returns the recent metric samples for a server, ordered
// oldest-to-newest for charting. limit defaults to 100, capped at 1000.
func (h *ServerHandler) metricHistory(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), serverTimeout)
	defer cancel()

	id := c.Param("id")
	if _, err := h.Repo.Get(ctx, id); err != nil {
		if errors.Is(err, models.ErrServerNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "server.history.lookup", err)
		return
	}

	limit := 100
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	rows, err := h.Metrics.History(ctx, id, limit)
	if err != nil {
		h.serverError(c, "server.history", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// getDiscovery returns auto-discovered services (PM2, Docker, tunnels,
// systemd, ports) for a server. This data is collected automatically
// via SSH during monitoring — no manual input needed.
func (h *ServerHandler) getDiscovery(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), serverTimeout)
	defer cancel()

	id := c.Param("id")
	if _, err := h.Repo.Get(ctx, id); err != nil {
		if errors.Is(err, models.ErrServerNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "server.discovery.lookup", err)
		return
	}

	discovery, err := h.Discoveries.Get(ctx, id)
	if err != nil {
		h.serverError(c, "server.discovery", err)
		return
	}

	if discovery == nil {
		c.JSON(http.StatusOK, gin.H{
			"data": nil,
			"message": "No discovery data yet. Data will appear after the first successful SSH connection.",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": discovery})
}
