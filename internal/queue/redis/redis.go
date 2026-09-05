package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/queue"
	"task-queue-system/internal/tracing"
)

// Now allows injection of the current time for deterministic testing.
// In production it defaults to time.Now.
var Now = time.Now

const (
	// defaultQueueKey is the Redis list that holds pending jobs.
	defaultQueueKey = "task_queue:jobs"
	// processingSetKey is a Redis Sorted Set (ZSET) that tracks in-flight jobs.
	// Score is the Unix timestamp (seconds) when the job becomes "visible" again (timeout).
	processingSetKey = "task_queue:in_flight"
	// delayedQueueKey is a Redis Sorted Set (ZSET) for scheduled jobs.
	// Score is the Unix timestamp (seconds) when the job should be enqueued.
	delayedQueueKey = "delayed_jobs"
	// visibilityTimeout is how long a worker has to process a job before it can be reclaimed.
	visibilityTimeout = 30 * time.Second
	// dequeueTimeout is how long BRPOP will block before returning a timeout error.
	dequeueTimeout = 5 * time.Second
	// processedSetKey is a Redis Set that stores IDs of successfully completed jobs.
	processedSetKey = "task_queue:processed"
	// tenantSetKey tracks every tenant that has submitted at least one job,
	// used to enumerate per-tenant rate-limit windows for the status endpoint.
	tenantSetKey = "task_queue:tenants"
)

// promoteScheduledJobsScript moves due jobs from ZSET to priority lists atomically.
var promoteScheduledJobsScript = redis.NewScript(`
	local delayedKey = KEYS[1]
	local now = ARGV[1]
	local numPartitions = tonumber(ARGV[2])
	local jobs = redis.call('ZRANGEBYSCORE', delayedKey, 0, now)
	
	for _, job in ipairs(jobs) do
		local decoded = cjson.decode(job)
		local targetKey = KEYS[2] -- medium by default
		if decoded.priority == 'high' then
			targetKey = KEYS[3]
		elseif decoded.priority == 'low' then
			targetKey = KEYS[4]
		end
		
		-- Partition hash matching Go implementation
		local sum = 0
		local id = decoded.id
		for i = 1, #id do
			sum = sum + string.byte(id, i)
		end
		local partition = (sum % numPartitions) + 1
		local partitionKey = targetKey .. ":" .. partition

		redis.call('LPUSH', partitionKey, job)
		redis.call('ZREM', delayedKey, job)
	end
	return #jobs
`)

// reclaimTimedOutJobsScript moves expired in-flight jobs back to priority lists.
var reclaimTimedOutJobsScript = redis.NewScript(`
	local inFlightKey = KEYS[1]
	local payloadKey = KEYS[2]
	local now = ARGV[1]
	local numPartitions = tonumber(ARGV[2])
	local jobs = redis.call('ZRANGEBYSCORE', inFlightKey, 0, now)
	
	for _, id in ipairs(jobs) do
		local payload = redis.call('HGET', payloadKey, id)
		if payload then
			local decoded = cjson.decode(payload)
			local targetKey = KEYS[3] -- medium by default
			if decoded.priority == 'high' then
				targetKey = KEYS[4]
			elseif decoded.priority == 'low' then
				targetKey = KEYS[5]
			end
			
			-- Partition hash matching Go implementation
			local sum = 0
			for i = 1, #id do
				sum = sum + string.byte(id, i)
			end
			local partition = (sum % numPartitions) + 1
			local partitionKey = targetKey .. ":" .. partition

			redis.call('LPUSH', partitionKey, payload)
		end
		redis.call('ZREM', inFlightKey, id)
		redis.call('HDEL', payloadKey, id)
	end
	return #jobs
`)



// RedisQueue is a Redis-backed implementation of the queue.Queue interface.
// It uses a Redis list for the pending queue and a Redis hash to track
// jobs that are currently being processed, enabling safe Ack and Fail semantics.
type RedisQueue struct {
	client  *redis.Client
	qHigh   string // high priority list
	qMedium string // medium priority list
	qLow    string // low priority list
	qDelayed string // scheduled jobs ZSET
	dlqKey  string // dead letter queue key
	numPartitions int

	metricsTotal     string
	metricsCompleted string
	metricsFailed    string
	heartbeatPrefix  string
	processedKey     string

	rateLimit int64 // per-tenant rate limit (req/s), 0 = unlimited
}

// New creates a new RedisQueue. The provided client must already be connected.
// Pass an empty queueName to use the default key.
func New(client *redis.Client, queueName string) *RedisQueue {
	return NewWithRateLimit(client, queueName, 10)
}

// NewWithRateLimit creates a new RedisQueue with a configurable per-tenant rate limit.
func NewWithRateLimit(client *redis.Client, queueName string, rateLimit int64) *RedisQueue {
	baseKey := defaultQueueKey
	if queueName != "" {
		baseKey = "task_queue:" + queueName
	}
	return &RedisQueue{
		rateLimit: rateLimit,
		client:           client,
		qHigh:            baseKey + ":high",
		qMedium:          baseKey + ":medium",
		qLow:             baseKey + ":low",
		qDelayed:         "delayed_jobs",
		dlqKey:           baseKey + ":dead_letter",
		metricsTotal:     baseKey + ":metrics:total",
		metricsCompleted: baseKey + ":metrics:completed",
		metricsFailed:    baseKey + ":metrics:failed",
		heartbeatPrefix:  baseKey + ":workers:heartbeat:",
		processedKey:     baseKey + ":processed",
		numPartitions:    3, // default to 3 partitions per priority
	}
}

// getPartitionedKey returns the specific partition key for a given job ID and priority.
func (q *RedisQueue) getPartitionedKey(jobID string, priority jobs.JobPriority) string {
	// Simple hash-based partitioning
	sum := 0
	for _, b := range jobID {
		sum += int(b)
	}
	partition := (sum % q.numPartitions) + 1

	base := q.qMedium
	switch priority {
	case jobs.PriorityHigh:
		base = q.qHigh
	case jobs.PriorityLow:
		base = q.qLow
	}

	return fmt.Sprintf("%s:%d", base, partition)
}

// getFairPartitionKeys returns all partition keys in a weighted randomized order
// to prevent starvation of lower-priority jobs (Weighted Round-Robin style).
// Default weight distribution: 70% High, 20% Medium, 10% Low.
func (q *RedisQueue) getFairPartitionKeys() []string {
	r := rand.Intn(100)

	var p1, p2, p3 string
	if r < 70 {
		p1, p2, p3 = q.qHigh, q.qMedium, q.qLow
	} else if r < 90 {
		p1, p2, p3 = q.qMedium, q.qHigh, q.qLow
	} else {
		p1, p2, p3 = q.qLow, q.qHigh, q.qMedium
	}

	var keys []string
	// Collect each priority's partitions in the chosen order.
	for _, base := range []string{p1, p2, p3} {
		for i := 1; i <= q.numPartitions; i++ {
			keys = append(keys, fmt.Sprintf("%s:%d", base, i))
		}
	}
	return keys
}



// Enqueue serialises the job as JSON and pushes it to the left of the Redis list.
// BRPOP pops from the right (FIFO ordering).
// Uses the job's Priority to determine the target queue.
func (q *RedisQueue) Enqueue(ctx context.Context, job *jobs.Job) error {
	if job == nil {
		return fmt.Errorf("queue: cannot enqueue a nil job")
	}

	job.TraceID = tracing.Inject(ctx)

	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("queue: failed to serialise job %s: %w", job.ID, err)
	}

	if job.RunAt.After(time.Now()) {
		// Scheduled job: put in ZSET with score = RunAt
		if err := q.client.ZAdd(ctx, q.qDelayed, redis.Z{
			Score:  float64(job.RunAt.Unix()),
			Member: payload,
		}).Err(); err != nil {
			return fmt.Errorf("queue: ZADD failed for scheduled job %s: %w", job.ID, err)
		}
	} else {
		// Immediate job: push to priority list
		targetKey := q.getPartitionedKey(job.ID, job.Priority)
		if err := q.client.LPush(ctx, targetKey, payload).Err(); err != nil {
			return fmt.Errorf("queue: LPUSH failed for job %s: %w", job.ID, err)
		}
	}

	// Increment total jobs counter only on initial enqueue (not retries).
	if job.Retries == 0 {
		q.client.Incr(ctx, q.metricsTotal)
	}

	return nil
}

// Size returns the total number of pending jobs across all priority queues.
func (q *RedisQueue) Size(ctx context.Context) (int64, error) {
	// We sum up high, medium, and low priority lists.
	hLen, _ := q.client.LLen(ctx, q.qHigh).Result()
	mLen, _ := q.client.LLen(ctx, q.qMedium).Result()
	lLen, _ := q.client.LLen(ctx, q.qLow).Result()
	return hLen + mLen + lLen, nil
}

// Dequeue blocks until a job becomes available or the context is cancelled.
// It pops from the right (BRPOP), checking queues in priority order.
func (q *RedisQueue) Dequeue(ctx context.Context) (*jobs.Job, error) {
	// Use the context deadline if set, otherwise fall back to dequeueTimeout.
	timeout := dequeueTimeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}

	keys := q.getFairPartitionKeys()

	for {
		result, err := q.client.BRPop(ctx, timeout, keys...).Result()
		if err != nil {
			if err == redis.Nil {
				return nil, fmt.Errorf("queue: dequeue timed out, no jobs available")
			}
			return nil, fmt.Errorf("queue: BRPOP failed: %w", err)
		}

		if len(result) < 2 {
			return nil, fmt.Errorf("queue: unexpected BRPOP result length %d", len(result))
		}

		var job jobs.Job
		if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
			return nil, fmt.Errorf("queue: failed to deserialise job payload: %w", err)
		}

		// Check Concurrency Limit
		limit, err := q.getJobTypeLimit(ctx, job.Type)
		if err != nil {
			return nil, fmt.Errorf("queue: failed to get limit for job type %s: %w", job.Type, err)
		}

		activeKey := "task_queue:active_type:" + job.Type

		if limit > 0 {
			// Clear expired active tracking (visibility timeout)
			now := time.Now().Unix()
			q.client.ZRemRangeByScore(ctx, activeKey, "-inf", fmt.Sprintf("%d", now))

			activeCount, err := q.client.ZCard(ctx, activeKey).Result()
			if err != nil {
				return nil, fmt.Errorf("queue: failed to check active count for %s: %w", job.Type, err)
			}

			if int(activeCount) >= limit {
				// Over limit: Push to deferred queue and continue loop
				deferredKey := "task_queue:deferred:" + job.Type
				if err := q.client.LPush(ctx, deferredKey, result[1]).Err(); err != nil {
					return nil, fmt.Errorf("queue: failed to defer job %s: %w", job.ID, err)
				}
				continue
			}
		}

		// Transition the job to processing state.
		job.Status = jobs.StatusProcessing
		job.UpdatedAt = time.Now().UTC()

		updated, err := json.Marshal(&job)
		if err != nil {
			return nil, fmt.Errorf("queue: failed to serialise processing job %s: %w", job.ID, err)
		}

		timeoutAt := time.Now().Add(visibilityTimeout).Unix()

		pipe := q.client.TxPipeline()
		pipe.HSet(ctx, "task_queue:payloads", job.ID, updated)
		pipe.ZAdd(ctx, processingSetKey, redis.Z{
			Score:  float64(timeoutAt),
			Member: job.ID,
		})
		
		// Track active concurrency
		pipe.ZAdd(ctx, activeKey, redis.Z{
			Score:  float64(timeoutAt),
			Member: job.ID,
		})
		// Keep a reverse mapping so we know which activeKey to remove from during Ack/Fail
		pipe.Set(ctx, "task_queue:job_active_key:"+job.ID, activeKey, visibilityTimeout+time.Hour)

		if _, err := pipe.Exec(ctx); err != nil {
			return nil, fmt.Errorf("queue: Dequeue tracking failed: %w", err)
		}

		return &job, nil
	}
}

// getJobTypeLimit fetches the concurrency limit for a job type.
func (q *RedisQueue) getJobTypeLimit(ctx context.Context, jobType string) (int, error) {
	data, err := q.client.HGet(ctx, "task_queue:job_types:registered", jobType).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil // Not registered = unlimited (built-ins fallback)
		}
		return 0, err
	}
	
	// Fast parse just the concurrency limit using unmarshal on a partial struct
	var partial struct {
		ConcurrencyLimit int `json:"concurrency_limit"`
	}
	if err := json.Unmarshal(data, &partial); err != nil {
		return 0, err
	}
	return partial.ConcurrencyLimit, nil
}

// Ack marks a successfully processed job as completed and removes it from
// the processing hash.
func (q *RedisQueue) Ack(ctx context.Context, jobID string) error {
	if jobID == "" {
		return fmt.Errorf("queue: jobID must not be empty")
	}

	// Because we store the whole object in the ZSET but only have the ID here,
	// we have to search the ZSET for the member.
	// Simple approach for this refactor: scan the ZSET.
	// Production-ready optimization: also keep a mapping of ID -> full payload.
	// For "simple but safe", we'll use a scan or just store ID in ZSET and keep payload in Hash.
	
	// Refined approach: Use a Hash for [ID -> Payload] and ZSET for [ID -> Timeout].
	// This makes ID-based lookups O(1) while allowing time-based queries.
	
	// Let's stick to the ZSET-only approach for now but we need the payload to remove it.
	// Actually, it's easier to remove by ID if we only store the ID in the ZSET.
	// I'll update Dequeue to store ONLY THE ID in the ZSET, and keep the ID -> JSON in a Hash.
	
	// Removing from both atomically.
	pipe := q.client.TxPipeline()
	pipe.HDel(ctx, "task_queue:payloads", jobID)
	pipe.ZRem(ctx, processingSetKey, jobID)
	
	// Remove from concurrency tracking
	activeKey, _ := q.client.Get(ctx, "task_queue:job_active_key:"+jobID).Result()
	if activeKey != "" {
		pipe.ZRem(ctx, activeKey, jobID)
		pipe.Del(ctx, "task_queue:job_active_key:"+jobID)
	}
	
	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("queue: Ack pipeline failed: %w", err)
	}

	// Check if we actually removed something
	hdelRes := cmds[0].(*redis.IntCmd)
	if hdelRes.Val() == 0 {
		return fmt.Errorf("queue: job %s not found in active set", jobID)
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

	// Retrieve the in-flight job payload from the hash.
	raw, err := q.client.HGet(ctx, "task_queue:payloads", jobID).Result()
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

	// Remove from both atomically.
	pipe := q.client.TxPipeline()
	pipe.HDel(ctx, "task_queue:payloads", jobID)
	pipe.ZRem(ctx, processingSetKey, jobID)
	
	activeKey, _ := q.client.Get(ctx, "task_queue:job_active_key:"+jobID).Result()
	if activeKey != "" {
		pipe.ZRem(ctx, activeKey, jobID)
		pipe.Del(ctx, "task_queue:job_active_key:"+jobID)
	}
	
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("queue: Fail cleanup failed: %w", err)
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

	// Active is defined as the number of jobs currently sitting in the in-flight ZSET
	active, err := q.client.ZCard(ctx, processingSetKey).Result()
	if err != nil {
		return queue.QueueMetrics{}, fmt.Errorf("queue: failed to fetch active count: %w", err)
	}

	// Fetch active workers count
	workers, _ := q.GetActiveWorkers(ctx)

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
		WorkerCount:   len(workers),
	}, nil
}

// RegisterHeartbeat sets a Redis key with a short expiration for the worker.
func (q *RedisQueue) RegisterHeartbeat(ctx context.Context, workerID string) error {
	key := q.heartbeatPrefix + workerID
	val := time.Now().Format(time.RFC3339)
	// 30 seconds TTL; if the worker crashes, the key expires automatically.
	return q.client.Set(ctx, key, val, 30*time.Second).Err()
}

// GetActiveWorkers scans for all heartbeat keys and returns their values.
func (q *RedisQueue) GetActiveWorkers(ctx context.Context) ([]queue.WorkerInfo, error) {
	var workers []queue.WorkerInfo

	// SCAN for keys matching the heartbeat prefix
	iter := q.client.Scan(ctx, 0, q.heartbeatPrefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		val, err := q.client.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		// Strip prefix from key to get worker ID
		workerID := key[len(q.heartbeatPrefix):]
		workers = append(workers, queue.WorkerInfo{
			ID:            workerID,
			LastHeartbeat: val,
		})
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("queue: heartbeat scan failed: %w", err)
	}

	return workers, nil
}

// PromoteScheduledJobs checks for jobs in the delayed set that are due and moves them
// to the active processing queues. Returns the number of promoted jobs.
func (q *RedisQueue) PromoteScheduledJobs(ctx context.Context) (int, error) {
	now := Now().Unix()
	
	// KEYS: [delayed_jobs, qMedium, qHigh, qLow]
	res, err := promoteScheduledJobsScript.Run(ctx, q.client, []string{
		q.qDelayed, q.qMedium, q.qHigh, q.qLow,
	}, now, q.numPartitions).Int()
	
	if err != nil && err != redis.Nil {
		return 0, fmt.Errorf("queue: failed to execute promotion script: %w", err)
	}
	
	return res, nil
}

// ReclaimTimedOutJobs identifies jobs that have exceeded their visibility timeout
// and moves them back to the active queues for another attempt.
func (q *RedisQueue) ReclaimTimedOutJobs(ctx context.Context) (int, error) {
	now := Now().Unix()
	
	// KEYS: [inFlightKey, payloadsKey, qMedium, qHigh, qLow]
	res, err := reclaimTimedOutJobsScript.Run(ctx, q.client, []string{
		processingSetKey, "task_queue:payloads", q.qMedium, q.qHigh, q.qLow,
	}, now, q.numPartitions).Int()
	
	if err != nil && err != redis.Nil {
		return 0, fmt.Errorf("queue: failed to execute reclaim script: %w", err)
	}
	
	return res, nil
}

// processedTTL is how long a processed job ID is kept for idempotency checks.
// After this period, Redis automatically evicts the key, bounding memory use.
const processedTTL = 24 * time.Hour

// IsProcessed checks if a job has already been marked as completed in Redis.
// Uses a TTL-backed key so old entries are automatically evicted.
func (q *RedisQueue) IsProcessed(ctx context.Context, jobID string) (bool, error) {
	_, err := q.client.Get(ctx, q.processedKey+":"+jobID).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	return err == nil, err
}

// MarkProcessed records a job ID as completed with a TTL, bounding memory growth.
func (q *RedisQueue) MarkProcessed(ctx context.Context, jobID string) error {
	return q.client.Set(ctx, q.processedKey+":"+jobID, "1", processedTTL).Err()
}

// CleanupProcessedIDs is a no-op now that processed IDs self-expire via TTL.
// Retained for backwards compatibility with any caller that expects a maintenance hook.
func (q *RedisQueue) CleanupProcessedIDs(ctx context.Context) (int64, error) {
	return 0, nil
}


// IsAllowed implements per-tenant rate limiting using a Redis-backed fixed window.
func (q *RedisQueue) IsAllowed(ctx context.Context, tenantID string) (bool, error) {
	if q.rateLimit == 0 {
		return true, nil // unlimited
	}

	if tenantID == "" {
		return true, nil // anonymous tenant (global pool)
	}

	key := "task_queue:tenant:" + tenantID + ":rate"
	limit := q.rateLimit

	val, err := q.client.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}

	if val == 1 {
		q.client.Expire(ctx, key, time.Second)
		q.client.SAdd(ctx, tenantSetKey, tenantID)
	}

	return val <= limit, nil
}

// RateLimitStatus reports the current per-tenant usage against the configured
// tenant rate limit. Tenants are tracked the first time they submit a job.
func (q *RedisQueue) RateLimitStatus(ctx context.Context) ([]queue.TenantRateStatus, error) {
	if q.rateLimit == 0 {
		return []queue.TenantRateStatus{}, nil
	}

	tenants, err := q.client.SMembers(ctx, tenantSetKey).Result()
	if err != nil {
		return nil, err
	}

	statuses := make([]queue.TenantRateStatus, 0, len(tenants))
	for _, t := range tenants {
		if t == "" {
			continue
		}
		val, err := q.client.Get(ctx, "task_queue:tenant:"+t+":rate").Int64()
		if err != nil {
			val = 0
		}
		statuses = append(statuses, queue.TenantRateStatus{
			Tenant:        t,
			Current:       val,
			Limit:         q.rateLimit,
			WindowSeconds: 1,
			Limited:       val >= q.rateLimit,
		})
	}

	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Tenant < statuses[j].Tenant })
	return statuses, nil
}

// PriorityPartitionDepths returns pending job counts for each priority tier and
// hash partition (job-ID mod numPartitions).
func (q *RedisQueue) PriorityPartitionDepths(ctx context.Context) (queue.PriorityDepthReport, error) {
	report := queue.PriorityDepthReport{
		DequeueWeights:        map[string]int{"high": 70, "medium": 20, "low": 10},
		PartitionsPerPriority: q.numPartitions,
		ByPriority:            make(map[string]queue.PriorityTierDepth),
	}

	for _, tier := range []struct {
		name string
		base string
	}{
		{"high", q.qHigh},
		{"medium", q.qMedium},
		{"low", q.qLow},
	} {
		td := queue.PriorityTierDepth{Partitions: make(map[string]int64)}
		for i := 1; i <= q.numPartitions; i++ {
			key := fmt.Sprintf("%s:%d", tier.base, i)
			n, err := q.client.LLen(ctx, key).Result()
			if err != nil {
				return report, fmt.Errorf("queue: LLen %s: %w", key, err)
			}
			partKey := fmt.Sprintf("%d", i)
			td.Partitions[partKey] = n
			td.Total += n
		}
		report.ByPriority[tier.name] = td
	}

	return report, nil
}

func (q *RedisQueue) PublishWebhookEvent(ctx context.Context, event interface{}) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return q.client.XAdd(ctx, &redis.XAddArgs{
		Stream: "task_queue:webhooks:stream",
		Values: map[string]interface{}{
			"data": string(data),
		},
	}).Err()
}


// ReconcileDeferredJobs moves jobs from deferred queues back to the main queue
// if their job type concurrency limits allow it.
func (q *RedisQueue) ReconcileDeferredJobs(ctx context.Context) (int, error) {
	// 1. Fetch all registered job types to get their limits.
	allTypes, err := q.client.HGetAll(ctx, "task_queue:job_types:registered").Result()
	if err != nil {
		return 0, err
	}

	movedTotal := 0
	for typeName, raw := range allTypes {
		var partial struct {
			ConcurrencyLimit int `json:"concurrency_limit"`
		}
		if err := json.Unmarshal([]byte(raw), &partial); err != nil {
			continue
		}

		limit := partial.ConcurrencyLimit
		if limit <= 0 {
			continue // No limit, meaning deferred shouldn't have been populated, but skip anyway
		}

		deferredKey := "task_queue:deferred:" + typeName
		activeKey := "task_queue:active_type:" + typeName

		// Check if there are any deferred jobs for this type
		deferredCount, err := q.client.LLen(ctx, deferredKey).Result()
		if err != nil || deferredCount == 0 {
			continue
		}

		// Check active count
		now := time.Now().Unix()
		q.client.ZRemRangeByScore(ctx, activeKey, "-inf", fmt.Sprintf("%d", now))
		activeCount, err := q.client.ZCard(ctx, activeKey).Result()
		if err != nil {
			continue
		}

		available := limit - int(activeCount)
		if available <= 0 {
			continue
		}

		// We can move up to `available` jobs back to the main queue.
		// For simplicity and speed in this loop, we pop and push one by one.
		for i := 0; i < available; i++ {
			payload, err := q.client.RPop(ctx, deferredKey).Result()
			if err != nil {
				if errors.Is(err, redis.Nil) {
					break // Queue is empty
				}
				continue
			}

			// Parse the priority to know which queue to put it back into
			var partialJob struct {
				ID       string           `json:"id"`
				Priority jobs.JobPriority `json:"priority"`
			}
			if err := json.Unmarshal([]byte(payload), &partialJob); err == nil {
				targetKey := q.getPartitionedKey(partialJob.ID, partialJob.Priority)
				q.client.LPush(ctx, targetKey, payload)
				movedTotal++
			}
		}
	}
	
	return movedTotal, nil
}
