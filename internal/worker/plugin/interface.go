package plugin

import (
	"context"

	"task-queue-system/internal/jobs"
)

// ProgressFunc is a callback for plugins to report execution progress (0.0 – 100.0).
type ProgressFunc func(progress float64)

type progressKeyType struct{}

var progressKey = progressKeyType{}

// WithProgressCallback embeds a progress reporter into the context.
func WithProgressCallback(ctx context.Context, fn ProgressFunc) context.Context {
	return context.WithValue(ctx, progressKey, fn)
}

// ReportProgress extracts the progress callback from the context and calls it.
// Plugins can call this during Execute to report incremental progress.
func ReportProgress(ctx context.Context, pct float64) {
	if fn, ok := ctx.Value(progressKey).(ProgressFunc); ok {
		fn(pct)
	}
}

// JobPlugin defines the contract for job execution units.
// This interface allows for an extensible system where new job types
// can be added as self-contained plugins.
type JobPlugin interface {
	// Type returns the unique identifier for the job type this plugin handles (e.g., "email").
	Type() string

	// Execute performs the actual work of the job using the provided payload.
	// It returns an optional result and an error if processing fails.
	Execute(ctx context.Context, job *jobs.Job) (interface{}, error)
}
