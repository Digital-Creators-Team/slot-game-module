package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// LoggingConfig holds logging middleware configuration
type LoggingConfig struct {
	SkipPaths []string // Paths to skip logging (e.g., health checks)
}

// Logging creates a logging middleware
func Logging(logger zerolog.Logger) gin.HandlerFunc {
	return LoggingWithConfig(logger, LoggingConfig{
		SkipPaths: []string{"/health", "/api/health"},
	})
}

// LoggingWithConfig creates a logging middleware with custom configuration
func LoggingWithConfig(logger zerolog.Logger, config LoggingConfig) gin.HandlerFunc {
	skipPaths := make(map[string]bool)
	for _, path := range config.SkipPaths {
		skipPaths[path] = true
	}

	return func(c *gin.Context) {
		// Skip logging for specified paths
		if skipPaths[c.Request.URL.Path] {
			c.Next()
			return
		}

		// Get trace_id from context
		traceID := GetTraceID(c)

		// Get request start time
		startTime := time.Now()

		// Create request logger with trace_id
		reqLogger := logger.With().
			Str("trace_id", traceID).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Str("client_ip", c.ClientIP()).
			Str("user_agent", c.Request.UserAgent()).
			Logger()

		// Log request start
		reqLogger.Info().Msg("Request started")

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(startTime)

		// Determine log level based on status code
		status := c.Writer.Status()
		var event *zerolog.Event
		switch {
		case status >= 500:
			event = reqLogger.Error()
		case status >= 400:
			event = reqLogger.Warn()
		default:
			event = reqLogger.Info()
		}

		// Log response
		event.
			Int("status", status).
			Dur("duration", duration).
			Int("response_size", c.Writer.Size()).
			Msg("Request completed")

		// Log errors if any
		if len(c.Errors) > 0 {
			for _, err := range c.Errors {
				reqLogger.Error().
					Err(err.Err).
					Uint64("type", uint64(err.Type)).
					Msg("Request error")
			}
		}
	}
}


