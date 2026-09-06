package middleware

import (
	"context"
	"log/slog"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/worker/plugin"
)

// LoggingMiddleware logs the start, end, and duration of job execution.
func LoggingMiddleware(logger *slog.Logger) plugin.Middleware {
	return func(ctx context.Context, job *jobs.Job, next plugin.NextFunc) (interface{}, error) {
		start := time.Now()
		logger.Debug("middleware: job started", "job_id", job.ID, "type", job.Type)
		
		res, err := next(ctx, job)
		
		duration := time.Since(start)
		if err != nil {
			logger.Error("middleware: job failed", "job_id", job.ID, "type", job.Type, "duration_ms", duration.Milliseconds(), "error", err)
		} else {
			logger.Debug("middleware: job completed", "job_id", job.ID, "type", job.Type, "duration_ms", duration.Milliseconds())
		}
		
		return res, err
	}
}
