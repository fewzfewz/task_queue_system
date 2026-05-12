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

// NewJobExecutor creates a JobExecutor using the system-wide global plugin registry.
func NewJobExecutor(logger *slog.Logger) *JobExecutor {
	return &JobExecutor{
		registry: plugin.GetGlobalRegistry(),
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
func (je *JobExecutor) Execute(ctx context.Context, job *jobs.Job) (res interface{}, err error) {
	if job == nil {
		return nil, fmt.Errorf("job_executor: cannot execute a nil job")
	}

	je.logger.Debug("executing job", "job_id", job.ID, "job_type", job.Type)

	// Fetch plugin from registry using job.Type
	p, err := je.registry.Get(job.Type)
	if err != nil {
		return nil, fmt.Errorf("job_executor: no plugin for %q: %w", job.Type, err)
	}

	// ── Fault Isolation: Recover from Panics ────────────────────────────────
	// This ensures that even if a plugin developer forgets a nil check or 
	// encounters an unexpected runtime error, the worker instance remains alive.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin panicked during execution: %v", r)
			je.logger.Error("plugin panicked during execution", 
				"job_type", job.Type, 
				"panic", r,
				"correlation_id", job.CorrelationID,
			)
		}
	}()

	return p.Execute(ctx, job.Payload)
}
