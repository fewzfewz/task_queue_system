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
// It is safe for concurrent use after initial plugin registration.
type Dispatcher struct {
	plugins map[string]plugin.JobPlugin
}

// NewDispatcher creates an empty Dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{plugins: make(map[string]plugin.JobPlugin)}
}

// Register binds a JobPlugin to the system.
// Panics if the same type is registered twice to catch misconfiguration early.
func (d *Dispatcher) Register(p plugin.JobPlugin) {
	jobType := p.Type()
	if _, exists := d.plugins[jobType]; exists {
		panic(fmt.Sprintf("processor: plugin already registered for job type %q", jobType))
	}
	d.plugins[jobType] = p
}

// Dispatch looks up and calls the plugin registered for job.Type.
// Returns an error if no plugin is found or if the plugin itself fails.
func (d *Dispatcher) Dispatch(ctx context.Context, job *jobs.Job) error {
	p, ok := d.plugins[job.Type]
	if !ok {
		return fmt.Errorf("processor: no plugin registered for job type %q", job.Type)
	}
	return p.Execute(job.Payload)
}
