package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/models"
)

const envHandlerTimeout = 8 * time.Second

// EnvHandler exposes /environments/* endpoints.
type EnvHandler struct {
	App  *app.App
	Repo *models.EnvOverrideRepo
}

// NewEnvHandler builds an EnvHandler bound to a.
func NewEnvHandler(a *app.App) *EnvHandler {
	repo := a.EnvOverrides
	if repo == nil && a.DB != nil {
		repo = models.NewEnvOverrideRepo(a.DB)
	}
	return &EnvHandler{App: a, Repo: repo}
}

// RegisterReads mounts read-only routes on the protected group.
func (h *EnvHandler) RegisterReads(rg *gin.RouterGroup) {
	rg.GET("/environments", h.list)
	rg.GET("/environments/defaults", h.defaults)
}

// RegisterWrites mounts admin-only routes.
func (h *EnvHandler) RegisterWrites(rg *gin.RouterGroup) {
	rg.PUT("/environments/:env", h.upsert)
}

func (h *EnvHandler) list(c *gin.Context) {
	if h.Repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "env_unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), envHandlerTimeout)
	defer cancel()

	rows, err := h.Repo.List(ctx)
	if err != nil {
		h.serverError(c, "env.list", err)
		return
	}

	// Ensure every canonical environment is represented in the response,
	// even if migrations were skipped or rows were deleted by hand.
	have := map[string]bool{}
	for _, r := range rows {
		have[r.Environment] = true
	}
	for _, d := range models.DefaultEnvOverrides() {
		if !have[d.Environment] {
			rows = append(rows, d)
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

func (h *EnvHandler) defaults(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": models.DefaultEnvOverrides()})
}

type envUpsertBody struct {
	Config map[string]any `json:"config"`
}

func (h *EnvHandler) upsert(c *gin.Context) {
	if h.Repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "env_unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), envHandlerTimeout)
	defer cancel()

	env := strings.TrimSpace(strings.ToLower(c.Param("env")))
	switch env {
	case models.ProjectEnvDevelopment, models.ProjectEnvStaging, models.ProjectEnvProduction:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_env"})
		return
	}

	var body envUpsertBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}
	if body.Config == nil {
		body.Config = map[string]any{}
	}
	if err := validateEnvConfig(body.Config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	res, err := h.Repo.Upsert(ctx, env, body.Config)
	if err != nil {
		h.serverError(c, "env.upsert", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": res})
}

// validateEnvConfig enforces the documented keys: healthcheck_multiplier
// is a number > 0; alert_severity_floor is one of the standard
// severities. Unknown keys are accepted (forward-compatibility) but the
// known keys are sanity-checked so misconfiguration is caught early.
func validateEnvConfig(cfg map[string]any) error {
	if v, ok := cfg["healthcheck_multiplier"]; ok {
		switch n := v.(type) {
		case float64:
			if n <= 0 || n > 100 {
				return fmt.Errorf("healthcheck_multiplier: must be > 0 and <= 100")
			}
		case int:
			if n <= 0 || n > 100 {
				return fmt.Errorf("healthcheck_multiplier: must be > 0 and <= 100")
			}
			cfg["healthcheck_multiplier"] = float64(n)
		default:
			return fmt.Errorf("healthcheck_multiplier: must be a number")
		}
	}
	if v, ok := cfg["alert_severity_floor"]; ok {
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("alert_severity_floor: must be a string")
		}
		switch s {
		case models.SeverityInfo, models.SeverityWarning, models.SeverityError, models.SeverityCritical:
		default:
			return fmt.Errorf("alert_severity_floor: invalid severity %q", s)
		}
	}
	return nil
}

func (h *EnvHandler) serverError(c *gin.Context, op string, err error) {
	h.App.Logger.Error().Err(err).Str("op", op).Msg("env_handler_error")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
}
