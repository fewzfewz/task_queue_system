package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"task-queue-system/internal/api/dto"
	"task-queue-system/internal/jobs"
	"task-queue-system/internal/queue"
	"task-queue-system/internal/service"
	"task-queue-system/internal/storage/models"
)

var zeroTime time.Time

type mockQueue struct {
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

func newTestHandler() (*JobHandler, *models.InMemoryStore) {
	store := models.NewInMemoryStore()
	q := &mockQueue{}
	svc := service.New(q, store, slog.Default(), 0)
	return New(svc, slog.Default()), store
}

func TestCreateJob_Success(t *testing.T) {
	h, _ := newTestHandler()

	body := `{"type":"email","payload":{"to":"a@b.com"},"priority":"medium","tenant_id":"tenant-a"}`
	req := httptest.NewRequest("POST", "/jobs", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateJob(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var resp dto.JobResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID == "" {
		t.Fatal("expected non-empty job ID")
	}
	if resp.Type != "email" {
		t.Fatalf("expected type 'email', got '%s'", resp.Type)
	}
}

func TestCreateJob_InvalidJSON(t *testing.T) {
	h, _ := newTestHandler()

	body := `not json`
	req := httptest.NewRequest("POST", "/jobs", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateJob(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateJob_MissingType(t *testing.T) {
	h, _ := newTestHandler()

	body := `{"payload":{}}`
	req := httptest.NewRequest("POST", "/jobs", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateJob(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateJob_InvalidType(t *testing.T) {
	h, _ := newTestHandler()

	body := `{"type":"unsupported"}`
	req := httptest.NewRequest("POST", "/jobs", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateJob(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateJob_RateLimited(t *testing.T) {
	store := models.NewInMemoryStore()
	q := &mockQueue{
		isAllowedFunc: func(_ context.Context, tenantID string) (bool, error) {
			return false, nil
		},
	}
	svc := service.New(q, store, slog.Default(), 0)
	h := New(svc, slog.Default())

	body := `{"type":"email","payload":{"to":"a@b.com"},"priority":"medium","tenant_id":"tenant-a"}`
	req := httptest.NewRequest("POST", "/jobs", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateJob(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetJobStatus_Success(t *testing.T) {
	h, _ := newTestHandler()

	// Create a job via the handler to get a real ID
	body := `{"type":"email","payload":{"to":"a@b.com"},"priority":"medium","tenant_id":"tenant-a"}`
	createReq := httptest.NewRequest("POST", "/jobs", bytes.NewReader([]byte(body)))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	h.CreateJob(createW, createReq)

	var created dto.JobResponse
	json.NewDecoder(createW.Body).Decode(&created)

	// Fetch it
	getReq := httptest.NewRequest("GET", "/jobs/"+created.ID, nil)
	getReq.SetPathValue("id", created.ID)
	getW := httptest.NewRecorder()
	h.GetJobStatus(getW, getReq)

	if getW.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", getW.Code, getW.Body.String())
	}

	var fetched dto.JobResponse
	json.NewDecoder(getW.Body).Decode(&fetched)
	if fetched.ID != created.ID {
		t.Fatalf("expected job ID %s, got %s", created.ID, fetched.ID)
	}
}

func TestGetJobStatus_NotFound(t *testing.T) {
	h, _ := newTestHandler()

	req := httptest.NewRequest("GET", "/jobs/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()
	h.GetJobStatus(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetJobStatus_TenantFilter(t *testing.T) {
	h, store := newTestHandler()

	job := jobs.NewJob("email", nil, jobs.PriorityMedium, 3, zeroTime, "", 60, 1, "tenant-a")
	store.Save(context.Background(), job)

	req := httptest.NewRequest("GET", "/jobs/"+job.ID+"?tenant_id=tenant-b", nil)
	req.SetPathValue("id", job.ID)
	w := httptest.NewRecorder()
	h.GetJobStatus(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListFailedJobs(t *testing.T) {
	h, store := newTestHandler()

	job := jobs.NewJob("email", nil, jobs.PriorityMedium, 3, zeroTime, "", 60, 1, "tenant-a")
	job.Status = jobs.StatusFailed
	store.Save(context.Background(), job)

	req := httptest.NewRequest("GET", "/api/v1/dlq?tenant_id=tenant-a", nil)
	w := httptest.NewRecorder()
	h.ListFailedJobs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var jobs []dto.JobResponse
	json.NewDecoder(w.Body).Decode(&jobs)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
}

func TestBulkPurgeDLQ_Success(t *testing.T) {
	h, store := newTestHandler()

	job := jobs.NewJob("email", nil, jobs.PriorityMedium, 3, zeroTime, "", 60, 1, "tenant-a")
	job.Status = jobs.StatusFailed
	store.Save(context.Background(), job)

	req := httptest.NewRequest("DELETE", "/api/v1/dlq?older_than=2026-12-31T23:59:59Z", nil)
	w := httptest.NewRecorder()
	h.BulkPurgeDLQ(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]int64
	json.NewDecoder(w.Body).Decode(&result)
	if result["deleted"] != 1 {
		t.Fatalf("expected 1 deleted, got %d", result["deleted"])
	}
}

func TestBulkPurgeDLQ_MissingTimestamp(t *testing.T) {
	h, _ := newTestHandler()

	req := httptest.NewRequest("DELETE", "/api/v1/dlq", nil)
	w := httptest.NewRecorder()
	h.BulkPurgeDLQ(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}


