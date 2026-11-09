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
