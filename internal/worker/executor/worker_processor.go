package executor

import (
	"context"
	"log/slog"
	"time"

	"golang.org/x/time/rate"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/queue"
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
	id       int
	queue    queue.Queue
	executor *JobExecutor
	limiter  *rate.Limiter
	logger   *slog.Logger
}

// NewWorkerProcessor creates a WorkerProcessor.
// id is used only for log attribution. limiter can be nil if no rate limiting applies.
func NewWorkerProcessor(id int, q queue.Queue, je *JobExecutor, limiter *rate.Limiter, logger *slog.Logger) *WorkerProcessor {
	return &WorkerProcessor{
		id:       id,
		queue:    q,
		executor: je,
		limiter:  limiter,
		logger:   logger.With("worker_id", id),
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
	// ── Step 1: Dequeue ──────────────────────────────────────────────────────
	job, err := wp.queue.Dequeue(ctx)
	if err != nil {
		return err // transient; caller (Run) will back off
	}

	log := wp.logger.With("job_id", job.ID, "job_type", job.Type)
	log.Info("job dequeued", "attempt", job.Retries+1, "max_retries", job.MaxRetries)

	// Detach context cancellation for the rest of the job lifecycle.
	// This guarantees that if a SIGINT/SIGTERM arrives while a job is running,
	// the worker will FINISH the job before exiting, rather than aborting
	// the active task mid-flight.
	execCtx := context.WithoutCancel(ctx)

	// ── Rate Limit Check ──────────────────────────────────────────────────────
	// Wait logic consumes a token globally. If blocked, the job sits in the processing hash.
	if wp.limiter != nil {
		if err := wp.limiter.Wait(execCtx); err != nil {
			// If context cancels while waiting for a token, we safely abandon.
			// The in-flight job will be rescued by dead-letter reconciler natively.
			return err
		}
	}

	// ── Step 2: Execute ───────────────────────────────────────────────────────
	start := time.Now()
	execErr := wp.executor.Execute(execCtx, job)
	elapsed := time.Since(start)

	if execErr == nil {
		// ── Step 3a: Success → Ack ───────────────────────────────────────────
		log.Info("job succeeded",
			"elapsed_ms", elapsed.Milliseconds(),
		)
		wp.ack(execCtx, job, log)
		return nil
	}

	// ── Step 3b: Failure → decide retry or permanent fail ────────────────────
	log.Error("job execution failed",
		"error", execErr,
		"elapsed_ms", elapsed.Milliseconds(),
		"retries_used", job.Retries,
		"max_retries", job.MaxRetries,
	)

	if job.Retries < job.MaxRetries {
		wp.retry(ctx, execCtx, job, log)
	} else {
		wp.permanentlyFail(execCtx, job, execErr, log)
	}

	return nil
}

// ── private helpers ───────────────────────────────────────────────────────────

// ack removes the job from the processing set and marks it as completed.
func (wp *WorkerProcessor) ack(ctx context.Context, job *jobs.Job, log *slog.Logger) {
	if err := wp.queue.Ack(ctx, job.ID); err != nil {
		log.Error("failed to ack job after success", "error", err)
		return
	}
	log.Info("job marked as completed")
}

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

	if err := wp.queue.Enqueue(execCtx, job); err != nil {
		log.Error("failed to re-enqueue job for retry", "error", err)
		return
	}

	// Ack removes the old in-flight record; the freshly enqueued copy takes over.
	if err := wp.queue.Ack(execCtx, job.ID); err != nil {
		log.Error("failed to ack original entry after re-enqueue", "error", err)
	}
}

// permanentlyFail calls queue.Fail which removes the job from the processing
// set and marks it as StatusFailed (dead-letter handling can be added there).
func (wp *WorkerProcessor) permanentlyFail(ctx context.Context, job *jobs.Job, reason error, log *slog.Logger) {
	log.Error("job exhausted all retries, marking as failed",
		"total_attempts", job.Retries+1,
	)

	if err := wp.queue.Fail(ctx, job.ID, reason); err != nil {
		log.Error("failed to mark job as permanently failed", "error", err)
	}
}
