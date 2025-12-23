package middleware

import (
	"context"
	"net/http"
	"time"

	"git.futuregamestudio.net/be-shared/slot-game-module.git/types"
	"github.com/gin-gonic/gin"
)

// Timeout creates a middleware that adds timeout to requests
func Timeout(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Create context with timeout
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		// Replace request context
		c.Request = c.Request.WithContext(ctx)

		// Create channel to signal completion
		done := make(chan struct{})

		go func() {
			c.Next()
			close(done)
		}()

		select {
		case <-done:
			// Request completed normally
		case <-ctx.Done():
			// Timeout exceeded
			errorResp := types.ErrorResponse{
				StatusCode: http.StatusRequestTimeout,
				IsSuccess:  false,
				Error: types.ErrorDetail{
					Timestamp:    time.Now().Format(time.RFC3339),
					Path:         c.Request.URL.Path,
					ErrorMessage: "Request timeout",
				},
			}
			c.AbortWithStatusJSON(http.StatusRequestTimeout, errorResp)
		}
	}
}


