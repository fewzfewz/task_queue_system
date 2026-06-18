package jobs

import (
	"math/rand"
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
	// StatusCancelled indicates the job was cancelled before completion.
	StatusCancelled JobStatus = "cancelled"
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



type BackoffAlgorithm string

const (
	BackoffExponential BackoffAlgorithm = "exponential"
	BackoffLinear      BackoffAlgorithm = "linear"
	BackoffFixed       BackoffAlgorithm = "fixed"
)

type BackoffJitter string

const (
	JitterNone  BackoffJitter = "none"
	JitterFull  BackoffJitter = "full"
	JitterEqual BackoffJitter = "equal"
)

// Job represents a task in the distributed task queue system.
type Job struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Payload    map[string]interface{} `json:"payload"`
	Labels     map[string]string      `json:"labels,omitempty"`
	Status     JobStatus              `json:"status"`
	Priority   JobPriority            `json:"priority"`
	Retries    int                    `json:"retries"`
	MaxRetries int                    `json:"max_retries"`
	BackoffAlgorithm BackoffAlgorithm `json:"backoff_algorithm,omitempty"`
	BackoffJitter    BackoffJitter    `json:"backoff_jitter,omitempty"`
	CronExpr      string               `json:"cron_expr,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	RunAt       time.Time              `json:"run_at"`
	ProcessedBy   string                 `json:"processed_by,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	Timeout       int                    `json:"timeout,omitempty"` // in seconds
	Version       int                    `json:"version"`           // schema version
	TenantID      string                 `json:"tenant_id"`         // for multi-tenancy
	Paused        bool                   `json:"paused"`
	Progress      float64                `json:"progress"`
	Result        interface{}            `json:"result,omitempty"`
	Webhook       *WebhookConfig         `json:"webhook,omitempty"`
	DedupKey      string                 `json:"dedup_key,omitempty"`
	Dependencies  []string               `json:"dependencies,omitempty"`
	ShardKey      string                 `json:"shard_key,omitempty"`
	ErrorHistory  []AttemptError         `json:"error_history,omitempty"`
}



// NewJob creates a new Job instance with initial values.
// The job will be initialized with a new UUID, pending status, and zero retries.
func NewJob(jobType string, payload map[string]interface{}, labels map[string]string, priority JobPriority, maxRetries int, runAt time.Time, correlationID string, timeout int, version int, tenantID string) *Job {
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

	if labels == nil {
		labels = make(map[string]string)
	}

	return &Job{
		ID:            uuid.New().String(),
		Type:          jobType,
		Payload:       payload,
		Labels:        labels,
		Status:        StatusPending,
		Priority:      priority,
		Retries:       0,
		MaxRetries:    maxRetries,
		BackoffAlgorithm: BackoffExponential,
		BackoffJitter:    JitterNone,
		CreatedAt:     now,
		UpdatedAt:     now,
		RunAt:         runAt,
		CorrelationID: correlationID,
		Timeout:       timeout,
		Version:       version,
		TenantID:      tenantID,
	}
}

// BackoffDelay computes the retry delay for a job based on its algorithm and
// jitter settings. The base unit is 1 second.
func BackoffDelay(job *Job) time.Duration {
	if job == nil {
		return time.Duration(1<<1) * time.Second
	}
	retry := job.Retries
	if retry <= 0 {
		retry = 1
	}

	var delay time.Duration
	switch job.BackoffAlgorithm {
	case BackoffLinear:
		delay = time.Duration(retry) * time.Second
	case BackoffFixed:
		delay = 1 * time.Second
	default: // exponential
		delay = time.Duration(1<<retry) * time.Second
	}

	switch job.BackoffJitter {
	case JitterFull:
		n := int64(delay)
		if n <= 0 {
			n = 1
		}
		delay = time.Duration(rand.Int63n(n))
	case JitterEqual:
		n := int64(delay)
		if n <= 0 {
			n = 1
		}
		half := n / 2
		if half <= 0 {
			half = 1
		}
		delay = time.Duration(half + rand.Int63n(half))
	}

	return delay
}
