package executor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/worker/plugin"
)

// JobExecutor manages the registration and execution of job plugins.
type JobExecutor struct {
	registry       *plugin.Registry
	circuitBreaker *plugin.CircuitBreaker
	logger         *slog.Logger
}

// NewJobExecutor creates a JobExecutor using the system-wide global plugin registry.
func NewJobExecutor(logger *slog.Logger) *JobExecutor {
	return &JobExecutor{
		registry:       plugin.GetGlobalRegistry(),
		circuitBreaker: plugin.NewCircuitBreaker(5, 30*time.Second),
		logger:         logger,
	}
}

// RegisterPlugin adds a new job type capability to the executor.
func (je *JobExecutor) RegisterPlugin(p plugin.JobPlugin) {
	if err := je.registry.Register(p); err != nil {
		je.logger.Warn("plugin registration failed", "job_type", p.Type(), "error", err)
	}
}

// ResetCircuitBreaker resets the circuit breaker for the given job type.
func (je *JobExecutor) ResetCircuitBreaker(jobType string) {
	je.circuitBreaker.Reset(jobType)
}

// CircuitBreakerStatus returns the current state of all monitored plugins.
func (je *JobExecutor) CircuitBreakerStatus() map[string]string {
	return je.circuitBreaker.Status()
}

// Execute performs the work for a given job by fetching the appropriate plugin.
// It fulfills the "registry instead of switch-case" requirement by dynamic lookup.
func (je *JobExecutor) Execute(ctx context.Context, job *jobs.Job) (res interface{}, err error) {
	if job == nil {
		return nil, fmt.Errorf("job_executor: cannot execute a nil job")
	}

	// ── Circuit Breaker Check ──────────────────────────────────────────────
	if !je.circuitBreaker.IsAllowed(job.Type) {
		return nil, fmt.Errorf("circuit breaker open for plugin %q — too many consecutive failures", job.Type)
	}

	je.logger.Debug("executing job", "job_id", job.ID, "job_type", job.Type)

	// Fetch plugin from registry using job.Type
	p, err := je.registry.Get(job.Type)
	if err != nil {
		return nil, fmt.Errorf("job_executor: no plugin for %q: %w", job.Type, err)
	}

	// ── Fault Isolation: Recover from Panics ────────────────────────────────
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin panicked during execution: %v", r)
			je.circuitBreaker.RecordFailure(job.Type, err)
			je.logger.Error("plugin panicked during execution",
				"job_type", job.Type,
				"panic", r,
				"correlation_id", job.CorrelationID,
			)
		}
	}()

	res, err = p.Execute(ctx, job)

	if err != nil {
		je.circuitBreaker.RecordFailure(job.Type, err)
	} else {
		je.circuitBreaker.RecordSuccess(job.Type)
	}

	return
}
