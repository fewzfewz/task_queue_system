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

// JobPriority defines the execution urgency for a job.
type JobPriority string

const (
	PriorityLow    JobPriority = "low"
	PriorityMedium JobPriority = "medium"
	PriorityHigh   JobPriority = "high"
)

// WebhookConfig holds the configuration for job status callbacks.
type WebhookConfig struct {
	URL        string   `json:"url,omitempty"`
	Secret     string   `json:"secret,omitempty"`
	Events     []string `json:"events,omitempty"`
	LastStatus int      `json:"last_status,omitempty"`
	Attempts   int      `json:"attempts,omitempty"`
}

// AttemptError records a single failure instance.
type AttemptError struct {
	Error     string    `json:"error"`
	Timestamp time.Time `json:"timestamp"`
	Attempt   int       `json:"attempt"`
}



// Job represents a task in the distributed task queue system.
type Job struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Payload    map[string]interface{} `json:"payload"`
	Status     JobStatus              `json:"status"`
	Priority   JobPriority            `json:"priority"`
	Retries    int                    `json:"retries"`
	MaxRetries int                    `json:"max_retries"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	RunAt       time.Time              `json:"run_at"`
	ProcessedBy   string                 `json:"processed_by,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	Timeout       int                    `json:"timeout,omitempty"` // in seconds
	Version       int                    `json:"version"`           // schema version
	TenantID      string                 `json:"tenant_id"`         // for multi-tenancy
	Result        interface{}            `json:"result,omitempty"`
	Webhook       *WebhookConfig         `json:"webhook,omitempty"`
	ErrorHistory  []AttemptError         `json:"error_history,omitempty"`
}



// NewJob creates a new Job instance with initial values.
// The job will be initialized with a new UUID, pending status, and zero retries.
func NewJob(jobType string, payload map[string]interface{}, priority JobPriority, maxRetries int, runAt time.Time, correlationID string, timeout int, version int, tenantID string) *Job {
	if priority == "" {
		priority = PriorityMedium
	}

	if correlationID == "" {
		correlationID = uuid.New().String()
	}

	if version <= 0 {
		version = 1 // default schema version
	}
	
	now := time.Now().UTC()
	if runAt.IsZero() {
		runAt = now
	}

	return &Job{
		ID:            uuid.New().String(),
		Type:          jobType,
		Payload:       payload,
		Status:        StatusPending,
		Priority:      priority,
		Retries:       0,
		MaxRetries:    maxRetries,
		CreatedAt:     now,
		UpdatedAt:     now,
		RunAt:         runAt,
		CorrelationID: correlationID,
		Timeout:       timeout,
		Version:       version,
		TenantID:      tenantID,
	}
}
