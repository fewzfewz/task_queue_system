// Package routes wires HTTP handlers to URL patterns using the standard
// net/http ServeMux (Go 1.22+ enhanced pattern syntax).
package routes

import (
	"log/slog"
	"net/http"

	"task-queue-system/internal/api/handler"
	"task-queue-system/internal/api/service"
	"task-queue-system/internal/queue"
	"task-queue-system/internal/storage/models"
)

// NewRouter builds and returns a fully configured http.ServeMux.
// It is the single source of truth for all route → handler mappings.
//
// Pass models.NewInMemoryStore() for local dev or a PostgresStore for production.
func NewRouter(q queue.Queue, store models.Store, logger *slog.Logger) http.Handler {
	svc := service.New(q, store, logger)
	h := handler.New(svc, logger)

	mux := http.NewServeMux()

	// POST /jobs        → create a new job and enqueue it
	mux.HandleFunc("POST /jobs", h.CreateJob)

	// GET  /jobs/{id}   → return the current status of a job
	mux.HandleFunc("GET /jobs/{id}", h.GetJobStatus)

	// GET  /metrics     → system performance numbers
	mux.HandleFunc("GET /metrics", h.GetMetrics)

	return mux
}
