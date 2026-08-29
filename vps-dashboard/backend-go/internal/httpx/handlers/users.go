package handlers

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/auth"
	"vps-dashboard-api/internal/httpx/middleware"
	"vps-dashboard-api/internal/models"
)

const userTimeout = 8 * time.Second

var usernameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.\-]{2,31}$`)

// UsersHandler exposes /users/* endpoints. Mounted under an admin-only
// router group in server.go.
type UsersHandler struct {
	App  *app.App
	Repo *models.UserRepo
}

// NewUsersHandler builds a handler with a repo bound to the app DB.
func NewUsersHandler(a *app.App) *UsersHandler {
	return &UsersHandler{App: a, Repo: models.NewUserRepo(a.DB)}
}

// RegisterWrites mounts every /users/* route on rg. Caller is expected
// to wrap rg in admin-role middleware (viewers must not see this).
func (h *UsersHandler) RegisterWrites(rg *gin.RouterGroup) {
	rg.GET("/users", h.list)
	rg.POST("/users", h.create)
	rg.PATCH("/users/:id", h.patch)
	rg.DELETE("/users/:id", h.delete)
}

func (h *UsersHandler) list(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), userTimeout)
	defer cancel()

	users, err := h.Repo.List(ctx)
	if err != nil {
		h.serverError(c, "users.list", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": users})
}

type createUserDTO struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (h *UsersHandler) create(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), userTimeout)
	defer cancel()

	var dto createUserDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}
	if !usernameRE.MatchString(dto.Username) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": "username: invalid format"})
		return
	}
	if len(dto.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": "password: minimum 8 characters"})
		return
	}
	if dto.Role != "admin" && dto.Role != "viewer" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": "role: must be admin or viewer"})
		return
	}

	hash, err := auth.Hash(dto.Password)
	if err != nil {
		h.serverError(c, "users.hash", err)
		return
	}

	created, err := h.Repo.Create(ctx, dto.Username, hash, dto.Role)
	if err != nil {
		if errors.Is(err, models.ErrUsernameTaken) {
			c.JSON(http.StatusConflict, gin.H{"error": "username_taken"})
			return
		}
		h.serverError(c, "users.create", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": created})
}

type patchUserDTO struct {
	Role     *string `json:"role"`
	Password *string `json:"password"`
}

func (h *UsersHandler) patch(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), userTimeout)
	defer cancel()

	id := c.Param("id")

	target, err := h.Repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "users.patch_lookup", err)
		return
	}

	var dto patchUserDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}
	if dto.Role == nil && dto.Password == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nothing_to_update"})
		return
	}

	// Role change validation.
	if dto.Role != nil {
		role := *dto.Role
		if role != "admin" && role != "viewer" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": "role: must be admin or viewer"})
			return
		}
		// Refuse to demote the last admin.
		if target.Role == "admin" && role != "admin" {
			n, err := h.Repo.CountByRole(ctx, "admin")
			if err != nil {
				h.serverError(c, "users.count_admins", err)
				return
			}
			if n <= 1 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "last_admin"})
				return
			}
		}
	}

	// Password validation.
	if dto.Password != nil {
		if len(*dto.Password) < 8 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": "password: minimum 8 characters"})
			return
		}
	}

	// Apply role first, then password. Either may return ErrUserNotFound
	// if the row vanished between fetch and update.
	if dto.Role != nil {
		if err := h.Repo.UpdateRole(ctx, id, *dto.Role); err != nil {
			if errors.Is(err, models.ErrUserNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
				return
			}
			h.serverError(c, "users.update_role", err)
			return
		}
	}
	if dto.Password != nil {
		hash, err := auth.Hash(*dto.Password)
		if err != nil {
			h.serverError(c, "users.hash", err)
			return
		}
		if err := h.Repo.UpdatePassword(ctx, id, hash); err != nil {
			if errors.Is(err, models.ErrUserNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
				return
			}
			h.serverError(c, "users.update_password", err)
			return
		}
	}

	updated, err := h.Repo.GetByID(ctx, id)
	if err != nil {
		h.serverError(c, "users.patch_reload", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": updated})
}

func (h *UsersHandler) delete(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), userTimeout)
	defer cancel()

	id := c.Param("id")

	if uid, ok := middleware.CurrentUserID(c); ok && uid == id {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot_delete_self"})
		return
	}

	target, err := h.Repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "users.delete_lookup", err)
		return
	}

	if target.Role == "admin" {
		n, err := h.Repo.CountByRole(ctx, "admin")
		if err != nil {
			h.serverError(c, "users.count_admins", err)
			return
		}
		if n <= 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "last_admin"})
			return
		}
	}

	if err := h.Repo.Delete(ctx, id); err != nil {
		if errors.Is(err, models.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "users.delete", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *UsersHandler) serverError(c *gin.Context, op string, err error) {
	h.App.Logger.Error().Err(err).Str("op", op).Msg("users_handler_error")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
}
