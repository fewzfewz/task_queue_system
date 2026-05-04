// Package processor defines the Handler interface and a job dispatcher
// that routes incoming jobs to the correct registered handler by job type.
package processor

import (
	"context"
	"fmt"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/worker/plugin"
)

// Dispatcher routes jobs to the registered plugin for their Type.
// It uses a Registry to manage plugin lookup and registration.
type Dispatcher struct {
	registry *plugin.Registry
}

// NewDispatcher creates a Dispatcher with an empty registry.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{registry: plugin.NewRegistry()}
}

// Register adds a plugin to the dispatcher's registry.
func (d *Dispatcher) Register(p plugin.JobPlugin) {
	if err := d.registry.Register(p); err != nil {
		panic(fmt.Sprintf("processor: %v", err))
	}
}

// Dispatch looks up and calls the plugin registered for job.Type.
func (d *Dispatcher) Dispatch(ctx context.Context, job *jobs.Job) error {
	p, err := d.registry.Get(job.Type)
	if err != nil {
		return fmt.Errorf("processor: %w", err)
	}
	return p.Execute(job.Payload)
}
