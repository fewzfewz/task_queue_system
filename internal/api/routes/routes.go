// Package routes wires HTTP handlers to URL patterns using the standard
// net/http ServeMux (Go 1.22+ enhanced pattern syntax).
package routes

import (
	"log/slog"
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "task-queue-system/docs" // Import generated swagger files natively
	"task-queue-system/internal/api/handler"
	"task-queue-system/internal/api/middleware"
	"task-queue-system/internal/queue"
	"task-queue-system/internal/service"
	"task-queue-system/internal/storage/models"
)

// NewRouter builds and returns a fully configured http.ServeMux.
// It is the single source of truth for all route → handler mappings.
//
// Pass models.NewInMemoryStore() for local dev or a PostgresStore for production.
func NewRouter(q queue.Queue, store models.Store, logger *slog.Logger, apiKey string, maxQueueSize int64) http.Handler {
	svc := service.New(q, store, logger, maxQueueSize)
	h := handler.New(svc, logger)

	mux := http.NewServeMux()

	// POST /jobs        → create a new job and enqueue it (PROTECTED)
	mux.Handle("POST /jobs", middleware.AuthRequired(apiKey)(http.HandlerFunc(h.CreateJob)))

	// GET  /jobs/{id}   → return the current status of a job
	mux.HandleFunc("GET /jobs/{id}", h.GetJobStatus)

	// GET  /metrics     → system performance numbers
	mux.HandleFunc("GET /metrics", h.GetMetrics)

	// GET  /workers     → worker health and count details
	mux.HandleFunc("GET /workers", h.GetWorkers)

	// Swagger UI integration
	// The http-swagger driver handles the static asset serving natively.
	mux.HandleFunc("GET /swagger/", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"), // Provides the canonical JSON schema natively
	))

	// Apply middleware: log all requests
	handlerWithMiddleware := middleware.RequestLogger(logger)(mux)

	return handlerWithMiddleware
}
