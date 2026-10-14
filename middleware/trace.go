package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// TraceIDKey is the key used to store trace ID in context
	TraceIDKey = "trace_id"
	// TraceIDHeader is the HTTP header name for trace ID
	TraceIDHeader = "X-Trace-ID"
	// RequestTimeKey is the key used to store request start time
	RequestTimeKey = "request_time"
)

// TraceID creates a middleware that adds a unique trace_id to each request
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if trace_id already exists in header
		traceID := c.GetHeader(TraceIDHeader)
		if traceID == "" {
			// Generate new trace ID
			traceID = uuid.New().String()
		}

		// Store trace_id in context
		c.Set(TraceIDKey, traceID)

		// Add trace_id to response header
		c.Header(TraceIDHeader, traceID)

		// Store request start time
		c.Set(RequestTimeKey, time.Now())

		c.Next()
	}
}

// GetTraceID extracts trace ID from gin context
func GetTraceID(c *gin.Context) string {
	if traceID, exists := c.Get(TraceIDKey); exists {
		if str, ok := traceID.(string); ok {
			return str
		}
	}
	return ""
}

// GetRequestTime extracts request start time from gin context
func GetRequestTime(c *gin.Context) time.Time {
	if reqTime, exists := c.Get(RequestTimeKey); exists {
		if t, ok := reqTime.(time.Time); ok {
			return t
		}
	}
	return time.Now()
}


