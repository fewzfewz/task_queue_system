// Package service contains the business logic that sits between the HTTP
// handlers and the queue / storage layers.
package service

import (
	"context"
	"errors"
	"fmt"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/queue"
	"task-queue-system/internal/storage/models"
)

// allowedJobTypes is the set of job types the system accepts.
// Extend this map when you register a new handler in the executor.
var allowedJobTypes = map[string]struct{}{
	"email": {},
	"image": {},
}

const defaultMaxRetries = 3

// ─── JobService ───────────────────────────────────────────────────────────────

// JobService orchestrates job creation and status retrieval.
// It is the single place where queue and storage are written to together,
// keeping both in sync on every mutation.
type JobService struct {
	queue queue.Queue
	store models.Store
}

// New creates a JobService. Both queue and store are required.
func New(q queue.Queue, store models.Store) *JobService {
	return &JobService{
		queue: q,
		store: store,
	}
}

// ─── CreateJob ────────────────────────────────────────────────────────────────

// CreateJob validates the request, builds a new Job, persists it to the store,
// and enqueues it — in that order. If enqueue fails after a successful save the
// job record is left in "pending" state; a periodic reconciler or manual retry
// can enqueue it again without data loss.
func (s *JobService) CreateJob(
	ctx context.Context,
	jobType string,
	payload map[string]interface{},
	maxRetries int,
) (*jobs.Job, error) {

	// ── 1. Validate ───────────────────────────────────────────────────────────
	if err := s.validateJobType(jobType); err != nil {
		return nil, err
	}
	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}

	// ── 2. Build ──────────────────────────────────────────────────────────────
	job := jobs.NewJob(jobType, payload, maxRetries)

	// ── 3. Persist ────────────────────────────────────────────────────────────
	if err := s.store.Save(ctx, job); err != nil {
		return nil, fmt.Errorf("service: failed to persist job: %w", err)
	}

	// ── 4. Enqueue ────────────────────────────────────────────────────────────
	if err := s.queue.Enqueue(ctx, job); err != nil {
		// Job is saved but not queued — safe to retry enqueue later.
		return nil, fmt.Errorf("service: job saved but failed to enqueue: %w", err)
	}

	return job, nil
}

// ─── GetJobStatus ─────────────────────────────────────────────────────────────

// GetJobStatus retrieves the current state of a job from the store.
// Returns a wrapped ErrJobNotFound when the ID does not exist so callers can
// distinguish not-found from other errors with errors.Is.
func (s *JobService) GetJobStatus(ctx context.Context, jobID string) (*jobs.Job, error) {
	if jobID == "" {
		return nil, fmt.Errorf("service: job ID is required")
	}

	job, err := s.store.GetByID(ctx, jobID)
	if err != nil {
		if errors.Is(err, models.ErrJobNotFound) {
			return nil, fmt.Errorf("service: %w", models.ErrJobNotFound)
		}
		return nil, fmt.Errorf("service: store lookup failed: %w", err)
	}

	return job, nil
}

// ─── UpdateJobStatus ──────────────────────────────────────────────────────────

// UpdateJobStatus changes a job's status in the store.
// Called by the worker layer after Ack or Fail to keep the store in sync.
func (s *JobService) UpdateJobStatus(ctx context.Context, jobID string, status jobs.JobStatus) error {
	if jobID == "" {
		return fmt.Errorf("service: job ID is required")
	}

	if err := s.store.UpdateStatus(ctx, jobID, status); err != nil {
		if errors.Is(err, models.ErrJobNotFound) {
			return fmt.Errorf("service: %w", models.ErrJobNotFound)
		}
		return fmt.Errorf("service: failed to update job status: %w", err)
	}

	return nil
}

// ─── private helpers ──────────────────────────────────────────────────────────

func (s *JobService) validateJobType(jobType string) error {
	if jobType == "" {
		return fmt.Errorf("service: job type is required")
	}
	if _, ok := allowedJobTypes[jobType]; !ok {
		return fmt.Errorf("service: unsupported job type %q", jobType)
	}
	return nil
}
