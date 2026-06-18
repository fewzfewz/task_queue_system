package executor

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/queue"
	"task-queue-system/internal/service"
	"task-queue-system/internal/storage/models"
	"task-queue-system/internal/worker/plugin"
)

type mockQ struct {
	enqueueFunc           func(ctx context.Context, job *jobs.Job) error
	dequeueFunc           func(ctx context.Context) (*jobs.Job, error)
	ackFunc               func(ctx context.Context, jobID string) error
	failFunc              func(ctx context.Context, jobID string, err error) error
	sizeFunc              func(ctx context.Context) (int64, error)
	isAllowedFunc         func(ctx context.Context, tenantID string) (bool, error)
	getFailedJobsFunc     func(ctx context.Context) ([]*jobs.Job, error)
	getMetricsFunc        func(ctx context.Context) (queue.QueueMetrics, error)
	registerHeartbeatFunc func(ctx context.Context, workerID string) error
	getActiveWorkersFunc  func(ctx context.Context) ([]queue.WorkerInfo, error)
	isProcessedFunc       func(ctx context.Context, jobID string) (bool, error)
	markProcessedFunc     func(ctx context.Context, jobID string) error
	promoteFunc           func(ctx context.Context) (int, error)
	reclaimFunc           func(ctx context.Context) (int, error)
	publishWebhookFunc    func(ctx context.Context, event interface{}) error
}

func (m *mockQ) Enqueue(ctx context.Context, job *jobs.Job) error {
	if m.enqueueFunc != nil {
		return m.enqueueFunc(ctx, job)
	}
	return nil
}
func (m *mockQ) Dequeue(ctx context.Context) (*jobs.Job, error) {
	if m.dequeueFunc != nil {
		return m.dequeueFunc(ctx)
	}
	return nil, nil
}
func (m *mockQ) Ack(ctx context.Context, jobID string) error {
	if m.ackFunc != nil {
		return m.ackFunc(ctx, jobID)
	}
	return nil
}
func (m *mockQ) Fail(ctx context.Context, jobID string, err error) error {
	if m.failFunc != nil {
		return m.failFunc(ctx, jobID, err)
	}
	return nil
}
func (m *mockQ) Size(ctx context.Context) (int64, error) {
	if m.sizeFunc != nil {
		return m.sizeFunc(ctx)
	}
	return 0, nil
}
func (m *mockQ) IsAllowed(ctx context.Context, tenantID string) (bool, error) {
	if m.isAllowedFunc != nil {
		return m.isAllowedFunc(ctx, tenantID)
	}
	return true, nil
}
func (m *mockQ) GetFailedJobs(ctx context.Context) ([]*jobs.Job, error) {
	if m.getFailedJobsFunc != nil {
		return m.getFailedJobsFunc(ctx)
	}
	return nil, nil
}
func (m *mockQ) GetMetrics(ctx context.Context) (queue.QueueMetrics, error) {
	if m.getMetricsFunc != nil {
		return m.getMetricsFunc(ctx)
	}
	return queue.QueueMetrics{}, nil
}
func (m *mockQ) RegisterHeartbeat(ctx context.Context, workerID string) error {
	if m.registerHeartbeatFunc != nil {
		return m.registerHeartbeatFunc(ctx, workerID)
	}
	return nil
}
func (m *mockQ) GetActiveWorkers(ctx context.Context) ([]queue.WorkerInfo, error) {
	if m.getActiveWorkersFunc != nil {
		return m.getActiveWorkersFunc(ctx)
	}
	return nil, nil
}
func (m *mockQ) IsProcessed(ctx context.Context, jobID string) (bool, error) {
	if m.isProcessedFunc != nil {
		return m.isProcessedFunc(ctx, jobID)
	}
	return false, nil
}
func (m *mockQ) MarkProcessed(ctx context.Context, jobID string) error {
	if m.markProcessedFunc != nil {
		return m.markProcessedFunc(ctx, jobID)
	}
	return nil
}
func (m *mockQ) PromoteScheduledJobs(ctx context.Context) (int, error) {
	if m.promoteFunc != nil {
		return m.promoteFunc(ctx)
	}
	return 0, nil
}
func (m *mockQ) ReclaimTimedOutJobs(ctx context.Context) (int, error) {
	if m.reclaimFunc != nil {
		return m.reclaimFunc(ctx)
	}
	return 0, nil
}
func (m *mockQ) PublishWebhookEvent(ctx context.Context, event interface{}) error {
	if m.publishWebhookFunc != nil {
		return m.publishWebhookFunc(ctx, event)
	}
	return nil
}

func setupProcessor(t *testing.T, executeFn func(ctx context.Context, job *jobs.Job) (interface{}, error)) (*WorkerProcessor, *models.InMemoryStore, *mockQ) {
	t.Helper()

	store := models.NewInMemoryStore()
	q := &mockQ{
		dequeueFunc: func(ctx context.Context) (*jobs.Job, error) {
			// retrieve a job from store that's pending
			jobList, _ := store.ListJobs(ctx, "", "pending", "", 1, 0)
			if len(jobList) == 0 {
				// simulate empty queue
				return nil, errors.New("queue: dequeue timed out, no jobs available")
			}
			j := jobList[0]
			j.Status = jobs.StatusProcessing
			return j, nil
		},
		ackFunc: func(ctx context.Context, jobID string) error {
			return nil
		},
		failFunc: func(ctx context.Context, jobID string, err error) error {
			return nil
		},
	}
	svc := service.New(q, store, slog.Default(), 0)

	reg := plugin.NewRegistry()
	reg.Register(&mockPlugin{
		jobType:   "test",
		executeFn: executeFn,
	})
	je := &JobExecutor{registry: reg, circuitBreaker: plugin.NewCircuitBreaker(5, 30*time.Second), logger: slog.Default()}

	wp := NewWorkerProcessor("test-worker", svc, je, nil, slog.Default(), 0)
	return wp, store, q
}

func TestProcessOnce_Success(t *testing.T) {
	wp, store, _ := setupProcessor(t, func(ctx context.Context, job *jobs.Job) (interface{}, error) {
		return "success", nil
	})

	job := jobs.NewJob("test", map[string]interface{}{"key": "val"}, nil, jobs.PriorityMedium, 3, zeroTime, "", 60, 1, "tenant-a")
	store.Save(context.Background(), job)

	err := wp.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	stored, _ := store.GetByID(context.Background(), job.ID)
	if stored.Status != jobs.StatusCompleted {
		t.Fatalf("expected status completed, got %s", stored.Status)
	}
}

func TestProcessOnce_RetryThenSuccess(t *testing.T) {
	attempt := 0
	var mockQ *mockQ

	wp, store, q := setupProcessor(t, func(ctx context.Context, job *jobs.Job) (interface{}, error) {
		attempt++
		if attempt < 2 {
			return nil, errors.New("temporary error")
		}
		return "success", nil
	})
	mockQ = q

	// Need to allow re-enqueue
	mockQ.enqueueFunc = func(ctx context.Context, job *jobs.Job) error {
		return store.Save(ctx, job)
	}

	job := jobs.NewJob("test", nil, nil, jobs.PriorityMedium, 3, zeroTime, "", 60, 1, "tenant-a")
	store.Save(context.Background(), job)

	// First attempt should fail and retry
	err := wp.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("expected no error from first attempt, got %v", err)
	}

	// Second attempt (retried job is now back in queue)
	err = wp.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("expected no error from second attempt, got %v", err)
	}

	stored, _ := store.GetByID(context.Background(), job.ID)
	if stored.Status != jobs.StatusCompleted {
		t.Fatalf("expected status completed, got %s", stored.Status)
	}
}

func TestProcessOnce_PermanentFail(t *testing.T) {
	wp, store, _ := setupProcessor(t, func(ctx context.Context, job *jobs.Job) (interface{}, error) {
		return nil, errors.New("fatal error")
	})

	job := jobs.NewJob("test", nil, nil, jobs.PriorityMedium, 0, zeroTime, "", 60, 1, "tenant-a")
	store.Save(context.Background(), job)

	err := wp.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	stored, _ := store.GetByID(context.Background(), job.ID)
	if stored.Status != jobs.StatusFailed {
		t.Fatalf("expected status failed, got %s", stored.Status)
	}
}

func TestProcessOnce_AlreadyProcessed(t *testing.T) {
	executed := false
	wp, store, q := setupProcessor(t, func(ctx context.Context, job *jobs.Job) (interface{}, error) {
		executed = true
		return "done", nil
	})

	// Save a job with completed status
	job := jobs.NewJob("test", nil, nil, jobs.PriorityMedium, 3, zeroTime, "", 60, 1, "tenant-a")
	job.Status = jobs.StatusCompleted
	store.Save(context.Background(), job)

	// Override dequeue to return this job regardless of status
	q.dequeueFunc = func(ctx context.Context) (*jobs.Job, error) {
		found, _ := store.GetByID(ctx, job.ID)
		if found == nil {
			return nil, errors.New("queue: dequeue timed out, no jobs available")
		}
		found.Status = jobs.StatusProcessing
		return found, nil
	}

	err := wp.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if executed {
		t.Fatal("expected job to NOT be executed (already completed in DB)")
	}
}

func TestProcessOnce_EmptyQueue(t *testing.T) {
	wp, _, _ := setupProcessor(t, nil)

	err := wp.ProcessOnce(context.Background())
	if err == nil {
		t.Fatal("expected error for empty queue")
	}
}

func TestIsEmptyQueueErr(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{errors.New("queue: dequeue timed out, no jobs available"), true},
		{errors.New("redis: nil"), true},
		{errors.New("queue: BRPOP failed: redis: connection pool timeout"), true},
		{errors.New("some other error"), false},
		{nil, false},
	}

	for _, tt := range tests {
		if got := isEmptyQueueErr(tt.err); got != tt.want {
			t.Errorf("isEmptyQueueErr(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestWorkerProcessor_Run_StopsOnContextCancel(t *testing.T) {
	wp, store, _ := setupProcessor(t, func(ctx context.Context, job *jobs.Job) (interface{}, error) {
		time.Sleep(100 * time.Millisecond)
		return "done", nil
	})

	job := jobs.NewJob("test", nil, nil, jobs.PriorityMedium, 3, zeroTime, "", 60, 1, "tenant-a")
	store.Save(context.Background(), job)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	wp.Run(ctx)
}
