package executor

import (
	"context"
	"fmt"
	"log/slog"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/worker/plugin"
)

// JobExecutor manages the registration and execution of job plugins.
type JobExecutor struct {
	registry *plugin.Registry
	logger   *slog.Logger
}

// NewJobExecutor creates a JobExecutor with built-in plugins registered by default.
func NewJobExecutor(logger *slog.Logger) *JobExecutor {
	reg := plugin.NewRegistry()

	// Register default plugins.
	_ = reg.Register(jobs.NewEmailHandler(logger))
	_ = reg.Register(jobs.NewImageHandler(logger))

	return &JobExecutor{
		registry: reg,
		logger:   logger,
	}
}

// RegisterPlugin adds a new job type capability to the executor.
func (je *JobExecutor) RegisterPlugin(p plugin.JobPlugin) {
	if err := je.registry.Register(p); err != nil {
		je.logger.Warn("plugin registration failed", "job_type", p.Type(), "error", err)
	}
}

// Execute performs the work for a given job by fetching the appropriate plugin.
// It fulfills the "registry instead of switch-case" requirement by dynamic lookup.
func (je *JobExecutor) Execute(ctx context.Context, job *jobs.Job) error {
	if job == nil {
		return fmt.Errorf("job_executor: cannot execute a nil job")
	}

	je.logger.Debug("executing job", "job_id", job.ID, "job_type", job.Type)

	// Fetch plugin from registry using job.Type
	p, err := je.registry.Get(job.Type)
	if err != nil {
		return fmt.Errorf("job_executor: no plugin for %q: %w", job.Type, err)
	}

	// Call plugin.Execute(payload)
	// Note: We use a deferred recover here to ensure worker threads don't panic on plugin mistakes.
	defer func() {
		if r := recover(); r != nil {
			je.logger.Error("plugin panicked during execution", "job_type", job.Type, "panic", r)
		}
	}()

	return p.Execute(job.Payload)
}
