package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/search"
)

// searchTimeout caps a search query.
const searchTimeout = 5 * time.Second

// SearchHandler exposes the global infrastructure search endpoint.
type SearchHandler struct {
	App *app.App
	Svc *search.Service
}

func NewSearchHandler(a *app.App) *SearchHandler {
	svc := search.NewService(
		models.NewServerRepo(a.DB),
		models.NewCommandSnippetRepo(a.DB),
		models.NewTunnelRepo(a.DB),
	)
	return &SearchHandler{App: a, Svc: svc}
}

// RegisterReads mounts the search route.
func (h *SearchHandler) RegisterReads(rg *gin.RouterGroup) {
	rg.GET("/search", h.search)
}

func (h *SearchHandler) search(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), searchTimeout)
	defer cancel()

	q := c.Query("q")
	results := h.Svc.Search(ctx, q)
	c.JSON(http.StatusOK, gin.H{"data": results})
}
