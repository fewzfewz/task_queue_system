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
	"task-queue-system/internal/api/middleware"
	"task-queue-system/internal/api/session"
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
func (m *mockQueue) UpdateTenantConfig(ctx context.Context, tenantID string, concurrencyLimit int) error { return nil }
func (m *mockQueue) RateLimitStatus(ctx context.Context) ([]queue.TenantRateStatus, error) {
	if m.rateLimitStatusFunc != nil {
		return m.rateLimitStatusFunc(ctx)
	}
	return []queue.TenantRateStatus{}, nil
}
func (m *mockQueue) PriorityPartitionDepths(_ context.Context) (queue.PriorityDepthReport, error) {
	return queue.PriorityDepthReport{
		DequeueWeights:        map[string]int{"high": 70, "medium": 20, "low": 10},
		PartitionsPerPriority: 3,
		ByPriority:            map[string]queue.PriorityTierDepth{},
	}, nil
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
	sessions := session.NewStore(time.Hour)
	return New(svc, slog.Default(), "test-api-key", "admin", "admin123", sessions, "localhost:8081", 5, 10), store
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
	sessions := session.NewStore(time.Hour)
	h := New(svc, slog.Default(), "test-api-key", "admin", "admin123", sessions, "localhost:8081", 5, 10)

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

	job := jobs.NewJob("email", nil, nil, jobs.PriorityMedium, 3, zeroTime, "", 60, 1, "tenant-a")
	store.Save(context.Background(), job)

	req := httptest.NewRequest("GET", "/jobs/"+job.ID, nil)
	req.SetPathValue("id", job.ID)
	ctx := context.WithValue(req.Context(), middleware.ContextKeyTenantID, "tenant-b")
	ctx = context.WithValue(ctx, middleware.ContextKeyAuthType, middleware.AuthTypeAPIKey)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.GetJobStatus(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListFailedJobs(t *testing.T) {
	h, store := newTestHandler()

	job := jobs.NewJob("email", nil, nil, jobs.PriorityMedium, 3, zeroTime, "", 60, 1, "tenant-a")
	job.Status = jobs.StatusFailed
	store.Save(context.Background(), job)

	req := httptest.NewRequest("GET", "/api/v1/dlq?tenant_id=tenant-a", nil)
	w := httptest.NewRecorder()
	h.ListFailedJobs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	jobs, _ := result["jobs"].([]interface{})
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if int(result["total"].(float64)) != 1 {
		t.Fatalf("expected total 1, got %v", result["total"])
	}
}

func TestBulkPurgeDLQ_Success(t *testing.T) {
	h, store := newTestHandler()

	job := jobs.NewJob("email", nil, nil, jobs.PriorityMedium, 3, zeroTime, "", 60, 1, "tenant-a")
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

func TestSSEEventMatchesTenant(t *testing.T) {
	tests := []struct {
		name       string
		msg        string
		tenantID   string
		wantMatch  bool
	}{
		{
			name:       "matching tenant",
			msg:        "data: {\"kind\":\"job\",\"job_id\":\"123\",\"status\":\"completed\",\"type\":\"email\",\"tenant_id\":\"tenant-a\"}\n\n",
			tenantID:   "tenant-a",
			wantMatch:  true,
		},
		{
			name:       "non-matching tenant",
			msg:        "data: {\"kind\":\"job\",\"job_id\":\"123\",\"status\":\"completed\",\"type\":\"email\",\"tenant_id\":\"tenant-b\"}\n\n",
			tenantID:   "tenant-a",
			wantMatch:  false,
		},
		{
			name:       "empty tenant in event matches nothing",
			msg:        "data: {\"kind\":\"job\",\"job_id\":\"123\",\"status\":\"completed\",\"type\":\"email\",\"tenant_id\":\"\"}\n\n",
			tenantID:   "tenant-a",
			wantMatch:  false,
		},
		{
			name:       "malformed message is delivered (lenient)",
			msg:        `not valid json`,
			tenantID:   "tenant-a",
			wantMatch:  true,
		},
		{
			name:       "rate_limit event with matching tenant",
			msg:        "data: {\"kind\":\"rate_limit\",\"type\":\"rate_limit\",\"status\":\"rejected\",\"tenant_id\":\"tenant-a\"}\n\n",
			tenantID:   "tenant-a",
			wantMatch:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sseEventMatchesTenant(tt.msg, tt.tenantID)
			if got != tt.wantMatch {
				t.Errorf("sseEventMatchesTenant(%q, %q) = %v, want %v", tt.msg, tt.tenantID, got, tt.wantMatch)
			}
		})
	}
}



func (m *mockQueue) ReconcileDeferredJobs(ctx context.Context) (int, error) { return 0, nil }

func (m *mockQueue) PublishCancellation(ctx context.Context, jobID string) error { return nil }
func (m *mockQueue) SubscribeCancellations(ctx context.Context) (<-chan string, error) { ch := make(chan string); return ch, nil }

// Panic Button mocks
func (m *mockQueue) PauseJobType(ctx context.Context, jobType string) error { return nil }
func (m *mockQueue) ResumeJobType(ctx context.Context, jobType string) error { return nil }
func (m *mockQueue) IsJobTypePaused(ctx context.Context, jobType string) (bool, error) { return false, nil }
func (m *mockQueue) GetPausedJobTypes(ctx context.Context) ([]string, error) { return nil, nil }
