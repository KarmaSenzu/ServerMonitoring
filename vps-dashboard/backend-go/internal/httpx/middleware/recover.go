package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// Recover catches panics from downstream handlers, logs a stack trace, and
// responds with a 500 carrying the request id.
func Recover(base zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				rid := GetRequestID(c)
				base.Error().
					Interface("panic", r).
					Str("request_id", rid).
					Bytes("stack", debug.Stack()).
					Msg("http_panic")

				if !c.Writer.Written() {
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
						"error":      "internal server error",
						"request_id": rid,
					})
				} else {
					c.Abort()
				}
			}
		}()
		c.Next()
	}
}
