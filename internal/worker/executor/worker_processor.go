package executor

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/metrics"
	"task-queue-system/internal/service"
	"task-queue-system/internal/worker/limiter"
	"task-queue-system/internal/worker/plugin"
)

// defaultSLATarget is the fallback SLA duration when none is configured.
const defaultSLATarget = 5 * time.Second

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

	slaTarget time.Duration

	onBusy    func()
	onIdle    func()
}


// NewWorkerProcessor creates a WorkerProcessor.
// name is used for log attribution and job metadata. limiter can be nil if no rate limiting applies.
// slaTarget of 0 means use the default (5s).
func NewWorkerProcessor(name string, svc *service.JobService, je *JobExecutor, l limiter.RateLimiter, logger *slog.Logger, slaTarget time.Duration) *WorkerProcessor {
	if slaTarget <= 0 {
		slaTarget = defaultSLATarget
	}
	return &WorkerProcessor{
		name:      name,
		service:   svc,
		exec:      je,
		limiter:   l,
		logger:    logger.With("worker_id", name),
		slaTarget: slaTarget,
	}
}

func (wp *WorkerProcessor) SetHooks(onBusy, onIdle func()) {
	wp.onBusy = onBusy
	wp.onIdle = onIdle
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
			if errors.Is(err, context.DeadlineExceeded) || isEmptyQueueErr(err) {
				continue
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

func isEmptyQueueErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return msg == "queue: dequeue timed out, no jobs available" ||
		msg == "redis: nil" ||
		msg == "queue: BRPOP failed: redis: connection pool timeout"
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

	log := wp.logger.With(
		"job_id", job.ID,
		"job_type", job.Type,
		"correlation_id", job.CorrelationID,
		"worker_id", wp.name,
	)
	log.Info("processing job dequeued", "status", jobs.StatusProcessing)

	// 1. Fast Redis Check
	isDone, _ := wp.service.IsProcessed(ctx, job.ID)
	if isDone {
		log.Info("skipping duplicate execution; job already marked as processed in Redis")
		_ = wp.service.Ack(ctx, job.ID)
		return nil
	}

	// 2. Persistent Store Check (Fallback)
	storedJob, err := wp.service.GetJobStatus(ctx, job.ID)
	if err == nil && storedJob != nil {
		if storedJob.Status == jobs.StatusCompleted || storedJob.Status == jobs.StatusFailed || storedJob.Status == jobs.StatusCancelled {
			log.Info("skipping duplicate execution; job already finished in database", "final_status", storedJob.Status)
			_ = wp.service.Ack(ctx, job.ID) // clear from broker
			return nil
		}
	}

	// Transition DB state to Processing immediately and attach worker ID.
	_ = wp.service.UpdateJobStatus(ctx, job.ID, jobs.StatusProcessing, wp.name)

	// ── Metrics: Busy ────────────────────────────────────────────────────────
	metrics.WorkerUtilization.Inc()
	if wp.onBusy != nil { wp.onBusy() }
	defer func() {
		metrics.WorkerUtilization.Dec()
		if wp.onIdle != nil { wp.onIdle() }
	}()


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
	// We wrap the execution in a timeout to ensure a single hanging task
	// does not block this worker thread indefinitely.
	timeout := 60 * time.Second
	if job.Timeout > 0 {
		timeout = time.Duration(job.Timeout) * time.Second
	}
	jobCtx, cancel := context.WithTimeout(execCtx, timeout)
	defer cancel()

	start := time.Now()
	progressCtx := plugin.WithProgressCallback(jobCtx, func(pct float64) {
		_ = wp.service.UpdateJobProgress(execCtx, job.ID, pct)
	})
	result, execErr := wp.exec.Execute(progressCtx, job)
	elapsed := time.Since(start)

	if execErr == nil {
		// ── Handle Success ────────────────────────────────────────────────────────
		log.Info("job succeeded", "status", jobs.StatusCompleted, "execution_time_ms", time.Since(start).Milliseconds())

		// Mark as processed in Redis for idempotency
		_ = wp.service.MarkProcessed(execCtx, job.ID)

		// Persist result, progress, and status
		_ = wp.service.UpdateJobProgress(execCtx, job.ID, 100)
		_ = wp.service.UpdateJobResult(execCtx, job.ID, jobs.StatusCompleted, wp.name, result)
		_ = wp.service.Ack(execCtx, job.ID)

		// ── Metrics: Success ─────────────────────────────────────────────────────
		metrics.JobTotal.WithLabelValues(job.Type, job.TenantID, "completed").Inc()
		metrics.JobLatency.WithLabelValues(job.Type, job.TenantID).Observe(elapsed.Seconds())

		// SLA Tracking
		compliant := "false"
		if elapsed < wp.slaTarget {
			compliant = "true"
		}
		metrics.JobSLACompliance.WithLabelValues(job.Type, job.TenantID, compliant).Inc()

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
		// Metrics: Retry counts as a partial failure but not a terminal one for this metric
		metrics.JobTotal.WithLabelValues(job.Type, job.TenantID, "retry").Inc()
	} else {
		_ = wp.service.UpdateJobResult(execCtx, job.ID, jobs.StatusFailed, wp.name, execErr.Error())
		wp.permanentlyFail(execCtx, job, execErr, log)
		metrics.JobTotal.WithLabelValues(job.Type, job.TenantID, "failed").Inc()
	}

	metrics.JobLatency.WithLabelValues(job.Type, job.TenantID).Observe(elapsed.Seconds())

	// SLA Tracking: Even failed jobs contribute to SLA statistics
	compliant := "false"
	if elapsed < wp.slaTarget {
		compliant = "true"
	}
	metrics.JobSLACompliance.WithLabelValues(job.Type, job.TenantID, compliant).Inc()

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

	delay := jobs.BackoffDelay(job)

	log.Warn("scheduling job for retry with backoff",
		"next_attempt", job.Retries+1,
		"max_retries", job.MaxRetries,
		"delay", delay.String(),
	)

	// Block the worker for the delay period to enforce backoff.
	// We listen to shutdownCtx.Done() so graceful shutdowns aren't stalled for seconds.
	select {
	case <-shutdownCtx.Done():
		log.Warn("shutdown interrupted retry delay; proactively re-enqueuing job")
		// We use execCtx (which isn't cancelled) to ensure the Enqueue/Ack finishes.
		if err := wp.service.Enqueue(execCtx, job); err != nil {
			log.Error("failed to re-enqueue during shutdown", "error", err)
		}
		_ = wp.service.Ack(execCtx, job.ID)
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
