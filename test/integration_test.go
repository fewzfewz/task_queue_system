package test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"task-queue-system/internal/jobs"
	queue_redis "task-queue-system/internal/queue/redis"
	"task-queue-system/internal/service"
	"task-queue-system/internal/storage/models"
	"task-queue-system/internal/worker/executor"
	"task-queue-system/internal/logger"
)

// InMemoryStore is a simple map-backed store for testing.
type InMemoryStore struct {
	jobs map[string]*jobs.Job
	mu   sync.RWMutex
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{jobs: make(map[string]*jobs.Job)}
}

func (s *InMemoryStore) Save(ctx context.Context, job *jobs.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
	return nil
}

func (s *InMemoryStore) GetByID(ctx context.Context, id string) (*jobs.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil, nil
	}
	return j, nil
}

func (s *InMemoryStore) UpdateStatus(ctx context.Context, id string, status jobs.JobStatus, workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		j.Status = status
		j.ProcessedBy = workerID
		j.UpdatedAt = time.Now().UTC()
	}
	return nil
}

func (s *InMemoryStore) UpdateResult(ctx context.Context, id string, status jobs.JobStatus, workerID string, result interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		j.Status = status
		j.ProcessedBy = workerID
		j.Result = result
		j.UpdatedAt = time.Now().UTC()
	}
	return nil
}

var _ models.Store = (*InMemoryStore)(nil)

// TestPlugin is a simple plugin for integration tests.
type TestPlugin struct {
	JobType    string
	ShouldFail bool
}

func (p *TestPlugin) Type() string { return p.JobType }
func (p *TestPlugin) Execute(payload map[string]interface{}) (interface{}, error) {
	if p.ShouldFail {
		return nil, fmt.Errorf("intentional failure")
	}
	return "success", nil
}

func TestIntegration_SubmissionAndExecution(t *testing.T) {
	ctx := context.Background()
	log := logger.Setup()
	
	// Setup Redis
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available on localhost:6379, skipping integration test")
	}
	defer client.FlushAll(ctx)

	// Setup System
	store := NewInMemoryStore()
	q := queue_redis.New(client, "test_queue")
	svc := service.New(q, store, log)
	
	exec := executor.NewJobExecutor(log)
	p := &TestPlugin{JobType: "test-success"}
	exec.RegisterPlugin(p)
	
	proc := executor.NewWorkerProcessor("worker-1", svc, exec, nil, log)

	// 1. Submit Job
	job, err := svc.CreateJob(ctx, "test-success", nil, "medium", 3, "")
	if err != nil {
		t.Fatalf("failed to create job: %v", err)
	}

	// 2. Execute
	err = proc.ProcessOnce(ctx)
	if err != nil {
		t.Fatalf("process once failed: %v", err)
	}

	// 3. Verify
	dbJob, _ := store.GetByID(ctx, job.ID)
	if dbJob.Status != jobs.StatusCompleted {
		t.Errorf("expected status completed, got %s", dbJob.Status)
	}
	if dbJob.Result != "success" {
		t.Errorf("expected result 'success', got %v", dbJob.Result)
	}
}

func TestIntegration_RetryMechanism(t *testing.T) {
	ctx := context.Background()
	log := logger.Setup()
	
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping")
	}
	defer client.FlushAll(ctx)

	store := NewInMemoryStore()
	q := queue_redis.New(client, "test_queue")
	svc := service.New(q, store, log)
	
	exec := executor.NewJobExecutor(log)
	p := &TestPlugin{JobType: "test-fail", ShouldFail: true} // This worker will fail
	exec.RegisterPlugin(p)
	
	proc := executor.NewWorkerProcessor("worker-1", svc, exec, nil, log)

	// Submit Job with 1 retry
	job, err := svc.CreateJob(ctx, "test-fail", nil, "medium", 1, "")
	if err != nil {
		t.Fatalf("failed to create job: %v", err)
	}

	// Process 1st attempt (should fail and schedule retry)
	// We handle the backoff by mocking the sleep or just letting it run if short.
	// Actually, WorkerProcessor.retry sleeps. This might slow down tests.
	// For integration tests, we'll just verify it transition to Pending and incremented retries.
	
	// We need to run ProcessOnce in a goroutine because it might block if we don't handle the retry sleep.
	// Actually, ProcessOnce calls retry() which sleeps.
	
	// Let's run it.
	go func() {
		_ = proc.ProcessOnce(ctx)
	}()

	// Wait a bit for the first attempt to finish
	time.Sleep(500 * time.Millisecond)

	dbJob, err := store.GetByID(ctx, job.ID)
	if err != nil || dbJob == nil {
		t.Fatalf("failed to get job from store: %v", err)
	}
	if dbJob.Status != jobs.StatusPending {
		t.Errorf("expected status pending (for retry), got %s", dbJob.Status)
	}
	
	// Since we set max_retries=1, and it failed once, it should have been re-enqueued.
}

func TestIntegration_Scheduling(t *testing.T) {
	ctx := context.Background()
	log := logger.Setup()
	
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping")
	}
	defer client.FlushAll(ctx)

	store := NewInMemoryStore()
	q := queue_redis.New(client, "test_queue")
	svc := service.New(q, store, log)
	
	exec := executor.NewJobExecutor(log)
	p := &TestPlugin{JobType: "test-scheduled"}
	exec.RegisterPlugin(p)
	
	proc := executor.NewWorkerProcessor("worker-1", svc, exec, nil, log)

	// Submit Job scheduled 1 second in the future
	runAt := time.Now().Add(1 * time.Second).Format(time.RFC3339)
	job, err := svc.CreateJob(ctx, "test-scheduled", nil, "medium", 3, runAt)
	if err != nil {
		t.Fatalf("failed to create scheduled job: %v", err)
	}

	// Attempt to process immediately (should find no jobs since it's scheduled)
	_ = proc.ProcessOnce(ctx)
	dbJob, _ := store.GetByID(ctx, job.ID)
	if dbJob != nil && dbJob.Status == jobs.StatusCompleted {
		t.Errorf("job should not have been processed immediately")
	}

	// Wait for maturity
	time.Sleep(1200 * time.Millisecond)

	// Promote
	count, _ := q.PromoteScheduledJobs(ctx)
	if count != 1 {
		t.Errorf("expected 1 job promoted, got %d", count)
	}

	// Process
	_ = proc.ProcessOnce(ctx)

	// Verify
	dbJob, _ = store.GetByID(ctx, job.ID)
	if dbJob.Status != jobs.StatusCompleted {
		t.Errorf("expected status completed after promotion, got %s", dbJob.Status)
	}
}

func dbJobFinished(s *InMemoryStore, id string) bool {
	j, _ := s.GetByID(context.Background(), id)
	return j != nil && j.Status == jobs.StatusCompleted
}
