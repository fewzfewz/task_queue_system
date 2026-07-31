package routes

import (
	"log/slog"
	"net/http"
	"time"

	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "task-queue-system/docs"
	"task-queue-system/internal/api/handler"
	"task-queue-system/internal/api/middleware"
	"task-queue-system/internal/api/session"
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

	sessions := session.NewStore(time.Duration(cfg.SessionTTLSeconds) * time.Second)
	h := handler.New(svc, logger, cfg.ApiKey, cfg.AdminUsername, cfg.AdminPassword, sessions, cfg.WorkerAddr, cfg.LoginRateLimit)
	h.SetReadonlyCredentials(cfg.ReadonlyUsername, cfg.ReadonlyPassword)
	svc.SetSSEBroker(h.SSEBroker())
	if webhookStore != nil {
		h.SetWebhookStore(webhookStore)
	}

	mux := http.NewServeMux()

	// ── Public routes ────────────────────────────────────────────────────────
	mux.HandleFunc("POST /api/v1/login", h.Login)
	mux.HandleFunc("GET /api/v1/session", h.GetSession)
	mux.HandleFunc("GET /", h.ServeAppUI)
	mux.HandleFunc("GET /ui", h.ServeAppUI)
	mux.HandleFunc("GET /login", h.ServeLoginPage)
	mux.HandleFunc("GET /metrics", h.GetMetrics) // Prometheus scrape target
	mux.HandleFunc("GET /admin/dlq", h.ServeAdminDLQ)

	// ── Authenticated routes (API key or session cookie) ──────────────────────
	auth := middleware.RequireAuth(cfg, sessions)
	csrf := middleware.CSRFProtect()

	// read registers an authenticated GET route. Any authenticated principal
	// (admin or viewer) may read.
	read := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, auth(http.HandlerFunc(fn)))
	}

	// write registers an authenticated, admin-only, CSRF-protected mutation.
	write := func(pattern string, fn http.HandlerFunc) {
		mw := middleware.RequireRole(middleware.RoleAdmin)(http.HandlerFunc(fn))
		mw = csrf(mw)
		mux.Handle(pattern, auth(mw))
	}

	read("GET /jobs", h.ListJobs)
	read("GET /jobs/{id}", h.GetJobStatus)
	read("GET /api/v1/jobs/{id}/deps", h.GetJobDeps)
	read("GET /api/v1/stats", h.GetStats)
	read("GET /workers", h.GetWorkers)
	read("GET /events", h.JobEventsSSE)
	read("GET /api/v1/dlq", h.ListFailedJobs)
	read("GET /api/v1/dlq/{id}", h.GetFailedJobDetail)
	read("GET /api/v1/circuit-breakers", h.GetCircuitBreakers)

	write("POST /jobs", h.CreateJob)
	write("POST /jobs/batch", h.CreateJobBatch)
	write("PATCH /jobs/{id}/progress", h.UpdateJobProgress)
	write("POST /jobs/{id}/cancel", h.CancelJob)
	write("POST /jobs/{id}/pause", h.PauseJob)
	write("POST /jobs/{id}/resume", h.ResumeJob)
	write("POST /api/v1/dlq/{id}/replay", h.ReplayFailedJob)
	write("DELETE /api/v1/dlq/{id}", h.DeleteFailedJob)
	write("DELETE /api/v1/dlq", h.BulkPurgeDLQ)
	write("POST /api/v1/circuit-breakers/reset/{type}", h.ResetCircuitBreaker)

	// Logout is CSRF-protected but available to any authenticated session.
	mux.Handle("POST /api/v1/logout", auth(csrf(http.HandlerFunc(h.Logout))))

	if webhookStore != nil {
		read("GET /api/v1/webhooks", h.ListWebhooks)
		read("GET /api/v1/webhooks/{id}", h.GetWebhook)
		write("POST /api/v1/webhooks", h.RegisterWebhook)
		write("PUT /api/v1/webhooks/{id}", h.UpdateWebhook)
		write("DELETE /api/v1/webhooks/{id}", h.DeleteWebhook)
	}

	mux.HandleFunc("GET /swagger/", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	return middleware.RequestLogger(logger)(mux)
}
