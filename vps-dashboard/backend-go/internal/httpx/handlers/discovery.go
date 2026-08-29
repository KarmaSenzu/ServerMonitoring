package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/discovery"
	"vps-dashboard-api/internal/docker"
	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/pm2"
	"vps-dashboard-api/internal/tunnel"
)

const (
	discoverySnapshotTimeout = 12 * time.Second
	discoveryAdoptTimeout    = 8 * time.Second
)

// DiscoveryHandler exposes /discovery/* endpoints. Read routes are
// authenticated; write routes additionally require admin.
type DiscoveryHandler struct {
	App  *app.App
	Svc  *discovery.Service
	Repo *models.ProjectRepo
}

// NewDiscoveryHandler wires a DiscoveryHandler with the default Docker /
// PM2 / Tunnel services.
func NewDiscoveryHandler(a *app.App) *DiscoveryHandler {
	dsvc := docker.NewService(a.Logger)
	psvc := pm2.NewService(a.Logger)
	tsvc := tunnel.NewService(a.Logger)
	return &DiscoveryHandler{
		App:  a,
		Svc:  discovery.NewService(a.Logger, dsvc, tsvc, psvc),
		Repo: models.NewProjectRepo(a.DB),
	}
}

// RegisterReads mounts read-only discovery routes.
func (h *DiscoveryHandler) RegisterReads(rg *gin.RouterGroup) {
	rg.GET("/discovery/snapshot", h.snapshot)
}

// RegisterWrites mounts mutating discovery routes. Caller wraps with
// admin-role middleware.
func (h *DiscoveryHandler) RegisterWrites(rg *gin.RouterGroup) {
	rg.POST("/discovery/adopt", h.adopt)
	rg.POST("/discovery/adopt-many", h.adoptMany)
}

func (h *DiscoveryHandler) snapshot(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), discoverySnapshotTimeout)
	defer cancel()

	snap, err := h.Svc.Capture(ctx, h.Repo)
	if err != nil {
		h.serverError(c, "discovery.snapshot", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": snap})
}

// adoptRequest is the body shape of POST /discovery/adopt. Overrides
// is structured so the operator can edit the suggested project before
// committing it (e.g. rename, add tags, drop the health URL).
type adoptRequest struct {
	Candidate discovery.Candidate `json:"candidate"`
	Overrides projectDTO          `json:"overrides"`
}

func (h *DiscoveryHandler) adopt(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), discoveryAdoptTimeout)
	defer cancel()

	var req adoptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}
	if req.Candidate.AlreadyAdopted {
		c.JSON(http.StatusConflict, gin.H{"error": "already_adopted", "adopted_as": req.Candidate.AdoptedAs})
		return
	}

	created, err := h.adoptOne(ctx, req.Candidate, &req.Overrides)
	if err != nil {
		if errors.Is(err, models.ErrDuplicateName) {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate_name"})
			return
		}
		if verr, ok := err.(validationErr); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": verr.Error()})
			return
		}
		h.serverError(c, "discovery.adopt", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": created})
}

type adoptManyRequest struct {
	Candidates []discovery.Candidate `json:"candidates"`
}

type adoptManyResult struct {
	Name   string `json:"name"`
	ID     string `json:"id,omitempty"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func (h *DiscoveryHandler) adoptMany(c *gin.Context) {
	// Use a generous timeout proportional to the batch — but cap it.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	var req adoptManyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	results := make([]adoptManyResult, 0, len(req.Candidates))
	for _, cand := range req.Candidates {
		name := cand.SuggestedName
		if cand.AlreadyAdopted {
			results = append(results, adoptManyResult{
				Name:   name,
				ID:     cand.AdoptedAs,
				Status: "skipped",
				Error:  "already_adopted",
			})
			continue
		}
		created, err := h.adoptOne(ctx, cand, nil)
		if err != nil {
			results = append(results, adoptManyResult{
				Name:   name,
				Status: "error",
				Error:  err.Error(),
			})
			continue
		}
		results = append(results, adoptManyResult{
			Name:   created.Name,
			ID:     created.ID,
			Status: "created",
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": results})
}

// adoptOne applies the candidate fields (and optional overrides) to a
// new Project, validates, and inserts it. validationErr is wrapped so
// the single-shot endpoint can map it to 400.
func (h *DiscoveryHandler) adoptOne(ctx context.Context, cand discovery.Candidate, overrides *projectDTO) (models.Project, error) {
	p := models.Project{
		Name:          cand.SuggestedName,
		Domain:        cand.Domain,
		Port:          cand.Port,
		ContainerName: cand.ContainerName,
		PM2Name:       cand.PM2Name,
		TunnelService: cand.TunnelService,
		HealthURL:     cand.HealthURL,
		Enabled:       true,
		Tags:          []string{},
	}
	if overrides != nil {
		applyOverrides(&p, overrides)
	}
	if err := p.Validate(); err != nil {
		return models.Project{}, validationErr{err}
	}
	created, err := h.Repo.Create(ctx, p)
	if err != nil {
		return models.Project{}, err
	}
	return created, nil
}

// applyOverrides copies any non-zero override fields onto p. Empty
// strings/zero ints are treated as "unset" so the candidate-derived
// defaults survive.
func applyOverrides(p *models.Project, o *projectDTO) {
	if o.Name != "" {
		p.Name = o.Name
	}
	if o.Description != "" {
		p.Description = o.Description
	}
	if o.Domain != "" {
		p.Domain = o.Domain
	}
	if o.Port != 0 {
		p.Port = o.Port
	}
	if o.ContainerName != "" {
		p.ContainerName = o.ContainerName
	}
	if o.PM2Name != "" {
		p.PM2Name = o.PM2Name
	}
	if o.TunnelService != "" {
		p.TunnelService = o.TunnelService
	}
	if o.HealthURL != "" {
		p.HealthURL = o.HealthURL
	}
	if o.Enabled != nil {
		p.Enabled = *o.Enabled
	}
	if len(o.Tags) > 0 {
		p.Tags = o.Tags
	}
	if o.Notes != "" {
		p.Notes = o.Notes
	}
}

// validationErr wraps a Project.Validate() failure so adopt() can map
// it to 400 without losing the underlying detail.
type validationErr struct{ err error }

func (e validationErr) Error() string { return e.err.Error() }
func (e validationErr) Unwrap() error { return e.err }

func (h *DiscoveryHandler) serverError(c *gin.Context, op string, err error) {
	h.App.Logger.Error().Err(err).Str("op", op).Msg("discovery_handler_error")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
}
