package jobs

import (
	"time"

	"github.com/google/uuid"
)

// JobStatus defines the type for the status of a job.
type JobStatus string

const (
	// StatusPending indicates the job is waiting to be processed.
	StatusPending JobStatus = "pending"
	// StatusProcessing indicates the job is currently being processed.
	StatusProcessing JobStatus = "processing"
	// StatusCompleted indicates the job has finished successfully.
	StatusCompleted JobStatus = "completed"
	// StatusFailed indicates the job has failed after all retries.
	StatusFailed JobStatus = "failed"
)

// Job represents a task in the distributed task queue system.
type Job struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Payload    map[string]interface{} `json:"payload"`
	Status     JobStatus              `json:"status"`
	Retries    int                    `json:"retries"`
	MaxRetries int                    `json:"max_retries"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

// NewJob creates a new Job instance with initial values.
// The job will be initialized with a new UUID, pending status, and zero retries.
func NewJob(jobType string, payload map[string]interface{}, maxRetries int) *Job {
	now := time.Now().UTC()
	return &Job{
		ID:         uuid.New().String(),
		Type:       jobType,
		Payload:    payload,
		Status:     StatusPending,
		Retries:    0,
		MaxRetries: maxRetries,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}
