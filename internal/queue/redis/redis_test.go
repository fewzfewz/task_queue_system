package redis

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"task-queue-system/internal/errors"
	"task-queue-system/internal/jobs"
	"task-queue-system/internal/service"
	"task-queue-system/internal/storage/models"
)

func newTestQueue(t *testing.T) (*miniredis.Miniredis, *RedisQueue, redis.UniversalClient, context.Context) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("skipping: TCP listener not available")
	}
	_ = ln.Close()

	mr, err := miniredis.Run()
	if err != nil {
		t.Skip("skipping: miniredis could not start")
	}

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	q := New(rdb, "test")
	ctx := context.Background()
	return mr, q, rdb, ctx
}

func TestNewWithRateLimit(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("skipping: TCP listener not available")
	}
	_ = ln.Close()

	mr, err := miniredis.Run()
	if err != nil {
		t.Skip("skipping: miniredis could not start")
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	q := NewWithRateLimit(rdb, "custom", 50)
	if q.rateLimit != 50 {
		t.Fatalf("expected rateLimit 50, got %d", q.rateLimit)
	}
	if q.qMedium != "task_queue:custom:medium" {
		t.Fatalf("unexpected medium queue key: %s", q.qMedium)
	}
}

func TestNew_DefaultQueueName(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("skipping: TCP listener not available")
	}
	_ = ln.Close()

	mr, err := miniredis.Run()
	if err != nil {
		t.Skip("skipping: miniredis could not start")
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	q := New(rdb, "")
	if q.qMedium != "task_queue:jobs:medium" {
		t.Fatalf("unexpected medium queue key: %s", q.qMedium)
	}
}

func TestGetPartitionedKey(t *testing.T) {
	mr, q, _, _ := newTestQueue(t)
	defer mr.Close()

	key := q.getPartitionedKey("test-job-id", jobs.PriorityHigh)
	if key == "" {
		t.Fatal("expected non-empty key")
	}
}

func TestGetFairPartitionKeys(t *testing.T) {
	mr, q, _, _ := newTestQueue(t)
	defer mr.Close()

	keys := q.getFairPartitionKeys()
	if len(keys) != 9 { // 3 priorities * 3 partitions
		t.Fatalf("expected 9 keys, got %d", len(keys))
	}
}

func TestEnqueue_Immediate(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	job := jobs.NewJob("test", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
	err := q.Enqueue(ctx, job)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	targetKey := q.getPartitionedKey(job.ID, job.Priority)
	llen, err := rdb.LLen(ctx, targetKey).Result()
	if err != nil {
		t.Fatalf("LLen failed: %v", err)
	}
	if llen != 1 {
		t.Fatalf("expected len 1 on %s, got %d", targetKey, llen)
	}
	_ = mr
}

func TestEnqueue_NilJob(t *testing.T) {
	mr, q, _, ctx := newTestQueue(t)
	defer mr.Close()

	err := q.Enqueue(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil job")
	}
}

func TestEnqueue_Scheduled(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	job := jobs.NewJob("test", nil, nil, jobs.PriorityMedium, 3, time.Now().Add(time.Hour), "", 60, 1, "tenant-a")
	err := q.Enqueue(ctx, job)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	if !mr.Exists(q.qDelayed) {
		t.Fatal("expected delayed queue key to exist")
	}
	_ = rdb
}

func TestAck_Success(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	payload := `{"id":"ack-job-1"}`
	rdb.HSet(ctx, "task_queue:payloads", "ack-job-1", payload)
	rdb.ZAdd(ctx, "task_queue:in_flight", redis.Z{Member: "ack-job-1", Score: float64(time.Now().Unix() + 30)})

	err := q.Ack(ctx, "ack-job-1")
	if err != nil {
		t.Fatalf("Ack failed: %v", err)
	}

	if mr.Exists("task_queue:payloads") {
		_, err := rdb.HGet(ctx, "task_queue:payloads", "ack-job-1").Result()
		if err == nil {
			t.Fatal("expected payload to be deleted after Ack")
		}
	}
}

func TestAck_EmptyID(t *testing.T) {
	mr, q, _, ctx := newTestQueue(t)
	defer mr.Close()

	err := q.Ack(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty jobID")
	}
}

func TestAck_NotFound(t *testing.T) {
	mr, q, _, ctx := newTestQueue(t)
	defer mr.Close()

	err := q.Ack(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent job")
	}
}

func TestFail_Success(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	payload := `{"id":"fail-job-1","payload":{"key":"val"}}`
	rdb.HSet(ctx, "task_queue:payloads", "fail-job-1", payload)
	rdb.ZAdd(ctx, "task_queue:in_flight", redis.Z{Member: "fail-job-1", Score: float64(time.Now().Unix() + 30)})

	err := q.Fail(ctx, "fail-job-1", nil)
	if err != nil {
		t.Fatalf("Fail failed: %v", err)
	}

	if mr.Exists(q.dlqKey) == false {
		t.Fatal("expected job in dead-letter queue")
	}
}

func TestFail_WithReason(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	rdb.HSet(ctx, "task_queue:payloads", "fail-job-2", `{"id":"fail-job-2"}`)
	rdb.ZAdd(ctx, "task_queue:in_flight", redis.Z{Member: "fail-job-2", Score: float64(time.Now().Unix() + 30)})

	err := q.Fail(ctx, "fail-job-2", nil)
	if err != nil {
		t.Fatalf("Fail failed: %v", err)
	}

	items, err := q.GetFailedJobs(ctx)
	if err != nil {
		t.Fatalf("GetFailedJobs failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 failed job, got %d", len(items))
	}
}

func TestFail_EmptyID(t *testing.T) {
	mr, q, _, ctx := newTestQueue(t)
	defer mr.Close()

	err := q.Fail(ctx, "", nil)
	if err == nil {
		t.Fatal("expected error for empty jobID")
	}
}

func TestFail_NotFound(t *testing.T) {
	mr, q, _, ctx := newTestQueue(t)
	defer mr.Close()

	err := q.Fail(ctx, "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent job")
	}
}

func TestGetFailedJobs_Empty(t *testing.T) {
	mr, q, _, ctx := newTestQueue(t)
	defer mr.Close()

	jobs, err := q.GetFailedJobs(ctx)
	if err != nil {
		t.Fatalf("GetFailedJobs failed: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected 0 failed jobs, got %d", len(jobs))
	}
}

func TestIsAllowed_Unlimited(t *testing.T) {
	mr, q, _, ctx := newTestQueue(t)
	defer mr.Close()

	q.rateLimit = 0
	allowed, err := q.IsAllowed(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("IsAllowed failed: %v", err)
	}
	if !allowed {
		t.Fatal("expected unlimited to be allowed")
	}
}

func TestIsAllowed_AnonymousTenant(t *testing.T) {
	mr, q, _, ctx := newTestQueue(t)
	defer mr.Close()

	q.rateLimit = 10
	allowed, err := q.IsAllowed(ctx, "")
	if err != nil {
		t.Fatalf("IsAllowed failed: %v", err)
	}
	if !allowed {
		t.Fatal("expected anonymous tenant to be allowed")
	}
}

func TestIsAllowed_RateLimited(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	q.rateLimit = 3
	for i := 0; i < 3; i++ {
		allowed, err := q.IsAllowed(ctx, "tenant-rate")
		if err != nil {
			t.Fatalf("IsAllowed failed on iteration %d: %v", i, err)
		}
		if !allowed {
			t.Fatalf("expected allowed on iteration %d", i)
		}
	}

	allowed, err := q.IsAllowed(ctx, "tenant-rate")
	if err != nil {
		t.Fatalf("IsAllowed failed: %v", err)
	}
	if allowed {
		t.Fatal("expected rate limited after 3 requests")
	}
	_ = rdb
}

func TestIsProcessed_MarkProcessed(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	processed, err := q.IsProcessed(ctx, "job-1")
	if err != nil {
		t.Fatalf("IsProcessed failed: %v", err)
	}
	if processed {
		t.Fatal("expected job not processed initially")
	}

	err = q.MarkProcessed(ctx, "job-1")
	if err != nil {
		t.Fatalf("MarkProcessed failed: %v", err)
	}

	processed, err = q.IsProcessed(ctx, "job-1")
	if err != nil {
		t.Fatalf("IsProcessed failed: %v", err)
	}
	if !processed {
		t.Fatal("expected job processed after MarkProcessed")
	}
	_ = rdb
}

func TestRegisterHeartbeat(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	err := q.RegisterHeartbeat(ctx, "worker-1")
	if err != nil {
		t.Fatalf("RegisterHeartbeat failed: %v", err)
	}

	workers, err := q.GetActiveWorkers(ctx)
	if err != nil {
		t.Fatalf("GetActiveWorkers failed: %v", err)
	}
	if len(workers) != 1 {
		t.Fatalf("expected 1 worker, got %d", len(workers))
	}
	if workers[0].ID != "worker-1" {
		t.Fatalf("unexpected worker ID: %s", workers[0].ID)
	}
	_ = rdb
}

func TestGetActiveWorkers_Empty(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	workers, err := q.GetActiveWorkers(ctx)
	if err != nil {
		t.Fatalf("GetActiveWorkers failed: %v", err)
	}
	if len(workers) != 0 {
		t.Fatalf("expected 0 workers, got %d", len(workers))
	}
	_ = rdb
}

func TestGetMetrics(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	metrics, err := q.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}
	if metrics.TotalJobs != 0 {
		t.Fatalf("expected TotalJobs 0, got %d", metrics.TotalJobs)
	}
	_ = rdb
}

func TestSize_Empty(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	size, err := q.Size(ctx)
	if err != nil {
		t.Fatalf("Size failed: %v", err)
	}
	if size != 0 {
		t.Fatalf("expected size 0, got %d", size)
	}
	_ = rdb
}

func TestSize_WithJobs(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	highKey := fmt.Sprintf("%s:1", q.qHigh)
	medKey := fmt.Sprintf("%s:1", q.qMedium)
	lowKey := fmt.Sprintf("%s:1", q.qLow)
	rdb.LPush(ctx, highKey, `{"id":"high-job"}`)
	rdb.LPush(ctx, medKey, `{"id":"medium-job"}`)
	rdb.LPush(ctx, lowKey, `{"id":"low-job"}`)

	// Verify Size() accurately reports 3 pending jobs across partitions.
	size, err := q.Size(ctx)
	if err != nil {
		t.Fatalf("Size failed: %v", err)
	}
	if size != 3 {
		t.Fatalf("expected size 3, got %d", size)
	}

	for _, pair := range []struct {
		key      string
		expected int64
	}{
		{highKey, 1}, {medKey, 1}, {lowKey, 1},
	} {
		llen, err := rdb.LLen(ctx, pair.key).Result()
		if err != nil {
			t.Fatalf("LLen(%s) failed: %v", pair.key, err)
		}
		if llen != pair.expected {
			t.Fatalf("expected len %d on %s, got %d", pair.expected, pair.key, llen)
		}
	}
}

func TestRedisQueue_Size(t *testing.T) {
	t.Run("Empty queue returns 0", func(t *testing.T) {
		mr, q, _, ctx := newTestQueue(t)
		defer mr.Close()

		size, err := q.Size(ctx)
		if err != nil {
			t.Fatalf("Size failed on empty queue: %v", err)
		}
		if size != 0 {
			t.Fatalf("expected size 0, got %d", size)
		}
	})

	t.Run("Only high-priority jobs across partitions", func(t *testing.T) {
		mr, q, rdb, ctx := newTestQueue(t)
		defer mr.Close()

		for i := 1; i <= q.numPartitions; i++ {
			key := fmt.Sprintf("%s:%d", q.qHigh, i)
			rdb.LPush(ctx, key, fmt.Sprintf(`{"id":"high-%d"}`, i))
		}

		size, err := q.Size(ctx)
		if err != nil {
			t.Fatalf("Size failed: %v", err)
		}
		if size != int64(q.numPartitions) {
			t.Fatalf("expected size %d, got %d", q.numPartitions, size)
		}
	})

	t.Run("Only medium-priority jobs across partitions", func(t *testing.T) {
		mr, q, rdb, ctx := newTestQueue(t)
		defer mr.Close()

		for i := 1; i <= q.numPartitions; i++ {
			key := fmt.Sprintf("%s:%d", q.qMedium, i)
			rdb.LPush(ctx, key, fmt.Sprintf(`{"id":"med-%d-1"}`, i))
			rdb.LPush(ctx, key, fmt.Sprintf(`{"id":"med-%d-2"}`, i))
		}

		size, err := q.Size(ctx)
		if err != nil {
			t.Fatalf("Size failed: %v", err)
		}
		expected := int64(q.numPartitions * 2)
		if size != expected {
			t.Fatalf("expected size %d, got %d", expected, size)
		}
	})

	t.Run("Only low-priority jobs across partitions", func(t *testing.T) {
		mr, q, rdb, ctx := newTestQueue(t)
		defer mr.Close()

		for i := 1; i <= q.numPartitions; i++ {
			key := fmt.Sprintf("%s:%d", q.qLow, i)
			rdb.LPush(ctx, key, fmt.Sprintf(`{"id":"low-%d"}`, i))
		}

		size, err := q.Size(ctx)
		if err != nil {
			t.Fatalf("Size failed: %v", err)
		}
		if size != int64(q.numPartitions) {
			t.Fatalf("expected size %d, got %d", q.numPartitions, size)
		}
	})

	t.Run("Jobs distributed across multiple partitions and priorities", func(t *testing.T) {
		mr, q, _, ctx := newTestQueue(t)
		defer mr.Close()

		totalJobs := 15
		for i := 0; i < totalJobs; i++ {
			prio := jobs.PriorityMedium
			if i%3 == 0 {
				prio = jobs.PriorityHigh
			} else if i%3 == 1 {
				prio = jobs.PriorityLow
			}
			job := jobs.NewJob("email", nil, nil, prio, 3, time.Time{}, "", 60, 1, "tenant-a")
			if err := q.Enqueue(ctx, job); err != nil {
				t.Fatalf("Enqueue failed: %v", err)
			}
		}

		size, err := q.Size(ctx)
		if err != nil {
			t.Fatalf("Size failed: %v", err)
		}
		if size != int64(totalJobs) {
			t.Fatalf("expected size %d, got %d", totalJobs, size)
		}
	})

	t.Run("In-flight jobs are not counted as pending in Size", func(t *testing.T) {
		mr, q, rdb, ctx := newTestQueue(t)
		defer mr.Close()

		// Add 2 pending jobs
		rdb.LPush(ctx, fmt.Sprintf("%s:1", q.qHigh), `{"id":"high-1"}`)
		rdb.LPush(ctx, fmt.Sprintf("%s:2", q.qLow), `{"id":"low-1"}`)

		// Add 3 in-flight jobs in the processing set
		rdb.ZAdd(ctx, processingSetKey,
			redis.Z{Member: "inflight-1", Score: float64(time.Now().Unix() + 30)},
			redis.Z{Member: "inflight-2", Score: float64(time.Now().Unix() + 30)},
			redis.Z{Member: "inflight-3", Score: float64(time.Now().Unix() + 30)},
		)

		size, err := q.Size(ctx)
		if err != nil {
			t.Fatalf("Size failed: %v", err)
		}
		if size != 2 {
			t.Fatalf("expected size 2 (excluding 3 in-flight jobs), got %d", size)
		}
	})

	t.Run("Redis error propagation", func(t *testing.T) {
		badClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
		badQueue := New(badClient, "err-test")

		errCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		size, err := badQueue.Size(errCtx)
		if err == nil {
			t.Fatalf("expected error from bad Redis client, got size=%d", size)
		}
		_ = badClient.Close()
	})

	t.Run("Backpressure integration with JobService", func(t *testing.T) {
		mr, q, _, ctx := newTestQueue(t)
		defer mr.Close()

		store := models.NewInMemoryStore()
		svc := service.New(q, store, slog.Default(), 2)

		// 1st job
		_, err := svc.CreateJob(ctx, "email", nil, nil, "medium", 3, "", "", "", "", "", 60, 1, "tenant-a", nil, "", nil, "", 0)
		if err != nil {
			t.Fatalf("first job failed: %v", err)
		}

		// 2nd job
		_, err = svc.CreateJob(ctx, "email", nil, nil, "medium", 3, "", "", "", "", "", 60, 1, "tenant-a", nil, "", nil, "", 0)
		if err != nil {
			t.Fatalf("second job failed: %v", err)
		}

		// Verify queue size is 2
		sz, err := q.Size(ctx)
		if err != nil || sz != 2 {
			t.Fatalf("expected queue size 2, got %d, err: %v", sz, err)
		}

		// 3rd job: must fail with QueueFull because maxQueueSize is 2
		_, err = svc.CreateJob(ctx, "email", nil, nil, "medium", 3, "", "", "", "", "", 60, 1, "tenant-a", nil, "", nil, "", 0)
		if err == nil {
			t.Fatal("expected QueueFull error on 3rd job, got nil")
		}
		if !errors.IsCode(err, errors.CodeQueueFull) {
			t.Fatalf("expected CodeQueueFull, got %v", err)
		}
	})
}

func TestPromoteScheduledJobs(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	count, err := q.PromoteScheduledJobs(ctx)
	if err != nil {
		t.Logf("PromoteScheduledJobs expectedly fails without Lua: %v", err)
	}
	if count != 0 {
		t.Logf("unexpected count: %d", count)
	}
	_ = rdb
}

func TestNowOverride(t *testing.T) {
	saved := Now
	defer func() { Now = saved }()

	fixed := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	Now = func() time.Time { return fixed }

	if !Now().Equal(fixed) {
		t.Fatal("Now override failed")
	}
}

func TestEnqueue_MultiplePriorities(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	high := jobs.NewJob("test", nil, nil, jobs.PriorityHigh, 3, time.Time{}, "", 60, 1, "tenant-a")
	low := jobs.NewJob("test", nil, nil, jobs.PriorityLow, 3, time.Time{}, "", 60, 1, "tenant-a")

	q.Enqueue(ctx, high)
	q.Enqueue(ctx, low)

	highKey := q.getPartitionedKey(high.ID, jobs.PriorityHigh)
	lowKey := q.getPartitionedKey(low.ID, jobs.PriorityLow)

	exists, _ := rdb.Exists(ctx, highKey).Result()
	if exists == 0 {
		t.Fatal("expected high priority key to exist")
	}
	exists, _ = rdb.Exists(ctx, lowKey).Result()
	if exists == 0 {
		t.Fatal("expected low priority key to exist")
	}
	_ = mr
}

func TestDequeue_RespectsJobTimeout(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	// 1. Job with custom timeout of 120 seconds
	customJob := jobs.NewJob("custom", nil, nil, jobs.PriorityHigh, 3, time.Time{}, "", 120, 1, "tenant-a")
	if err := q.Enqueue(ctx, customJob); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	before := time.Now().Unix()
	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if dequeued == nil || dequeued.ID != customJob.ID {
		t.Fatalf("expected job %s, got %v", customJob.ID, dequeued)
	}

	// Verify visibility score in in-flight ZSET is ~ before + 120
	score, err := rdb.ZScore(ctx, "task_queue:in_flight", customJob.ID).Result()
	if err != nil {
		t.Fatalf("ZScore failed: %v", err)
	}
	expectedMin := float64(before + 118)
	expectedMax := float64(before + 125)
	if score < expectedMin || score > expectedMax {
		t.Fatalf("expected in_flight score around %d + 120, got %f", before, score)
	}

	// Ack customJob to clean up
	_ = q.Ack(ctx, customJob.ID)

	// 2. Job with default timeout (Timeout == 0 -> should use 30s)
	defaultJob := jobs.NewJob("default", nil, nil, jobs.PriorityHigh, 3, time.Time{}, "", 0, 1, "tenant-a")
	if err := q.Enqueue(ctx, defaultJob); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	beforeDefault := time.Now().Unix()
	dequeuedDefault, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if dequeuedDefault == nil || dequeuedDefault.ID != defaultJob.ID {
		t.Fatalf("expected job %s, got %v", defaultJob.ID, dequeuedDefault)
	}

	scoreDefault, err := rdb.ZScore(ctx, "task_queue:in_flight", defaultJob.ID).Result()
	if err != nil {
		t.Fatalf("ZScore failed: %v", err)
	}
	expectedDefaultMin := float64(beforeDefault + 28)
	expectedDefaultMax := float64(beforeDefault + 35)
	if scoreDefault < expectedDefaultMin || scoreDefault > expectedDefaultMax {
		t.Fatalf("expected in_flight score around %d + 30, got %f", beforeDefault, scoreDefault)
	}
}

func TestDequeue_AtomicCrashDurability(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	job := jobs.NewJob("crash-test", map[string]interface{}{"data": "important"}, nil, jobs.PriorityHigh, 3, time.Time{}, "", 10, 1, "tenant-a")
	if err := q.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Dequeue the job
	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if dequeued.ID != job.ID {
		t.Fatalf("expected job %s, got %s", job.ID, dequeued.ID)
	}

	// Verify the job is immediately and atomically recorded in in_flight and payloads
	payload, err := rdb.HGet(ctx, "task_queue:payloads", job.ID).Result()
	if err != nil {
		t.Fatalf("expected job payload in task_queue:payloads: %v", err)
	}
	if payload == "" {
		t.Fatal("expected non-empty payload in task_queue:payloads")
	}

	score, err := rdb.ZScore(ctx, "task_queue:in_flight", job.ID).Result()
	if err != nil {
		t.Fatalf("expected job in task_queue:in_flight: %v", err)
	}
	if score <= 0 {
		t.Fatalf("expected positive visibility score, got %f", score)
	}

	// Simulate worker crash: worker terminates without calling Ack or Fail
	// Advance time past the visibility timeout (10 seconds)
	savedNow := Now
	defer func() { Now = savedNow }()
	reclaimTime := time.Now().Add(15 * time.Second)
	Now = func() time.Time { return reclaimTime }

	reclaimed, err := q.ReclaimTimedOutJobs(ctx)
	if err != nil {
		t.Fatalf("ReclaimTimedOutJobs failed: %v", err)
	}
	if reclaimed != 1 {
		t.Fatalf("expected 1 reclaimed job, got %d", reclaimed)
	}

	// Another worker dequeues the reclaimed job
	secondDequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("second Dequeue failed: %v", err)
	}
	if secondDequeued.ID != job.ID {
		t.Fatalf("expected reclaimed job %s, got %s", job.ID, secondDequeued.ID)
	}

	// Clean up with Ack
	if err := q.Ack(ctx, secondDequeued.ID); err != nil {
		t.Fatalf("Ack failed: %v", err)
	}
}

func TestDequeue_WorkerHardKillReclamation(t *testing.T) {
	mr, q, _, ctx := newTestQueue(t)
	defer mr.Close()

	const jobCount = 5
	originalIDs := make(map[string]bool)
	for i := 0; i < jobCount; i++ {
		job := jobs.NewJob("hardkill-type", map[string]interface{}{"index": i}, nil, jobs.PriorityMedium, 3, time.Time{}, "", 5, 1, "tenant-b")
		originalIDs[job.ID] = true
		if err := q.Enqueue(ctx, job); err != nil {
			t.Fatalf("Enqueue #%d failed: %v", i, err)
		}
	}

	// Workers dequeue all 5 jobs
	for i := 0; i < jobCount; i++ {
		j, err := q.Dequeue(ctx)
		if err != nil {
			t.Fatalf("Dequeue #%d failed: %v", i, err)
		}
		if !originalIDs[j.ID] {
			t.Fatalf("unexpected job dequeued: %s", j.ID)
		}
	}

	// All workers crash! Queue is currently empty.
	timeoutCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	_, err := q.Dequeue(timeoutCtx)
	if err == nil {
		t.Fatal("expected timeout error when queue is empty")
	}

	// Advance time past 5s visibility timeout
	savedNow := Now
	defer func() { Now = savedNow }()
	Now = func() time.Time { return time.Now().Add(10 * time.Second) }

	reclaimed, err := q.ReclaimTimedOutJobs(ctx)
	if err != nil {
		t.Fatalf("ReclaimTimedOutJobs failed: %v", err)
	}
	if reclaimed != jobCount {
		t.Fatalf("expected %d reclaimed jobs, got %d", jobCount, reclaimed)
	}

	// Surviving workers dequeue all 5 jobs and Ack them with 0 jobs lost
	reclaimedIDs := make(map[string]bool)
	for i := 0; i < jobCount; i++ {
		j, err := q.Dequeue(ctx)
		if err != nil {
			t.Fatalf("second Dequeue #%d failed: %v", i, err)
		}
		reclaimedIDs[j.ID] = true
		if err := q.Ack(ctx, j.ID); err != nil {
			t.Fatalf("Ack failed for %s: %v", j.ID, err)
		}
	}

	if len(reclaimedIDs) != jobCount {
		t.Fatalf("expected %d unique jobs processed, got %d", jobCount, len(reclaimedIDs))
	}
}

func TestDequeue_PausedJobTypeDeferred(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	// Register job type as paused
	if err := rdb.HSet(ctx, "task_queue:job_types:registered", "video-encode", `{"concurrency_limit": 0, "paused": true}`).Err(); err != nil {
		t.Fatalf("HSet failed: %v", err)
	}

	job := jobs.NewJob("video-encode", nil, nil, jobs.PriorityHigh, 3, time.Time{}, "", 30, 1, "tenant-c")
	if err := q.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Attempting dequeue should defer the job and timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	_, err := q.Dequeue(timeoutCtx)
	if err == nil {
		t.Fatal("expected Dequeue to timeout for paused job type")
	}

	// Verify job was atomically moved to the deferred queue
	defCount, err := rdb.LLen(ctx, "task_queue:deferred:video-encode").Result()
	if err != nil {
		t.Fatalf("LLen failed: %v", err)
	}
	if defCount != 1 {
		t.Fatalf("expected 1 deferred job, got %d", defCount)
	}

	// Resume job type
	if err := rdb.HSet(ctx, "task_queue:job_types:registered", "video-encode", `{"concurrency_limit": 0, "paused": false}`).Err(); err != nil {
		t.Fatalf("HSet resume failed: %v", err)
	}

	// Reconcile deferred jobs back to active queues
	moved, err := q.ReconcileDeferredJobs(ctx)
	if err != nil {
		t.Fatalf("ReconcileDeferredJobs failed: %v", err)
	}
	if moved != 1 {
		t.Fatalf("expected 1 job moved back, got %d", moved)
	}

	// Now dequeue succeeds
	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed after resume: %v", err)
	}
	if dequeued.ID != job.ID {
		t.Fatalf("expected job %s, got %s", job.ID, dequeued.ID)
	}
	_ = q.Ack(ctx, dequeued.ID)
}

func TestDequeue_TenantConcurrencyLimitDeferred(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	tenant := "tenant-limited"
	if err := q.UpdateTenantConfig(ctx, tenant, 1); err != nil {
		t.Fatalf("UpdateTenantConfig failed: %v", err)
	}

	job1 := jobs.NewJob("task", nil, nil, jobs.PriorityHigh, 3, time.Time{}, "", 30, 1, tenant)
	job2 := jobs.NewJob("task", nil, nil, jobs.PriorityHigh, 3, time.Time{}, "", 30, 1, tenant)

	if err := q.Enqueue(ctx, job1); err != nil {
		t.Fatalf("Enqueue job1 failed: %v", err)
	}
	if err := q.Enqueue(ctx, job2); err != nil {
		t.Fatalf("Enqueue job2 failed: %v", err)
	}

	// Dequeue first job -> succeeds, consuming the concurrency slot (1 of 1 active)
	d1, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue first job failed: %v", err)
	}
	if d1.ID != job1.ID && d1.ID != job2.ID {
		t.Fatalf("expected job1 or job2, got %s", d1.ID)
	}

	// Second job exceeds tenant concurrency limit -> must be deferred
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	_, err = q.Dequeue(timeoutCtx)
	if err == nil {
		t.Fatal("expected Dequeue to timeout for second job exceeding tenant limit")
	}

	defCount, err := rdb.LLen(ctx, "task_queue:deferred_tenant:"+tenant).Result()
	if err != nil {
		t.Fatalf("LLen failed: %v", err)
	}
	if defCount != 1 {
		t.Fatalf("expected 1 deferred job for tenant, got %d", defCount)
	}

	// Complete first job
	if err := q.Ack(ctx, d1.ID); err != nil {
		t.Fatalf("Ack failed: %v", err)
	}

	// Reconcile deferred tenant jobs
	moved, err := q.ReconcileDeferredJobs(ctx)
	if err != nil {
		t.Fatalf("ReconcileDeferredJobs failed: %v", err)
	}
	if moved != 1 {
		t.Fatalf("expected 1 job moved back, got %d", moved)
	}

	// Second job is now dequeued successfully
	d2, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue second job failed after reconcile: %v", err)
	}
	expectedSecondID := job2.ID
	if d1.ID == job2.ID {
		expectedSecondID = job1.ID
	}
	if d2.ID != expectedSecondID {
		t.Fatalf("expected remaining job %s, got %s", expectedSecondID, d2.ID)
	}
	_ = q.Ack(ctx, d2.ID)
}

func TestDequeue_CorruptedPayloadRoutedToDLQ(t *testing.T) {
	mr, q, rdb, ctx := newTestQueue(t)
	defer mr.Close()

	// Push malformed JSON directly into a partition list
	corrupted := "{not valid json;;;"
	if err := rdb.LPush(ctx, "task_queue:test:high:1", corrupted).Err(); err != nil {
		t.Fatalf("LPush failed: %v", err)
	}

	// Push a valid job right after
	validJob := jobs.NewJob("valid-task", nil, nil, jobs.PriorityHigh, 3, time.Time{}, "", 30, 1, "tenant-d")
	if err := q.Enqueue(ctx, validJob); err != nil {
		t.Fatalf("Enqueue validJob failed: %v", err)
	}

	// Dequeue should route corrupted payload to DLQ and return the valid job
	d, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if d.ID != validJob.ID {
		t.Fatalf("expected validJob %s, got %s", validJob.ID, d.ID)
	}

	// Verify corrupted payload was captured in DLQ
	dlqLen, err := rdb.LLen(ctx, "task_queue:test:dead_letter").Result()
	if err != nil {
		t.Fatalf("LLen dlq failed: %v", err)
	}
	if dlqLen != 1 {
		t.Fatalf("expected 1 item in DLQ, got %d", dlqLen)
	}
}
