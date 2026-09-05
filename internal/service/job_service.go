package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
	"task-queue-system/internal/jobs"
	"task-queue-system/internal/jobtypes"
	"task-queue-system/internal/queue"
	"task-queue-system/internal/sse"
	"task-queue-system/internal/storage/models"
	"task-queue-system/internal/webhooks"
	apperr "task-queue-system/internal/errors"
)

// allowedJobTypes is the set of built-in job types (test types for CI only).
var allowedJobTypes = map[string]struct{}{
	"email": {},
	"image": {},
	"http":  {},
	"test":           {},
	"test-success":   {},
	"test-fail":      {},
	"test-scheduled": {},
}

const defaultMaxRetries = 3

// JobService orchestrates job creation, state transitions, and coordination
// between the queue backend and the persistent datastore.
type JobService struct {
	queue         queue.Queue
	store         models.Store
	logger        *slog.Logger
	maxQueueSize  int64
	webhookStore  *webhooks.WebhookStore
	jobTypeStore  *jobtypes.Store
	sseBroker     *sse.Broker
}

// SetSSEBroker attaches an SSE broker for real-time job event streaming.
func (s *JobService) SetSSEBroker(b *sse.Broker) { s.sseBroker = b }

// New creates a JobService.
func New(q queue.Queue, store models.Store, logger *slog.Logger, maxQueueSize int64) *JobService {
	return &JobService{
		queue:        q,
		store:        store,
		logger:       logger,
		maxQueueSize: maxQueueSize,
	}
}

// SetWebhookStore attaches a persistent webhook store for tenant-level webhooks.
func (s *JobService) SetWebhookStore(ws *webhooks.WebhookStore) {
	s.webhookStore = ws
}

// SetJobTypeStore attaches the dynamic job type registry.
func (s *JobService) SetJobTypeStore(jts *jobtypes.Store) {
	s.jobTypeStore = jts
}

// JobTypeStore returns the job type registry, if configured.
func (s *JobService) JobTypeStore() *jobtypes.Store { return s.jobTypeStore }

func (s *JobService) isJobTypeAllowed(ctx context.Context, jobType string) bool {
	if _, ok := allowedJobTypes[jobType]; ok {
		return true
	}
	if s.jobTypeStore != nil && s.jobTypeStore.IsAllowed(ctx, jobType) {
		return true
	}
	return false
}

// Store returns the underlying models.Store.
func (s *JobService) Store() models.Store { return s.store }

// CreateJob validates a new request, saves it to the DB, and enqueues it.
func (s *JobService) CreateJob(ctx context.Context, jobType string, payload map[string]interface{}, labels map[string]string, priority string, maxRetries int, backoffAlgorithm, backoffJitter, cronExpr string, runAtStr string, correlationID string, timeout int, version int, tenantID string, webhook *jobs.WebhookConfig, dedupKey string, dependencies []string, shardKey string) (*jobs.Job, error) {
	if !s.isJobTypeAllowed(ctx, jobType) {
		return nil, apperr.NewInvalidArgument(fmt.Sprintf("unsupported job type %q", jobType))
	}
	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}

	// ── Exactly-Once Dedup Check ────────────────────────────────────────────
	if dedupKey != "" {
		taken, err := s.store.IsDedupKeyTaken(ctx, dedupKey, tenantID)
		if err != nil {
			s.logger.Warn("dedup check failed; allowing as fallback", "dedup_key", dedupKey, "error", err)
		} else if taken {
			return nil, apperr.NewConflict("job with this dedup_key already exists")
		}
	}

	// ── DAG Dependency Validation ───────────────────────────────────────────
	if len(dependencies) > 0 {
		deps, err := s.store.GetByIDs(ctx, dependencies)
		if err != nil {
			return nil, apperr.NewInvalidArgument("failed to validate dependencies: " + err.Error())
		}
		if len(deps) != len(dependencies) {
			return nil, apperr.NewInvalidArgument("one or more dependency job IDs not found")
		}
		for _, dep := range deps {
			if dep.Status != jobs.StatusCompleted {
				return nil, apperr.NewInvalidArgument(fmt.Sprintf("dependency %q is not completed (status: %s)", dep.ID, dep.Status))
			}
		}
	}

	// ── Multi-tenancy Rate Limit ──────────────────────────────────────────────
	allowed, err := s.queue.IsAllowed(ctx, tenantID)
	if err != nil {
		s.logger.Warn("rate limit check failed; allowing as fallback", "tenant_id", tenantID, "error", err)
	} else if !allowed {
		s.publishRateLimitSSE(tenantID)
		return nil, apperr.NewTooManyRequests("tenant rate limit exceeded")
	}

	var runAt time.Time
	if runAtStr != "" {
		var err error
		runAt, err = time.Parse(time.RFC3339, runAtStr)
		if err != nil {
			return nil, apperr.NewInvalidArgument("invalid run_at timestamp: " + err.Error())
		}
		if runAt.Before(time.Now()) {
			return nil, apperr.NewInvalidArgument("run_at timestamp must be in the future")
		}
	}

	if s.maxQueueSize > 0 {
		count, err := s.queue.Size(ctx)
		if err == nil && count >= s.maxQueueSize {
			return nil, apperr.NewQueueFull()
		}
	}

	if timeout <= 0 {
		timeout = 60 // default 60s
	}

	job := jobs.NewJob(jobType, payload, labels, jobs.JobPriority(priority), maxRetries, runAt, correlationID, timeout, version, tenantID)
	job.Webhook = webhook
	job.DedupKey = dedupKey
	job.Dependencies = dependencies
	job.ShardKey = shardKey

	if backoffAlgorithm != "" {
		job.BackoffAlgorithm = jobs.BackoffAlgorithm(backoffAlgorithm)
	}
	if backoffJitter != "" {
		job.BackoffJitter = jobs.BackoffJitter(backoffJitter)
	}
	if cronExpr != "" {
		job.CronExpr = cronExpr
		job.Status = jobs.StatusRecurring
		
		// Parse the cron expression to set the initial next run time (RunAt)
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if sched, err := parser.Parse(cronExpr); err == nil {
			job.RunAt = sched.Next(time.Now().UTC())
		}
	}

	if err := s.store.Save(ctx, job); err != nil {
		s.logger.Error("failed to persist job", "job_id", job.ID, "error", err)
		return nil, apperr.NewInternal("failed to persist job", err)
	}

	// Only enqueue to Redis if it's an actual active job (not a recurring template)
	if job.Status != jobs.StatusRecurring {
		if err := s.queue.Enqueue(ctx, job); err != nil {
			s.logger.Error("failed to enqueue job", "job_id", job.ID, "error", err)
			return nil, apperr.NewInternal("failed to enqueue job", err)
		}
	}

	s.publishWebhookEvent(ctx, job, "created", map[string]interface{}{
		"type": job.Type,
	})
	s.publishSSE(job.ID, "created", job.Type, job.TenantID)

	return job, nil
}

// ─── Queue Proxies ───────────────────────────────────────────────────────────

// Dequeue retrieves a job from the queue, verifying DAG dependencies are satisfied.
func (s *JobService) Dequeue(ctx context.Context) (*jobs.Job, error) {
	job, err := s.queue.Dequeue(ctx)
	if err != nil || job == nil {
		return job, err
	}

	// ── DAG Dependency Check ───────────────────────────────────────────────
	// If any dependency isn't completed, release the job back to pending.
	if len(job.Dependencies) > 0 {
		deps, err := s.store.GetByIDs(ctx, job.Dependencies)
		if err != nil {
			s.logger.Warn("dependency check failed, allowing job anyway", "job_id", job.ID, "error", err)
			return job, nil
		}
		for _, dep := range deps {
			if dep.Status != jobs.StatusCompleted {
				s.logger.Info("dependency not satisfied, requeueing job",
					"job_id", job.ID, "dep_id", dep.ID, "dep_status", dep.Status)
				_ = s.store.UpdateStatus(ctx, job.ID, jobs.StatusPending, "")
				_ = s.queue.Ack(ctx, job.ID)
				return nil, nil
			}
		}
	}

	return job, nil
}

// Enqueue proxies the enqueue request to the underlying queue.
func (s *JobService) Enqueue(ctx context.Context, job *jobs.Job) error {
	return s.queue.Enqueue(ctx, job)
}

// Ack proxies the acknowledgement to the underlying queue.
func (s *JobService) Ack(ctx context.Context, jobID string) error {
	return s.queue.Ack(ctx, jobID)
}

// Fail proxies the failure report to the underlying queue.
func (s *JobService) Fail(ctx context.Context, jobID string, err error) error {
	return s.queue.Fail(ctx, jobID, err)
}

// ─── GetJobStatus ───────────────────────────────────────────
func (s *JobService) GetJobStatus(ctx context.Context, jobID string) (*jobs.Job, error) {
	j, err := s.store.GetByID(ctx, jobID)
	if err != nil {
		if errors.Is(err, models.ErrJobNotFound) {
			return nil, apperr.NewNotFound("job", jobID)
		}
		return nil, apperr.NewInternal("database query failed", err)
	}
	return j, nil
}

// PauseJob sets the paused flag on a job (prevents recurring cron instances).
func (s *JobService) PauseJob(ctx context.Context, jobID string) error {
	job, err := s.store.GetByID(ctx, jobID)
	if err != nil {
		return err
	}
	job.Paused = true
	job.UpdatedAt = time.Now().UTC()
	return s.store.Save(ctx, job)
}

// ResumeJob clears the paused flag on a job.
func (s *JobService) ResumeJob(ctx context.Context, jobID string) error {
	job, err := s.store.GetByID(ctx, jobID)
	if err != nil {
		return err
	}
	job.Paused = false
	job.UpdatedAt = time.Now().UTC()
	return s.store.Save(ctx, job)
}

// CancelJob marks a job as cancelled and removes it from the processing queue.
func (s *JobService) CancelJob(ctx context.Context, jobID string) error {
	if err := s.store.UpdateStatus(ctx, jobID, jobs.StatusCancelled, ""); err != nil {
		return err
	}
	_ = s.queue.Ack(ctx, jobID)
	s.logger.Info("job cancelled", "job_id", jobID)
	s.publishSSE(jobID, string(jobs.StatusCancelled), "", "")
	return nil
}

// UpdateJobProgress sets the progress percentage (0.0 – 100.0) for an in-flight job.
func (s *JobService) UpdateJobProgress(ctx context.Context, jobID string, progress float64) error {
	if progress < 0 || progress > 100 {
		return fmt.Errorf("progress must be between 0 and 100")
	}
	return s.store.UpdateProgress(ctx, jobID, progress)
}

// UpdateJobStatus updates the job state in the persistent store.
func (s *JobService) UpdateJobStatus(ctx context.Context, jobID string, status jobs.JobStatus, workerID string) error {
	if err := s.store.UpdateStatus(ctx, jobID, status, workerID); err != nil {
		s.logger.Error("failed to update job status", "job_id", jobID, "status", status, "worker", workerID, "error", err)
		return err
	}
	s.logger.Info("job status updated", "job_id", jobID, "status", status, "worker", workerID)
	return nil
}

// UpdateJobResult propagates execution outcomes (success values or error strings) to the store.
func (s *JobService) UpdateJobResult(ctx context.Context, jobID string, status jobs.JobStatus, workerID string, result interface{}) error {
	if err := s.store.UpdateResult(ctx, jobID, status, workerID, result); err != nil {
		return fmt.Errorf("service: failed to update job result: %w", err)
	}

	s.logger.Info("job result updated", "job_id", jobID, "status", status, "worker", workerID)

	job, err := s.store.GetByID(ctx, jobID)
	if err != nil || job == nil {
		return nil
	}

	// Publish SSE event.
	s.publishSSE(jobID, string(status), job.Type, job.TenantID)

	// Publish webhook events: from per-job webhook config AND from persistent tenant webhooks.
	s.publishWebhookEvents(ctx, job, status, result)

	return nil
}

func (s *JobService) publishSSE(jobID, status, jobType, tenantID string) {
	if s.sseBroker == nil {
		return
	}
	s.sseBroker.Publish(sse.Event{
		Kind:   "job",
		JobID:  jobID,
		Status: status,
		Type:   jobType,
		Tenant: tenantID,
	})
}

// publishRateLimitSSE broadcasts a tenant rate-limit rejection to connected
// operators so they can watch limits being hit live.
func (s *JobService) publishRateLimitSSE(tenantID string) {
	if s.sseBroker == nil {
		return
	}
	s.sseBroker.Publish(sse.Event{
		Kind:   "rate_limit",
		Type:   "rate_limit",
		Status: "rejected",
		Tenant: tenantID,
	})
}

func (s *JobService) publishWebhookEvents(ctx context.Context, job *jobs.Job, status jobs.JobStatus, result interface{}) {
	s.publishWebhookEvent(ctx, job, string(status), result)
}

func (s *JobService) publishWebhookEvent(ctx context.Context, job *jobs.Job, event string, result interface{}) {
	webhooksToSend := s.collectWebhookTargetsForEvent(ctx, job, event)

	for _, target := range webhooksToSend {
		errStr := ""
		if event == string(jobs.StatusFailed) {
			if s, ok := result.(string); ok {
				errStr = s
			} else if result != nil {
				errStr = fmt.Sprintf("%v", result)
			}
		}

		eventPayload := map[string]interface{}{
			"job_id":    job.ID,
			"tenant_id": job.TenantID,
			"status":    event,
			"result":    result,
			"error":     errStr,
			"timestamp": time.Now().UTC(),
			"url":       target.URL,
			"secret":    target.Secret,
		}
		if err := s.queue.PublishWebhookEvent(ctx, eventPayload); err != nil {
			s.logger.Error("failed to publish webhook event", "job_id", job.ID, "url", target.URL, "error", err)
		}
	}
}

type webhookTarget struct {
	URL    string
	Secret string
}

func (s *JobService) collectWebhookTargets(ctx context.Context, job *jobs.Job, status jobs.JobStatus) []webhookTarget {
	return s.collectWebhookTargetsForEvent(ctx, job, string(status))
}

func (s *JobService) collectWebhookTargetsForEvent(ctx context.Context, job *jobs.Job, event string) []webhookTarget {
	var targets []webhookTarget

	// 1. Per-job webhook
	if job.Webhook != nil && job.Webhook.URL != "" {
		for _, e := range job.Webhook.Events {
			if webhooks.NormalizeEvent(e) == webhooks.NormalizeEvent(event) {
				targets = append(targets, webhookTarget{URL: job.Webhook.URL, Secret: job.Webhook.Secret})
				break
			}
		}
	}

	// 2. Persistent tenant webhooks
	if s.webhookStore != nil {
		matched, err := s.webhookStore.Match(ctx, job.TenantID, event)
		if err != nil {
			s.logger.Error("failed to match persistent webhooks", "tenant_id", job.TenantID, "error", err)
		} else {
			for _, w := range matched {
				targets = append(targets, webhookTarget{URL: w.URL, Secret: w.Secret})
			}
		}
	}

	return targets
}

// RegisterHeartbeat logs the worker's presence in the queue system.
func (s *JobService) RegisterHeartbeat(ctx context.Context, workerID string) error {
	return s.queue.RegisterHeartbeat(ctx, workerID)
}

// GetActiveWorkers returns information about all connected worker instances.
func (s *JobService) GetActiveWorkers(ctx context.Context) ([]queue.WorkerInfo, error) {
	return s.queue.GetActiveWorkers(ctx)
}

// GetMetrics returns system execution metrics.
func (s *JobService) GetMetrics(ctx context.Context) (queue.QueueMetrics, error) {
	return s.queue.GetMetrics(ctx)
}

// IsProcessed checks if the job already succeeded (idempotency guard).
func (s *JobService) IsProcessed(ctx context.Context, jobID string) (bool, error) {
	return s.queue.IsProcessed(ctx, jobID)
}

// MarkProcessed flags the job as having successfully completed.
func (s *JobService) MarkProcessed(ctx context.Context, jobID string) error {
	return s.queue.MarkProcessed(ctx, jobID)
}

// PromoteScheduledJobs pushes matured delayed tasks to active queues.
func (s *JobService) PromoteScheduledJobs(ctx context.Context) (int, error) {
	return s.queue.PromoteScheduledJobs(ctx)
}

// ReclaimTimedOutJobs finds and re-queues jobs from crashed workers.
func (s *JobService) ReclaimTimedOutJobs(ctx context.Context) (int, error) {
	return s.queue.ReclaimTimedOutJobs(ctx)
}

// ReconcileOrphanedJobs finds jobs stuck in "processing" for this worker
// and moves them back to the queue. Called on worker startup.
func (s *JobService) ReconcileOrphanedJobs(ctx context.Context, workerID string) (int, error) {
	stuckJobs, err := s.store.GetByWorkerAndStatus(ctx, workerID, jobs.StatusProcessing)
	if err != nil {
		return 0, err
	}

	for _, j := range stuckJobs {
		s.logger.Info("reconciling orphaned job", "job_id", j.ID, "worker_id", workerID)
		// Reset status to pending so it can be picked up again
		_ = s.UpdateJobStatus(ctx, j.ID, jobs.StatusPending, "")
		
		// Re-enqueue the job and CLEAR the old in-flight entry in one sequence.
		if err := s.queue.Enqueue(ctx, j); err == nil {
			_ = s.queue.Ack(ctx, j.ID)
		}
	}

	return len(stuckJobs), nil
}

// GetJobByIDs returns multiple jobs by their IDs.
func (s *JobService) GetJobByIDs(ctx context.Context, ids []string) ([]*jobs.Job, error) {
	return s.store.GetByIDs(ctx, ids)
}

// QueueLengths returns pending job counts segmented by queue type and tenant.
func (s *JobService) QueueLengths(ctx context.Context) (map[string]map[string]int64, error) {
	return s.store.GetQueueLengths(ctx)
}

// RateLimitStatus returns the current per-tenant rate-limit usage.
func (s *JobService) RateLimitStatus(ctx context.Context) ([]queue.TenantRateStatus, error) {
	return s.queue.RateLimitStatus(ctx)
}

// PriorityPartitionDepths returns queue depths by priority tier and partition.
func (s *JobService) PriorityPartitionDepths(ctx context.Context) (queue.PriorityDepthReport, error) {
	return s.queue.PriorityPartitionDepths(ctx)
}

// SearchJobs returns jobs matching the given filter.
func (s *JobService) SearchJobs(ctx context.Context, filter models.JobFilter) ([]*jobs.Job, error) {
	return s.store.SearchJobs(ctx, filter)
}

// RecurringJobsDue scans for recurring job templates that are due,
// spawns a single active instance, and updates the template's next RunAt time.
func (s *JobService) RecurringJobsDue(ctx context.Context) (int, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

	// Fetch ALL recurring job templates. Since there shouldn't be millions of templates, Limit: 10000 is safer.
	templates, err := s.store.SearchJobs(ctx, models.JobFilter{
		Status: string(jobs.StatusRecurring),
		Limit:  10000,
	})
	if err != nil {
		return 0, err
	}

	now := time.Now().UTC()
	var count int

	for _, t := range templates {
		if t.Paused || t.CronExpr == "" {
			continue
		}

		// If RunAt is in the future, it's not due yet.
		if t.RunAt.After(now) {
			continue
		}

		sched, err := parser.Parse(t.CronExpr)
		if err != nil {
			s.logger.Warn("invalid cron expr on job template", "job_id", t.ID, "cron", t.CronExpr, "error", err)
			continue
		}

		// 1. Spawn a single active instance for this tick
		instance := *t // shallow copy
		instance.ID = "" // let store/queue generate a new ID, but wait, jobs.NewJob logic does this.
		instance.ID = jobs.NewJob(t.Type, t.Payload, t.Labels, t.Priority, t.MaxRetries, time.Time{}, t.CorrelationID, t.Timeout, t.Version, t.TenantID).ID
		instance.Status = jobs.StatusPending
		instance.CronExpr = "" // The instance is not a recurring job itself
		instance.CreatedAt = now
		instance.UpdatedAt = now
		instance.RunAt = now
		instance.Retries = 0

		if err := s.store.Save(ctx, &instance); err != nil {
			s.logger.Error("failed to save recurring job instance", "error", err)
			continue
		}
		if err := s.queue.Enqueue(ctx, &instance); err != nil {
			s.logger.Error("failed to enqueue recurring job instance", "error", err)
			continue
		}
		count++

		// 2. Update the template's next RunAt
		t.RunAt = sched.Next(now)
		t.UpdatedAt = now
		if err := s.store.Save(ctx, t); err != nil {
			s.logger.Error("failed to update recurring job template run_at", "error", err)
		}
	}

	return count, nil
}

// ListQueueLengths returns pending job counts segmented by queue and tenant.
func (s *JobService) ListQueueLengths(ctx context.Context) (map[string]map[string]int64, error) {
	return s.store.GetQueueLengths(ctx)
}

// ─── DLQ Methods ──────────────────────────────────────────────────────────────

func (s *JobService) ListFailedJobs(ctx context.Context, tenantID, jobType string, limit, offset int) ([]*jobs.Job, error) {
	return s.store.ListJobs(ctx, tenantID, string(jobs.StatusFailed), jobType, limit, offset)
}

// CountFailedJobs returns the total number of permanently failed jobs matching filters.
func (s *JobService) CountFailedJobs(ctx context.Context, tenantID, jobType string) (int64, error) {
	return s.store.CountJobs(ctx, models.JobFilter{
		TenantID: tenantID,
		Type:     jobType,
		Status:   string(jobs.StatusFailed),
	})
}

// JobStatusCounts returns job counts grouped by status, optionally scoped to a tenant.
func (s *JobService) JobStatusCounts(ctx context.Context, tenantID string) (map[string]int64, error) {
	statuses := []jobs.JobStatus{
		jobs.StatusPending,
		jobs.StatusProcessing,
		jobs.StatusCompleted,
		jobs.StatusFailed,
		jobs.StatusCancelled,
	}
	counts := make(map[string]int64, len(statuses))
	for _, st := range statuses {
		n, err := s.store.CountJobs(ctx, models.JobFilter{
			TenantID: tenantID,
			Status:   string(st),
		})
		if err != nil {
			return nil, err
		}
		counts[string(st)] = n
	}
	return counts, nil
}

// CountJobs returns the number of jobs matching a filter.
func (s *JobService) CountJobs(ctx context.Context, filter models.JobFilter) (int64, error) {
	return s.store.CountJobs(ctx, filter)
}

// RevokeClient removes all API keys for a tenant.
func (s *JobService) RevokeClient(ctx context.Context, tenantID string) error {
	return s.store.RevokeClient(ctx, tenantID)
}

// RotateClientKey revokes existing keys and registers a new one.
func (s *JobService) RotateClientKey(ctx context.Context, tenantID string) (string, error) {
	if err := s.store.RevokeClient(ctx, tenantID); err != nil {
		return "", err
	}
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", err
	}
	rawKey := "tq_live_" + hex.EncodeToString(keyBytes)
	hash := sha256.Sum256([]byte(rawKey))
	if err := s.store.RegisterClient(ctx, tenantID, hex.EncodeToString(hash[:])); err != nil {
		return "", err
	}
	return rawKey, nil
}

func (s *JobService) ReplayJob(ctx context.Context, jobID, tenantID string) (*jobs.Job, error) {
	job, err := s.store.GetByID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if tenantID != "" && tenantID != "operator" && job.TenantID != tenantID {
		return nil, apperr.NewForbidden("you do not own this job")
	}

	// Reset job state
	job.Status = jobs.StatusPending
	job.Retries = 0
	job.UpdatedAt = time.Now().UTC()
	job.ProcessedBy = ""
	job.Result = nil

	if err := s.store.Save(ctx, job); err != nil {
		return nil, err
	}

	if err := s.queue.Enqueue(ctx, job); err != nil {
		return nil, err
	}

	return job, nil
}

func (s *JobService) DeleteJob(ctx context.Context, jobID, tenantID string) error {
	job, err := s.store.GetByID(ctx, jobID)
	if err != nil {
		return err
	}
	if tenantID != "" && tenantID != "operator" && job.TenantID != tenantID {
		return apperr.NewForbidden("you do not own this job")
	}
	return s.store.DeleteJob(ctx, jobID)
}

// PurgeJobsBefore deletes terminal jobs (completed/failed/cancelled) older than the given timestamp.
// Returns the count of purged jobs. Used for TTL-based auto-cleanup in the scheduler.
func (s *JobService) PurgeJobsBefore(ctx context.Context, olderThan time.Time) (int64, error) {
	var total int64
	for _, status := range []jobs.JobStatus{jobs.StatusCompleted, jobs.StatusFailed, jobs.StatusCancelled} {
		n, err := s.store.DeleteJobsBefore(ctx, "", string(status), "", olderThan)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (s *JobService) BulkPurgeDLQ(ctx context.Context, tenantID, jobType string, olderThan time.Time) (int64, error) {
	return s.store.DeleteJobsBefore(ctx, tenantID, string(jobs.StatusFailed), jobType, olderThan)
}



