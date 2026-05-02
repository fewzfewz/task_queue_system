// Package dto defines the request and response shapes for the HTTP API.
// Keeping them separate from the domain model (jobs.Job) means the API
// contract can evolve independently of the internal representation.
package dto

import "task-queue-system/internal/jobs"

// CreateJobRequest is the JSON body expected on POST /jobs.
type CreateJobRequest struct {
	// Type must match one of the registered handler types (e.g. "email", "image").
	Type string `json:"type"`
	// Payload is an arbitrary map of key/value pairs specific to the job type.
	Payload map[string]interface{} `json:"payload"`
	// MaxRetries defaults to 3 if omitted (zero value).
	MaxRetries int `json:"max_retries"`
}

// JobResponse is the JSON body returned for both POST /jobs and GET /jobs/{id}.
type JobResponse struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Payload    map[string]interface{} `json:"payload"`
	Status     string                 `json:"status"`
	Retries    int                    `json:"retries"`
	MaxRetries int                    `json:"max_retries"`
	CreatedAt  string                 `json:"created_at"`
	UpdatedAt  string                 `json:"updated_at"`
}

// ErrorResponse is the JSON body returned on any error.
type ErrorResponse struct {
	Error string `json:"error"`
}

// FromJob converts a domain Job into a JobResponse DTO.
func FromJob(j *jobs.Job) JobResponse {
	return JobResponse{
		ID:         j.ID,
		Type:       j.Type,
		Payload:    j.Payload,
		Status:     string(j.Status),
		Retries:    j.Retries,
		MaxRetries: j.MaxRetries,
		CreatedAt:  j.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:  j.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
