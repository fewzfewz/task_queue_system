// Package handler provides the HTTP handlers for the task-queue API.
package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"task-queue-system/internal/api/dto"
	"task-queue-system/internal/service"
	apperr "task-queue-system/internal/errors"
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

	// ── Delegate to service ───────────────────────────────────────────────────
	job, err := h.service.CreateJob(r.Context(), req.Type, req.Payload, req.Priority, req.MaxRetries, req.RunAt, req.CorrelationID)
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
	metrics, err := h.service.GetMetrics(r.Context())
	if err != nil {
		h.writeAppError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, metrics)
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
	case apperr.CodeQueueFull:
		status = http.StatusTooManyRequests
	case apperr.CodeInternal:
		status = http.StatusInternalServerError
	}

	h.writeError(w, status, appErr.Code, appErr.Message)
}

// sentinel used by tests / future middleware.
var ErrNotImplemented = errors.New("not implemented")
