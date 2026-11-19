package models

import (
	"context"
	"log/slog"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/metrics"
)

// DualStore implements the Store interface by delegating to two backends.
// This is primarily used for migrations, where we want to keep Redis in sync
// while transitioning to Postgres as the primary source of truth.
type DualStore struct {
	Primary   Store // Usually Postgres
	Secondary Store // Usually Redis
	Logger    *slog.Logger
}

func NewDualStore(primary, secondary Store) *DualStore {
	return &DualStore{
		Primary:   primary,
		Secondary: secondary,
		Logger:    slog.Default(),
	}
}

func NewDualStoreWithLogger(primary, secondary Store, logger *slog.Logger) *DualStore {
	if logger == nil {
		logger = slog.Default()
	}
	return &DualStore{
		Primary:   primary,
		Secondary: secondary,
		Logger:    logger,
	}
}

func (s *DualStore) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

func (s *DualStore) recordSecondaryError(op string, err error, attrs ...any) {
	metrics.DualStoreSyncErrorsTotal.WithLabelValues(op).Inc()
	args := make([]any, 0, len(attrs)+4)
	args = append(args, "operation", op, "error", err)
	args = append(args, attrs...)
	s.logger().Warn("dual_store: secondary store sync failed", args...)
}

// SaveBatch writes to both stores.
func (s *DualStore) SaveBatch(ctx context.Context, batch []*jobs.Job) error {
	if err := s.Primary.SaveBatch(ctx, batch); err != nil {
		return err
	}
	if s.Secondary != nil {
		if err := s.Secondary.SaveBatch(ctx, batch); err != nil {
			s.recordSecondaryError("SaveBatch", err, "batch_size", len(batch))
		}
	}
	return nil
}

// Save writes to both stores.
func (s *DualStore) Save(ctx context.Context, job *jobs.Job) error {
	if err := s.Primary.Save(ctx, job); err != nil {
		return err
	}
	if s.Secondary != nil {
		if err := s.Secondary.Save(ctx, job); err != nil {
			var jobID, tenantID string
			if job != nil {
				jobID = job.ID
				tenantID = job.TenantID
			}
			s.recordSecondaryError("Save", err, "job_id", jobID, "tenant_id", tenantID)
		}
	}
	return nil
}

// GetByID reads only from the primary store.
func (s *DualStore) GetByID(ctx context.Context, id string) (*jobs.Job, error) {
	return s.Primary.GetByID(ctx, id)
}

func (s *DualStore) UpdateStatus(ctx context.Context, id string, status jobs.JobStatus, workerID string) error {
	if err := s.Primary.UpdateStatus(ctx, id, status, workerID); err != nil {
		return err
	}
	if s.Secondary != nil {
		if err := s.Secondary.UpdateStatus(ctx, id, status, workerID); err != nil {
			s.recordSecondaryError("UpdateStatus", err, "job_id", id, "status", string(status), "worker_id", workerID)
		}
	}
	return nil
}

func (s *DualStore) UpdateProgress(ctx context.Context, id string, progress float64) error {
	if err := s.Primary.UpdateProgress(ctx, id, progress); err != nil {
		return err
	}
	if s.Secondary != nil {
		if err := s.Secondary.UpdateProgress(ctx, id, progress); err != nil {
			s.recordSecondaryError("UpdateProgress", err, "job_id", id, "progress", progress)
		}
	}
	return nil
}

func (s *DualStore) UpdateResult(ctx context.Context, id string, status jobs.JobStatus, workerID string, result interface{}) error {
	if err := s.Primary.UpdateResult(ctx, id, status, workerID, result); err != nil {
		return err
	}
	if s.Secondary != nil {
		if err := s.Secondary.UpdateResult(ctx, id, status, workerID, result); err != nil {
			s.recordSecondaryError("UpdateResult", err, "job_id", id, "status", string(status), "worker_id", workerID)
		}
	}
	return nil
}

func (s *DualStore) GetByWorkerAndStatus(ctx context.Context, workerID string, status jobs.JobStatus) ([]*jobs.Job, error) {
	return s.Primary.GetByWorkerAndStatus(ctx, workerID, status)
}

// ─── New Interface Methods ──────────────────────────────────────────────────

func (s *DualStore) Enqueue(ctx context.Context, job *jobs.Job) error {
	if err := s.Primary.Enqueue(ctx, job); err != nil {
		return err
	}
	if s.Secondary != nil {
		if err := s.Secondary.Enqueue(ctx, job); err != nil {
			var jobID, tenantID string
			if job != nil {
				jobID = job.ID
				tenantID = job.TenantID
			}
			s.recordSecondaryError("Enqueue", err, "job_id", jobID, "tenant_id", tenantID)
		}
	}
	return nil
}

func (s *DualStore) Dequeue(ctx context.Context, tenantID string, shardKey string) (*jobs.Job, error) {
	// Dequeue involves a state change 'pending' -> 'processing'.
	// We MUST perform this on primary.
	job, err := s.Primary.Dequeue(ctx, tenantID, shardKey)
	if err == nil && job != nil {
		// Try to sync the state change to secondary if possible
		if s.Secondary != nil {
			if secErr := s.Secondary.UpdateStatus(ctx, job.ID, jobs.StatusProcessing, job.ProcessedBy); secErr != nil {
				s.recordSecondaryError("Dequeue", secErr, "job_id", job.ID, "tenant_id", job.TenantID)
			}
		}
	}
	return job, err
}

func (s *DualStore) Heartbeat(ctx context.Context, jobID string) error {
	if err := s.Primary.Heartbeat(ctx, jobID); err != nil {
		return err
	}
	if s.Secondary != nil {
		if err := s.Secondary.Heartbeat(ctx, jobID); err != nil {
			s.recordSecondaryError("Heartbeat", err, "job_id", jobID)
		}
	}
	return nil
}

func (s *DualStore) Complete(ctx context.Context, jobID string, result interface{}) error {
	if err := s.Primary.Complete(ctx, jobID, result); err != nil {
		return err
	}
	if s.Secondary != nil {
		if err := s.Secondary.Complete(ctx, jobID, result); err != nil {
			s.recordSecondaryError("Complete", err, "job_id", jobID)
		}
	}
	return nil
}

func (s *DualStore) Fail(ctx context.Context, jobID string, err error, requeue bool) error {
	if primaryErr := s.Primary.Fail(ctx, jobID, err, requeue); primaryErr != nil {
		return primaryErr
	}
	if s.Secondary != nil {
		if secErr := s.Secondary.Fail(ctx, jobID, err, requeue); secErr != nil {
			s.recordSecondaryError("Fail", secErr, "job_id", jobID, "requeue", requeue)
		}
	}
	return nil
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
	if err := s.Primary.DeleteJob(ctx, jobID); err != nil {
		return err
	}
	if s.Secondary != nil {
		if err := s.Secondary.DeleteJob(ctx, jobID); err != nil {
			s.recordSecondaryError("DeleteJob", err, "job_id", jobID)
		}
	}
	return nil
}

func (s *DualStore) DeleteJobsBefore(ctx context.Context, tenantID, status, jobType string, before time.Time) (int64, error) {
	n, err := s.Primary.DeleteJobsBefore(ctx, tenantID, status, jobType, before)
	if err != nil {
		return 0, err
	}
	if s.Secondary != nil {
		if _, secErr := s.Secondary.DeleteJobsBefore(ctx, tenantID, status, jobType, before); secErr != nil {
			s.recordSecondaryError("DeleteJobsBefore", secErr, "tenant_id", tenantID, "status", status, "type", jobType)
		}
	}
	return n, nil
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
	if err := s.Primary.RegisterClient(ctx, tenantID, apiKeyHash); err != nil {
		return err
	}
	if s.Secondary != nil {
		if err := s.Secondary.RegisterClient(ctx, tenantID, apiKeyHash); err != nil {
			s.recordSecondaryError("RegisterClient", err, "tenant_id", tenantID)
		}
	}
	return nil
}

func (s *DualStore) VerifyClient(ctx context.Context, apiKeyHash string) (string, error) {
	return s.Primary.VerifyClient(ctx, apiKeyHash)
}

func (s *DualStore) ListClients(ctx context.Context) ([]*ClientRecord, error) {
	return s.Primary.ListClients(ctx)
}

func (s *DualStore) RevokeClient(ctx context.Context, tenantID string) error {
	if err := s.Primary.RevokeClient(ctx, tenantID); err != nil {
		return err
	}
	if s.Secondary != nil {
		if err := s.Secondary.RevokeClient(ctx, tenantID); err != nil {
			s.recordSecondaryError("RevokeClient", err, "tenant_id", tenantID)
		}
	}
	return nil
}

// ArchiveOldJobs archives jobs in both stores. It primarily relies on the primary store's count.
func (s *DualStore) ArchiveOldJobs(ctx context.Context, cutoff time.Time) (int64, error) {
	n, err := s.Primary.ArchiveOldJobs(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	if s.Secondary != nil {
		if _, secErr := s.Secondary.ArchiveOldJobs(ctx, cutoff); secErr != nil {
			s.recordSecondaryError("ArchiveOldJobs", secErr, "cutoff", cutoff.String())
		}
	}
	return n, nil
}

// ── DLQ (Delegate to Postgres with Secondary Sync) ──
func (s *DualStore) MoveToDLQ(ctx context.Context, jobID string, reason string) error {
	if err := s.Primary.MoveToDLQ(ctx, jobID, reason); err != nil {
		return err
	}
	if s.Secondary != nil {
		if err := s.Secondary.MoveToDLQ(ctx, jobID, reason); err != nil {
			s.recordSecondaryError("MoveToDLQ", err, "job_id", jobID, "reason", reason)
		}
	}
	return nil
}

func (s *DualStore) ListDLQ(ctx context.Context, tenantID, jobType string, limit, offset int) ([]*jobs.Job, error) {
	return s.Primary.ListDLQ(ctx, tenantID, jobType, limit, offset)
}

func (s *DualStore) CountDLQ(ctx context.Context, tenantID, jobType string) (int64, error) {
	return s.Primary.CountDLQ(ctx, tenantID, jobType)
}

func (s *DualStore) DeleteDLQBefore(ctx context.Context, tenantID, jobType string, before time.Time) (int64, error) {
	n, err := s.Primary.DeleteDLQBefore(ctx, tenantID, jobType, before)
	if err != nil {
		return 0, err
	}
	if s.Secondary != nil {
		if _, secErr := s.Secondary.DeleteDLQBefore(ctx, tenantID, jobType, before); secErr != nil {
			s.recordSecondaryError("DeleteDLQBefore", secErr, "tenant_id", tenantID, "type", jobType)
		}
	}
	return n, nil
}

func (s *DualStore) GetFromDLQ(ctx context.Context, jobID string) (*jobs.Job, error) {
	return s.Primary.GetFromDLQ(ctx, jobID)
}

func (s *DualStore) DeleteFromDLQ(ctx context.Context, jobID string) error {
	if err := s.Primary.DeleteFromDLQ(ctx, jobID); err != nil {
		return err
	}
	if s.Secondary != nil {
		if err := s.Secondary.DeleteFromDLQ(ctx, jobID); err != nil {
			s.recordSecondaryError("DeleteFromDLQ", err, "job_id", jobID)
		}
	}
	return nil
}

func (s *DualStore) LogAuditAction(ctx context.Context, log *AuditLog) error {
	return s.Primary.LogAuditAction(ctx, log)
}

func (s *DualStore) ListAuditLogs(ctx context.Context, limit, offset int) ([]*AuditLog, error) {
	return s.Primary.ListAuditLogs(ctx, limit, offset)
}
