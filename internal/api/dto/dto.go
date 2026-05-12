// Package dto defines the request and response shapes for the HTTP API.
// Keeping them separate from the domain model (jobs.Job) means the API
// contract can evolve independently of the internal representation.
package dto

import (
	"fmt"
	"time"

	"task-queue-system/internal/jobs"
)

// CreateJobRequest is the JSON body expected on POST /jobs.
type CreateJobRequest struct {
	// Type must match one of the registered handler types (e.g. "email", "image").
	Type string `json:"type"`
	// Payload is an arbitrary map of key/value pairs specific to the job type.
	Payload map[string]interface{} `json:"payload"`
	// Priority is optional, defaults to "medium"
	Priority string `json:"priority"`
	// MaxRetries defaults to 3 if omitted (zero value).
	MaxRetries int `json:"max_retries"`
	// RunAt is optional. If provided and in the future, the job will be scheduled.
	RunAt string `json:"run_at"`
	// CorrelationID is optional. If provided, it will be used for tracing logs.
	CorrelationID string `json:"correlation_id"`
	// Timeout is optional (in seconds). Default: 60s
	Timeout int `json:"timeout"`
	// Version is optional. Default: 1
	Version int `json:"version"`
	// TenantID is for multi-tenancy.
	TenantID string `json:"tenant_id"`
}

// Validate performs strict input validation on the request.
func (r *CreateJobRequest) Validate() error {
	if r.Type == "" {
		return fmt.Errorf("job type is required")
	}

	if r.Payload == nil {
		return fmt.Errorf("payload is required")
	}

	// ── Type-Specific Payload Validation ───────────────────────────────────
	switch r.Type {
	case "email":
		if to, _ := r.Payload["to"].(string); to == "" {
			return fmt.Errorf("email jobs require a 'to' field in the payload")
		}
	case "image":
		if url, _ := r.Payload["source_url"].(string); url == "" {
			return fmt.Errorf("image jobs require a 'source_url' field in the payload")
		}
	}

	// ── Timestamp Validation ───────────────────────────────────────────────
	if r.RunAt != "" {
		runAt, err := time.Parse(time.RFC3339, r.RunAt)
		if err != nil {
			return fmt.Errorf("invalid run_at format (wait for RFC3339): %w", err)
		}
		if runAt.Before(time.Now().Add(-1 * time.Minute)) {
			return fmt.Errorf("run_at cannot be in the past")
		}
	}

	return nil
}


// JobResponse is the JSON body returned for both POST /jobs and GET /jobs/{id}.
type JobResponse struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Payload    map[string]interface{} `json:"payload"`
	Priority   string                 `json:"priority"`
	Status     string                 `json:"status"`
	Retries    int                    `json:"retries"`
	MaxRetries int                    `json:"max_retries"`
	CreatedAt  string                 `json:"created_at"`
	UpdatedAt  string                 `json:"updated_at"`
	RunAt      string                 `json:"run_at"`
	CorrelationID string              `json:"correlation_id,omitempty"`
	Timeout       int                 `json:"timeout,omitempty"`
	Version       int                 `json:"version"`
	TenantID      string              `json:"tenant_id,omitempty"`
}

// ErrorResponse is the JSON body returned on any error.
type ErrorResponse struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}

// FromJob converts a domain Job into a JobResponse DTO.
func FromJob(j *jobs.Job) JobResponse {
	return JobResponse{
		ID:         j.ID,
		Type:       j.Type,
		Payload:    j.Payload,
		Priority:   string(j.Priority),
		Status:     string(j.Status),
		Retries:    j.Retries,
		MaxRetries: j.MaxRetries,
		CreatedAt:     j.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     j.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		RunAt:         j.RunAt.Format("2006-01-02T15:04:05Z07:00"),
		CorrelationID: j.CorrelationID,
		Timeout:       j.Timeout,
		Version:       j.Version,
		TenantID:      j.TenantID,
	}
}
