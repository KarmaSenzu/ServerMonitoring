package handlers

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/backup"
	"vps-dashboard-api/internal/httpx/middleware"
)

const backupTimeout = 8 * time.Second

// BackupsHandler exposes /backups/* endpoints.
type BackupsHandler struct {
	App     *app.App
	Service *backup.Service
	Repo    *backup.Repo
}

// NewBackupsHandler builds a BackupsHandler bound to a.
func NewBackupsHandler(a *app.App) *BackupsHandler {
	repo := a.Backups
	if repo == nil && a.DB != nil {
		repo = backup.NewRepo(a.DB)
	}
	return &BackupsHandler{
		App:     a,
		Service: a.BackupService,
		Repo:    repo,
	}
}

// RegisterReads mounts the read endpoints on the protected group.
func (h *BackupsHandler) RegisterReads(rg *gin.RouterGroup) {
	rg.GET("/backups", h.list)
	rg.GET("/backups/:id", h.get)
}

// RegisterWrites mounts admin-only endpoints. Caller wraps with admin
// role middleware.
func (h *BackupsHandler) RegisterWrites(rg *gin.RouterGroup) {
	rg.POST("/backups/run", h.runNow)
	rg.GET("/backups/:id/download", h.download)
	rg.DELETE("/backups/:id", h.delete)
}

func (h *BackupsHandler) list(c *gin.Context) {
	if h.Repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backup_unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), backupTimeout)
	defer cancel()

	limit := 50
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	rows, err := h.Repo.List(ctx, limit)
	if err != nil {
		h.serverError(c, "backups.list", err)
		return
	}
	// Strip on-disk paths for backups stored outside the configured dir
	// so curious viewers cannot guess unexpected locations.
	if h.Service != nil {
		for i := range rows {
			if !backup.PathInside(h.Service.Dir, rows[i].Path) {
				rows[i].Path = ""
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

func (h *BackupsHandler) get(c *gin.Context) {
	if h.Repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backup_unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), backupTimeout)
	defer cancel()

	b, err := h.Repo.Get(ctx, c.Param("id"))
	if err != nil {
		if errors.Is(err, backup.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "backups.get", err)
		return
	}
	if h.Service != nil && !backup.PathInside(h.Service.Dir, b.Path) {
		b.Path = ""
	}
	c.JSON(http.StatusOK, gin.H{"data": b})
}

func (h *BackupsHandler) runNow(c *gin.Context) {
	if h.Service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backup_unavailable"})
		return
	}

	// Run the backup synchronously with a generous timeout. VACUUM INTO
	// blocks while it copies, so the timeout depends on DB size.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()

	userID, _ := middleware.CurrentUserID(c)
	if userID == "" {
		userID = "unknown"
	}
	b, err := h.Service.RunOnce(ctx, "manual:"+userID)
	if err != nil {
		h.serverError(c, "backups.run", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": b})
}

func (h *BackupsHandler) download(c *gin.Context) {
	if h.Service == nil || h.Repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backup_unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), backupTimeout)
	defer cancel()

	b, err := h.Repo.Get(ctx, c.Param("id"))
	if err != nil {
		if errors.Is(err, backup.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "backups.download.lookup", err)
		return
	}
	if !b.OK {
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": "backup_failed"})
		return
	}
	if !backup.PathInside(h.Service.Dir, b.Path) {
		c.JSON(http.StatusForbidden, gin.H{"error": "path_outside_backup_dir"})
		return
	}
	if _, err := os.Stat(b.Path); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file_missing", "detail": err.Error()})
		return
	}
	filename := filepath.Base(b.Path)
	c.Writer.Header().Set("Content-Type", "application/octet-stream")
	c.Writer.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.File(b.Path)
}

func (h *BackupsHandler) delete(c *gin.Context) {
	if h.Service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backup_unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), backupTimeout)
	defer cancel()

	if err := h.Service.Delete(ctx, c.Param("id")); err != nil {
		switch {
		case errors.Is(err, backup.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		case errors.Is(err, backup.ErrLastBackup):
			c.JSON(http.StatusConflict, gin.H{"error": "last_backup", "detail": err.Error()})
		default:
			h.serverError(c, "backups.delete", err)
		}
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *BackupsHandler) serverError(c *gin.Context, op string, err error) {
	h.App.Logger.Error().Err(err).Str("op", op).Msg("backups_handler_error")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
}
