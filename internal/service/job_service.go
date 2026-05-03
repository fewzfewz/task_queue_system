package service

import (
	"context"
	"fmt"
	"log/slog"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/queue"
	"task-queue-system/internal/storage/models"
)

// allowedJobTypes is the set of job types the system accepts.
var allowedJobTypes = map[string]struct{}{
	"email": {},
	"image": {},
}

const defaultMaxRetries = 3

// JobService orchestrates job creation, state transitions, and coordination
// between the queue backend and the persistent datastore.
type JobService struct {
	queue  queue.Queue
	store  models.Store
	logger *slog.Logger
}

// New creates a JobService.
func New(q queue.Queue, store models.Store, logger *slog.Logger) *JobService {
	return &JobService{
		queue:  q,
		store:  store,
		logger: logger,
	}
}

// CreateJob validates a new request, saves it to the DB, and enqueues it.
func (s *JobService) CreateJob(ctx context.Context, jobType string, payload map[string]interface{}, priority string, maxRetries int) (*jobs.Job, error) {
	if _, ok := allowedJobTypes[jobType]; !ok {
		return nil, fmt.Errorf("service: unsupported job type %q", jobType)
	}
	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}

	job := jobs.NewJob(jobType, payload, jobs.JobPriority(priority), maxRetries)

	if err := s.store.Save(ctx, job); err != nil {
		s.logger.Error("failed to persist job", "job_id", job.ID, "error", err)
		return nil, err
	}

	if err := s.queue.Enqueue(ctx, job); err != nil {
		s.logger.Error("failed to enqueue job", "job_id", job.ID, "error", err)
		return nil, err
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
	return s.store.GetByID(ctx, jobID)
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

// GetMetrics returns system execution metrics.
func (s *JobService) GetMetrics(ctx context.Context) (queue.QueueMetrics, error) {
	return s.queue.GetMetrics(ctx)
}
