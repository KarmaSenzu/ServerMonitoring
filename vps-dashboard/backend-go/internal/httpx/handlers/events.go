package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/models"
)

// eventsTimeout caps every /events request.
const eventsTimeout = 8 * time.Second

// EventsHandler exposes the read-only /events listing.
type EventsHandler struct {
	App  *app.App
	Repo *models.EventRepo
}

// NewEventsHandler builds a handler with a repo bound to the app DB.
// When a.Events is set the existing repo is reused; otherwise a fresh
// one is constructed from a.DB so tests that bypass main wiring still
// work.
func NewEventsHandler(a *app.App) *EventsHandler {
	repo := a.Events
	if repo == nil {
		repo = models.NewEventRepo(a.DB)
	}
	return &EventsHandler{App: a, Repo: repo}
}

// RegisterReads mounts GET /events on the given router group. The
// caller is expected to require authentication; both admin and viewer
// roles can read events.
func (h *EventsHandler) RegisterReads(rg *gin.RouterGroup) {
	rg.GET("/events", h.list)
}

func (h *EventsHandler) list(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), eventsTimeout)
	defer cancel()

	filter := models.EventFilter{
		Category:  strings.TrimSpace(c.Query("category")),
		Severity:  strings.TrimSpace(c.Query("severity")),
		ProjectID: strings.TrimSpace(c.Query("project_id")),
		Search:    strings.TrimSpace(c.Query("q")),
	}

	if v := strings.TrimSpace(c.Query("since")); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.Since = t
		}
	}
	if v := strings.TrimSpace(c.Query("until")); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.Until = t
		}
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
	filter.Limit = limit

	offset := 0
	if v := strings.TrimSpace(c.Query("offset")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	filter.Offset = offset

	rows, err := h.Repo.List(ctx, filter)
	if err != nil {
		h.serverError(c, "events.list", err)
		return
	}
	total, err := h.Repo.Count(ctx, filter)
	if err != nil {
		h.serverError(c, "events.count", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": rows, "total": total})
}

func (h *EventsHandler) serverError(c *gin.Context, op string, err error) {
	h.App.Logger.Error().Err(err).Str("op", op).Msg("events_handler_error")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
}
