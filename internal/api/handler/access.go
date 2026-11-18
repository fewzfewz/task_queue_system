package handler

import (
	"net/http"

	"task-queue-system/internal/api/middleware"
	apperr "task-queue-system/internal/errors"
	"task-queue-system/internal/jobs"
	"task-queue-system/internal/webhooks"
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

// checkWebhookTenantAccess returns false (and writes a 403) when a client API key
// tries to access a webhook belonging to another tenant. Operator sessions and the
// static operator API key are unrestricted.
func (h *JobHandler) checkWebhookTenantAccess(w http.ResponseWriter, r *http.Request, wh *webhooks.RegisteredWebhook) bool {
	ctxTenant := middleware.TenantIDFromContext(r.Context())
	if middleware.IsClientTenant(ctxTenant) && wh.TenantID != ctxTenant {
		h.writeError(w, http.StatusForbidden, apperr.CodePermissionDenied, "access to webhook denied for tenant")
		return false
	}
	return true
}

// loadWebhookForTenant fetches a webhook and enforces client tenant isolation.
func (h *JobHandler) loadWebhookForTenant(w http.ResponseWriter, r *http.Request, id string) (*webhooks.RegisteredWebhook, bool) {
	wh, err := h.webhookStore.GetByID(r.Context(), id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, apperr.CodeNotFound, "webhook not found")
		return nil, false
	}
	if !h.checkWebhookTenantAccess(w, r, wh) {
		return nil, false
	}
	return wh, true
}
