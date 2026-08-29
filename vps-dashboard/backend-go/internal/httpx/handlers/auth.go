package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/auth"
	"vps-dashboard-api/internal/httpx/middleware"
	"vps-dashboard-api/internal/models"
)

// dummyHash is a precomputed bcrypt hash used to keep login latency
// roughly constant when the requested user does not exist. The plaintext
// it corresponds to is irrelevant; only the work factor matters.
// Cost 12, plaintext "vpsd-dummy-password".
const dummyHash = "$2a$12$wQYFSZ6q9b5cV6pqHk6aOu1c0Eko0Bz6nhA5KYa1Y3wq8K8gXF/Yi"

// AuthHandler bundles the auth-related HTTP handlers.
type AuthHandler struct {
	App  *app.App
	repo *models.UserRepo
}

// NewAuthHandler constructs an AuthHandler.
func NewAuthHandler(a *app.App) *AuthHandler {
	return &AuthHandler{App: a, repo: models.NewUserRepo(a.DB)}
}

// Register mounts public auth routes under rg.
// /auth/me and /auth/refresh must be mounted under a protected group via
// RegisterProtected.
func (h *AuthHandler) Register(rg *gin.RouterGroup) {
	rg.POST("/auth/login", h.login)
	rg.POST("/auth/logout", h.logout)
}

// RegisterProtected mounts auth routes that require RequireAuth.
func (h *AuthHandler) RegisterProtected(rg *gin.RouterGroup) {
	rg.GET("/auth/me", h.me)
	rg.POST("/auth/refresh", h.refresh)
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type userPayload struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func (h *AuthHandler) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_body",
			"details": err.Error(),
		})
		return
	}

	username := strings.TrimSpace(req.Username)

	user, hashPtr, err := h.repo.GetByUsername(c.Request.Context(), username)
	var hash string
	if err != nil {
		if !errors.Is(err, models.ErrUserNotFound) {
			h.App.Logger.Error().Err(err).Str("username", username).Msg("auth.login.lookup_error")
			// Still run a verify on the dummy hash to keep timing similar,
			// then surface a 500 without leaking the underlying cause.
			_ = auth.Verify(dummyHash, req.Password)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
			return
		}
		// User not found: run verify against dummy hash to mitigate
		// user-enumeration via timing.
		hash = dummyHash
	} else if hashPtr != nil {
		hash = *hashPtr
	} else {
		hash = dummyHash
	}

	ok := auth.Verify(hash, req.Password)
	if !ok || user == nil {
		h.App.Logger.Warn().
			Str("username", username).
			Str("client_ip", c.ClientIP()).
			Msg("auth.login.fail")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
		return
	}

	token, err := auth.Issue(user.ID, user.Username, user.Role,
		[]byte(h.App.Cfg.JWTSecret), h.App.Cfg.JWTTTL)
	if err != nil {
		h.App.Logger.Error().Err(err).Str("username", username).Msg("auth.login.issue_error")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	h.setAuthCookie(c, token)

	h.App.Logger.Info().
		Str("user_id", user.ID).
		Str("username", user.Username).
		Str("role", user.Role).
		Str("client_ip", c.ClientIP()).
		Msg("auth.login.ok")

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  toPayload(user),
	})
}

func (h *AuthHandler) logout(c *gin.Context) {
	h.clearAuthCookie(c)
	c.Status(http.StatusNoContent)
}

func (h *AuthHandler) me(c *gin.Context) {
	uid, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	user, err := h.repo.GetByID(c.Request.Context(), uid)
	if err != nil {
		if errors.Is(err, models.ErrUserNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user_not_found"})
			return
		}
		h.App.Logger.Error().Err(err).Str("user_id", uid).Msg("auth.me.lookup_error")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": toPayload(user)})
}

func (h *AuthHandler) refresh(c *gin.Context) {
	uid, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	user, err := h.repo.GetByID(c.Request.Context(), uid)
	if err != nil {
		if errors.Is(err, models.ErrUserNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user_not_found"})
			return
		}
		h.App.Logger.Error().Err(err).Str("user_id", uid).Msg("auth.refresh.lookup_error")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	token, err := auth.Issue(user.ID, user.Username, user.Role,
		[]byte(h.App.Cfg.JWTSecret), h.App.Cfg.JWTTTL)
	if err != nil {
		h.App.Logger.Error().Err(err).Str("user_id", uid).Msg("auth.refresh.issue_error")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	h.setAuthCookie(c, token)

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  toPayload(user),
	})
}

func (h *AuthHandler) setAuthCookie(c *gin.Context, token string) {
	maxAge := int(h.App.Cfg.JWTTTL.Seconds())
	if maxAge <= 0 {
		maxAge = 60
	}
	secure := h.App.Cfg.IsProduction()
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(middleware.CookieName, token, maxAge, "/", "", secure, true)
}

func (h *AuthHandler) clearAuthCookie(c *gin.Context) {
	secure := h.App.Cfg.IsProduction()
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(middleware.CookieName, "", -1, "/", "", secure, true)
}

func toPayload(u *models.User) userPayload {
	return userPayload{ID: u.ID, Username: u.Username, Role: u.Role}
}
