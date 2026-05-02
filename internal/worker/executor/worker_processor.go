package executor

import (
	"context"
	"log/slog"
	"time"

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
	logger   *slog.Logger
}

// NewWorkerProcessor creates a WorkerProcessor.
// id is used only for log attribution.
func NewWorkerProcessor(id int, q queue.Queue, je *JobExecutor, logger *slog.Logger) *WorkerProcessor {
	return &WorkerProcessor{
		id:       id,
		queue:    q,
		executor: je,
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

	// ── Step 2: Execute ───────────────────────────────────────────────────────
	start := time.Now()
	execErr := wp.executor.Execute(ctx, job)
	elapsed := time.Since(start)

	if execErr == nil {
		// ── Step 3a: Success → Ack ───────────────────────────────────────────
		log.Info("job succeeded",
			"elapsed_ms", elapsed.Milliseconds(),
		)
		wp.ack(ctx, job, log)
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
		wp.retry(ctx, job, log)
	} else {
		wp.permanentlyFail(ctx, job, execErr, log)
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

// retry increments the retry counter, re-enqueues the job for another
// attempt, then acks the current in-flight entry so the processing set
// stays clean.
func (wp *WorkerProcessor) retry(ctx context.Context, job *jobs.Job, log *slog.Logger) {
	job.Retries++
	job.Status = jobs.StatusPending
	job.UpdatedAt = time.Now().UTC()

	log.Warn("scheduling job for retry",
		"next_attempt", job.Retries+1,
		"max_retries", job.MaxRetries,
	)

	if err := wp.queue.Enqueue(ctx, job); err != nil {
		log.Error("failed to re-enqueue job for retry", "error", err)
		return
	}

	// Ack removes the old in-flight record; the freshly enqueued copy takes over.
	if err := wp.queue.Ack(ctx, job.ID); err != nil {
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
