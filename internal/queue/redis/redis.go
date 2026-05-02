package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/queue"
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
	client  *redis.Client
	qHigh   string // high priority list
	qMedium string // medium priority list
	qLow    string // low priority list
	dlqKey  string // dead letter queue key

	metricsTotal     string
	metricsCompleted string
	metricsFailed    string
}

// New creates a new RedisQueue. The provided client must already be connected.
// Pass an empty queueName to use the default key.
func New(client *redis.Client, queueName string) *RedisQueue {
	baseKey := defaultQueueKey
	if queueName != "" {
		baseKey = "task_queue:" + queueName
	}
	return &RedisQueue{
		client:           client,
		qHigh:            baseKey + ":high",
		qMedium:          baseKey + ":medium",
		qLow:             baseKey + ":low",
		dlqKey:           baseKey + ":dead_letter",
		metricsTotal:     baseKey + ":metrics:total",
		metricsCompleted: baseKey + ":metrics:completed",
		metricsFailed:    baseKey + ":metrics:failed",
	}
}

// Enqueue serialises the job as JSON and pushes it to the left of the Redis list.
// BRPOP pops from the right (FIFO ordering).
// Uses the job's Priority to determine the target queue.
func (q *RedisQueue) Enqueue(ctx context.Context, job *jobs.Job) error {
	if job == nil {
		return fmt.Errorf("queue: cannot enqueue a nil job")
	}

	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("queue: failed to serialise job %s: %w", job.ID, err)
	}

	var targetKey string
	switch job.Priority {
	case jobs.PriorityHigh:
		targetKey = q.qHigh
	case jobs.PriorityLow:
		targetKey = q.qLow
	case jobs.PriorityMedium:
		fallthrough
	default:
		targetKey = q.qMedium
	}

	if err := q.client.LPush(ctx, targetKey, payload).Err(); err != nil {
		return fmt.Errorf("queue: LPUSH failed for job %s: %w", job.ID, err)
	}

	// Increment total jobs counter only on initial enqueue (not retries).
	if job.Retries == 0 {
		q.client.Incr(ctx, q.metricsTotal)
	}

	return nil
}

// Dequeue blocks until a job becomes available or the context is cancelled.
// It pops from the right (BRPOP), checking queues in priority order.
func (q *RedisQueue) Dequeue(ctx context.Context) (*jobs.Job, error) {
	// Use the context deadline if set, otherwise fall back to dequeueTimeout.
	timeout := dequeueTimeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}

	// BRPop checks keys left-to-right, ensuring strict priority handling
	result, err := q.client.BRPop(ctx, timeout, q.qHigh, q.qMedium, q.qLow).Result()
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

	// Atomic increment for metrics (fire and forget)
	q.client.Incr(ctx, q.metricsCompleted)

	return nil
}

// Fail marks a job as permanently failed. It is removed from the processing
// hash and pushed directly into the dead-letter list for future inspection.
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

	job.Status = jobs.StatusFailed
	job.UpdatedAt = time.Now().UTC()
	// Optionally embed the reason string into the job payload for debugging
	if reason != nil {
		if job.Payload == nil {
			job.Payload = make(map[string]interface{})
		}
		job.Payload["_error_reason"] = reason.Error()
	}

	failedPayload, err := json.Marshal(&job)
	if err != nil {
		return fmt.Errorf("queue: failed to serialise failed job: %w", err)
	}

	// Push the job to the dead letter queue (Left push to allow chronologic access)
	if err := q.client.LPush(ctx, q.dlqKey, failedPayload).Err(); err != nil {
		return fmt.Errorf("queue: failed to push job %s to dead-letter queue: %w", job.ID, err)
	}

	// Atomic increment for metrics (fire and forget)
	q.client.Incr(ctx, q.metricsFailed)

	return nil
}

// GetFailedJobs retrieves all jobs currently in the dead-letter queue.
// It returns them without removing them from Redis (like a peek).
func (q *RedisQueue) GetFailedJobs(ctx context.Context) ([]*jobs.Job, error) {
	// Fetch all elements from the list
	items, err := q.client.LRange(ctx, q.dlqKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("queue: failed to fetch dead-letter queue: %w", err)
	}

	var failedJobs []*jobs.Job
	for _, item := range items {
		var job jobs.Job
		if err := json.Unmarshal([]byte(item), &job); err != nil {
			// If one is corrupt, we ignore it or log it, but we shouldn't fail everything
			continue
		}
		failedJobs = append(failedJobs, &job)
	}

	return failedJobs, nil
}

// GetMetrics retrieves current execution statistics.
func (q *RedisQueue) GetMetrics(ctx context.Context) (queue.QueueMetrics, error) {
	// Retrieve counters natively
	res, err := q.client.MGet(ctx, q.metricsTotal, q.metricsCompleted, q.metricsFailed).Result()
	if err != nil {
		return queue.QueueMetrics{}, fmt.Errorf("queue: failed to fetch metrics: %w", err)
	}

	// Active is defined as the number of jobs currently sitting in the processing hash
	active, err := q.client.HLen(ctx, processingSetKey).Result()
	if err != nil {
		return queue.QueueMetrics{}, fmt.Errorf("queue: failed to fetch active count: %w", err)
	}

	parseStr2Int := func(val interface{}) int64 {
		if val == nil {
			return 0
		}
		// Redis INCR uses strings underneath when parsed via MGET
		if s, ok := val.(string); ok {
			var i int64
			fmt.Sscanf(s, "%d", &i)
			return i
		}
		return 0
	}

	return queue.QueueMetrics{
		TotalJobs:     parseStr2Int(res[0]),
		CompletedJobs: parseStr2Int(res[1]),
		FailedJobs:    parseStr2Int(res[2]),
		ActiveJobs:    active,
	}, nil
}
