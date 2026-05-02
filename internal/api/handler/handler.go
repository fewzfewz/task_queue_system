// Package handler provides the HTTP handlers for the task-queue API.
package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"task-queue-system/internal/api/dto"
	"task-queue-system/internal/api/service"
	"task-queue-system/internal/storage/models"
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
//	Request  body: dto.CreateJobRequest  (JSON)
//	Response body: dto.JobResponse       (JSON, 201 Created)
//	Error    body: dto.ErrorResponse     (JSON, 4xx / 5xx)
func (h *JobHandler) CreateJob(w http.ResponseWriter, r *http.Request) {
	// ── Parse ─────────────────────────────────────────────────────────────────
	var req dto.CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	defer r.Body.Close()

	// ── Delegate to service ───────────────────────────────────────────────────
	job, err := h.service.CreateJob(r.Context(), req.Type, req.Payload, req.MaxRetries)
	if err != nil {
		h.logger.Warn("create job failed", "error", err)
		// Validation errors are client mistakes; everything else is server-side.
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "failed to enqueue") {
			status = http.StatusInternalServerError
		}
		h.writeError(w, status, err.Error())
		return
	}

	h.logger.Info("job created", "job_id", job.ID, "job_type", job.Type)
	h.writeJSON(w, http.StatusCreated, dto.FromJob(job))
}

// GetJobStatus handles GET /jobs/{id}.
//
//	Response body: dto.JobResponse   (JSON, 200 OK)
//	Error    body: dto.ErrorResponse (JSON, 404 / 501 / 500)
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
		h.writeError(w, http.StatusBadRequest, "missing job ID in path")
		return
	}

	// ── Delegate to service ───────────────────────────────────────────────────
	job, err := h.service.GetJobStatus(r.Context(), jobID)
	if err != nil {
		h.logger.Warn("get job status failed", "job_id", jobID, "error", err)

		switch {
		case errors.Is(err, models.ErrJobNotFound):
			h.writeError(w, http.StatusNotFound, "job not found: "+jobID)
		default:
			h.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	h.writeJSON(w, http.StatusOK, dto.FromJob(job))
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
func (h *JobHandler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, dto.ErrorResponse{Error: message})
}

// sentinel used by tests / future middleware.
var ErrNotImplemented = errors.New("not implemented")
