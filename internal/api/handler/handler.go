// Package handler provides the HTTP handlers for the task-queue API.
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"task-queue-system/internal/api/dto"
	"task-queue-system/internal/api/middleware"
	"task-queue-system/internal/api/session"
	apperr "task-queue-system/internal/errors"
	"task-queue-system/internal/jobs"
	"task-queue-system/internal/service"
	"task-queue-system/internal/sse"
	"task-queue-system/internal/storage/models"
	"task-queue-system/internal/webhooks"
)

// JobHandler holds the dependencies for the job-related HTTP handlers.
type JobHandler struct {
	service            *service.JobService
	webhookStore       *webhooks.WebhookStore
	sseBroker          *sse.Broker
	logger             *slog.Logger
	apiKey             string
	adminUsername      string
	adminPassword      string
	readonlyUsername   string
	readonlyPassword   string
	sessions           *session.Store
	loginLimiter       *session.LoginLimiter
	workerAddr         string
	sseCheckInterval   time.Duration
}

// New creates a JobHandler. The session store backs the operator UI login; the
// worker address is used to proxy circuit-breaker access through the API.
func New(svc *service.JobService, logger *slog.Logger, apiKey, adminUsername, adminPassword string, sessions *session.Store, workerAddr string, loginRateLimit int) *JobHandler {
	return &JobHandler{
		service:          svc,
		sseBroker:        sse.NewBroker(logger.With("component", "sse")),
		logger:           logger,
		apiKey:           apiKey,
		adminUsername:    adminUsername,
		adminPassword:    adminPassword,
		sessions:         sessions,
		loginLimiter:     session.NewLoginLimiter(loginRateLimit, time.Minute),
		workerAddr:       workerAddr,
		sseCheckInterval: 15 * time.Second,
	}
}

// SetReadonlyCredentials enables an optional viewer-only login account.
func (h *JobHandler) SetReadonlyCredentials(username, password string) {
	h.readonlyUsername = username
	h.readonlyPassword = password
}

// SSEBroker returns the SSE broker for wiring into other components.
func (h *JobHandler) SSEBroker() *sse.Broker { return h.sseBroker }

// SetWebhookStore attaches the persistent webhook store for CRUD endpoints.
func (h *JobHandler) SetWebhookStore(ws *webhooks.WebhookStore) {
	h.webhookStore = ws
}

// CreateJob handles POST /jobs.
//
// @Summary      Create a new job
// @Description  Submits a job to the queue for asynchronous execution.
// @Tags         jobs
// @Accept       json
// @Produce      json
// @Param        request  body      dto.CreateJobRequest  true  "Job Creation Request"
// @Success      201      {object}  dto.JobResponse
// @Failure      400      {object}  dto.ErrorResponse
// @Failure      500      {object}  dto.ErrorResponse
// @Router       /jobs [post]
func (h *JobHandler) CreateJob(w http.ResponseWriter, r *http.Request) {
	// ── Parse ─────────────────────────────────────────────────────────────────
	var req dto.CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "invalid JSON body: "+err.Error())
		return
	}
	defer r.Body.Close()

	// ── Validate ──────────────────────────────────────────────────────────────
	if err := req.Validate(); err != nil {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, err.Error())
		return
	}

	// ── Delegate to service ───────────────────────────────────────────────────
	var webhookConfig *jobs.WebhookConfig
	if req.Webhook != nil {
		webhookConfig = &jobs.WebhookConfig{
			URL:    req.Webhook.URL,
			Secret: req.Webhook.Secret,
			Events: req.Webhook.Events,
		}
		if len(webhookConfig.Events) == 0 {
			webhookConfig.Events = []string{"completed", "failed"}
		}
	}

	job, err := h.service.CreateJob(r.Context(), req.Type, req.Payload, req.Labels, req.Priority, req.MaxRetries, req.BackoffAlgorithm, req.BackoffJitter, req.CronExpr, req.RunAt, req.CorrelationID, req.Timeout, req.Version, req.TenantID, webhookConfig, req.DedupKey, req.Dependencies, req.ShardKey)
	if err != nil {
		h.writeAppError(w, err)
		return
	}

	h.logger.Info("job created", 
		"job_id", job.ID, 
		"job_type", job.Type, 
		"correlation_id", job.CorrelationID,
	)
	h.writeJSON(w, http.StatusCreated, dto.FromJob(job))
}

// CreateJobBatch handles POST /jobs/batch.
//
// @Summary      Create multiple jobs
// @Description  Submits up to 100 jobs in a single request.
// @Tags         jobs
// @Accept       json
// @Produce      json
// @Param        request  body      dto.BatchJobRequest  true  "Batch Job Creation Request"
// @Success      200      {object}  dto.BatchJobResponse
// @Failure      400      {object}  dto.ErrorResponse
// @Failure      500      {object}  dto.ErrorResponse
// @Router       /jobs/batch [post]
func (h *JobHandler) CreateJobBatch(w http.ResponseWriter, r *http.Request) {
	var req dto.BatchJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "invalid JSON body: "+err.Error())
		return
	}
	defer r.Body.Close()

	if err := req.Validate(); err != nil {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, err.Error())
		return
	}

	res := dto.BatchJobResponse{
		Total:     len(req.Jobs),
		Processed: len(req.Jobs),
	}

	for i, jr := range req.Jobs {
		var webhookConfig *jobs.WebhookConfig
		if jr.Webhook != nil {
			webhookConfig = &jobs.WebhookConfig{
				URL:    jr.Webhook.URL,
				Secret: jr.Webhook.Secret,
				Events: jr.Webhook.Events,
			}
			if len(webhookConfig.Events) == 0 {
				webhookConfig.Events = []string{"completed", "failed"}
			}
		}

		job, err := h.service.CreateJob(r.Context(), jr.Type, jr.Payload, jr.Labels, jr.Priority, jr.MaxRetries, jr.BackoffAlgorithm, jr.BackoffJitter, jr.CronExpr, jr.RunAt, jr.CorrelationID, jr.Timeout, jr.Version, jr.TenantID, webhookConfig, jr.DedupKey, jr.Dependencies, jr.ShardKey)
		if err != nil {
			res.Processed--
			res.Failed = append(res.Failed, dto.BatchJobError{Index: i, Error: err.Error()})
			continue
		}

		res.Successful = append(res.Successful, dto.BatchJobResult{Index: i, Job: ptr(dto.FromJob(job))})
	}

	h.writeJSON(w, http.StatusOK, res)
}

func ptr[T any](v T) *T { return &v }

// ListJobs handles GET /jobs with optional query filters.
//
// @Summary      List/search jobs
// @Description  Returns a paginated, filtered list of jobs. Supports filtering by status, type,
//               tenant, label key/value, and creation time range.
// @Tags         jobs
// @Produce      json
// @Param        status        query  string  false  "Filter by status (pending, processing, completed, failed)"
// @Param        type          query  string  false  "Filter by job type"
// @Param        label_key     query  string  false  "Filter by label key"
// @Param        label_value   query  string  false  "Filter by label value (requires label_key)"
// @Param        created_after query  string  false  "ISO8601 timestamp"
// @Param        created_before query string  false  "ISO8601 timestamp"
// @Param        limit         query  int     false  "Page size (default 20)"
// @Param        offset        query  int     false  "Page offset (default 0)"
// @Success      200           {array} dto.JobResponse
// @Failure      400           {object} dto.ErrorResponse
// @Router       /jobs [get]
func (h *JobHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	filter := models.JobFilter{
		TenantID: r.URL.Query().Get("tenant_id"),
		Status:   r.URL.Query().Get("status"),
		Type:     r.URL.Query().Get("type"),
		LabelKey: r.URL.Query().Get("label_key"),
		LabelValue: r.URL.Query().Get("label_value"),
	}

	if after := r.URL.Query().Get("created_after"); after != "" {
		if t, err := time.Parse(time.RFC3339, after); err == nil {
			filter.CreatedAfter = t
		}
	}
	if before := r.URL.Query().Get("created_before"); before != "" {
		if t, err := time.Parse(time.RFC3339, before); err == nil {
			filter.CreatedBefore = t
		}
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 { page = 1 }

	filter.Limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	filter.Offset = (page - 1) * filter.Limit
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	jobs, err := h.service.SearchJobs(r.Context(), filter)
	if err != nil {
		h.writeAppError(w, err)
		return
	}

	res := make([]dto.JobResponse, len(jobs))
	for i, j := range jobs {
		res[i] = dto.FromJob(j)
	}

	// Count total matching for pagination metadata
	totalFilter := filter
	totalFilter.Limit = 0
	totalFilter.Offset = 0
	totalJobs, _ := h.service.SearchJobs(r.Context(), totalFilter)

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"jobs":  res,
		"total": len(totalJobs),
		"page":  page,
		"limit": filter.Limit,
	})
}

// GetJobStatus handles GET /jobs/{id}.
//
// @Summary      Get job status
// @Description  Retrieves the current execution status and metadata of a specific job by ID.
// @Tags         jobs
// @Produce      json
// @Param        id       path      string  true  "Job ID"
// @Success      200      {object}  dto.JobResponse
// @Failure      400      {object}  dto.ErrorResponse
// @Failure      404      {object}  dto.ErrorResponse
// @Failure      500      {object}  dto.ErrorResponse
// @Router       /jobs/{id} [get]
func (h *JobHandler) GetJobStatus(w http.ResponseWriter, r *http.Request) {
	// ── Extract path parameter ─────────────────────────────────────────────
	// Compatible with both net/http 1.22+ path params and manual parsing.
	jobID := r.PathValue("id")
	if jobID == "" {
		// Fallback: parse manually for older Go versions.
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/jobs/"), "/")
		jobID = parts[0]
	}

	if jobID == "" {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "missing job ID in path")
		return
	}

	// ── Delegate to service ───────────────────────────────────────────────────
	job, err := h.service.GetJobStatus(r.Context(), jobID)
	if err != nil {
		h.writeAppError(w, err)
		return
	}

	// ── Multi-tenancy Filter ────────────────────────────────────────────────
	// If a tenant_id is provided in the query, we only return the job if it matches.
	// In a real system, this would be extracted from an auth token.
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID != "" && job.TenantID != tenantID {
		h.writeError(w, http.StatusForbidden, apperr.CodePermissionDenied, "access to job denied for tenant")
		return
	}

	h.writeJSON(w, http.StatusOK, dto.FromJob(job))
}

// GetStats handles GET /api/v1/stats.
//
// @Summary      Get system stats
// @Description  Returns queue lengths, worker count, and approximate job counts.
// @Tags         stats
// @Produce      json
// @Success      200 {object} map[string]interface{}
// @Router       /api/v1/stats [get]
func (h *JobHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	queueLengths, err := h.service.QueueLengths(r.Context())
	if err != nil {
		h.writeAppError(w, err)
		return
	}
	workers, _ := h.service.GetActiveWorkers(r.Context())

	// Aggregate counts
	totalPending := int64(0)
	byQueue := make(map[string]int64)
	for qtype, tenants := range queueLengths {
		var queueTotal int64
		for _, count := range tenants {
			queueTotal += count
		}
		byQueue[qtype] = queueTotal
		totalPending += queueTotal
	}

	// Count by status via search
	completedJobs, _ := h.service.SearchJobs(r.Context(), models.JobFilter{Status: string(jobs.StatusCompleted), Limit: 1})
	failedJobs, _ := h.service.SearchJobs(r.Context(), models.JobFilter{Status: string(jobs.StatusFailed), Limit: 1})

	stats := map[string]interface{}{
		"total_pending":    totalPending,
		"queue_breakdown":  queueLengths,
		"worker_count":     len(workers),
		"workers":          workers,
		"approx_completed": len(completedJobs), // hacky but gives a sense
		"approx_failed":    len(failedJobs),
	}
	h.writeJSON(w, http.StatusOK, stats)
}

// GetJobDeps returns the DAG dependency chain for a job.
//
// @Summary      Get job dependency graph
// @Description  Returns upstream dependencies and downstream dependents for a job.
// @Tags         jobs
// @Produce      json
// @Param        id   path      string  true  "Job ID"
// @Success      200  {object}  map[string]interface{}
// @Failure      404  {object}  dto.ErrorResponse
// @Router       /api/v1/jobs/{id}/deps [get]
func (h *JobHandler) GetJobDeps(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "missing job ID in path")
		return
	}

	job, err := h.service.GetJobStatus(r.Context(), jobID)
	if err != nil {
		h.writeAppError(w, err)
		return
	}

	deps := make([]dto.JobResponse, 0)
	if len(job.Dependencies) > 0 {
		depJobs, err := h.service.GetJobByIDs(r.Context(), job.Dependencies)
		if err == nil {
			for _, d := range depJobs {
				deps = append(deps, dto.FromJob(d))
			}
		}
	}

	// Also find dependents (jobs that list this job as a dependency)
	dependents := make([]dto.JobResponse, 0)
	allJobs, _ := h.service.SearchJobs(r.Context(), models.JobFilter{Limit: 500})
	for _, j := range allJobs {
		for _, depID := range j.Dependencies {
			if depID == jobID {
				dependents = append(dependents, dto.FromJob(j))
				break
			}
		}
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"job":         dto.FromJob(job),
		"depends_on":  deps,
		"dependents":  dependents,
	})
}

// GetMetrics handles GET /metrics.
//
// @Summary      Get queue metrics
// @Description  Retrieves system-wide execution statistics including totals, active jobs, and failure counts.
// @Tags         metrics
// @Produce      json
// @Success      200      {object}  queue.QueueMetrics
// @Failure      500      {object}  dto.ErrorResponse
// @Router       /metrics [get]
func (h *JobHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	// Standard Prometheus metrics format.
	promhttp.Handler().ServeHTTP(w, r)
}



// GetWorkers handles GET /workers.
//
// @Summary      Get active workers
// @Description  Retrieves a list of all currently active worker instances and their last heartbeat timestamp.
// @Tags         metrics
// @Produce      json
// @Success      200      {array}   queue.WorkerInfo
// @Failure      500      {object}  dto.ErrorResponse
// @Router       /workers [get]
func (h *JobHandler) GetWorkers(w http.ResponseWriter, r *http.Request) {
	workers, err := h.service.GetActiveWorkers(r.Context())
	if err != nil {
		h.writeAppError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, workers)
}

// ── Webhook CRUD ──────────────────────────────────────────────────────────────

// RegisterWebhook handles POST /api/v1/webhooks.
//
// @Summary      Register a webhook
// @Description  Creates a new webhook that will be called on specified events.
// @Tags         webhooks
// @Accept       json
// @Produce      json
// @Param        body  body      object  true  "Webhook registration payload"
// @Success      201   {object}  object
// @Failure      400   {object}  dto.ErrorResponse
// @Router       /api/v1/webhooks [post]
func (h *JobHandler) RegisterWebhook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL    string   `json:"url"`
		Secret string   `json:"secret"`
		Events []string `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "invalid JSON: "+err.Error())
		return
	}
	defer r.Body.Close()
	if req.URL == "" {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "url is required")
		return
	}
	tenantID, _ := r.Context().Value(middleware.ContextKeyTenantID).(string)
	wh, err := h.webhookStore.Create(r.Context(), tenantID, req.URL, req.Secret, req.Events)
	if err != nil {
		h.writeAppError(w, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, wh)
}

// ListWebhooks handles GET /api/v1/webhooks.
//
// @Summary      List webhooks
// @Description  Returns all registered webhooks.
// @Tags         webhooks
// @Produce      json
// @Success      200  {array}   object
// @Router       /api/v1/webhooks [get]
func (h *JobHandler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := r.Context().Value(middleware.ContextKeyTenantID).(string)
	list, err := h.webhookStore.List(r.Context(), tenantID)
	if err != nil {
		h.writeAppError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, list)
}

// GetWebhook handles GET /api/v1/webhooks/{id}.
//
// @Summary      Get webhook by ID
// @Description  Returns a single webhook configuration.
// @Tags         webhooks
// @Produce      json
// @Param        id   path      string  true  "Webhook ID"
// @Success      200  {object}  object
// @Failure      404  {object}  dto.ErrorResponse
// @Router       /api/v1/webhooks/{id} [get]
func (h *JobHandler) GetWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wh, err := h.webhookStore.GetByID(r.Context(), id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, apperr.CodeNotFound, "webhook not found")
		return
	}
	h.writeJSON(w, http.StatusOK, wh)
}

// UpdateWebhook handles PUT /api/v1/webhooks/{id}.
//
// @Summary      Update a webhook
// @Description  Updates an existing webhook's URL, secret or event filters.
// @Tags         webhooks
// @Accept       json
// @Produce      json
// @Param        id    path    string  true  "Webhook ID"
// @Param        body  body    object  true  "Updated webhook fields"
// @Success      200   {object}  object
// @Failure      404   {object}  dto.ErrorResponse
// @Router       /api/v1/webhooks/{id} [put]
func (h *JobHandler) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		URL    string   `json:"url"`
		Secret string   `json:"secret"`
		Events []string `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "invalid JSON: "+err.Error())
		return
	}
	defer r.Body.Close()
	wh, err := h.webhookStore.Update(r.Context(), id, req.URL, req.Secret, req.Events)
	if err != nil {
		h.writeError(w, http.StatusNotFound, apperr.CodeNotFound, "webhook not found")
		return
	}
	h.writeJSON(w, http.StatusOK, wh)
}

// DeleteWebhook handles DELETE /api/v1/webhooks/{id}.
//
// @Summary      Delete a webhook
// @Description  Removes a webhook by ID.
// @Tags         webhooks
// @Param        id   path      string  true  "Webhook ID"
// @Success      204  "No Content"
// @Failure      404  {object}  dto.ErrorResponse
// @Router       /api/v1/webhooks/{id} [delete]
func (h *JobHandler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.webhookStore.Delete(r.Context(), id); err != nil {
		h.writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PauseJob handles POST /jobs/{id}/pause.
//
// @Summary      Pause a job
// @Description  Prevents a job from being processed until resumed.
// @Tags         jobs
// @Produce      json
// @Param        id   path      string  true  "Job ID"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  dto.ErrorResponse
// @Router       /api/v1/jobs/{id}/pause [post]
func (h *JobHandler) PauseJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "missing job ID in path")
		return
	}
	if err := h.service.PauseJob(r.Context(), jobID); err != nil {
		h.writeAppError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "paused", "job_id": jobID})
}

// ResumeJob handles POST /jobs/{id}/resume.
//
// @Summary      Resume a job
// @Description  Resumes processing for a previously paused job.
// @Tags         jobs
// @Produce      json
// @Param        id   path      string  true  "Job ID"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  dto.ErrorResponse
// @Router       /api/v1/jobs/{id}/resume [post]
func (h *JobHandler) ResumeJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "missing job ID in path")
		return
	}
	if err := h.service.ResumeJob(r.Context(), jobID); err != nil {
		h.writeAppError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "resumed", "job_id": jobID})
}

// JobEventsSSE handles GET /events — SSE stream of job status changes.
//
// @Summary      Stream job events (SSE)
// @Description  Opens a server-sent event stream for real-time job status changes.
// @Tags         events
// @Produce      text/event-stream
// @Success      200  "SSE stream"
// @Router       /api/v1/events [get]
func (h *JobHandler) JobEventsSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := h.sseBroker.Subscribe()
	defer h.sseBroker.Unsubscribe(ch)

	// Send initial keepalive.
	fmt.Fprintf(w, ": keepalive\n\n")
	flusher.Flush()

	// Session-based connections are re-validated periodically so the stream is
	// torn down when the operator session expires or is revoked.
	authType, _ := r.Context().Value(middleware.ContextKeyAuthType).(string)
	sess := middleware.SessionFromContext(r.Context())

	var tickerC <-chan time.Time
	if authType == middleware.AuthTypeSession && sess != nil {
		ticker := time.NewTicker(h.sseCheckInterval)
		defer ticker.Stop()
		tickerC = ticker.C
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-tickerC:
			if _, ok := h.sessions.Get(sess.ID); !ok {
				fmt.Fprintf(w, "event: session_expired\n\ndata: {\"error\":\"session expired\"}\n\n")
				flusher.Flush()
				return
			}
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprint(w, msg)
			flusher.Flush()
		}
	}
}

// CancelJob handles POST /jobs/{id}/cancel.
//
// @Summary      Cancel a job
// @Description  Cancels a pending or running job.
// @Tags         jobs
// @Produce      json
// @Param        id   path      string  true  "Job ID"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  dto.ErrorResponse
// @Router       /api/v1/jobs/{id}/cancel [post]
func (h *JobHandler) CancelJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "missing job ID in path")
		return
	}
	if err := h.service.CancelJob(r.Context(), jobID); err != nil {
		h.writeAppError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled", "job_id": jobID})
}

// UpdateJobProgress handles PATCH /jobs/{id}/progress.
//
// @Summary      Update job progress
// @Description  Sets the progress percentage for an in-flight job.
// @Tags         jobs
// @Accept       json
// @Produce      json
// @Param        id    path    string  true  "Job ID"
// @Param        body  body    object  true  "Progress payload"
// @Success      200   {object}  map[string]interface{}
// @Failure      400   {object}  dto.ErrorResponse
// @Router       /api/v1/jobs/{id}/progress [patch]
func (h *JobHandler) UpdateJobProgress(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "missing job ID in path")
		return
	}

	var req struct {
		Progress float64 `json:"progress"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "invalid JSON body: "+err.Error())
		return
	}
	defer r.Body.Close()

	if err := h.service.UpdateJobProgress(r.Context(), jobID, req.Progress); err != nil {
		h.writeAppError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{"job_id": jobID, "progress": req.Progress})
}

// ── helpers ───────────────────────────────────────────────────────────────────

// writeJSON serialises v and writes it with the given status code.
func (h *JobHandler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.logger.Error("failed to write JSON response", "error", err)
	}
}

// writeError writes a standardised error response.
func (h *JobHandler) writeError(w http.ResponseWriter, status int, code, message string) {
	h.writeJSON(w, status, dto.ErrorResponse{Code: code, Error: message})
}

// writeAppError translates a domain AppError into an HTTP response.
func (h *JobHandler) writeAppError(w http.ResponseWriter, err error) {
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		h.logger.Error("unexpected error type", "error", err)
		h.writeError(w, http.StatusInternalServerError, apperr.CodeInternal, "an unexpected error occurred")
		return
	}

	h.logger.Warn("request failed", "code", appErr.Code, "error", appErr.Message)

	status := http.StatusInternalServerError
	switch appErr.Code {
	case apperr.CodeNotFound:
		status = http.StatusNotFound
	case apperr.CodeInvalidArgument:
		status = http.StatusBadRequest
	case apperr.CodeUnauthorized:
		status = http.StatusUnauthorized
	case apperr.CodeQueueFull, apperr.CodeTooManyRequests:
		status = http.StatusTooManyRequests
	case apperr.CodePermissionDenied:
		status = http.StatusForbidden
	case apperr.CodeConflict:
		status = http.StatusConflict
	case apperr.CodeInternal:
		status = http.StatusInternalServerError
	}

	h.writeError(w, status, appErr.Code, appErr.Message)
}

// ListFailedJobs handles GET /api/v1/dlq.
//
// @Summary      List failed jobs
// @Description  Returns a paginated list of jobs that have permanently failed for the tenant.
// @Tags         dlq
// @Produce      json
// @Param        queue  query     string  false  "Queue type filter"
// @Param        limit  query     int     false  "Page size"
// @Param        page   query     int     false  "Page number"
// @Success      200    {array}   dto.JobResponse
// @Failure      401    {object}  dto.ErrorResponse
// @Router       /api/v1/dlq [get]
func (h *JobHandler) ListFailedJobs(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := r.Context().Value(middleware.ContextKeyTenantID).(string)
	jobType := r.URL.Query().Get("queue")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 { limit = 20 }
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 { page = 1 }
	offset := (page - 1) * limit

	jobs, err := h.service.ListFailedJobs(r.Context(), tenantID, jobType, limit, offset)
	if err != nil {
		h.writeAppError(w, err)
		return
	}

	res := make([]dto.JobResponse, len(jobs))
	for i, j := range jobs {
		res[i] = dto.FromJob(j)
	}

	h.writeJSON(w, http.StatusOK, res)
}

// GetFailedJobDetail handles GET /api/v1/dlq/{id}.
//
// @Summary      Get failed job details
// @Description  Returns the full state of a failed job, including its error history.
// @Tags         dlq
// @Produce      json
// @Param        id   path      string  true  "Job ID"
// @Success      200  {object}  dto.JobResponse
// @Failure      404  {object}  dto.ErrorResponse
// @Router       /api/v1/dlq/{id} [get]
func (h *JobHandler) GetFailedJobDetail(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := r.Context().Value(middleware.ContextKeyTenantID).(string)
	id := r.PathValue("id")

	job, err := h.service.GetJobStatus(r.Context(), id)
	if err != nil {
		h.writeAppError(w, err)
		return
	}

	if job.TenantID != tenantID {
		h.writeError(w, http.StatusForbidden, apperr.CodePermissionDenied, "forbidden")
		return
	}

	h.writeJSON(w, http.StatusOK, dto.FromJob(job))
}

// ReplayFailedJob handles POST /api/v1/dlq/{id}/replay.
//
// @Summary      Re-enqueue a failed job
// @Description  Resets a failed job to 'pending' state and enqueues it for immediate retry.
// @Tags         dlq
// @Produce      json
// @Param        id   path      string  true  "Job ID"
// @Success      200  {object}  dto.JobResponse
// @Failure      404  {object}  dto.ErrorResponse
// @Router       /api/v1/dlq/{id}/replay [post]
func (h *JobHandler) ReplayFailedJob(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := r.Context().Value(middleware.ContextKeyTenantID).(string)
	id := r.PathValue("id")

	job, err := h.service.ReplayJob(r.Context(), id, tenantID)
	if err != nil {
		h.writeAppError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, dto.FromJob(job))
}

// DeleteFailedJob handles DELETE /api/v1/dlq/{id}.
//
// @Summary      Purge a failed job
// @Description  Permanently deletes a single failed job record from the store.
// @Tags         dlq
// @Param        id   path      string  true  "Job ID"
// @Success      204  "No Content"
// @Failure      404  {object}  dto.ErrorResponse
// @Router       /api/v1/dlq/{id} [delete]
func (h *JobHandler) DeleteFailedJob(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := r.Context().Value(middleware.ContextKeyTenantID).(string)
	id := r.PathValue("id")

	if err := h.service.DeleteJob(r.Context(), id, tenantID); err != nil {
		h.writeAppError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// BulkPurgeDLQ handles DELETE /api/v1/dlq.
//
// @Summary      Bulk purge failed jobs
// @Description  Deletes all failed jobs for a tenant/queue that are older than the specified timestamp.
// @Tags         dlq
// @Param        queue       query     string  false  "Queue type filter"
// @Param        older_than  query     string  true   "ISO8601 Timestamp"
// @Success      200         {object}  map[string]int64
// @Router       /api/v1/dlq [delete]
func (h *JobHandler) BulkPurgeDLQ(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := r.Context().Value(middleware.ContextKeyTenantID).(string)
	jobType := r.URL.Query().Get("queue")
	olderThanStr := r.URL.Query().Get("older_than")

	if olderThanStr == "" {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "older_than is required")
		return
	}

	olderThan, err := time.Parse(time.RFC3339, olderThanStr)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "invalid timestamp format")
		return
	}

	count, err := h.service.BulkPurgeDLQ(r.Context(), tenantID, jobType, olderThan)
	if err != nil {
		h.writeAppError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]int64{"deleted": count})
}
