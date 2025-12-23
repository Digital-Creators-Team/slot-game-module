package middleware

import (
	"net/http"
	"runtime/debug"
	"time"

	"git.futuregamestudio.net/be-shared/slot-game-module.git/types"
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
				errorResp := types.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					IsSuccess:  false,
					Error: types.ErrorDetail{
						Timestamp:    time.Now().Format(time.RFC3339),
						Path:         c.Request.URL.Path,
						ErrorMessage: "Internal server error",
					},
				}
				c.JSON(http.StatusInternalServerError, errorResp)

				c.Abort()
			}
		}()

		c.Next()
	}
}


