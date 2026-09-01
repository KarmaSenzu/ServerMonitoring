package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/commands"
	"vps-dashboard-api/internal/httpx/middleware"
	"vps-dashboard-api/internal/models"
)

// commandTimeout caps each SSH command execution.
const commandTimeout = 30 * time.Second

// CommandHandler exposes snippet CRUD, blast-radius preview, and
// multi-host execution endpoints.
type CommandHandler struct {
	App    *app.App
	Snippets *models.CommandSnippetRepo
	Runs    *models.CommandRunRepo
	Svc     *commands.Service
	Servers *models.ServerRepo
}

func NewCommandHandler(a *app.App) *CommandHandler {
	runsRepo := models.NewCommandRunRepo(a.DB)
	return &CommandHandler{
		App:     a,
		Snippets: models.NewCommandSnippetRepo(a.DB),
		Runs:    runsRepo,
		Svc:     commands.NewService(a.Logger, a.SSHService, runsRepo),
		Servers: models.NewServerRepo(a.DB),
	}
}

// RegisterReads mounts read-only routes.
func (h *CommandHandler) RegisterReads(rg *gin.RouterGroup) {
	rg.GET("/commands/snippets", h.listSnippets)
	rg.GET("/commands/snippets/:id", h.getSnippet)
	rg.GET("/commands/history", h.history)
}

// RegisterWrites mounts admin-only routes (snippet CRUD).
// Execute/preview are operator-level — see RegisterOperatorWrites.
func (h *CommandHandler) RegisterWrites(rg *gin.RouterGroup) {
	rg.POST("/commands/snippets", h.createSnippet)
	rg.PUT("/commands/snippets/:id", h.updateSnippet)
	rg.DELETE("/commands/snippets/:id", h.deleteSnippet)
}

// RegisterOperatorWrites mounts command execution routes that
// operators can access (execute + preview). Snippet CRUD stays
// admin-only (managing command definitions = ADMIN per §32).
func (h *CommandHandler) RegisterOperatorWrites(rg *gin.RouterGroup) {
	rg.POST("/commands/preview", h.preview)
	rg.POST("/commands/execute", h.execute)
}

// --- Snippet CRUD ---

type snippetDTO struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	Command     string   `json:"command" binding:"required"`
	Variables   []string `json:"variables"`
	DangerLevel string   `json:"danger_level"`
}

func (d snippetDTO) toModel(createdBy string) models.CommandSnippet {
	return models.CommandSnippet{
		Name:        d.Name,
		Description: d.Description,
		Command:     d.Command,
		Variables:   d.Variables,
		DangerLevel: d.DangerLevel,
		CreatedBy:   createdBy,
		UpdatedBy:   createdBy,
	}
}

func (h *CommandHandler) listSnippets(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	snippets, err := h.Snippets.List(ctx)
	if err != nil {
		h.cmdError(c, "snippets.list", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": snippets})
}

func (h *CommandHandler) getSnippet(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	s, err := h.Snippets.Get(ctx, c.Param("id"))
	if err != nil {
		if errors.Is(err, models.ErrSnippetNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.cmdError(c, "snippets.get", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": s})
}

func (h *CommandHandler) createSnippet(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	var dto snippetDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}
	userID, _ := middleware.CurrentUserID(c)
	s, err := h.Snippets.Create(ctx, dto.toModel(userID))
	if err != nil {
		if errors.Is(err, models.ErrSnippetDuplicateName) {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate_name"})
			return
		}
		h.cmdError(c, "snippets.create", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": s})
}

func (h *CommandHandler) updateSnippet(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	id := c.Param("id")
	existing, err := h.Snippets.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrSnippetNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.cmdError(c, "snippets.update_lookup", err)
		return
	}
	var dto snippetDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}
	userID, _ := middleware.CurrentUserID(c)
	existing.Name = dto.Name
	existing.Description = dto.Description
	existing.Command = dto.Command
	existing.Variables = dto.Variables
	existing.DangerLevel = dto.DangerLevel
	existing.UpdatedBy = userID
	updated, err := h.Snippets.Update(ctx, existing)
	if err != nil {
		if errors.Is(err, models.ErrSnippetDuplicateName) {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate_name"})
			return
		}
		h.cmdError(c, "snippets.update", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": updated})
}

func (h *CommandHandler) deleteSnippet(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	if err := h.Snippets.Delete(ctx, c.Param("id")); err != nil {
		if errors.Is(err, models.ErrSnippetNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.cmdError(c, "snippets.delete", err)
		return
	}
	c.Status(http.StatusNoContent)
}

// --- Preview & Execute ---

type executeBody struct {
	Command   string   `json:"command" binding:"required"`
	ServerIDs []string `json:"server_ids" binding:"required"`
	SnippetID string   `json:"snippet_id"`
}

func (h *CommandHandler) preview(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	var body executeBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}
	servers, err := h.resolveServers(ctx, body.ServerIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_servers", "detail": err.Error()})
		return
	}
	preview := h.Svc.Preview(body.Command, servers)
	c.JSON(http.StatusOK, gin.H{"data": preview})
}

func (h *CommandHandler) execute(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), commandTimeout)
	defer cancel()
	var body executeBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}
	servers, err := h.resolveServers(ctx, body.ServerIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_servers", "detail": err.Error()})
		return
	}
	if len(servers) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no_targets"})
		return
	}
	if len(body.Command) > 8192 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command_too_long"})
		return
	}

	userID, _ := middleware.CurrentUserID(c)

	// Audit the multi-host execution.
	h.appendCommandEvent(c, "command_execute", body.Command, servers, userID)

	result := h.Svc.Execute(ctx, body.Command, servers, userID, body.SnippetID)
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// --- History ---

func (h *CommandHandler) history(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	serverID := c.Query("server_id")
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := parseIntDefault(v, 50); err == nil && n > 0 {
			limit = n
		}
	}
	runs, err := h.Runs.History(ctx, serverID, limit)
	if err != nil {
		h.cmdError(c, "runs.history", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": runs})
}

// --- Helpers ---

func (h *CommandHandler) resolveServers(ctx context.Context, ids []string) ([]models.Server, error) {
	servers := make([]models.Server, 0, len(ids))
	for _, id := range ids {
		s, err := h.Servers.Get(ctx, id)
		if err != nil {
			if errors.Is(err, models.ErrServerNotFound) {
				return nil, fmt.Errorf("server %s not found", id)
			}
			return nil, err
		}
		servers = append(servers, s)
	}
	return servers, nil
}

func (h *CommandHandler) appendCommandEvent(c *gin.Context, action, command string, servers []models.Server, userID string) {
	if h.App.Events == nil {
		return
	}
	names := make([]string, 0, len(servers))
	for _, s := range servers {
		names = append(names, s.Name)
	}
	evCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = h.App.Events.Append(evCtx, models.Event{
		Category: models.EventCategorySystem,
		Severity: models.SeverityInfo,
		Source:   "commands",
		Message:  fmt.Sprintf("%s on %d host(s): %s", action, len(servers), truncateCmd(command, 120)),
		Data: map[string]any{
			"action":       action,
			"command":      command,
			"server_names":  names,
			"target_count": len(servers),
			"by_user_id":   userID,
		},
	})
}

func (h *CommandHandler) cmdError(c *gin.Context, op string, err error) {
	h.App.Logger.Error().Err(err).Str("op", op).Msg("command_handler_error")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
}

func truncateCmd(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func parseIntDefault(s string, def int) (int, error) {
	if s == "" {
		return def, nil
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return def, fmt.Errorf("not a number")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

var _ = strings.TrimSpace
