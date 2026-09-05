package models

import (
	"context"
	"time"

	"task-queue-system/internal/jobs"
)

// DualStore implements the Store interface by delegating to two backends.
// This is primarily used for migrations, where we want to keep Redis in sync
// while transitioning to Postgres as the primary source of truth.
type DualStore struct {
	Primary   Store // Usually Postgres
	Secondary Store // Usually Redis
}

func NewDualStore(primary, secondary Store) *DualStore {
	return &DualStore{
		Primary:   primary,
		Secondary: secondary,
	}
}

// SaveBatch writes to both stores.
func (s *DualStore) SaveBatch(ctx context.Context, batch []*jobs.Job) error {
	if err := s.Primary.SaveBatch(ctx, batch); err != nil {
		return err
	}
	s.Secondary.SaveBatch(ctx, batch)
	return nil
}

// Save writes to both stores.
func (s *DualStore) Save(ctx context.Context, job *jobs.Job) error {
	_ = s.Secondary.Save(ctx, job) // Opportunistic write to secondary
	return s.Primary.Save(ctx, job)
}

// GetByID reads only from the primary store.
func (s *DualStore) GetByID(ctx context.Context, id string) (*jobs.Job, error) {
	return s.Primary.GetByID(ctx, id)
}

func (s *DualStore) UpdateStatus(ctx context.Context, id string, status jobs.JobStatus, workerID string) error {
	_ = s.Secondary.UpdateStatus(ctx, id, status, workerID)
	return s.Primary.UpdateStatus(ctx, id, status, workerID)
}

func (s *DualStore) UpdateProgress(ctx context.Context, id string, progress float64) error {
	_ = s.Secondary.UpdateProgress(ctx, id, progress)
	return s.Primary.UpdateProgress(ctx, id, progress)
}

func (s *DualStore) UpdateResult(ctx context.Context, id string, status jobs.JobStatus, workerID string, result interface{}) error {
	_ = s.Secondary.UpdateResult(ctx, id, status, workerID, result)
	return s.Primary.UpdateResult(ctx, id, status, workerID, result)
}

func (s *DualStore) GetByWorkerAndStatus(ctx context.Context, workerID string, status jobs.JobStatus) ([]*jobs.Job, error) {
	return s.Primary.GetByWorkerAndStatus(ctx, workerID, status)
}

// ─── New Interface Methods ──────────────────────────────────────────────────

func (s *DualStore) Enqueue(ctx context.Context, job *jobs.Job) error {
	_ = s.Secondary.Enqueue(ctx, job)
	return s.Primary.Enqueue(ctx, job)
}

func (s *DualStore) Dequeue(ctx context.Context, tenantID string, shardKey string) (*jobs.Job, error) {
	// Dequeue involves a state change 'pending' -> 'processing'.
	// We MUST perform this on primary.
	job, err := s.Primary.Dequeue(ctx, tenantID, shardKey)
	if err == nil && job != nil {
		// Try to sync the state change to secondary if possible
		_ = s.Secondary.UpdateStatus(ctx, job.ID, jobs.StatusProcessing, job.ProcessedBy)
	}
	return job, err
}

func (s *DualStore) Heartbeat(ctx context.Context, jobID string) error {
	_ = s.Secondary.Heartbeat(ctx, jobID)
	return s.Primary.Heartbeat(ctx, jobID)
}

func (s *DualStore) Complete(ctx context.Context, jobID string, result interface{}) error {
	_ = s.Secondary.Complete(ctx, jobID, result)
	return s.Primary.Complete(ctx, jobID, result)
}

func (s *DualStore) Fail(ctx context.Context, jobID string, err error, requeue bool) error {
	_ = s.Secondary.Fail(ctx, jobID, err, requeue)
	return s.Primary.Fail(ctx, jobID, err, requeue)
}

func (s *DualStore) ListJobs(ctx context.Context, tenantID string, status string, typeStr string, limit, offset int) ([]*jobs.Job, error) {
	return s.Primary.ListJobs(ctx, tenantID, status, typeStr, limit, offset)
}

func (s *DualStore) SearchJobs(ctx context.Context, filter JobFilter) ([]*jobs.Job, error) {
	return s.Primary.SearchJobs(ctx, filter)
}

func (s *DualStore) CountJobs(ctx context.Context, filter JobFilter) (int64, error) {
	return s.Primary.CountJobs(ctx, filter)
}

func (s *DualStore) RecoverOrphans(ctx context.Context, timeout time.Duration) (int64, error) {
	// Recovery should happen on primary.
	return s.Primary.RecoverOrphans(ctx, timeout)
}

func (s *DualStore) DeleteJob(ctx context.Context, jobID string) error {
	_ = s.Secondary.DeleteJob(ctx, jobID)
	return s.Primary.DeleteJob(ctx, jobID)
}

func (s *DualStore) DeleteJobsBefore(ctx context.Context, tenantID, status, jobType string, before time.Time) (int64, error) {
	_, _ = s.Secondary.DeleteJobsBefore(ctx, tenantID, status, jobType, before)
	return s.Primary.DeleteJobsBefore(ctx, tenantID, status, jobType, before)
}

func (s *DualStore) IsDedupKeyTaken(ctx context.Context, dedupKey, tenantID string) (bool, error) {
	return s.Primary.IsDedupKeyTaken(ctx, dedupKey, tenantID)
}

func (s *DualStore) GetReadyDAGJobs(ctx context.Context, limit int) ([]*jobs.Job, error) {
	return s.Primary.GetReadyDAGJobs(ctx, limit)
}
func (s *DualStore) GetDependentJobs(ctx context.Context, parentID string) ([]*jobs.Job, error) {
	return s.Primary.GetDependentJobs(ctx, parentID)
}

func (s *DualStore) GetByIDs(ctx context.Context, ids []string) ([]*jobs.Job, error) {
	return s.Primary.GetByIDs(ctx, ids)
}

func (s *DualStore) GetQueueLengths(ctx context.Context) (map[string]map[string]int64, error) {
	return s.Primary.GetQueueLengths(ctx)
}

func (s *DualStore) RegisterClient(ctx context.Context, tenantID, apiKeyHash string) error {
	_ = s.Secondary.RegisterClient(ctx, tenantID, apiKeyHash)
	return s.Primary.RegisterClient(ctx, tenantID, apiKeyHash)
}

func (s *DualStore) VerifyClient(ctx context.Context, apiKeyHash string) (string, error) {
	return s.Primary.VerifyClient(ctx, apiKeyHash)
}

func (s *DualStore) ListClients(ctx context.Context) ([]*ClientRecord, error) {
	return s.Primary.ListClients(ctx)
}

func (s *DualStore) RevokeClient(ctx context.Context, tenantID string) error {
	_ = s.Secondary.RevokeClient(ctx, tenantID)
	return s.Primary.RevokeClient(ctx, tenantID)
}


// ArchiveOldJobs archives jobs in both stores. It primarily relies on the primary store's count.
func (s *DualStore) ArchiveOldJobs(ctx context.Context, cutoff time.Time) (int64, error) {
	// Archive in secondary first
	_, _ = s.Secondary.ArchiveOldJobs(ctx, cutoff)
	
	// Archive in primary
	return s.Primary.ArchiveOldJobs(ctx, cutoff)
}
