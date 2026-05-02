package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"task-queue-system/internal/jobs"
)

const (
	// defaultQueueKey is the Redis list that holds pending jobs.
	defaultQueueKey = "task_queue:jobs"
	// processingSetKey is a Redis hash that tracks in-flight jobs by ID.
	// This gives us an audit trail for Ack and Fail without losing the payload.
	processingSetKey = "task_queue:processing"
	// dequeueTimeout is how long BRPOP will block before returning a timeout error.
	// A context deadline will also interrupt it earlier if set.
	dequeueTimeout = 5 * time.Second
)

// RedisQueue is a Redis-backed implementation of the queue.Queue interface.
// It uses a Redis list for the pending queue and a Redis hash to track
// jobs that are currently being processed, enabling safe Ack and Fail semantics.
type RedisQueue struct {
	client   *redis.Client
	queueKey string // allows multiple logical queues on one Redis instance
}

// New creates a new RedisQueue. The provided client must already be connected.
// Pass an empty queueName to use the default key.
func New(client *redis.Client, queueName string) *RedisQueue {
	key := defaultQueueKey
	if queueName != "" {
		key = "task_queue:" + queueName
	}
	return &RedisQueue{
		client:   client,
		queueKey: key,
	}
}

// Enqueue serialises the job as JSON and pushes it to the left of the Redis list.
// BRPOP pops from the right (FIFO ordering).
func (q *RedisQueue) Enqueue(ctx context.Context, job *jobs.Job) error {
	if job == nil {
		return fmt.Errorf("queue: cannot enqueue a nil job")
	}

	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("queue: failed to serialise job %s: %w", job.ID, err)
	}

	if err := q.client.LPush(ctx, q.queueKey, payload).Err(); err != nil {
		return fmt.Errorf("queue: LPUSH failed for job %s: %w", job.ID, err)
	}

	return nil
}

// Dequeue blocks until a job becomes available or the context is cancelled.
// It pops from the right (BRPOP), deserialises the payload, marks the job as
// StatusProcessing, and stores it in the processing hash so Ack/Fail can look
// it up later.
func (q *RedisQueue) Dequeue(ctx context.Context) (*jobs.Job, error) {
	// Use the context deadline if set, otherwise fall back to dequeueTimeout.
	timeout := dequeueTimeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}

	result, err := q.client.BRPop(ctx, timeout, q.queueKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("queue: dequeue timed out, no jobs available")
		}
		return nil, fmt.Errorf("queue: BRPOP failed: %w", err)
	}

	// BRPop returns [key, value]; the actual payload is at index 1.
	if len(result) < 2 {
		return nil, fmt.Errorf("queue: unexpected BRPOP result length %d", len(result))
	}

	var job jobs.Job
	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		return nil, fmt.Errorf("queue: failed to deserialise job payload: %w", err)
	}

	// Transition the job to processing state.
	job.Status = jobs.StatusProcessing
	job.UpdatedAt = time.Now().UTC()

	// Persist the updated job to the processing hash so Ack/Fail can reference it.
	updated, err := json.Marshal(&job)
	if err != nil {
		return nil, fmt.Errorf("queue: failed to serialise processing job %s: %w", job.ID, err)
	}

	if err := q.client.HSet(ctx, processingSetKey, job.ID, updated).Err(); err != nil {
		return nil, fmt.Errorf("queue: failed to store processing job %s: %w", job.ID, err)
	}

	return &job, nil
}

// Ack marks a successfully processed job as completed and removes it from
// the processing hash.
func (q *RedisQueue) Ack(ctx context.Context, jobID string) error {
	if jobID == "" {
		return fmt.Errorf("queue: jobID must not be empty")
	}

	deleted, err := q.client.HDel(ctx, processingSetKey, jobID).Result()
	if err != nil {
		return fmt.Errorf("queue: HDel failed for job %s: %w", jobID, err)
	}
	if deleted == 0 {
		return fmt.Errorf("queue: job %s not found in processing set", jobID)
	}

	return nil
}

// Fail marks a job as failed. If it has remaining retries it is re-enqueued
// with an incremented retry count; otherwise it is removed from the processing
// hash and left for dead-letter handling (extendable).
func (q *RedisQueue) Fail(ctx context.Context, jobID string, reason error) error {
	if jobID == "" {
		return fmt.Errorf("queue: jobID must not be empty")
	}

	// Retrieve the in-flight job from the processing hash.
	raw, err := q.client.HGet(ctx, processingSetKey, jobID).Result()
	if err != nil {
		if err == redis.Nil {
			return fmt.Errorf("queue: job %s not found in processing set", jobID)
		}
		return fmt.Errorf("queue: HGet failed for job %s: %w", jobID, err)
	}

	var job jobs.Job
	if err := json.Unmarshal([]byte(raw), &job); err != nil {
		return fmt.Errorf("queue: failed to deserialise job %s: %w", jobID, err)
	}

	// Remove from the processing hash regardless of what happens next.
	if err := q.client.HDel(ctx, processingSetKey, jobID).Err(); err != nil {
		return fmt.Errorf("queue: HDel failed for job %s: %w", jobID, err)
	}

	job.Retries++
	job.UpdatedAt = time.Now().UTC()

	if job.Retries <= job.MaxRetries {
		// Re-enqueue for another attempt.
		job.Status = jobs.StatusPending
		return q.Enqueue(ctx, &job)
	}

	// Exhausted retries — mark as permanently failed.
	// A real system might push to a dead-letter list here.
	job.Status = jobs.StatusFailed
	_ = reason // available for structured logging / dead-letter enrichment

	return nil
}
