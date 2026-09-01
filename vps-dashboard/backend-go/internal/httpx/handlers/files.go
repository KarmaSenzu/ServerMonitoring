package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/files"
	"vps-dashboard-api/internal/httpx/middleware"
	"vps-dashboard-api/internal/models"
)

// fileTimeout caps SFTP operations.
const fileTimeout = 15 * time.Second

// FileHandler exposes /servers/:id/files/* endpoints for remote file
// management over SFTP (Phase 7).
type FileHandler struct {
	App    *app.App
	Repo   *models.ServerRepo
	Svc    *files.Service
}

func NewFileHandler(a *app.App) *FileHandler {
	return &FileHandler{
		App:  a,
		Repo: models.NewServerRepo(a.DB),
		Svc:  files.NewService(a.SSHService),
	}
}

// RegisterReads mounts read-only file routes.
func (h *FileHandler) RegisterReads(rg *gin.RouterGroup) {
	rg.GET("/servers/:id/files", h.browse)
	rg.GET("/servers/:id/files/*path", h.getOrDownload)
}

// RegisterWrites mounts mutating file routes (admin-only).
func (h *FileHandler) RegisterWrites(rg *gin.RouterGroup) {
	rg.POST("/servers/:id/files/mkdir", h.mkdir)
	rg.POST("/servers/:id/files/upload", h.upload)
	rg.POST("/servers/:id/files/rename", h.rename)
	rg.DELETE("/servers/:id/files/*path", h.remove)
}

// browse lists directory contents.
func (h *FileHandler) browse(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), fileTimeout)
	defer cancel()

	srv, err := h.lookupServer(c, ctx)
	if err != nil {
		return
	}

	dir := c.DefaultQuery("path", "/")
	client, err := h.App.SSHService.DialClient(ctx, srv)
	if err != nil {
		h.respondFileError(c, err)
		return
	}
	defer func() { _ = client.Close() }()

	entries, err := h.Svc.Browse(ctx, client, dir)
	if err != nil {
		h.respondFileError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"path":    files.SafePath(dir),
			"entries": entries,
		},
	})
}

// getOrDownload handles GET /servers/:id/files/*path:
//   ?action=download → stream file content
//   ?action=stat     → metadata
//   default          → file content as JSON (for small text files)
func (h *FileHandler) getOrDownload(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), fileTimeout)
	defer cancel()

	srv, err := h.lookupServer(c, ctx)
	if err != nil {
		return
	}

	p := c.Param("path")
	if p == "" {
		p = "/"
	}
	action := c.DefaultQuery("action", "stat")

	client, err := h.App.SSHService.DialClient(ctx, srv)
	if err != nil {
		h.respondFileError(c, err)
		return
	}
	defer func() { _ = client.Close() }()

	switch action {
	case "download":
		reader, size, err := h.Svc.Download(ctx, client, p)
		if err != nil {
			h.respondFileError(c, err)
			return
		}
		defer func() { _ = reader.Close() }()

		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", path.Base(p)))
		c.Header("Content-Type", "application/octet-stream")
		if size > 0 {
			c.Header("Content-Length", strconv.FormatInt(size, 10))
		}
		c.Status(http.StatusOK)
		_, _ = io.Copy(c.Writer, reader)

	case "stat":
		meta, err := h.Svc.Stat(ctx, client, p)
		if err != nil {
			h.respondFileError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": meta})

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_action", "detail": "action must be download or stat"})
	}
}

// mkdirBody is the payload for creating a directory.
type mkdirBody struct {
	Path string `json:"path" binding:"required"`
}

func (h *FileHandler) mkdir(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), fileTimeout)
	defer cancel()

	srv, err := h.lookupServer(c, ctx)
	if err != nil {
		return
	}

	var body mkdirBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	client, err := h.App.SSHService.DialClient(ctx, srv)
	if err != nil {
		h.respondFileError(c, err)
		return
	}
	defer func() { _ = client.Close() }()

	if err := h.Svc.Mkdir(ctx, client, body.Path); err != nil {
		h.respondFileError(c, err)
		return
	}

	h.auditFile(c, "file_mkdir", srv, body.Path)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"path": files.SafePath(body.Path)}})
}

// upload streams a file from the request body to the remote server.
func (h *FileHandler) upload(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()

	srv, err := h.lookupServer(c, ctx)
	if err != nil {
		return
	}

	p := c.Query("path")
	if p == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path_required"})
		return
	}

	client, err := h.App.SSHService.DialClient(ctx, srv)
	if err != nil {
		h.respondFileError(c, err)
		return
	}
	defer func() { _ = client.Close() }()

	n, err := h.Svc.Upload(ctx, client, p, c.Request.Body)
	if err != nil {
		h.respondFileError(c, err)
		return
	}

	h.auditFile(c, "file_upload", srv, p)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"path": files.SafePath(p), "bytes": n}})
}

// renameBody is the payload for renaming a file/directory.
type renameBody struct {
	OldPath string `json:"old_path" binding:"required"`
	NewPath string `json:"new_path" binding:"required"`
}

func (h *FileHandler) rename(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), fileTimeout)
	defer cancel()

	srv, err := h.lookupServer(c, ctx)
	if err != nil {
		return
	}

	var body renameBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	client, err := h.App.SSHService.DialClient(ctx, srv)
	if err != nil {
		h.respondFileError(c, err)
		return
	}
	defer func() { _ = client.Close() }()

	if err := h.Svc.Rename(ctx, client, body.OldPath, body.NewPath); err != nil {
		h.respondFileError(c, err)
		return
	}

	h.auditFile(c, "file_rename", srv, body.OldPath+" → "+body.NewPath)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"old_path": files.SafePath(body.OldPath), "new_path": files.SafePath(body.NewPath)}})
}

// remove deletes a file or directory.
func (h *FileHandler) remove(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), fileTimeout)
	defer cancel()

	srv, err := h.lookupServer(c, ctx)
	if err != nil {
		return
	}

	p := c.Param("path")
	if p == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path_required"})
		return
	}

	client, err := h.App.SSHService.DialClient(ctx, srv)
	if err != nil {
		h.respondFileError(c, err)
		return
	}
	defer func() { _ = client.Close() }()

	if err := h.Svc.Remove(ctx, client, p); err != nil {
		h.respondFileError(c, err)
		return
	}

	h.auditFile(c, "file_delete", srv, p)
	c.Status(http.StatusNoContent)
}

// lookupServer resolves the :id parameter.
func (h *FileHandler) lookupServer(c *gin.Context, ctx context.Context) (models.Server, error) {
	id := c.Param("id")
	s, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrServerNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return models.Server{}, err
		}
		h.fileError(c, "files.lookup", err)
		return models.Server{}, err
	}
	return s, nil
}

func (h *FileHandler) respondFileError(c *gin.Context, err error) {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "host unreachable"):
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": "ssh_host_unreachable", "detail": msg})
	case strings.Contains(msg, "authentication failed"):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ssh_auth_failed", "detail": msg})
	case strings.Contains(msg, "host key changed"):
		c.JSON(http.StatusConflict, gin.H{"error": "ssh_host_key_changed", "detail": msg})
	case strings.Contains(msg, "not exist"):
		c.JSON(http.StatusNotFound, gin.H{"error": "file_not_found", "detail": msg})
	default:
		c.JSON(http.StatusBadGateway, gin.H{"error": "file_error", "detail": msg})
	}
}

func (h *FileHandler) auditFile(c *gin.Context, action string, srv models.Server, target string) {
	if h.App.Events == nil {
		return
	}
	userID, _ := middleware.CurrentUserID(c)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = h.App.Events.Append(ctx, models.Event{
		Category: models.EventCategorySystem,
		Severity: models.SeverityInfo,
		Source:   "files:" + srv.Name,
		Message:  fmt.Sprintf("%s %s on %s", action, target, srv.Name),
		Data: map[string]any{
			"action":      action,
			"server_id":   srv.ID,
			"server_name": srv.Name,
			"target":      target,
			"by_user_id":  userID,
		},
	})
}

func (h *FileHandler) fileError(c *gin.Context, op string, err error) {
	h.App.Logger.Error().Err(err).Str("op", op).Msg("file_handler_error")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
}
