package plugin

import (
	"context"

	"task-queue-system/internal/jobs"
)

// NextFunc represents the next handler in the middleware chain.
// It will eventually call the actual JobPlugin.Execute.
type NextFunc func(ctx context.Context, job *jobs.Job) (interface{}, error)

// Middleware is a function that intercepts job execution.
// It can perform actions before and after the next handler is called,
// modify the context, or even completely bypass the execution.
type Middleware func(ctx context.Context, job *jobs.Job, next NextFunc) (interface{}, error)

// BuildChain wraps the final execution function with a slice of middlewares.
// Middlewares are executed in the order they are provided.
func BuildChain(middlewares []Middleware, final NextFunc) NextFunc {
	chain := final
	for i := len(middlewares) - 1; i >= 0; i-- {
		m := middlewares[i]
		next := chain // capture the current chain step
		chain = func(ctx context.Context, job *jobs.Job) (interface{}, error) {
			return m(ctx, job, next)
		}
	}
	return chain
}
