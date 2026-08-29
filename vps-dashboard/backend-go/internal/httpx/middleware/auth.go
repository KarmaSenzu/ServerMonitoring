package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/auth"
)

// CookieName is the name of the cookie used to carry the JWT.
const CookieName = "vpsd_token"

// Context keys for values populated by RequireAuth.
const (
	ctxKeyUserID   = "user_id"
	ctxKeyUsername = "username"
	ctxKeyRole     = "role"
)

// RequireAuth returns a middleware that validates a JWT supplied via the
// Authorization header (Bearer scheme) or, as a fallback, the vpsd_token
// cookie. On success, the user id, username, and role from the token are
// stored on the gin context.
func RequireAuth(secret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c)
		if token == "" {
			if cookie, err := c.Cookie(CookieName); err == nil {
				token = cookie
			}
		}
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		claims, err := auth.Parse(token, secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
			return
		}

		c.Set(ctxKeyUserID, claims.Subject)
		c.Set(ctxKeyUsername, claims.Username)
		c.Set(ctxKeyRole, claims.Role)
		c.Next()
	}
}

// RequireRole returns a middleware that ensures the authenticated user's role
// is in the allowed set. Must be mounted after RequireAuth.
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(c *gin.Context) {
		role, ok := CurrentRole(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		if _, ok := allowed[role]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}

// CurrentUserID returns the authenticated user's id, if present.
func CurrentUserID(c *gin.Context) (string, bool) {
	return ctxString(c, ctxKeyUserID)
}

// CurrentUsername returns the authenticated user's username, if present.
func CurrentUsername(c *gin.Context) (string, bool) {
	return ctxString(c, ctxKeyUsername)
}

// CurrentRole returns the authenticated user's role, if present.
func CurrentRole(c *gin.Context) (string, bool) {
	return ctxString(c, ctxKeyRole)
}

func ctxString(c *gin.Context, key string) (string, bool) {
	v, ok := c.Get(key)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

func bearerToken(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if h == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}
