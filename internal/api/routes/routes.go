package routes

import (
	"log/slog"
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "task-queue-system/docs"
	"task-queue-system/internal/api/handler"
	"task-queue-system/internal/api/middleware"
	"task-queue-system/internal/config"
	"task-queue-system/internal/queue"
	"task-queue-system/internal/service"
	"task-queue-system/internal/storage/models"
	"task-queue-system/internal/webhooks"
)

func NewRouter(q queue.Queue, store models.Store, logger *slog.Logger, cfg *config.Config, webhookStore *webhooks.WebhookStore) http.Handler {
	svc := service.New(q, store, logger, cfg.MaxQueueSize)
	if webhookStore != nil {
		svc.SetWebhookStore(webhookStore)
	}
	h := handler.New(svc, logger, cfg.ApiKey, cfg.AdminUsername, cfg.AdminPassword)
	svc.SetSSEBroker(h.SSEBroker())
	if webhookStore != nil {
		h.SetWebhookStore(webhookStore)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/login", h.Login)

	mux.HandleFunc("GET /", h.ServeAppUI)
	mux.HandleFunc("GET /ui", h.ServeAppUI)

	mux.Handle("POST /jobs", middleware.AuthRequired(cfg)(http.HandlerFunc(h.CreateJob)))
	mux.Handle("POST /jobs/batch", middleware.AuthRequired(cfg)(http.HandlerFunc(h.CreateJobBatch)))

	mux.Handle("GET /jobs", middleware.AuthRequired(cfg)(http.HandlerFunc(h.ListJobs)))
	mux.HandleFunc("GET /jobs/{id}", h.GetJobStatus)
	mux.HandleFunc("PATCH /jobs/{id}/progress", h.UpdateJobProgress)
	mux.HandleFunc("POST /jobs/{id}/cancel", h.CancelJob)
	mux.HandleFunc("POST /jobs/{id}/pause", h.PauseJob)
	mux.HandleFunc("POST /jobs/{id}/resume", h.ResumeJob)

	mux.HandleFunc("GET /metrics", h.GetMetrics)
	mux.HandleFunc("GET /workers", h.GetWorkers)
	mux.HandleFunc("GET /events", h.JobEventsSSE)
	mux.HandleFunc("GET /admin/dlq", h.ServeAdminDLQ)

	mux.HandleFunc("GET /api/v1/stats", h.GetStats)
	mux.HandleFunc("GET /api/v1/jobs/{id}/deps", h.GetJobDeps)

	auth := middleware.AuthRequired(cfg)

	mux.Handle("GET /api/v1/dlq", auth(http.HandlerFunc(h.ListFailedJobs)))
	mux.Handle("GET /api/v1/dlq/{id}", auth(http.HandlerFunc(h.GetFailedJobDetail)))
	mux.Handle("POST /api/v1/dlq/{id}/replay", auth(http.HandlerFunc(h.ReplayFailedJob)))
	mux.Handle("DELETE /api/v1/dlq/{id}", auth(http.HandlerFunc(h.DeleteFailedJob)))
	mux.Handle("DELETE /api/v1/dlq", auth(http.HandlerFunc(h.BulkPurgeDLQ)))

	if webhookStore != nil {
		ws := middleware.AuthRequired(cfg)
		mux.Handle("POST /api/v1/webhooks", ws(http.HandlerFunc(h.RegisterWebhook)))
		mux.Handle("GET /api/v1/webhooks", ws(http.HandlerFunc(h.ListWebhooks)))
		mux.Handle("GET /api/v1/webhooks/{id}", ws(http.HandlerFunc(h.GetWebhook)))
		mux.Handle("PUT /api/v1/webhooks/{id}", ws(http.HandlerFunc(h.UpdateWebhook)))
		mux.Handle("DELETE /api/v1/webhooks/{id}", ws(http.HandlerFunc(h.DeleteWebhook)))
	}

	mux.HandleFunc("GET /swagger/", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	return middleware.RequestLogger(logger)(mux)
}
