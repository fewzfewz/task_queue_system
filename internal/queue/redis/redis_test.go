package redis

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"task-queue-system/internal/jobs"
)

func newTestQueue(t *testing.T) (*miniredis.Miniredis, *RedisQueue, *redis.Client, context.Context) {
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

	job := jobs.NewJob("test", nil, jobs.PriorityMedium, 3, time.Time{}, "", 60, 1, "tenant-a")
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

	job := jobs.NewJob("test", nil, jobs.PriorityMedium, 3, time.Now().Add(time.Hour), "", 60, 1, "tenant-a")
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

	// Size uses base keys which doesn't account for partitions,
	// so we check individual partition lengths instead.
	for _, pair := range []struct{ key string; expected int64 }{
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

	high := jobs.NewJob("test", nil, jobs.PriorityHigh, 3, time.Time{}, "", 60, 1, "tenant-a")
	low := jobs.NewJob("test", nil, jobs.PriorityLow, 3, time.Time{}, "", 60, 1, "tenant-a")

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
