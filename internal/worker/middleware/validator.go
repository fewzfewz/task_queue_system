package middleware

import (
	"context"
	"fmt"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/worker/plugin"
)

// ValidatorMiddleware ensures that specific required fields are present in the job payload.
// If validation fails, it aborts execution and returns an error immediately.
func ValidatorMiddleware(requiredFields ...string) plugin.Middleware {
	return func(ctx context.Context, job *jobs.Job, next plugin.NextFunc) (interface{}, error) {
		for _, field := range requiredFields {
			if _, exists := job.Payload[field]; !exists {
				return nil, fmt.Errorf("middleware validation failed: missing required payload field %q", field)
			}
		}
		
		// All checks passed, continue to the next middleware or plugin
		return next(ctx, job)
	}
}
