package handler

import (
	"net/http"

	"task-queue-system/internal/api/middleware"
	apperr "task-queue-system/internal/errors"
	"task-queue-system/internal/jobs"
)

// checkJobTenantAccess returns false (and writes a 403) when a client API key
// tries to access a job belonging to another tenant. Operator sessions and the
// static operator API key are unrestricted.
func (h *JobHandler) checkJobTenantAccess(w http.ResponseWriter, r *http.Request, job *jobs.Job) bool {
	ctxTenant := middleware.TenantIDFromContext(r.Context())
	if middleware.IsClientTenant(ctxTenant) && job.TenantID != ctxTenant {
		h.writeError(w, http.StatusForbidden, apperr.CodePermissionDenied, "access to job denied for tenant")
		return false
	}
	return true
}

// loadJobForTenant fetches a job and enforces client tenant isolation.
func (h *JobHandler) loadJobForTenant(w http.ResponseWriter, r *http.Request, jobID string) (*jobs.Job, bool) {
	job, err := h.service.GetJobStatus(r.Context(), jobID)
	if err != nil {
		h.writeAppError(w, err)
		return nil, false
	}
	if !h.checkJobTenantAccess(w, r, job) {
		return nil, false
	}
	return job, true
}
