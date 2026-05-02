package queue

import (
	"context"

	"task-queue-system/internal/jobs"
)

// Queue defines the interface for our distributed task queue system.
// Any backend (e.g., Redis, RabbitMQ, Amazon SQS, In-Memory) can be swapped
// by implementing this interface.
type Queue interface {
	// Enqueue adds a new job to the queue.
	// Context is included for timeout/cancellation of the enqueue operation.
	Enqueue(ctx context.Context, job *jobs.Job) error

	// Dequeue blocks and retrieves the next available job from the queue.
	// The context can be used to cancel a long-polling operation.
	Dequeue(ctx context.Context) (*jobs.Job, error)

	// Ack acknowledges that a job was processed successfully.
	// This usually removes the job from the backend or marks it completed.
	Ack(ctx context.Context, jobID string) error

	// Fail marks a job as failed, providing an error reason.
	// The underlying implementation moves it to a dead-letter queue.
	Fail(ctx context.Context, jobID string, err error) error

	// GetFailedJobs retrieves all jobs that have permanently failed
	// and are currently in the dead-letter queue.
	GetFailedJobs(ctx context.Context) ([]*jobs.Job, error)
}
