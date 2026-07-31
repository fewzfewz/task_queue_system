package service

import (
	"context"
	"testing"
	"time"

	"log/slog"

	"task-queue-system/internal/errors"
	"task-queue-system/internal/jobs"
	"task-queue-system/internal/queue"
	"task-queue-system/internal/storage/models"
)

// mockQueue implements queue.Queue for testing.
type mockQueue struct {
	enqueueFunc           func(ctx context.Context, job *jobs.Job) error
	dequeueFunc           func(ctx context.Context) (*jobs.Job, error)
	ackFunc               func(ctx context.Context, jobID string) error
	failFunc              func(ctx context.Context, jobID string, err error) error
	sizeFunc              func(ctx context.Context) (int64, error)
	isAllowedFunc         func(ctx context.Context, tenantID string) (bool, error)
	rateLimitStatusFunc   func(ctx context.Context) ([]queue.TenantRateStatus, error)
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

func (m *mockQueue) Enqueue(ctx context.Context, job *jobs.Job) error {
	if m.enqueueFunc != nil {
		return m.enqueueFunc(ctx, job)
	}
	return nil
}

func (m *mockQueue) Dequeue(ctx context.Context) (*jobs.Job, error) {
	if m.dequeueFunc != nil {
		return m.dequeueFunc(ctx)
	}
	return nil, nil
}

func (m *mockQueue) Ack(ctx context.Context, jobID string) error {
	if m.ackFunc != nil {
		return m.ackFunc(ctx, jobID)
	}
	return nil
}

func (m *mockQueue) Fail(ctx context.Context, jobID string, err error) error {
	if m.failFunc != nil {
		return m.failFunc(ctx, jobID, err)
	}
	return nil
}

func (m *mockQueue) Size(ctx context.Context) (int64, error) {
	if m.sizeFunc != nil {
		return m.sizeFunc(ctx)
	}
	return 0, nil
}

func (m *mockQueue) IsAllowed(ctx context.Context, tenantID string) (bool, error) {
	if m.isAllowedFunc != nil {
		return m.isAllowedFunc(ctx, tenantID)
	}
	return true, nil
}

func (m *mockQueue) RateLimitStatus(ctx context.Context) ([]queue.TenantRateStatus, error) {
	if m.rateLimitStatusFunc != nil {
		return m.rateLimitStatusFunc(ctx)
	}
	return []queue.TenantRateStatus{}, nil
}

func (m *mockQueue) PriorityPartitionDepths(_ context.Context) (queue.PriorityDepthReport, error) {
	return queue.PriorityDepthReport{}, nil
}

func (m *mockQueue) GetFailedJobs(ctx context.Context) ([]*jobs.Job, error) {
	if m.getFailedJobsFunc != nil {
		return m.getFailedJobsFunc(ctx)
	}
	return nil, nil
}

func (m *mockQueue) GetMetrics(ctx context.Context) (queue.QueueMetrics, error) {
	if m.getMetricsFunc != nil {
		return m.getMetricsFunc(ctx)
	}
	return queue.QueueMetrics{}, nil
}

func (m *mockQueue) RegisterHeartbeat(ctx context.Context, workerID string) error {
	if m.registerHeartbeatFunc != nil {
		return m.registerHeartbeatFunc(ctx, workerID)
	}
	return nil
}

func (m *mockQueue) GetActiveWorkers(ctx context.Context) ([]queue.WorkerInfo, error) {
	if m.getActiveWorkersFunc != nil {
		return m.getActiveWorkersFunc(ctx)
	}
	return nil, nil
}

func (m *mockQueue) IsProcessed(ctx context.Context, jobID string) (bool, error) {
	if m.isProcessedFunc != nil {
		return m.isProcessedFunc(ctx, jobID)
	}
	return false, nil
}

func (m *mockQueue) MarkProcessed(ctx context.Context, jobID string) error {
	if m.markProcessedFunc != nil {
		return m.markProcessedFunc(ctx, jobID)
	}
	return nil
}

func (m *mockQueue) PromoteScheduledJobs(ctx context.Context) (int, error) {
	if m.promoteFunc != nil {
		return m.promoteFunc(ctx)
	}
	return 0, nil
}

func (m *mockQueue) ReclaimTimedOutJobs(ctx context.Context) (int, error) {
	if m.reclaimFunc != nil {
		return m.reclaimFunc(ctx)
	}
	return 0, nil
}

func (m *mockQueue) PublishWebhookEvent(ctx context.Context, event interface{}) error {
	if m.publishWebhookFunc != nil {
		return m.publishWebhookFunc(ctx, event)
	}
	return nil
}

func newService(q queue.Queue, store models.Store) *JobService {
	return New(q, store, slog.Default(), 0)
}

func TestCreateJob_Valid(t *testing.T) {
	store := models.NewInMemoryStore()
	q := &mockQueue{}
	svc := newService(q, store)

	job, err := svc.CreateJob(context.Background(), "email", map[string]interface{}{"to": "a@b.com"}, nil, "medium", 3, "", "", "", "", "", 60, 1, "tenant-a", nil, "", nil, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if job.ID == "" {
		t.Fatal("expected non-empty job ID")
	}
	if job.Status != jobs.StatusPending {
		t.Fatalf("expected status pending, got %s", job.Status)
	}
	if job.TenantID != "tenant-a" {
		t.Fatalf("expected tenant-a, got %s", job.TenantID)
	}

	// Verify it was persisted
	stored, err := store.GetByID(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("expected job to be persisted: %v", err)
	}
	if stored.Type != "email" {
		t.Fatalf("expected job type 'email', got '%s'", stored.Type)
	}
}

func TestCreateJob_InvalidType(t *testing.T) {
	svc := newService(&mockQueue{}, models.NewInMemoryStore())

	_, err := svc.CreateJob(context.Background(), "invalid-type", nil, nil, "medium", 3, "", "", "", "", "", 60, 1, "tenant-a", nil, "", nil, "")
	if err == nil {
		t.Fatal("expected error for invalid job type")
	}
	if !errors.IsCode(err, errors.CodeInvalidArgument) {
		t.Fatalf("expected InvalidArgument error, got %v", err)
	}
}

func TestCreateJob_RateLimited(t *testing.T) {
	store := models.NewInMemoryStore()
	q := &mockQueue{
		isAllowedFunc: func(_ context.Context, tenantID string) (bool, error) {
			return false, nil
		},
	}
	svc := newService(q, store)

	_, err := svc.CreateJob(context.Background(), "email", map[string]interface{}{"to": "a@b.com"}, nil, "medium", 3, "", "", "", "", "", 60, 1, "tenant-a", nil, "", nil, "")
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if !errors.IsCode(err, errors.CodeTooManyRequests) {
		t.Fatalf("expected TooManyRequests error, got %v", err)
	}
}

func TestCreateJob_QueueFull(t *testing.T) {
	store := models.NewInMemoryStore()
	q := &mockQueue{
		sizeFunc: func(_ context.Context) (int64, error) {
			return 5, nil
		},
	}
	svc := New(q, store, slog.Default(), 5)

	_, err := svc.CreateJob(context.Background(), "email", map[string]interface{}{"to": "a@b.com"}, nil, "medium", 3, "", "", "", "", "", 60, 1, "tenant-a", nil, "", nil, "")
	if err == nil {
		t.Fatal("expected queue full error")
	}
	if !errors.IsCode(err, errors.CodeQueueFull) {
		t.Fatalf("expected QueueFull error, got %v", err)
	}
}

func TestCreateJob_DefaultMaxRetries(t *testing.T) {
	store := models.NewInMemoryStore()
	svc := newService(&mockQueue{}, store)

	job, err := svc.CreateJob(context.Background(), "email", nil, nil, "medium", 0, "", "", "", "", "", 60, 1, "tenant-a", nil, "", nil, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if job.MaxRetries != 3 {
		t.Fatalf("expected default max retries of 3, got %d", job.MaxRetries)
	}
}

func TestCreateJob_FutureSchedule(t *testing.T) {
	store := models.NewInMemoryStore()
	svc := newService(&mockQueue{}, store)

	future := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	job, err := svc.CreateJob(context.Background(), "email", nil, nil, "medium", 3, "", "", "", future, "", 60, 1, "tenant-a", nil, "", nil, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if job.RunAt.Before(time.Now()) {
		t.Fatal("expected RunAt to be in the future")
	}
}

func TestCreateJob_PastSchedule(t *testing.T) {
	svc := newService(&mockQueue{}, models.NewInMemoryStore())

	past := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	_, err := svc.CreateJob(context.Background(), "email", nil, nil, "medium", 3, "", "", "", past, "", 60, 1, "tenant-a", nil, "", nil, "")
	if err == nil {
		t.Fatal("expected error for past schedule")
	}
}

func TestGetJobStatus_Found(t *testing.T) {
	store := models.NewInMemoryStore()
	svc := newService(&mockQueue{}, store)

	created, _ := svc.CreateJob(context.Background(), "email", nil, nil, "medium", 3, "", "", "", "", "", 60, 1, "tenant-a", nil, "", nil, "")
	found, err := svc.GetJobStatus(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if found.ID != created.ID {
		t.Fatalf("expected job ID %s, got %s", created.ID, found.ID)
	}
}

func TestGetJobStatus_NotFound(t *testing.T) {
	svc := newService(&mockQueue{}, models.NewInMemoryStore())

	_, err := svc.GetJobStatus(context.Background(), "nonexistent-id")
	if err == nil {
		t.Fatal("expected error for nonexistent job")
	}
	if !errors.IsCode(err, errors.CodeNotFound) {
		t.Fatalf("expected NotFound error, got %v", err)
	}
}

func TestReplayJob(t *testing.T) {
	store := models.NewInMemoryStore()
	svc := newService(&mockQueue{}, store)

	created, _ := svc.CreateJob(context.Background(), "email", nil, nil, "medium", 3, "", "", "", "", "", 60, 1, "tenant-a", nil, "", nil, "")
	created.Status = jobs.StatusFailed
	store.Save(context.Background(), created)

	replayed, err := svc.ReplayJob(context.Background(), created.ID, "tenant-a")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if replayed.Status != jobs.StatusPending {
		t.Fatalf("expected status pending after replay, got %s", replayed.Status)
	}
	if replayed.Retries != 0 {
		t.Fatalf("expected retries reset to 0, got %d", replayed.Retries)
	}
}

func TestReplayJob_WrongTenant(t *testing.T) {
	store := models.NewInMemoryStore()
	svc := newService(&mockQueue{}, store)

	created, _ := svc.CreateJob(context.Background(), "email", nil, nil, "medium", 3, "", "", "", "", "", 60, 1, "tenant-a", nil, "", nil, "")
	created.Status = jobs.StatusFailed
	store.Save(context.Background(), created)

	_, err := svc.ReplayJob(context.Background(), created.ID, "tenant-b")
	if err == nil {
		t.Fatal("expected error for wrong tenant")
	}
	if !errors.IsCode(err, errors.CodePermissionDenied) {
		t.Fatalf("expected PermissionDenied error, got %v", err)
	}
}

func TestDeleteJob(t *testing.T) {
	store := models.NewInMemoryStore()
	svc := newService(&mockQueue{}, store)

	created, _ := svc.CreateJob(context.Background(), "email", nil, nil, "medium", 3, "", "", "", "", "", 60, 1, "tenant-a", nil, "", nil, "")

	err := svc.DeleteJob(context.Background(), created.ID, "tenant-a")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, err = store.GetByID(context.Background(), created.ID)
	if err == nil {
		t.Fatal("expected job to be deleted")
	}
}

func TestUpdateJobResult(t *testing.T) {
	store := models.NewInMemoryStore()
	q := &mockQueue{}
	svc := newService(q, store)

	created, _ := svc.CreateJob(context.Background(), "email", nil, nil, "medium", 3, "", "", "", "", "", 60, 1, "tenant-a", nil, "", nil, "")

	err := svc.UpdateJobResult(context.Background(), created.ID, jobs.StatusCompleted, "worker-1", "success")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	updated, _ := store.GetByID(context.Background(), created.ID)
	if updated.Status != jobs.StatusCompleted {
		t.Fatalf("expected status completed, got %s", updated.Status)
	}
}

func TestReconcileOrphanedJobs(t *testing.T) {
	store := models.NewInMemoryStore()
	svc := newService(&mockQueue{}, store)

	created, _ := svc.CreateJob(context.Background(), "email", nil, nil, "medium", 3, "", "", "", "", "", 60, 1, "tenant-a", nil, "", nil, "")
	created.Status = jobs.StatusProcessing
	created.ProcessedBy = "worker-1"
	store.Save(context.Background(), created)

	count, err := svc.ReconcileOrphanedJobs(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 orphaned job, got %d", count)
	}

	reconciled, _ := store.GetByID(context.Background(), created.ID)
	if reconciled.Status != jobs.StatusPending {
		t.Fatalf("expected status pending after reconcile, got %s", reconciled.Status)
	}
}

func TestBulkPurgeDLQ(t *testing.T) {
	store := models.NewInMemoryStore()
	svc := newService(&mockQueue{}, store)

	old := time.Now().Add(-48 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour)

	for i := 0; i < 3; i++ {
		job, _ := svc.CreateJob(context.Background(), "email", nil, nil, "medium", 3, "", "", "", "", "", 60, 1, "tenant-a", nil, "", nil, "")
		job.Status = jobs.StatusFailed
		job.CreatedAt = old
		store.Save(context.Background(), job)
	}
	for i := 0; i < 2; i++ {
		job, _ := svc.CreateJob(context.Background(), "email", nil, nil, "medium", 3, "", "", "", "", "", 60, 1, "tenant-a", nil, "", nil, "")
		job.Status = jobs.StatusFailed
		job.CreatedAt = recent
		store.Save(context.Background(), job)
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	count, err := svc.BulkPurgeDLQ(context.Background(), "", "", cutoff)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 deleted jobs, got %d", count)
	}
}
