package middleware

import (
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// Recovery creates a recovery middleware that recovers from panics
func Recovery(logger zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Get trace_id if available
				traceID, _ := c.Get("trace_id")
				traceIDStr, _ := traceID.(string)

				// Get stack trace
				stack := debug.Stack()

				// Log panic
				logger.Error().
					Str("trace_id", traceIDStr).
					Str("method", c.Request.Method).
					Str("path", c.Request.URL.Path).
					Str("client_ip", c.ClientIP()).
					Interface("error", err).
					Str("stack", string(stack)).
					Msg("Panic recovered")

				// Return error response
				c.JSON(http.StatusInternalServerError, gin.H{
					"status_code": http.StatusInternalServerError,
					"is_success":  false,
					"error": gin.H{
						"timestamp":     time.Now(),
						"path":          c.Request.URL.Path,
						"error_message": "Internal server error",
					},
				})

				c.Abort()
			}
		}()

		c.Next()
	}
}


