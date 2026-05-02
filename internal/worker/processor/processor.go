// Package processor defines the Handler interface and a job dispatcher
// that routes incoming jobs to the correct registered handler by job type.
package processor

import (
	"context"
	"fmt"

	"task-queue-system/internal/jobs"
)

// Handler is the contract that every job-type handler must satisfy.
// Implementations receive a fully-populated Job and return an error if
// processing fails (which will trigger retry / fail logic in the executor).
type Handler interface {
	Handle(ctx context.Context, job *jobs.Job) error
}

// HandlerFunc is a convenience adapter that lets a plain function act as a Handler.
type HandlerFunc func(ctx context.Context, job *jobs.Job) error

func (f HandlerFunc) Handle(ctx context.Context, job *jobs.Job) error {
	return f(ctx, job)
}

// Dispatcher routes jobs to the registered handler for their Type.
// It is safe for concurrent use after initial handler registration.
type Dispatcher struct {
	handlers map[string]Handler
}

// NewDispatcher creates an empty Dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{handlers: make(map[string]Handler)}
}

// Register binds a Handler to the given job type (e.g. "email", "image").
// Panics if the same type is registered twice to catch misconfiguration early.
func (d *Dispatcher) Register(jobType string, h Handler) {
	if _, exists := d.handlers[jobType]; exists {
		panic(fmt.Sprintf("processor: handler already registered for job type %q", jobType))
	}
	d.handlers[jobType] = h
}

// Dispatch looks up and calls the handler registered for job.Type.
// Returns an error if no handler is found or if the handler itself fails.
func (d *Dispatcher) Dispatch(ctx context.Context, job *jobs.Job) error {
	h, ok := d.handlers[job.Type]
	if !ok {
		return fmt.Errorf("processor: no handler registered for job type %q", job.Type)
	}
	return h.Handle(ctx, job)
}
