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
	"task-queue-system/internal/jobtypes"
	"task-queue-system/internal/queue"
	"task-queue-system/internal/service"
	"task-queue-system/internal/storage/models"
	"task-queue-system/internal/webhooks"
)

func NewRouter(q queue.Queue, store models.Store, logger *slog.Logger, cfg *config.Config, webhookStore *webhooks.WebhookStore, jobTypeStore *jobtypes.Store, sessionStores ...session.Store) http.Handler {
	svc := service.New(q, store, logger, cfg.MaxQueueSize)
	if webhookStore != nil {
		svc.SetWebhookStore(webhookStore)
	}
	if jobTypeStore != nil {
		svc.SetJobTypeStore(jobTypeStore)
	}

	var sessions session.Store
	if len(sessionStores) > 0 && sessionStores[0] != nil {
		sessions = sessionStores[0]
	} else {
		sessions = session.NewStore(time.Duration(cfg.SessionTTLSeconds) * time.Second)
	}

	h := handler.New(svc, logger, cfg.ApiKey, cfg.AdminUsername, cfg.AdminPassword, sessions, cfg.WorkerAddr, cfg.LoginRateLimit, cfg.RegisterRateLimit)
	h.SetReadonlyCredentials(cfg.ReadonlyUsername, cfg.ReadonlyPassword)
	svc.SetSSEBroker(h.SSEBroker())
	if webhookStore != nil {
		h.SetWebhookStore(webhookStore)
	}
	if jobTypeStore != nil {
		h.SetJobTypeStore(jobTypeStore)
	}

	mux := http.NewServeMux()

	// ── Public routes ────────────────────────────────────────────────────────
	mux.HandleFunc("POST /api/v1/login", h.Login)
	mux.HandleFunc("GET /api/v1/session", h.GetSession)
	mux.HandleFunc("GET /features", h.ServeFeaturesPage)
	mux.HandleFunc("GET /docs", h.ServeDocsPage)
	mux.HandleFunc("GET /docs/sdk", h.ServeSDKPage)
	mux.HandleFunc("GET /", h.ServeLandingPage) // public landing + registration
	mux.HandleFunc("GET /ui", h.ServeAppUI)     // operator dashboard
	mux.HandleFunc("GET /login", h.ServeLoginPage)
	mux.HandleFunc("GET /metrics", h.GetMetrics) // Prometheus scrape target
	mux.HandleFunc("GET /admin/dlq", h.ServeAdminDLQ)

	// ── Client Portal UI (auth handled client-side via localStorage) ──────────
	mux.HandleFunc("GET /client/register", h.ServeClientRegisterPage)
	mux.HandleFunc("GET /client/login", h.ServeClientLoginPage)
	mux.HandleFunc("GET /client/dashboard", h.ServeClientDashboard)

	// ── Client registration API (open, no auth required) ──────────────────────
	mux.HandleFunc("POST /api/v1/register", h.RegisterClient)

	// ── Authenticated routes (API key or session cookie) ──────────────────────
	auth := middleware.RequireAuth(cfg, sessions, store)
	csrf := middleware.CSRFProtect()

	// clientRead registers an authenticated GET route accessible to any authenticated principal
	// (admin, tenant client, or viewer).
	clientRead := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, auth(http.HandlerFunc(fn)))
	}

	// operatorRead registers an authenticated GET route restricted to operator/admin or viewer.
	// Tenant clients are rejected with 403 Forbidden.
	operatorRead := func(pattern string, fn http.HandlerFunc) {
		mw := middleware.RequireAnyRole(middleware.RoleAdmin, middleware.RoleViewer)(http.HandlerFunc(fn))
		mux.Handle(pattern, auth(mw))
	}

	// clientWrite registers an authenticated mutation route accessible to admin and tenant clients.
	clientWrite := func(pattern string, fn http.HandlerFunc) {
		mw := middleware.RequireAnyRole(middleware.RoleAdmin, middleware.RoleTenantClient)(http.HandlerFunc(fn))
		mw = csrf(mw)
		mux.Handle(pattern, auth(mw))
	}

	// operatorWrite registers an authenticated mutation route strictly restricted to admin/operator.
	// Tenant clients and viewers are rejected with 403 Forbidden.
	operatorWrite := func(pattern string, fn http.HandlerFunc) {
		mw := middleware.RequireRole(middleware.RoleAdmin)(http.HandlerFunc(fn))
		mw = csrf(mw)
		mux.Handle(pattern, auth(mw))
	}

	clientRead("GET /jobs", h.ListJobs)
	clientRead("GET /jobs/{id}", h.GetJobStatus)
	clientRead("GET /api/v1/jobs/{id}", h.GetJobStatus)
	clientRead("GET /api/v1/jobs/{id}/deps", h.GetJobDeps)
	clientRead("GET /api/v1/stats", h.GetStats)
	clientRead("GET /workers", h.GetWorkers)
	clientRead("GET /events", h.JobEventsSSE)
	clientRead("GET /api/v1/events", h.JobEventsSSE)
	clientRead("GET /api/v1/dlq", h.ListFailedJobs)
	clientRead("GET /api/v1/dlq/{id}", h.GetFailedJobDetail)
	clientRead("GET /api/v1/circuit-breakers", h.GetCircuitBreakers)
	clientRead("GET /api/v1/rate-limits", h.GetRateLimits)
	clientRead("GET /api/v1/client/me", h.GetClientInfo)
	clientRead("GET /api/v1/job-types", h.ListJobTypes)

	operatorRead("GET /api/v1/clients", h.ListClients)
	operatorRead("GET /api/v1/audit-logs", h.ListAuditLogs)
	operatorRead("GET /api/v1/operator/paused-queues", h.GetPausedQueues)

	clientWrite("POST /jobs", h.CreateJob)
	clientWrite("POST /jobs/batch", h.CreateJobBatch)
	clientWrite("PATCH /jobs/{id}/progress", h.UpdateJobProgress)
	clientWrite("POST /jobs/{id}/cancel", h.CancelJob)
	clientWrite("POST /jobs/{id}/pause", h.PauseJob)
	clientWrite("POST /jobs/{id}/resume", h.ResumeJob)
	clientWrite("POST /api/v1/dlq/{id}/replay", h.ReplayFailedJob)
	clientWrite("DELETE /api/v1/dlq/{id}", h.DeleteFailedJob)

	operatorWrite("DELETE /api/v1/dlq", h.BulkPurgeDLQ)
	operatorWrite("POST /api/v1/circuit-breakers/reset/{type}", h.ResetCircuitBreaker)
	operatorWrite("POST /api/v1/job-types", h.CreateJobType)
	operatorWrite("DELETE /api/v1/job-types/{name}", h.DeleteJobType)
	operatorWrite("PATCH /api/v1/job-types/{name}", h.UpdateJobType)
	operatorWrite("DELETE /api/v1/clients/{tenant_id}", h.RevokeClient)
	operatorWrite("POST /api/v1/clients/{tenant_id}/rotate", h.RotateClientKey)
	operatorWrite("PUT /api/v1/clients/{tenant_id}/config", h.UpdateClientConfig)

	// Operator Panic Buttons
	operatorWrite("POST /api/v1/operator/queue/{type}/pause", h.PauseQueue)
	operatorWrite("POST /api/v1/operator/queue/{type}/resume", h.ResumeQueue)

	// Logout is CSRF-protected but available to any authenticated session.
	mux.Handle("POST /api/v1/logout", auth(csrf(http.HandlerFunc(h.Logout))))

	if webhookStore != nil {
		clientRead("GET /api/v1/webhooks", h.ListWebhooks)
		clientRead("GET /api/v1/webhooks/{id}", h.GetWebhook)
		clientRead("GET /api/v1/webhooks/{id}/deliveries", h.GetWebhookDeliveries)
		clientWrite("POST /api/v1/webhooks", h.RegisterWebhook)
		clientWrite("PUT /api/v1/webhooks/{id}", h.UpdateWebhook)
		clientWrite("DELETE /api/v1/webhooks/{id}", h.DeleteWebhook)
	}

	mux.HandleFunc("GET /swagger/", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	return middleware.RequestLogger(logger)(mux)
}
