// Package executor implements a single worker goroutine that continuously
// dequeues jobs, dispatches them, and acks or fails them based on the result.
package executor

import (
	"context"
	"log/slog"
	"time"

	"task-queue-system/internal/queue"
	"task-queue-system/internal/worker/processor"
)

// Executor is a single worker that runs its own dequeue-dispatch-ack loop.
type Executor struct {
	id         int
	queue      queue.Queue
	dispatcher *processor.Dispatcher
	logger     *slog.Logger
}

// New creates a new Executor. id is used only for log attribution.
func New(id int, q queue.Queue, d *processor.Dispatcher, logger *slog.Logger) *Executor {
	return &Executor{
		id:         id,
		queue:      q,
		dispatcher: d,
		logger:     logger.With("worker_id", id),
	}
}

// Run starts the dequeue-dispatch-ack loop. It exits cleanly when ctx is
// cancelled (graceful shutdown). Transient dequeue errors are logged and
// backed off briefly before retrying.
func (e *Executor) Run(ctx context.Context) {
	e.logger.Info("worker started")
	defer e.logger.Info("worker stopped")

	for {
		// Check for shutdown before blocking on Dequeue.
		select {
		case <-ctx.Done():
			return
		default:
		}

		job, err := e.queue.Dequeue(ctx)
		if err != nil {
			// Context cancelled → clean shutdown.
			if ctx.Err() != nil {
				return
			}
			// Transient error (e.g. Redis blip, timeout) → back off and retry.
			e.logger.Warn("dequeue error, backing off", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}

		log := e.logger.With("job_id", job.ID, "job_type", job.Type)
		log.Info("job started", "retries", job.Retries)

		start := time.Now()
		dispatchErr := e.dispatcher.Dispatch(ctx, job)
		elapsed := time.Since(start)

		if dispatchErr != nil {
			log.Error("job failed", "error", dispatchErr, "elapsed_ms", elapsed.Milliseconds())
			if ackErr := e.queue.Fail(ctx, job.ID, dispatchErr); ackErr != nil {
				log.Error("failed to mark job as failed", "error", ackErr)
			}
			continue
		}

		log.Info("job completed", "elapsed_ms", elapsed.Milliseconds())
		if ackErr := e.queue.Ack(ctx, job.ID); ackErr != nil {
			log.Error("failed to ack job", "error", ackErr)
		}
	}
}
