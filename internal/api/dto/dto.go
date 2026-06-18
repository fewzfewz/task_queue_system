// Package dto defines the request and response shapes for the HTTP API.
// Keeping them separate from the domain model (jobs.Job) means the API
// contract can evolve independently of the internal representation.
package dto

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"

	"task-queue-system/internal/jobs"
)

// WebhookRequest configures the callback settings for a job.
type WebhookRequest struct {
	URL    string   `json:"url"`
	Secret string   `json:"secret"`
	Events []string `json:"events"`
}


// CreateJobRequest is the JSON body expected on POST /jobs.
type CreateJobRequest struct {
	// Type must match one of the registered handler types (e.g. "email", "image").
	Type string `json:"type"`
	// Payload is an arbitrary map of key/value pairs specific to the job type.
	Payload map[string]interface{} `json:"payload"`
	// Labels is an optional arbitrary key/value metadata map for filtering and grouping.
	Labels map[string]string `json:"labels,omitempty"`
	// Priority is optional, defaults to "medium"
	Priority string `json:"priority"`
	// MaxRetries defaults to 3 if omitted (zero value).
	MaxRetries int `json:"max_retries"`
	// BackoffAlgorithm is the retry backoff strategy: "exponential", "linear", or "fixed". Default: "exponential".
	BackoffAlgorithm string `json:"backoff_algorithm,omitempty"`
	// BackoffJitter adds randomness to the delay: "none", "full", or "equal". Default: "none".
	BackoffJitter string `json:"backoff_jitter,omitempty"`
	// CronExpr is an optional cron expression for recurring jobs (e.g. "*/5 * * * *").
	CronExpr string `json:"cron_expr,omitempty"`
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
	// DedupKey is an optional idempotency key for exactly-once deduplication.
	DedupKey string `json:"dedup_key,omitempty"`
	// Dependencies is an optional list of job IDs that must complete before this job runs.
	Dependencies []string `json:"dependencies,omitempty"`
	// ShardKey is an optional key for distributing jobs across queue partitions.
	ShardKey string `json:"shard_key,omitempty"`
	// Webhook is optional.
	Webhook *WebhookRequest `json:"webhook"`
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

	// ── Cron Expression Validation ─────────────────────────────────────────
	if r.CronExpr != "" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if _, err := parser.Parse(r.CronExpr); err != nil {
			return fmt.Errorf("invalid cron_expr: %w", err)
		}
	}

	// ── Backoff Validation ─────────────────────────────────────────────────
	if r.BackoffAlgorithm != "" {
		switch r.BackoffAlgorithm {
		case "exponential", "linear", "fixed":
		default:
			return fmt.Errorf("invalid backoff_algorithm %q: must be exponential, linear, or fixed", r.BackoffAlgorithm)
		}
	}
	if r.BackoffJitter != "" {
		switch r.BackoffJitter {
		case "none", "full", "equal":
		default:
			return fmt.Errorf("invalid backoff_jitter %q: must be none, full, or equal", r.BackoffJitter)
		}
	}

	// ── Exactly-Once DedupKey Validation ───────────────────────────────────
	// (No format validation — the store checks for collisions at creation time.)

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
	Labels     map[string]string      `json:"labels,omitempty"`
	Priority   string                 `json:"priority"`
	Status     string                 `json:"status"`
	Retries    int                    `json:"retries"`
	MaxRetries int                    `json:"max_retries"`
	BackoffAlgorithm string           `json:"backoff_algorithm,omitempty"`
	BackoffJitter    string           `json:"backoff_jitter,omitempty"`
	CronExpr         string           `json:"cron_expr,omitempty"`
	CreatedAt  string                 `json:"created_at"`
	UpdatedAt  string                 `json:"updated_at"`
	RunAt      string                 `json:"run_at"`
	CorrelationID string              `json:"correlation_id,omitempty"`
	Timeout       int                 `json:"timeout,omitempty"`
	Version       int                 `json:"version"`
	TenantID      string              `json:"tenant_id,omitempty"`
	Paused        bool                 `json:"paused"`
	Progress      float64              `json:"progress"`
	DedupKey      string               `json:"dedup_key,omitempty"`
	Dependencies  []string             `json:"dependencies,omitempty"`
	ShardKey      string               `json:"shard_key,omitempty"`
	ErrorHistory  []jobs.AttemptError `json:"error_history,omitempty"`
}


// BatchJobRequest is the JSON body expected on POST /jobs/batch.
type BatchJobRequest struct {
	Jobs []CreateJobRequest `json:"jobs"`
}

// BatchJobResponse is returned from POST /jobs/batch.
type BatchJobResponse struct {
	Successful []BatchJobResult `json:"successful"`
	Failed     []BatchJobError  `json:"failed,omitempty"`
	Total      int              `json:"total"`
	Processed  int              `json:"processed"`
}

type BatchJobResult struct {
	Index int          `json:"index"`
	Job   *JobResponse `json:"job"`
}

type BatchJobError struct {
	Index int    `json:"index"`
	Error string `json:"error"`
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
		Labels:     j.Labels,
		Priority:   string(j.Priority),
		Status:     string(j.Status),
		Retries:    j.Retries,
		MaxRetries: j.MaxRetries,
		BackoffAlgorithm: string(j.BackoffAlgorithm),
		BackoffJitter:    string(j.BackoffJitter),
		CronExpr:         j.CronExpr,
		CreatedAt:     j.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     j.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		RunAt:         j.RunAt.Format("2006-01-02T15:04:05Z07:00"),
		Paused:        j.Paused,
		Progress:      j.Progress,
		DedupKey:      j.DedupKey,
		Dependencies:  j.Dependencies,
		ShardKey:      j.ShardKey,
		CorrelationID: j.CorrelationID,
		Timeout:       j.Timeout,
		Version:       j.Version,
		TenantID:      j.TenantID,
		ErrorHistory:  j.ErrorHistory,
	}
}

// Validate performs strict input validation on the batch request.
func (r *BatchJobRequest) Validate() error {
	if len(r.Jobs) == 0 {
		return fmt.Errorf("at least one job is required")
	}
	if len(r.Jobs) > 100 {
		return fmt.Errorf("batch size exceeds maximum of 100")
	}
	for i, j := range r.Jobs {
		if err := j.Validate(); err != nil {
			return fmt.Errorf("job[%d]: %w", i, err)
		}
	}
	return nil
}
