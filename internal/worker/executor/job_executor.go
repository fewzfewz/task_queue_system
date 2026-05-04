package executor

import (
	"context"
	"fmt"
	"log/slog"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/worker/plugin"
	"task-queue-system/internal/worker/processor"
)

// JobExecutor is a high-level executor that wraps a Dispatcher pre-loaded
// with all built-in handlers. Call Execute to process a single job
// synchronously — useful for testing, one-off runs, or embedding in the
// worker loop's Executor.Run.
//
// To add a new job type, call RegisterHandler before any jobs arrive.
type JobExecutor struct {
	dispatcher *processor.Dispatcher
	logger     *slog.Logger
}

// NewJobExecutor creates a JobExecutor and registers the default handlers:
//
//	"email"  → jobs.EmailHandler
//	"image"  → jobs.ImageHandler
func NewJobExecutor(logger *slog.Logger) *JobExecutor {
	d := processor.NewDispatcher()

	d.Register(jobs.NewEmailHandler(logger))
	d.Register(jobs.NewImageHandler(logger))

	return &JobExecutor{
		dispatcher: d,
		logger:     logger,
	}
}

// RegisterPlugin adds a new plugin to the executor.
func (je *JobExecutor) RegisterPlugin(p plugin.JobPlugin) {
	jobType := p.Type()
	// Wrap in a recover so we can give a useful warning instead of panicking
	// when re-registering during testing or hot-reload scenarios.
	defer func() {
		if r := recover(); r != nil {
			je.logger.Warn("plugin already registered, skipping", "job_type", jobType)
		}
	}()
	je.dispatcher.Register(p)
}

// Execute processes a single job synchronously.
// It delegates to the dispatcher which routes by job.Type.
// Returns an error if no handler is registered or the handler fails.
func (je *JobExecutor) Execute(ctx context.Context, job *jobs.Job) error {
	if job == nil {
		return fmt.Errorf("job_executor: cannot execute a nil job")
	}

	je.logger.Debug("executing job", "job_id", job.ID, "job_type", job.Type)

	if err := je.dispatcher.Dispatch(ctx, job); err != nil {
		return fmt.Errorf("job_executor: job %s (%s) failed: %w", job.ID, job.Type, err)
	}

	return nil
}
