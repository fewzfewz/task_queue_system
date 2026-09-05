package tq

import "time"

// JobStatus represents the state of a job.
type JobStatus string

const (
	StatusPending    JobStatus = "pending"
	StatusProcessing JobStatus = "processing"
	StatusCompleted  JobStatus = "completed"
	StatusFailed     JobStatus = "failed"
	StatusCancelled  JobStatus = "cancelled"
	StatusRecurring  JobStatus = "recurring"
)

// WebhookConfig specifies a callback URL and the events that trigger it.
type WebhookConfig struct {
	URL    string   `json:"url"`
	Secret string   `json:"secret,omitempty"`
	Events []string `json:"events,omitempty"`
}

// SubmitJobRequest holds the data for submitting a new job.
type SubmitJobRequest struct {
	Type             string                 `json:"type"`
	Payload          map[string]interface{} `json:"payload"`
	Labels           map[string]string      `json:"labels,omitempty"`
	Priority         string                 `json:"priority,omitempty"`
	MaxRetries       *int                   `json:"max_retries,omitempty"`
	BackoffAlgorithm string                 `json:"backoff_algorithm,omitempty"`
	BackoffJitter    string                 `json:"backoff_jitter,omitempty"`
	CronExpr         string                 `json:"cron_expr,omitempty"`
	RunAt            *time.Time             `json:"run_at,omitempty"`
	CorrelationID    string                 `json:"correlation_id,omitempty"`
	Timeout          int                    `json:"timeout_seconds,omitempty"`
	Version          int                    `json:"version,omitempty"`
	Webhook          *WebhookConfig         `json:"webhook,omitempty"`
	DedupKey         string                 `json:"dedup_key,omitempty"`
	Dependencies     []string               `json:"dependencies,omitempty"`
	ShardKey         string                 `json:"shard_key,omitempty"`
}

// Job represents a job record returned by the API.
type Job struct {
	ID            string                 `json:"id"`
	TenantID      string                 `json:"tenant_id"`
	Type          string                 `json:"type"`
	Payload       map[string]interface{} `json:"payload"`
	Status        JobStatus              `json:"status"`
	Priority      string                 `json:"priority"`
	Retries       int                    `json:"retries"`
	MaxRetries    int                    `json:"max_retries"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	Timeout       int                    `json:"timeout_seconds"`
	Version       int                    `json:"version"`
	Progress      float64                `json:"progress"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	RunAt         time.Time              `json:"run_at"`
	ProcessedBy   string                 `json:"processed_by,omitempty"`
	Result        interface{}            `json:"result,omitempty"`
	Error         string                 `json:"error,omitempty"`
	DedupKey      string                 `json:"dedup_key,omitempty"`
	Dependencies  []string               `json:"dependencies,omitempty"`
	ShardKey      string                 `json:"shard_key,omitempty"`
	CronExpr      string                 `json:"cron_expr,omitempty"`
	ErrorHistory  []JobErrorRecord       `json:"error_history,omitempty"`
	Webhook       *WebhookConfig         `json:"webhook,omitempty"`
}

// JobErrorRecord records a previous failure attempt.
type JobErrorRecord struct {
	Attempt   int       `json:"attempt"`
	Error     string    `json:"error"`
	Timestamp time.Time `json:"timestamp"`
}

// Webhook represents a registered global webhook.
type Webhook struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BatchJobResponse is the response from a batch submission.
type BatchJobResponse struct {
	Total      int              `json:"total"`
	Processed  int              `json:"processed"`
	Successful []BatchJobResult `json:"successful"`
	Failed     []BatchJobError  `json:"failed"`
}

// BatchJobResult contains the successful job response.
type BatchJobResult struct {
	Index int  `json:"index"`
	Job   *Job `json:"job"`
}

// BatchJobError contains the failure reason.
type BatchJobError struct {
	Index int    `json:"index"`
	Error string `json:"error"`
}

// ListJobsResponse is the response from a search/list query.
type ListJobsResponse struct {
	Jobs  []Job `json:"jobs"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Limit int   `json:"limit"`
}
