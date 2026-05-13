package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type contextKey string

const traceIDKey contextKey = "trace_id"

// Start returns a context carrying a trace/request identifier.
// It is intentionally lightweight so the project can adopt OTEL later
// without changing call sites.
func Start(ctx context.Context) (context.Context, string) {
	if existing, ok := FromContext(ctx); ok && existing != "" {
		return ctx, existing
	}
	traceID := newID()
	return context.WithValue(ctx, traceIDKey, traceID), traceID
}

// FromContext extracts the trace identifier if one has been attached.
func FromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(traceIDKey).(string)
	return v, ok && v != ""
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
