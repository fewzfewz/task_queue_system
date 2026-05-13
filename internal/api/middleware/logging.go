package middleware

import (
	"log/slog"
	"net/http"
	"time"

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

// RequestLogger returns a middleware that logs every incoming HTTP request.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, traceID := tracing.Start(r.Context())
			r = r.WithContext(ctx)
			start := time.Now()
			
			rw := &responseWriter{w, http.StatusOK}
			
			next.ServeHTTP(rw, r)
			
			duration := time.Since(start)
			
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
