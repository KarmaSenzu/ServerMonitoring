package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
)

// Health returns a handler for GET /health.
func Health(_ *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"version":   app.Version,
		})
	}
}
