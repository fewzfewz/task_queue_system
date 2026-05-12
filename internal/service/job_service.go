package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/queue"
	"task-queue-system/internal/storage/models"
	apperr "task-queue-system/internal/errors"
)

// allowedJobTypes is the set of job types the system accepts.
var allowedJobTypes = map[string]struct{}{
	"email": {},
	"image": {},
	"test":           {},
	"test-success":   {},
	"test-fail":      {},
	"test-scheduled": {},
}

const defaultMaxRetries = 3

// JobService orchestrates job creation, state transitions, and coordination
// between the queue backend and the persistent datastore.
type JobService struct {
	queue        queue.Queue
	store        models.Store
	logger       *slog.Logger
	maxQueueSize int64
}

// New creates a JobService.
func New(q queue.Queue, store models.Store, logger *slog.Logger, maxQueueSize int64) *JobService {
	return &JobService{
		queue:        q,
		store:        store,
		logger:       logger,
		maxQueueSize: maxQueueSize,
	}
}

// CreateJob validates a new request, saves it to the DB, and enqueues it.
func (s *JobService) CreateJob(ctx context.Context, jobType string, payload map[string]interface{}, priority string, maxRetries int, runAtStr string, correlationID string) (*jobs.Job, error) {
	if _, ok := allowedJobTypes[jobType]; !ok {
		return nil, apperr.NewInvalidArgument(fmt.Sprintf("unsupported job type %q", jobType))
	}
	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}

	var runAt time.Time
	if runAtStr != "" {
		var err error
		runAt, err = time.Parse(time.RFC3339, runAtStr)
		if err != nil {
			return nil, apperr.NewInvalidArgument("invalid run_at timestamp: " + err.Error())
		}
		if runAt.Before(time.Now()) {
			return nil, apperr.NewInvalidArgument("run_at timestamp must be in the future")
		}
	}

	// ── Backpressure Check ───────────────────────────────────────────────────
	if s.maxQueueSize > 0 {
		count, err := s.queue.Size(ctx)
		if err == nil && count >= s.maxQueueSize {
			return nil, apperr.NewQueueFull()
		}
	}

	job := jobs.NewJob(jobType, payload, jobs.JobPriority(priority), maxRetries, runAt, correlationID)

	if err := s.store.Save(ctx, job); err != nil {
		s.logger.Error("failed to persist job", "job_id", job.ID, "error", err)
		return nil, apperr.NewInternal("failed to persist job", err)
	}

	if err := s.queue.Enqueue(ctx, job); err != nil {
		s.logger.Error("failed to enqueue job", "job_id", job.ID, "error", err)
		return nil, apperr.NewInternal("failed to enqueue job", err)
	}

	return job, nil
}

// ─── Queue Proxies ───────────────────────────────────────────────────────────

// Dequeue proxies the dequeue request to the underlying queue.
func (s *JobService) Dequeue(ctx context.Context) (*jobs.Job, error) {
	return s.queue.Dequeue(ctx)
}

// Enqueue proxies the enqueue request to the underlying queue.
func (s *JobService) Enqueue(ctx context.Context, job *jobs.Job) error {
	return s.queue.Enqueue(ctx, job)
}

// Ack proxies the acknowledgement to the underlying queue.
func (s *JobService) Ack(ctx context.Context, jobID string) error {
	return s.queue.Ack(ctx, jobID)
}

// Fail proxies the failure report to the underlying queue.
func (s *JobService) Fail(ctx context.Context, jobID string, err error) error {
	return s.queue.Fail(ctx, jobID, err)
}

// ─── GetJobStatus ───────────────────────────────────────────
func (s *JobService) GetJobStatus(ctx context.Context, jobID string) (*jobs.Job, error) {
	j, err := s.store.GetByID(ctx, jobID)
	if err != nil {
		return nil, apperr.NewInternal("database query failed", err)
	}
	if j == nil {
		return nil, apperr.NewNotFound("job", jobID)
	}
	return j, nil
}

// UpdateJobStatus updates the job state in the persistent store.
func (s *JobService) UpdateJobStatus(ctx context.Context, jobID string, status jobs.JobStatus, workerID string) error {
	if err := s.store.UpdateStatus(ctx, jobID, status, workerID); err != nil {
		s.logger.Error("failed to update job status", "job_id", jobID, "status", status, "worker", workerID, "error", err)
		return err
	}
	s.logger.Info("job status updated", "job_id", jobID, "status", status, "worker", workerID)
	return nil
}

// UpdateJobResult propagates execution outcomes (success values or error strings) to the store.
func (s *JobService) UpdateJobResult(ctx context.Context, jobID string, status jobs.JobStatus, workerID string, result interface{}) error {
	if err := s.store.UpdateResult(ctx, jobID, status, workerID, result); err != nil {
		return fmt.Errorf("service: failed to update job result: %w", err)
	}

	s.logger.Info("job result updated", "job_id", jobID, "status", status, "worker", workerID)
	return nil
}

// RegisterHeartbeat logs the worker's presence in the queue system.
func (s *JobService) RegisterHeartbeat(ctx context.Context, workerID string) error {
	return s.queue.RegisterHeartbeat(ctx, workerID)
}

// GetActiveWorkers returns information about all connected worker instances.
func (s *JobService) GetActiveWorkers(ctx context.Context) ([]queue.WorkerInfo, error) {
	return s.queue.GetActiveWorkers(ctx)
}

// GetMetrics returns system execution metrics.
func (s *JobService) GetMetrics(ctx context.Context) (queue.QueueMetrics, error) {
	return s.queue.GetMetrics(ctx)
}

// IsProcessed checks if the job already succeeded (idempotency guard).
func (s *JobService) IsProcessed(ctx context.Context, jobID string) (bool, error) {
	return s.queue.IsProcessed(ctx, jobID)
}

// MarkProcessed flags the job as having successfully completed.
func (s *JobService) MarkProcessed(ctx context.Context, jobID string) error {
	return s.queue.MarkProcessed(ctx, jobID)
}

// PromoteScheduledJobs pushes matured delayed tasks to active queues.
func (s *JobService) PromoteScheduledJobs(ctx context.Context) (int, error) {
	return s.queue.PromoteScheduledJobs(ctx)
}

// ReclaimTimedOutJobs finds and re-queues jobs from crashed workers.
func (s *JobService) ReclaimTimedOutJobs(ctx context.Context) (int, error) {
	return s.queue.ReclaimTimedOutJobs(ctx)
}

