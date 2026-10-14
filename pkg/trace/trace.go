package trace

import (
	"context"
)

const (
	// TraceIDHeader is the HTTP header name for trace ID
	TraceIDHeader = "X-Trace-ID"
)

var (
	TraceIDContextKey = traceIDContextKey{}
)

type traceIDContextKey struct{}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDContextKey, traceID)
}

func GetTraceID(ctx context.Context) string {
	if traceID := ctx.Value(TraceIDContextKey); traceID != nil {
		if str, ok := traceID.(string); ok {
			return str
		}
	}

	return ""
}
