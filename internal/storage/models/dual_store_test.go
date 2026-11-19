package models

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"task-queue-system/internal/jobs"
	"task-queue-system/internal/metrics"
)

// mockStore implements a minimal in-memory Store for testing DualStore delegation.
type mockStore struct {
	LogAuditActionFunc func(ctx context.Context, log *AuditLog) error
	ListAuditLogsFunc  func(ctx context.Context, limit, offset int) ([]*AuditLog, error)
	Store
	saveCalled             int
	getByIDCalled          int
	updateStatusCalled     int
	updateProgressCalled   int
	updateResultCalled     int
	deleteJobCalled        int
	deleteJobsBeforeCalled int
	enqueueCalled          int
	dequeueCalled          int
	heartbeatCalled        int
	completeCalled         int
	failCalled             int
	listJobsCalled         int
	searchJobsCalled       int
	recoverOrphansCalled   int
	dedupCalled            int
	getByIDsCalled         int
	getQueueLengthsCalled  int
	getByWorkerCalled      int
	jobs                   map[string]*jobs.Job
}

func newMockStore() *mockStore {
	return &mockStore{jobs: make(map[string]*jobs.Job)}
}

func (m *mockStore) Save(ctx context.Context, job *jobs.Job) error {
	m.saveCalled++
	m.jobs[job.ID] = job
	return nil
}

func (m *mockStore) GetByID(ctx context.Context, id string) (*jobs.Job, error) {
	m.getByIDCalled++
	j, ok := m.jobs[id]
	if !ok {
		return nil, ErrJobNotFound
	}
	return j, nil
}

func (m *mockStore) UpdateStatus(ctx context.Context, id string, status jobs.JobStatus, workerID string) error {
	m.updateStatusCalled++
	return nil
}

func (m *mockStore) UpdateProgress(ctx context.Context, id string, progress float64) error {
	m.updateProgressCalled++
	return nil
}

func (m *mockStore) UpdateResult(ctx context.Context, id string, status jobs.JobStatus, workerID string, result interface{}) error {
	m.updateResultCalled++
	return nil
}

func (m *mockStore) DeleteJob(ctx context.Context, jobID string) error {
	m.deleteJobCalled++
	return nil
}

func (m *mockStore) DeleteJobsBefore(ctx context.Context, tenantID, status, jobType string, before time.Time) (int64, error) {
	m.deleteJobsBeforeCalled++
	return 0, nil
}

func (m *mockStore) Enqueue(ctx context.Context, job *jobs.Job) error {
	m.enqueueCalled++
	return nil
}

func (m *mockStore) Dequeue(ctx context.Context, tenantID string, shardKey string) (*jobs.Job, error) {
	m.dequeueCalled++
	for _, j := range m.jobs {
		if j.Status == jobs.StatusPending || j.Status == "" {
			j.Status = jobs.StatusProcessing
			return j, nil
		}
	}
	return nil, nil
}

func (m *mockStore) Heartbeat(ctx context.Context, jobID string) error {
	m.heartbeatCalled++
	return nil
}

func (m *mockStore) Complete(ctx context.Context, jobID string, result interface{}) error {
	m.completeCalled++
	return nil
}

func (m *mockStore) Fail(ctx context.Context, jobID string, err error, requeue bool) error {
	m.failCalled++
	return nil
}

func (m *mockStore) ListJobs(ctx context.Context, tenantID string, status string, typeStr string, limit, offset int) ([]*jobs.Job, error) {
	m.listJobsCalled++
	return nil, nil
}

func (m *mockStore) SearchJobs(ctx context.Context, filter JobFilter) ([]*jobs.Job, error) {
	m.searchJobsCalled++
	return nil, nil
}

func (m *mockStore) CountJobs(ctx context.Context, filter JobFilter) (int64, error) {
	return int64(len(m.jobs)), nil
}

func (m *mockStore) RecoverOrphans(ctx context.Context, timeout time.Duration) (int64, error) {
	m.recoverOrphansCalled++
	return 0, nil
}

func (m *mockStore) IsDedupKeyTaken(ctx context.Context, dedupKey, tenantID string) (bool, error) {
	m.dedupCalled++
	return false, nil
}

func (m *mockStore) GetByIDs(ctx context.Context, ids []string) ([]*jobs.Job, error) {
	m.getByIDsCalled++
	return nil, nil
}

func (m *mockStore) GetQueueLengths(ctx context.Context) (map[string]map[string]int64, error) {
	m.getQueueLengthsCalled++
	return nil, nil
}

func (m *mockStore) GetByWorkerAndStatus(ctx context.Context, workerID string, status jobs.JobStatus) ([]*jobs.Job, error) {
	m.getByWorkerCalled++
	return nil, nil
}

func (m *mockStore) RegisterClient(ctx context.Context, tenantID, apiKeyHash string) error {
	return nil
}
func (m *mockStore) VerifyClient(ctx context.Context, apiKeyHash string) (string, error) {
	return "", ErrClientNotFound
}
func (m *mockStore) ListClients(ctx context.Context) ([]*ClientRecord, error) { return nil, nil }
func (m *mockStore) RevokeClient(ctx context.Context, tenantID string) error  { return nil }

func (m *mockStore) SaveBatch(ctx context.Context, batch []*jobs.Job) error {
	m.saveCalled += len(batch)
	for _, j := range batch {
		m.jobs[j.ID] = j
	}
	return nil
}
func (m *mockStore) ArchiveOldJobs(ctx context.Context, cutoff time.Time) (int64, error) {
	return 0, nil
}
func (m *mockStore) MoveToDLQ(ctx context.Context, jobID string, reason string) error {
	return nil
}
func (m *mockStore) ListDLQ(ctx context.Context, tenantID, jobType string, limit, offset int) ([]*jobs.Job, error) {
	return nil, nil
}
func (m *mockStore) CountDLQ(ctx context.Context, tenantID, jobType string) (int64, error) {
	return 0, nil
}
func (m *mockStore) DeleteDLQBefore(ctx context.Context, tenantID, jobType string, before time.Time) (int64, error) {
	return 0, nil
}
func (m *mockStore) GetFromDLQ(ctx context.Context, jobID string) (*jobs.Job, error) {
	return nil, ErrJobNotFound
}
func (m *mockStore) DeleteFromDLQ(ctx context.Context, jobID string) error {
	return nil
}
func (m *mockStore) GetReadyDAGJobs(ctx context.Context, limit int) ([]*jobs.Job, error) {
	return nil, nil
}
func (m *mockStore) GetDependentJobs(ctx context.Context, parentID string) ([]*jobs.Job, error) {
	return nil, nil
}

func TestDualStoreSave(t *testing.T) {
	primary := newMockStore()
	secondary := newMockStore()
	d := NewDualStore(primary, secondary)

	job := &jobs.Job{ID: "job-1", Type: "email", TenantID: "tenant-a"}
	ctx := context.Background()

	if err := d.Save(ctx, job); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if primary.saveCalled != 1 {
		t.Fatalf("expected primary.Save called 1, got %d", primary.saveCalled)
	}
	if secondary.saveCalled != 1 {
		t.Fatalf("expected secondary.Save called 1, got %d", secondary.saveCalled)
	}
}

func TestDualStoreGetByID(t *testing.T) {
	primary := newMockStore()
	secondary := newMockStore()
	d := NewDualStore(primary, secondary)

	ctx := context.Background()
	primary.jobs["job-1"] = &jobs.Job{ID: "job-1"}

	job, err := d.GetByID(ctx, "job-1")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if job.ID != "job-1" {
		t.Fatalf("expected job-1, got %s", job.ID)
	}
	if primary.getByIDCalled != 1 {
		t.Fatalf("expected primary.GetByID called, got %d", primary.getByIDCalled)
	}
	if secondary.getByIDCalled != 0 {
		t.Fatal("expected secondary.GetByID not called")
	}
}

func TestDualStoreDelegatesToPrimaryAndSecondary(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		exec func(primary, secondary *mockStore)
	}{
		{"UpdateStatus", func(p, s *mockStore) { _ = NewDualStore(p, s).UpdateStatus(ctx, "j1", jobs.StatusCompleted, "w1") }},
		{"UpdateProgress", func(p, s *mockStore) { _ = NewDualStore(p, s).UpdateProgress(ctx, "j1", 0.5) }},
		{"UpdateResult", func(p, s *mockStore) { _ = NewDualStore(p, s).UpdateResult(ctx, "j1", jobs.StatusFailed, "w1", "err") }},
		{"Enqueue", func(p, s *mockStore) { _ = NewDualStore(p, s).Enqueue(ctx, &jobs.Job{ID: "j1"}) }},
		{"Heartbeat", func(p, s *mockStore) { _ = NewDualStore(p, s).Heartbeat(ctx, "j1") }},
		{"Complete", func(p, s *mockStore) { _ = NewDualStore(p, s).Complete(ctx, "j1", nil) }},
		{"Fail", func(p, s *mockStore) { _ = NewDualStore(p, s).Fail(ctx, "j1", nil, false) }},
		{"DeleteJob", func(p, s *mockStore) { _ = NewDualStore(p, s).DeleteJob(ctx, "j1") }},
		{"DeleteJobsBefore", func(p, s *mockStore) { _, _ = NewDualStore(p, s).DeleteJobsBefore(ctx, "", "", "", time.Now()) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newMockStore()
			s := newMockStore()
			tt.exec(p, s)
			if p.saveCalled != 0 && tt.name != "Save" {
				t.Fatalf("%s: expected primary not Save, got saveCalled=%d", tt.name, p.saveCalled)
			}
			if p.getByIDCalled != 0 {
				t.Fatalf("%s: expected primary.GetByID not called", tt.name)
			}
		})
	}
}

func TestDualStoreDelegatesToPrimaryOnly(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		check func(p, s *mockStore) (primaryCalls, secondaryCalls int)
	}{
		{"GetByID", func(p, s *mockStore) (int, int) {
			_, _ = NewDualStore(p, s).GetByID(ctx, "j1")
			return p.getByIDCalled, s.getByIDCalled
		}},
		{"ListJobs", func(p, s *mockStore) (int, int) {
			_, _ = NewDualStore(p, s).ListJobs(ctx, "", "", "", 0, 0)
			return p.listJobsCalled, s.listJobsCalled
		}},
		{"SearchJobs", func(p, s *mockStore) (int, int) {
			_, _ = NewDualStore(p, s).SearchJobs(ctx, JobFilter{})
			return p.searchJobsCalled, s.searchJobsCalled
		}},
		{"RecoverOrphans", func(p, s *mockStore) (int, int) {
			_, _ = NewDualStore(p, s).RecoverOrphans(ctx, time.Minute)
			return p.recoverOrphansCalled, s.recoverOrphansCalled
		}},
		{"IsDedupKeyTaken", func(p, s *mockStore) (int, int) {
			_, _ = NewDualStore(p, s).IsDedupKeyTaken(ctx, "k", "t")
			return p.dedupCalled, s.dedupCalled
		}},
		{"GetByIDs", func(p, s *mockStore) (int, int) {
			_, _ = NewDualStore(p, s).GetByIDs(ctx, nil)
			return p.getByIDsCalled, s.getByIDsCalled
		}},
		{"GetQueueLengths", func(p, s *mockStore) (int, int) {
			_, _ = NewDualStore(p, s).GetQueueLengths(ctx)
			return p.getQueueLengthsCalled, s.getQueueLengthsCalled
		}},
		{"GetByWorkerAndStatus", func(p, s *mockStore) (int, int) {
			_, _ = NewDualStore(p, s).GetByWorkerAndStatus(ctx, "w", jobs.StatusProcessing)
			return p.getByWorkerCalled, s.getByWorkerCalled
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newMockStore()
			s := newMockStore()
			pCalls, sCalls := tt.check(p, s)
			if pCalls != 1 {
				t.Fatalf("expected primary called 1, got %d", pCalls)
			}
			if sCalls != 0 {
				t.Fatalf("expected secondary not called, got %d", sCalls)
			}
		})
	}
}

func TestDualStoreDequeueSyncsSecondary(t *testing.T) {
	primary := newMockStore()
	secondary := newMockStore()
	d := NewDualStore(primary, secondary)

	primary.jobs["job-1"] = &jobs.Job{ID: "job-1", ProcessedBy: "worker-1"}

	job, err := d.Dequeue(context.Background(), "", "")
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if job == nil {
		t.Fatal("expected a job from Dequeue")
	}
	if primary.dequeueCalled != 1 {
		t.Fatalf("expected primary.Dequeue called 1, got %d", primary.dequeueCalled)
	}
	if secondary.updateStatusCalled != 1 {
		t.Fatalf("expected secondary.UpdateStatus called 1, got %d", secondary.updateStatusCalled)
	}
}

func TestDualStoreImplementsStore(t *testing.T) {
	var _ Store = (*DualStore)(nil)
}

func (m *mockStore) LogAuditAction(ctx context.Context, log *AuditLog) error {
	if m.LogAuditActionFunc != nil {
		return m.LogAuditActionFunc(ctx, log)
	}
	return nil
}
func (m *mockStore) ListAuditLogs(ctx context.Context, limit, offset int) ([]*AuditLog, error) {
	if m.ListAuditLogsFunc != nil {
		return m.ListAuditLogsFunc(ctx, limit, offset)
	}
	return nil, nil
}

// failingSecondaryStore is a Store mock that fails all mutation operations.
type failingSecondaryStore struct {
	mockStore
	failErr error
}

func newFailingSecondaryStore(err error) *failingSecondaryStore {
	return &failingSecondaryStore{
		mockStore: *newMockStore(),
		failErr:   err,
	}
}

func (f *failingSecondaryStore) Save(ctx context.Context, job *jobs.Job) error {
	return f.failErr
}
func (f *failingSecondaryStore) SaveBatch(ctx context.Context, batch []*jobs.Job) error {
	return f.failErr
}
func (f *failingSecondaryStore) UpdateStatus(ctx context.Context, id string, status jobs.JobStatus, workerID string) error {
	return f.failErr
}
func (f *failingSecondaryStore) UpdateProgress(ctx context.Context, id string, progress float64) error {
	return f.failErr
}
func (f *failingSecondaryStore) UpdateResult(ctx context.Context, id string, status jobs.JobStatus, workerID string, result interface{}) error {
	return f.failErr
}
func (f *failingSecondaryStore) Enqueue(ctx context.Context, job *jobs.Job) error {
	return f.failErr
}
func (f *failingSecondaryStore) Heartbeat(ctx context.Context, jobID string) error {
	return f.failErr
}
func (f *failingSecondaryStore) Complete(ctx context.Context, jobID string, result interface{}) error {
	return f.failErr
}
func (f *failingSecondaryStore) Fail(ctx context.Context, jobID string, err error, requeue bool) error {
	return f.failErr
}
func (f *failingSecondaryStore) DeleteJob(ctx context.Context, jobID string) error {
	return f.failErr
}
func (f *failingSecondaryStore) DeleteJobsBefore(ctx context.Context, tenantID, status, jobType string, before time.Time) (int64, error) {
	return 0, f.failErr
}
func (f *failingSecondaryStore) RegisterClient(ctx context.Context, tenantID, apiKeyHash string) error {
	return f.failErr
}
func (f *failingSecondaryStore) RevokeClient(ctx context.Context, tenantID string) error {
	return f.failErr
}
func (f *failingSecondaryStore) ArchiveOldJobs(ctx context.Context, cutoff time.Time) (int64, error) {
	return 0, f.failErr
}
func (f *failingSecondaryStore) MoveToDLQ(ctx context.Context, jobID string, reason string) error {
	return f.failErr
}
func (f *failingSecondaryStore) DeleteDLQBefore(ctx context.Context, tenantID, jobType string, before time.Time) (int64, error) {
	return 0, f.failErr
}
func (f *failingSecondaryStore) DeleteFromDLQ(ctx context.Context, jobID string) error {
	return f.failErr
}

func TestDualStoreSecondaryFailureMetricsAndLogging(t *testing.T) {
	ctx := context.Background()
	injectedErr := errors.New("simulated redis failure")

	ops := []struct {
		name string
		call func(d *DualStore) error
	}{
		{
			name: "Save",
			call: func(d *DualStore) error {
				return d.Save(ctx, &jobs.Job{ID: "j-save", TenantID: "t-1"})
			},
		},
		{
			name: "SaveBatch",
			call: func(d *DualStore) error {
				return d.SaveBatch(ctx, []*jobs.Job{{ID: "j-batch", TenantID: "t-1"}})
			},
		},
		{
			name: "UpdateStatus",
			call: func(d *DualStore) error {
				return d.UpdateStatus(ctx, "j-status", jobs.StatusProcessing, "w-1")
			},
		},
		{
			name: "UpdateProgress",
			call: func(d *DualStore) error {
				return d.UpdateProgress(ctx, "j-prog", 45.0)
			},
		},
		{
			name: "UpdateResult",
			call: func(d *DualStore) error {
				return d.UpdateResult(ctx, "j-res", jobs.StatusCompleted, "w-1", "ok")
			},
		},
		{
			name: "Enqueue",
			call: func(d *DualStore) error {
				return d.Enqueue(ctx, &jobs.Job{ID: "j-enq", TenantID: "t-1"})
			},
		},
		{
			name: "Dequeue",
			call: func(d *DualStore) error {
				// Dequeue triggers secondary UpdateStatus on non-nil job
				_, err := d.Dequeue(ctx, "t-1", "")
				return err
			},
		},
		{
			name: "Heartbeat",
			call: func(d *DualStore) error {
				return d.Heartbeat(ctx, "j-hb")
			},
		},
		{
			name: "Complete",
			call: func(d *DualStore) error {
				return d.Complete(ctx, "j-comp", "result")
			},
		},
		{
			name: "Fail",
			call: func(d *DualStore) error {
				return d.Fail(ctx, "j-fail", errors.New("job err"), false)
			},
		},
		{
			name: "DeleteJob",
			call: func(d *DualStore) error {
				return d.DeleteJob(ctx, "j-del")
			},
		},
		{
			name: "DeleteJobsBefore",
			call: func(d *DualStore) error {
				_, err := d.DeleteJobsBefore(ctx, "t-1", "completed", "email", time.Now())
				return err
			},
		},
		{
			name: "RegisterClient",
			call: func(d *DualStore) error {
				return d.RegisterClient(ctx, "t-1", "hash")
			},
		},
		{
			name: "RevokeClient",
			call: func(d *DualStore) error {
				return d.RevokeClient(ctx, "t-1")
			},
		},
		{
			name: "ArchiveOldJobs",
			call: func(d *DualStore) error {
				_, err := d.ArchiveOldJobs(ctx, time.Now())
				return err
			},
		},
		{
			name: "MoveToDLQ",
			call: func(d *DualStore) error {
				return d.MoveToDLQ(ctx, "j-dlq", "timeout")
			},
		},
		{
			name: "DeleteDLQBefore",
			call: func(d *DualStore) error {
				_, err := d.DeleteDLQBefore(ctx, "t-1", "email", time.Now())
				return err
			},
		},
		{
			name: "DeleteFromDLQ",
			call: func(d *DualStore) error {
				return d.DeleteFromDLQ(ctx, "j-dlq")
			},
		},
	}

	for _, tc := range ops {
		t.Run(tc.name, func(t *testing.T) {
			var logBuf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

			primary := newMockStore()
			// Ensure Dequeue returns a job from primary so that secondary sync is attempted
			if tc.name == "Dequeue" {
				primary.jobs["j-deq"] = &jobs.Job{ID: "j-deq", TenantID: "t-1", Status: jobs.StatusPending, ProcessedBy: "w-1"}
			}

			secondary := newFailingSecondaryStore(injectedErr)
			d := NewDualStoreWithLogger(primary, secondary, logger)

			beforeCount := testutil.ToFloat64(metrics.DualStoreSyncErrorsTotal.WithLabelValues(tc.name))

			// Execution must NOT return error to caller even though secondary failed
			err := tc.call(d)
			if err != nil {
				t.Fatalf("expected nil error despite secondary failure, got: %v", err)
			}

			// Verify Prometheus metric incremented by 1
			afterCount := testutil.ToFloat64(metrics.DualStoreSyncErrorsTotal.WithLabelValues(tc.name))
			if afterCount != beforeCount+1 {
				t.Fatalf("expected metric to increment from %v to %v, got %v", beforeCount, beforeCount+1, afterCount)
			}

			// Verify structured log output
			logOutput := logBuf.String()
			if !strings.Contains(logOutput, "dual_store: secondary store sync failed") {
				t.Fatalf("expected log output to contain failure warning, got: %s", logOutput)
			}
			if !strings.Contains(logOutput, tc.name) {
				t.Fatalf("expected log output to contain operation %q, got: %s", tc.name, logOutput)
			}
			if !strings.Contains(logOutput, injectedErr.Error()) {
				t.Fatalf("expected log output to contain error %q, got: %s", injectedErr.Error(), logOutput)
			}
		})
	}
}

func TestDualStorePrimaryFailureStopsSecondary(t *testing.T) {
	ctx := context.Background()
	primaryErr := errors.New("primary db down")

	primary := newFailingSecondaryStore(primaryErr)
	secondary := newMockStore()
	d := NewDualStore(primary, secondary)

	// Save should fail because primary failed
	err := d.Save(ctx, &jobs.Job{ID: "j-1"})
	if !errors.Is(err, primaryErr) {
		t.Fatalf("expected primary error %v, got %v", primaryErr, err)
	}
	if secondary.saveCalled != 0 {
		t.Fatalf("expected secondary.Save not called when primary fails, got %d", secondary.saveCalled)
	}
}

func TestDualStoreNilSecondary(t *testing.T) {
	ctx := context.Background()
	primary := newMockStore()
	d := NewDualStore(primary, nil)

	if err := d.Save(ctx, &jobs.Job{ID: "j-nil"}); err != nil {
		t.Fatalf("Save with nil secondary failed: %v", err)
	}
	if err := d.UpdateStatus(ctx, "j-nil", jobs.StatusCompleted, "w1"); err != nil {
		t.Fatalf("UpdateStatus with nil secondary failed: %v", err)
	}
	if err := d.MoveToDLQ(ctx, "j-nil", "reason"); err != nil {
		t.Fatalf("MoveToDLQ with nil secondary failed: %v", err)
	}
	if err := d.DeleteJob(ctx, "j-nil"); err != nil {
		t.Fatalf("DeleteJob with nil secondary failed: %v", err)
	}
}

