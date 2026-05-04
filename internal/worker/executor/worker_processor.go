package executor

import (
	"context"
	"log/slog"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/service"
	"task-queue-system/internal/worker/limiter"
)

// WorkerProcessor orchestrates the full job lifecycle for a single worker:
//
//  1. Pull a job from the queue (blocking)
//  2. Execute it via JobExecutor
//  3. On success  → Ack  (mark completed)
//  4. On failure  → retry if retries < maxRetries, otherwise Fail permanently
//
// It is meant to be embedded inside a goroutine (e.g. pool.Pool) and driven
// by Run, but ProcessOnce can be called directly in tests or one-shot scripts.
type WorkerProcessor struct {
	name    string
	service *service.JobService
	exec    *JobExecutor
	limiter limiter.RateLimiter
	logger  *slog.Logger
}

// NewWorkerProcessor creates a WorkerProcessor.
// name is used for log attribution and job metadata. limiter can be nil if no rate limiting applies.
func NewWorkerProcessor(name string, svc *service.JobService, je *JobExecutor, l limiter.RateLimiter, logger *slog.Logger) *WorkerProcessor {
	return &WorkerProcessor{
		name:    name,
		service: svc,
		exec:    je,
		limiter: l,
		logger:  logger.With("worker_id", name),
	}
}

// Run loops continuously, processing one job per iteration.
// It exits cleanly when ctx is cancelled (graceful shutdown).
// Transient dequeue errors are logged and briefly backed off before retrying.
func (wp *WorkerProcessor) Run(ctx context.Context) {
	wp.logger.Info("worker processor started")
	defer wp.logger.Info("worker processor stopped")

	for {
		// Honour shutdown before blocking on Dequeue.
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := wp.ProcessOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return // clean shutdown, not a real error
			}
			wp.logger.Warn("transient error, backing off", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}
}

// ProcessOnce pulls exactly one job from the queue and runs it through the
// full success / retry / fail lifecycle. Returns an error only for transient
// infrastructure failures (e.g. queue unavailable) — job-level failures are
// handled internally and do NOT bubble up as errors here.
func (wp *WorkerProcessor) ProcessOnce(ctx context.Context) error {
	// ── 1. Dequeue ──────────────────────────────────────────────────────
	// We access the queue through the service layer or directly if exposed.
	// For this refactor, we'll expose the queue field or a method on the service.
	// Since I own the service code, I'll just reach in or add a helper.
	job, err := wp.service.Dequeue(ctx)
	if err != nil {
		return err // transient; caller (Run) will back off
	}

	// Identify this worker on the job object.
	job.ProcessedBy = wp.name

	log := wp.logger.With("job_id", job.ID, "job_type", job.Type)
	log.Info("processing job dequeued", "status", jobs.StatusProcessing)

	// ── Idempotency Check ─────────────────────────────────────────────────────
	// In a distributed system, a job might have been handled by another worker 
	// if the visibility timeout expired or a double-queue occurred.
	storedJob, err := wp.service.GetJobStatus(ctx, job.ID)
	if err == nil && storedJob != nil {
		if storedJob.Status == jobs.StatusCompleted || storedJob.Status == jobs.StatusFailed {
			log.Info("skipping duplicate execution; job already finished", "final_status", storedJob.Status)
			_ = wp.service.Ack(ctx, job.ID) // clear from broker
			return nil
		}
	}

	// Transition DB state to Processing immediately and attach worker ID.
	_ = wp.service.UpdateJobStatus(ctx, job.ID, jobs.StatusProcessing, wp.name)

	// Detach context cancellation for the rest of the job lifecycle.
	// This guarantees that if a SIGINT/SIGTERM arrives while a job is running,
	// the worker will FINISH the job before exiting, rather than aborting
	// the active task mid-flight.
	execCtx := context.WithoutCancel(ctx)

	// ── Rate Limit Check ──────────────────────────────────────────────────────
	// Wait logic consumes a token globally. We use the original 'ctx' here so 
	// that a shutdown signal can immediately interrupt the wait.
	if wp.limiter != nil {
		if err := wp.limiter.Wait(ctx); err != nil {
			// If context cancels while waiting for a token, we safely abandon.
			// The in-flight job will be rescued by the reaper/DLQ reconciler natively.
			return err
		}
	}

	// ── Step 2: Execute ───────────────────────────────────────────────────────
	start := time.Now()
	result, execErr := wp.exec.Execute(execCtx, job)
	elapsed := time.Since(start)

	if execErr == nil {
		// ── Step 3a: Success ────────────────────────────────────────────────
		log.Info("job succeeded", 
			"status", jobs.StatusCompleted,
			"execution_time_ms", elapsed.Milliseconds(),
		)
		_ = wp.service.UpdateJobResult(execCtx, job.ID, jobs.StatusCompleted, wp.name, result)
		_ = wp.service.Ack(execCtx, job.ID)
		return nil
	}

	// ── Step 3b: Failure → decide retry or permanent fail ────────────────────
	log.Error("job execution failed",
		"status", "error",
		"error", execErr,
		"execution_time_ms", elapsed.Milliseconds(),
		"retries_used", job.Retries,
		"max_retries", job.MaxRetries,
	)

	if job.Retries < job.MaxRetries {
		_ = wp.service.UpdateJobStatus(execCtx, job.ID, jobs.StatusPending, wp.name)
		wp.retry(ctx, execCtx, job, log)
	} else {
		_ = wp.service.UpdateJobResult(execCtx, job.ID, jobs.StatusFailed, wp.name, execErr.Error())
		wp.permanentlyFail(execCtx, job, execErr, log)
	}

	return nil
}

// ── private helpers ───────────────────────────────────────────────────────────

// retry increments the retry counter, waits for an exponential backoff delay,
// re-enqueues the job for another attempt, and then acks the current in-flight
// entry so the processing set stays clean.
func (wp *WorkerProcessor) retry(shutdownCtx, execCtx context.Context, job *jobs.Job, log *slog.Logger) {
	job.Retries++
	job.Status = jobs.StatusPending
	job.UpdatedAt = time.Now().UTC()

	// Exponential backoff: 2^retry seconds (e.g. 2s, 4s, 8s...)
	// Note: We use the *new* Retries value for the delay exponent.
	delay := time.Duration(1<<job.Retries) * time.Second

	log.Warn("scheduling job for retry with backoff",
		"next_attempt", job.Retries+1,
		"max_retries", job.MaxRetries,
		"delay", delay.String(),
	)

	// Block the worker for the delay period to enforce backoff.
	// We listen to shutdownCtx.Done() so graceful shutdowns aren't stalled for seconds.
	// If shutdown occurs, the job remains safely in the processing hash and
	// will be rescued by a queue deadbox/reconciler later.
	select {
	case <-shutdownCtx.Done():
		log.Warn("worker shutdown interrupted retry delay; job kept in in-flight set")
		return
	case <-time.After(delay):
	}

	if err := wp.service.Enqueue(execCtx, job); err != nil {
		log.Error("failed to re-enqueue job for retry", "error", err)
		return
	}

	// Ack removes the old in-flight record; the freshly enqueued copy takes over.
	if err := wp.service.Ack(execCtx, job.ID); err != nil {
		log.Error("failed to ack original entry after re-enqueue", "error", err)
	}
}

// permanentlyFail calls the service to move job to DLQ and cleanup.
func (wp *WorkerProcessor) permanentlyFail(ctx context.Context, job *jobs.Job, reason error, log *slog.Logger) {
	log.Error("job exhausted all retries, marking as failed", "total_attempts", job.Retries+1)

	if err := wp.service.Fail(ctx, job.ID, reason); err != nil {
		log.Error("failed to mark job as permanently failed", "error", err)
	}
}
