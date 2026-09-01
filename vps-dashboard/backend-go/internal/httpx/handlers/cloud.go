package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/cloud"
	"vps-dashboard-api/internal/httpx/middleware"
	"vps-dashboard-api/internal/models"
)

// cloudTimeout caps discovery operations.
const cloudTimeout = 15 * time.Second

// CloudHandler exposes /cloud/* endpoints for discovery and import.
type CloudHandler struct {
	App     *app.App
	Repo    *models.ServerRepo
	Reg     *cloud.Registry
}

func NewCloudHandler(a *app.App) *CloudHandler {
	reg := cloud.NewRegistry()
	// Register the manual provider by default. Real cloud providers
	// (AWS, GCP, ...) are plugged in when configured.
	reg.Register(cloud.NewManualProvider(nil))

	return &CloudHandler{
		App:  a,
		Repo: models.NewServerRepo(a.DB),
		Reg:  reg,
	}
}

// RegisterReads mounts read-only cloud routes.
func (h *CloudHandler) RegisterReads(rg *gin.RouterGroup) {
	rg.GET("/cloud/providers", h.listProviders)
	rg.GET("/cloud/instances", h.listInstances)
	rg.GET("/cloud/instances/:provider/:id", h.getInstance)
}

// RegisterWrites mounts mutating cloud routes (admin-only).
func (h *CloudHandler) RegisterWrites(rg *gin.RouterGroup) {
	rg.POST("/cloud/import", h.importInstance)
}

// listProviders returns the names of all registered cloud providers.
func (h *CloudHandler) listProviders(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": h.Reg.Names()})
}

// listInstances discovers instances across all providers.
func (h *CloudHandler) listInstances(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), cloudTimeout)
	defer cancel()

	results, errs := h.Reg.ListAll(ctx)
	c.JSON(http.StatusOK, gin.H{
		"data":    results,
		"errors":  errs,
	})
}

// getInstance returns a single instance from a specific provider.
func (h *CloudHandler) getInstance(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), cloudTimeout)
	defer cancel()

	providerName := c.Param("provider")
	p := h.Reg.Get(providerName)
	if p == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider_not_found"})
		return
	}

	inst, err := p.GetInstance(ctx, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance_not_found", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": inst})
}

// importInstance creates a Server Registry entry from a discovered
// cloud instance. The imported server starts with status "unknown"
// (§24: discovery does not auto-grant management authorization).
func (h *CloudHandler) importInstance(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), cloudTimeout)
	defer cancel()

	var body cloud.ImportCandidate
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	// Validate provider exists and instance is real.
	p := h.Reg.Get(body.Provider)
	if p == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider_not_found"})
		return
	}
	inst, err := p.GetInstance(ctx, body.InstanceID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "instance_not_found", "detail": err.Error()})
		return
	}

	// Build server from candidate + discovered data.
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = inst.Name
	}
	hostname := strings.TrimSpace(body.Hostname)
	if hostname == "" {
		hostname = inst.PublicIP
		if hostname == "" {
			hostname = inst.PrivateIP
		}
	}
	if hostname == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no_hostname", "detail": "Instance has no public or private IP"})
		return
	}
	sshPort := body.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}
	sshUser := strings.TrimSpace(body.SSHUsername)
	if sshUser == "" {
		sshUser = "root"
	}
	env := strings.TrimSpace(body.Environment)
	if env == "" {
		env = models.ServerEnvProduction
	}

	server := models.Server{
		Name:           name,
		Hostname:       hostname,
		IPAddress:      inst.PrivateIP,
		SSHPort:        sshPort,
		SSHUsername:    sshUser,
		CredentialType: models.ServerCredentialSSHKey,
		Provider:       body.Provider,
		ProviderInstanceID: inst.ID,
		Environment:    env,
		Tags:           body.Tags,
		Enabled:        true,
		Status:         models.ServerStatusUnknown,
	}

	if err := server.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	created, err := h.Repo.Create(ctx, server)
	if err != nil {
		if errors.Is(err, models.ErrServerDuplicateName) {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate_name"})
			return
		}
		h.cloudError(c, "cloud.import", err)
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	h.auditCloud(c, "cloud_import", created, inst, body.Provider, userID)

	c.JSON(http.StatusCreated, gin.H{"data": created})
}

func (h *CloudHandler) auditCloud(c *gin.Context, action string, server models.Server, inst *cloud.Instance, providerName, userID string) {
	if h.App.Events == nil {
		return
	}
	evCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = h.App.Events.Append(evCtx, models.Event{
		Category: models.EventCategorySystem,
		Severity: models.SeverityInfo,
		Source:   "cloud:" + providerName,
		Message:  "Imported " + inst.Name + " from " + providerName + " as " + server.Name,
		Data: map[string]any{
			"action":        action,
			"server_id":     server.ID,
			"server_name":   server.Name,
			"provider":      providerName,
			"instance_id":   inst.ID,
			"instance_name": inst.Name,
			"by_user_id":    userID,
		},
	})
}

func (h *CloudHandler) cloudError(c *gin.Context, op string, err error) {
	h.App.Logger.Error().Err(err).Str("op", op).Msg("cloud_handler_error")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
}
