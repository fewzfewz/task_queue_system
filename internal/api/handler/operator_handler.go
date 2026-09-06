package handler

import (
	"encoding/json"
	"net/http"
	apperr "task-queue-system/internal/errors"
)

// GetPausedQueues returns a list of all currently paused job types.
func (h *JobHandler) GetPausedQueues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	pausedTypes, err := h.service.GetPausedJobTypes(ctx)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, apperr.CodeInternal, err.Error())
		return
	}

	if pausedTypes == nil {
		pausedTypes = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"paused_queues": pausedTypes,
	})
}

// PauseQueue hits the panic button for a specific job type.
func (h *JobHandler) PauseQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	jobType := r.PathValue("type")

	if jobType == "" {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "job type is required")
		return
	}

	if err := h.service.PauseJobType(ctx, jobType); err != nil {
		h.writeError(w, http.StatusInternalServerError, apperr.CodeInternal, err.Error())
		return
	}

	h.logger.Warn("operator panic button engaged", "job_type", jobType, "admin_user", "operator")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "paused", "type": jobType})
}

// ResumeQueue clears the panic button for a specific job type.
func (h *JobHandler) ResumeQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	jobType := r.PathValue("type")

	if jobType == "" {
		h.writeError(w, http.StatusBadRequest, apperr.CodeInvalidArgument, "job type is required")
		return
	}

	if err := h.service.ResumeJobType(ctx, jobType); err != nil {
		h.writeError(w, http.StatusInternalServerError, apperr.CodeInternal, err.Error())
		return
	}

	h.logger.Info("operator panic button cleared", "job_type", jobType, "admin_user", "operator")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "resumed", "type": jobType})
}
