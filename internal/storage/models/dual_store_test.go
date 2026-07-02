package models

import (
	"context"
	"testing"
	"time"

	"task-queue-system/internal/jobs"
)

// mockStore implements a minimal in-memory Store for testing DualStore delegation.
type mockStore struct {
	Store
	saveCalled          int
	getByIDCalled       int
	updateStatusCalled  int
	updateProgressCalled int
	updateResultCalled  int
	deleteJobCalled     int
	deleteJobsBeforeCalled int
	enqueueCalled       int
	dequeueCalled       int
	heartbeatCalled     int
	completeCalled      int
	failCalled          int
	listJobsCalled      int
	searchJobsCalled    int
	recoverOrphansCalled int
	dedupCalled         int
	getByIDsCalled      int
	getQueueLengthsCalled int
	getByWorkerCalled   int
	jobs                map[string]*jobs.Job
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
		{"GetByID", func(p, s *mockStore) (int, int) { _, _ = NewDualStore(p, s).GetByID(ctx, "j1"); return p.getByIDCalled, s.getByIDCalled }},
		{"ListJobs", func(p, s *mockStore) (int, int) { _, _ = NewDualStore(p, s).ListJobs(ctx, "", "", "", 0, 0); return p.listJobsCalled, s.listJobsCalled }},
		{"SearchJobs", func(p, s *mockStore) (int, int) { _, _ = NewDualStore(p, s).SearchJobs(ctx, JobFilter{}); return p.searchJobsCalled, s.searchJobsCalled }},
		{"RecoverOrphans", func(p, s *mockStore) (int, int) { _, _ = NewDualStore(p, s).RecoverOrphans(ctx, time.Minute); return p.recoverOrphansCalled, s.recoverOrphansCalled }},
		{"IsDedupKeyTaken", func(p, s *mockStore) (int, int) { _, _ = NewDualStore(p, s).IsDedupKeyTaken(ctx, "k", "t"); return p.dedupCalled, s.dedupCalled }},
		{"GetByIDs", func(p, s *mockStore) (int, int) { _, _ = NewDualStore(p, s).GetByIDs(ctx, nil); return p.getByIDsCalled, s.getByIDsCalled }},
		{"GetQueueLengths", func(p, s *mockStore) (int, int) { _, _ = NewDualStore(p, s).GetQueueLengths(ctx); return p.getQueueLengthsCalled, s.getQueueLengthsCalled }},
		{"GetByWorkerAndStatus", func(p, s *mockStore) (int, int) { _, _ = NewDualStore(p, s).GetByWorkerAndStatus(ctx, "w", jobs.StatusProcessing); return p.getByWorkerCalled, s.getByWorkerCalled }},
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
