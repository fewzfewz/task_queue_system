// Package handler provides the HTTP handlers for the task-queue API.
package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"task-queue-system/internal/api/dto"
	"task-queue-system/internal/api/middleware"
	apperr "task-queue-system/internal/errors"
	"task-queue-system/internal/jobs"
	"task-queue-system/internal/service"
	"strconv"
	"time"
)

// JobHandler holds the dependencies for the job-related HTTP handlers.
type JobHandler struct {
	service *service.JobService
	logger  *slog.Logger
}

// New creates a JobHandler.
func New(svc *service.JobService, logger *slog.Logger) *JobHandler {
	return &JobHandler{service: svc, logger: logger}
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

	job, err := h.service.CreateJob(r.Context(), req.Type, req.Payload, req.Priority, req.MaxRetries, req.RunAt, req.CorrelationID, req.Timeout, req.Version, req.TenantID, webhookConfig)
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
