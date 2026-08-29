package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestIDHeader is the canonical header used for tracing.
const RequestIDHeader = "X-Request-ID"

// RequestIDContextKey is the gin context key under which the request id is stored.
const RequestIDContextKey = "request_id"

// RequestID assigns or propagates an X-Request-ID header for every request.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set(RequestIDContextKey, id)
		c.Writer.Header().Set(RequestIDHeader, id)
		c.Next()
	}
}

// GetRequestID returns the request id stored in the gin context, or "".
func GetRequestID(c *gin.Context) string {
	v, ok := c.Get(RequestIDContextKey)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
