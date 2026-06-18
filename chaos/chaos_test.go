//go:build chaos
// +build chaos

package chaos

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/logger"
	queue_redis "task-queue-system/internal/queue/redis"
	"task-queue-system/internal/service"
	"task-queue-system/internal/storage/models"

	"github.com/docker/docker/api/types"
	"github.com/redis/go-redis/v9"
)

func TestRedisCrashMidTransition(t *testing.T) {
	cli := requireDocker(t)
	containerID, redisHost, cleanup := newRedisContainer(t, cli)
	defer cleanup()

	binary := buildWorkerBinary(t)
	workerCmd := startWorkerProcess(t, binary, redisHost)
	defer stopProcess(t, workerCmd)

	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 0})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis ping failed: %v", err)
	}

	q := queue_redis.New(rdb, "jobs")
	store := models.NewRedisStore(rdb)
	log := logger.Setup()
	svc := service.New(q, store, log, 0)

	jobIDs := make([]string, 0, 50)
	for i := 0; i < 50; i++ {
		job, err := svc.CreateJob(ctx, "image", map[string]interface{}{
			"source_url": fmt.Sprintf("http://example.com/%d.jpg", i),
			"operation":  "resize",
			"sleep_ms":   5000,
		}, nil, "medium", 1, "", "", "", "", "", 0, 0, "tenant-1", nil, "", nil, "")
		if err != nil {
			t.Fatalf("failed to create job: %v", err)
		}
		jobIDs = append(jobIDs, job.ID)
	}

	start := time.Now()
	time.Sleep(2 * time.Second)
	if err := cli.ContainerKill(ctx, containerID, "KILL"); err != nil {
		t.Fatalf("failed to kill redis container: %v", err)
	}
	time.Sleep(5 * time.Second)
	if err := cli.ContainerStart(ctx, containerID, types.ContainerStartOptions{}); err != nil {
		t.Fatalf("failed to restart redis container: %v", err)
	}

	completed := waitForJobsComplete(t, store, jobIDs, 120*time.Second)
	report := Report{
		Scenario:      "Redis Crash Mid-Transition",
		JobsEnqueued:  len(jobIDs),
		JobsCompleted: completed,
		JobsLost:      len(jobIDs) - completed,
		Duration:      time.Since(start),
		Passed:        completed == len(jobIDs),
	}
	if !report.Passed {
		t.Fatal(report.String())
	}
}

func TestWorkerHardKill(t *testing.T) {
	cli := requireDocker(t)
	_, redisHost, cleanup := newRedisContainer(t, cli)
	defer cleanup()

	binary := buildWorkerBinary(t)
	workerCmd := startWorkerProcess(t, binary, redisHost)

	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 0})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		stopProcess(t, workerCmd)
		t.Fatalf("redis ping failed: %v", err)
	}

	q := queue_redis.New(rdb, "jobs")
	store := models.NewRedisStore(rdb)
	log := logger.Setup()
	svc := service.New(q, store, log, 0)

	jobIDs := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		job, err := svc.CreateJob(ctx, "image", map[string]interface{}{
			"source_url": fmt.Sprintf("http://example.com/hardkill/%d.jpg", i),
			"operation":  "compress",
			"sleep_ms":   5000,
		}, nil, "medium", 1, "", "", "", "", "", 0, 0, "tenant-1", nil, "", nil, "")
		if err != nil {
			stopProcess(t, workerCmd)
			t.Fatalf("failed to create job: %v", err)
		}
		jobIDs = append(jobIDs, job.ID)
	}

	time.Sleep(3 * time.Second)
	killProcess(t, workerCmd)
	workerCmd = startWorkerProcess(t, binary, redisHost)
	defer stopProcess(t, workerCmd)
	start := time.Now()

	completed := waitForJobsComplete(t, store, jobIDs, 120*time.Second)
	report := Report{
		Scenario:      "Worker Hard-Kill",
		JobsEnqueued:  len(jobIDs),
		JobsCompleted: completed,
		JobsLost:      len(jobIDs) - completed,
		Duration:      time.Since(start),
		Passed:        completed == len(jobIDs),
	}
	if !report.Passed {
		t.Fatal(report.String())
	}
}

func TestNetworkPartition(t *testing.T) {
	cli := requireDocker(t)
	_, redisHost, cleanup := newRedisContainer(t, cli)
	defer cleanup()

	binary := buildWorkerBinary(t)
	workerCmd := startWorkerProcess(t, binary, redisHost)
	defer stopProcess(t, workerCmd)

	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 0})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		stopProcess(t, workerCmd)
		t.Fatalf("redis ping failed: %v", err)
	}

	q := queue_redis.New(rdb, "jobs")
	store := models.NewRedisStore(rdb)
	log := logger.Setup()
	svc := service.New(q, store, log, 0)

	jobIDs := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		job, err := svc.CreateJob(ctx, "image", map[string]interface{}{
			"source_url": fmt.Sprintf("http://example.com/partition/%d.jpg", i),
			"operation":  "watermark",
			"sleep_ms":   1000,
		}, nil, "medium", 1, "", "", "", "", "", 0, 0, "tenant-1", nil, "", nil, "")
		if err != nil {
			stopProcess(t, workerCmd)
			t.Fatalf("failed to create job: %v", err)
		}
		jobIDs = append(jobIDs, job.ID)
	}

	time.Sleep(2 * time.Second)
	unblock := blockRedisTraffic(t, "6379")
	defer unblock()
	time.Sleep(10 * time.Second)
	start := time.Now()

	completed := waitForJobsComplete(t, store, jobIDs, 120*time.Second)
	report := Report{
		Scenario:      "Network Partition",
		JobsEnqueued:  len(jobIDs),
		JobsCompleted: completed,
		JobsLost:      len(jobIDs) - completed,
		Duration:      time.Since(start),
		Passed:        completed == len(jobIDs),
	}
	if !report.Passed {
		t.Fatal(report.String())
	}
}

func TestClockSkewOrphanRecovery(t *testing.T) {
	cli := requireDocker(t)
	_, redisHost, cleanup := newRedisContainer(t, cli)
	defer cleanup()

	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 0})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis ping failed: %v", err)
	}

	q := queue_redis.New(rdb, "jobs")
	store := models.NewRedisStore(rdb)
	log := logger.Setup()
	svc := service.New(q, store, log, 0)

	freshJob, err := svc.CreateJob(ctx, "image", map[string]interface{}{
		"source_url": "http://example.com/skew/fresh.jpg",
		"operation":  "process",
		"sleep_ms":   100,
	}, nil, "medium", 1, "", "", "", "", "", 0, 0, "tenant-1", nil, "", nil, "")
	if err != nil {
		t.Fatalf("failed to create fresh job: %v", err)
	}

	staleJob, err := svc.CreateJob(ctx, "image", map[string]interface{}{
		"source_url": "http://example.com/skew/stale.jpg",
		"operation":  "process",
		"sleep_ms":   100,
	}, nil, "medium", 1, "", "", "", "", "", 0, 0, "tenant-1", nil, "", nil, "")
	if err != nil {
		t.Fatalf("failed to create stale job: %v", err)
	}

	// Manually mark both jobs as processing with different visibility windows.
	payloadA, _ := json.Marshal(&jobs.Job{ID: staleJob.ID, Type: staleJob.Type, Priority: staleJob.Priority, TenantID: staleJob.TenantID})
	payloadB, _ := json.Marshal(&jobs.Job{ID: freshJob.ID, Type: freshJob.Type, Priority: freshJob.Priority, TenantID: freshJob.TenantID})
	if err := rdb.HSet(ctx, "task_queue:payloads", staleJob.ID, payloadA).Err(); err != nil {
		t.Fatalf("failed to set stale payload: %v", err)
	}
	if err := rdb.HSet(ctx, "task_queue:payloads", freshJob.ID, payloadB).Err(); err != nil {
		t.Fatalf("failed to set fresh payload: %v", err)
	}
	if err := store.UpdateStatus(ctx, staleJob.ID, jobs.StatusProcessing, "worker-1"); err != nil {
		t.Fatalf("failed to mark stale job processing: %v", err)
	}
	if err := store.UpdateStatus(ctx, freshJob.ID, jobs.StatusProcessing, "worker-1"); err != nil {
		t.Fatalf("failed to mark fresh job processing: %v", err)
	}

	if err := rdb.ZAdd(ctx, "task_queue:in_flight", &redis.Z{Score: float64(time.Now().Add(-2 * time.Hour).Unix()), Member: staleJob.ID}).Err(); err != nil {
		t.Fatalf("failed to add stale in-flight job: %v", err)
	}
	if err := rdb.ZAdd(ctx, "task_queue:in_flight", &redis.Z{Score: float64(time.Now().Add(30 * time.Second).Unix()), Member: freshJob.ID}).Err(); err != nil {
		t.Fatalf("failed to add fresh in-flight job: %v", err)
	}
	start := time.Now()

	originalNow := queue_redis.Now
	queue_redis.Now = func() time.Time { return time.Now().Add(1 * time.Hour) }
	defer func() { queue_redis.Now = originalNow }()

	reclaimed, err := q.ReclaimTimedOutJobs(ctx)
	if err != nil {
		t.Fatalf("failed to reclaim timed out jobs: %v", err)
	}

	if reclaimed != 1 {
		t.Fatalf("expected 1 stale job reclaimed, got %d", reclaimed)
	}

	isStaleInFlight := rdb.ZScore(ctx, "task_queue:in_flight", staleJob.ID).Val()
	isFreshInFlight := rdb.ZScore(ctx, "task_queue:in_flight", freshJob.ID).Val()

	if isFreshInFlight == 0 {
		t.Fatal("fresh processing job was incorrectly reclaimed")
	}

	if _, err := rdb.ZScore(ctx, "task_queue:in_flight", staleJob.ID).Result(); err == nil {
		t.Fatal("stale job still remains in in-flight set after reclaim")
	}

	report := Report{
		Scenario:      "Clock Skew Orphan Recovery",
		JobsEnqueued:  2,
		JobsCompleted: 0,
		JobsLost:      0,
		Duration:      time.Since(start),
		Passed:        reclaimed == 1 && isFreshInFlight != 0,
	}
	if !report.Passed {
		t.Fatal(report.String())
	}
}
