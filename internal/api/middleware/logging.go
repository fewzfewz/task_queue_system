package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"task-queue-system/internal/metrics"
	"task-queue-system/internal/tracing"
)

// responseWriter is a minimal wrapper to capture the HTTP status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// RequestLogger returns a middleware that logs every incoming HTTP request.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, span := tracing.Start(r.Context(), "http.request")
			defer span.End()

			traceID := tracing.Inject(ctx)
			r = r.WithContext(ctx)
			start := time.Now()

			span.SetAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.target", r.URL.Path),
				attribute.String("http.route", r.URL.Path),
				attribute.String("http.user_agent", r.UserAgent()),
				attribute.String("net.peer.ip", r.RemoteAddr),
				attribute.String("trace.id", traceID),
			)

			metrics.APIRequestTotal.Inc()

			rw := &responseWriter{w, http.StatusOK}

			next.ServeHTTP(rw, r)

			duration := time.Since(start)

			span.SetAttributes(attribute.Int("http.status_code", rw.statusCode))
			if rw.statusCode >= 500 {
				span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", rw.statusCode))
			} else {
				span.SetStatus(codes.Ok, "OK")
			}

			logger.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.statusCode,
				"duration_ms", duration.Milliseconds(),
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
				"trace_id", traceID,
			)
		})
	}
}
