package trace

import "github.com/rs/zerolog"

type traceHook struct {
}

func NewTraceHook() zerolog.Hook {
	return &traceHook{}
}

func (h *traceHook) Run(e *zerolog.Event, level zerolog.Level, message string) {
	ctx := e.GetCtx()

	if traceID := GetTraceID(ctx); traceID != "" {
		e.Str("trace_id", traceID)
	}
}
