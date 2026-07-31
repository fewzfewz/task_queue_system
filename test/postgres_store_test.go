package test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/storage/models"
	"task-queue-system/internal/storage/postgres"
)

// setupPostgresStore boots a real Postgres store, truncates the jobs table
// before and after the test, and returns a raw pool for direct SQL assertions.
// It is gated on POSTGRES_CONN_STR, mirroring the existing opt-in convention.
func setupPostgresStore(t *testing.T) (*postgres.PostgresStore, *pgxpool.Pool, context.Context) {
	t.Helper()
	conn := os.Getenv("POSTGRES_CONN_STR")
	if conn == "" {
		t.Skip("POSTGRES_CONN_STR not set, skipping Postgres integration tests (run: make test-postgres)")
	}
	if os.Getenv("POSTGRES_MIGRATIONS_DIR") == "" {
		_ = os.Setenv("POSTGRES_MIGRATIONS_DIR", resolveMigrationsDir())
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, conn)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	// Drop the jobs table and migration ledger so the latest schema applies
	// fresh even if a previous run created the tables from an older migration.
	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS jobs, schema_migrations CASCADE`); err != nil {
		pool.Close()
		t.Fatalf("drop tables: %v", err)
	}

	store, err := postgres.New(ctx, conn)
	if err != nil {
		pool.Close()
		t.Skipf("Postgres not available: %v", err)
	}

	if _, err := pool.Exec(ctx, `TRUNCATE TABLE jobs`); err != nil {
		pool.Close()
		store.Close()
		t.Fatalf("truncate jobs table: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `TRUNCATE TABLE jobs`)
		pool.Close()
		store.Close()
	})

	return store, pool, ctx
}

func makeStoreJob(jobType string, priority jobs.JobPriority, tenantID string) *jobs.Job {
	return jobs.NewJob(jobType, map[string]interface{}{"k": "v"}, nil, priority, 3, time.Now().Add(-time.Minute), "", 30, 1, tenantID)
}

func TestPostgresStore_SaveAndGetByID(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	job := makeStoreJob("roundtrip", jobs.PriorityHigh, "tenant-rt")
	job.Payload = map[string]interface{}{"to": "a@b.com", "nested": map[string]interface{}{"x": 1}}
	job.DedupKey = "dedup-rt"
	job.ShardKey = "shard-rt"
	job.CorrelationID = "corr-rt"
	job.Timeout = 120
	job.Version = 2
	job.Progress = 42.5
	job.Webhook = &jobs.WebhookConfig{
		URL: "http://hook.example", Secret: "sec", Events: []string{"completed"}, LastStatus: 1, Attempts: 2,
	}
	job.Dependencies = []string{"dep-1", "dep-2"}

	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := store.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.ID != job.ID || got.Type != job.Type || got.TenantID != job.TenantID {
		t.Fatalf("identity fields mismatch: %#v", got)
	}
	if got.Status != jobs.StatusPending {
		t.Fatalf("expected pending status, got %s", got.Status)
	}
	if got.Priority != jobs.PriorityHigh {
		t.Fatalf("expected high priority, got %s", got.Priority)
	}
	if got.DedupKey != job.DedupKey || got.ShardKey != job.ShardKey {
		t.Fatalf("dedup/shard mismatch: dedup=%q shard=%q", got.DedupKey, got.ShardKey)
	}
	if got.CorrelationID != job.CorrelationID || got.Timeout != 120 || got.Version != 2 {
		t.Fatalf("meta mismatch: corr=%q timeout=%d version=%d", got.CorrelationID, got.Timeout, got.Version)
	}
	if got.Progress != 42.5 {
		t.Fatalf("expected progress 42.5, got %v", got.Progress)
	}
	nested, ok := got.Payload["nested"].(map[string]interface{})
	if !ok || nested["x"] != float64(1) {
		t.Fatalf("nested payload not round-tripped: %#v", got.Payload)
	}
	if got.Webhook == nil || got.Webhook.URL != job.Webhook.URL || got.Webhook.Secret != job.Webhook.Secret {
		t.Fatalf("webhook not round-tripped: %#v", got.Webhook)
	}
	if len(got.Webhook.Events) != 1 || got.Webhook.Events[0] != "completed" {
		t.Fatalf("webhook events mismatch: %#v", got.Webhook.Events)
	}
	if len(got.Dependencies) != 2 {
		t.Fatalf("expected 2 dependencies, got %#v", got.Dependencies)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatal("expected timestamps to be populated")
	}
}

func TestPostgresStore_GetByIDNotFound(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	_, err := store.GetByID(ctx, "missing-job")
	if !errors.Is(err, models.ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound, got %v", err)
	}
}

func TestPostgresStore_UpdateStatusAndProgress(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	job := makeStoreJob("lifecycle", jobs.PriorityMedium, "tenant-lc")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if err := store.UpdateStatus(ctx, job.ID, jobs.StatusProcessing, "worker-1"); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}
	if err := store.UpdateProgress(ctx, job.ID, 33.3); err != nil {
		t.Fatalf("UpdateProgress failed: %v", err)
	}

	got, err := store.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != jobs.StatusProcessing || got.ProcessedBy != "worker-1" {
		t.Fatalf("expected processing/worker-1, got %s/%s", got.Status, got.ProcessedBy)
	}
	if got.Progress != 33.3 {
		t.Fatalf("expected progress 33.3, got %v", got.Progress)
	}

	if err := store.UpdateStatus(ctx, "missing", jobs.StatusCompleted, "w"); !errors.Is(err, models.ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound on UpdateStatus, got %v", err)
	}
	if err := store.UpdateProgress(ctx, "missing", 50); !errors.Is(err, models.ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound on UpdateProgress, got %v", err)
	}
}

func TestPostgresStore_UpdateResultCompleted(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	job := makeStoreJob("result", jobs.PriorityMedium, "tenant-res")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	result := map[string]interface{}{"output": "done"}
	if err := store.UpdateResult(ctx, job.ID, jobs.StatusCompleted, "worker-1", result); err != nil {
		t.Fatalf("UpdateResult failed: %v", err)
	}

	got, err := store.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != jobs.StatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
	out, ok := got.Result.(map[string]interface{})
	if !ok || out["output"] != "done" {
		t.Fatalf("result not round-tripped: %#v", got.Result)
	}
}

func TestPostgresStore_UpdateResultFailedAppendsErrorHistory(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	job := makeStoreJob("fail", jobs.PriorityMedium, "tenant-fail")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	for i, msg := range []string{"boom-1", "boom-2"} {
		if err := store.UpdateResult(ctx, job.ID, jobs.StatusFailed, "worker-1", msg); err != nil {
			t.Fatalf("UpdateResult #%d failed: %v", i, err)
		}
	}

	got, err := store.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != jobs.StatusFailed {
		t.Fatalf("expected failed, got %s", got.Status)
	}
	if got.Result != "boom-2" {
		t.Fatalf("expected last error as result, got %#v", got.Result)
	}
	if len(got.ErrorHistory) != 2 {
		t.Fatalf("expected 2 error history entries, got %d: %#v", len(got.ErrorHistory), got.ErrorHistory)
	}
	if got.ErrorHistory[0].Error != "boom-1" || got.ErrorHistory[1].Error != "boom-2" {
		t.Fatalf("error history order/content mismatch: %#v", got.ErrorHistory)
	}
}

func TestPostgresStore_GetByWorkerAndStatus(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	job := makeStoreJob("worker", jobs.PriorityMedium, "tenant-w")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := store.UpdateStatus(ctx, job.ID, jobs.StatusProcessing, "worker-7"); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	found, err := store.GetByWorkerAndStatus(ctx, "worker-7", jobs.StatusProcessing)
	if err != nil {
		t.Fatalf("GetByWorkerAndStatus failed: %v", err)
	}
	if len(found) != 1 || found[0].ID != job.ID {
		t.Fatalf("expected 1 job for worker-7, got %d", len(found))
	}

	none, err := store.GetByWorkerAndStatus(ctx, "worker-8", jobs.StatusProcessing)
	if err != nil {
		t.Fatalf("GetByWorkerAndStatus(worker-8) failed: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected 0 jobs for worker-8, got %d", len(none))
	}
}

func TestPostgresStore_EnqueueAndGet(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	job := makeStoreJob("enqueue", jobs.PriorityLow, "tenant-enq")
	if err := store.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	got, err := store.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.ID != job.ID || got.Status != jobs.StatusPending {
		t.Fatalf("enqueued job mismatch: %#v", got)
	}
}

func TestPostgresStore_DequeuePriorityOrder(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	high := makeStoreJob("prio", jobs.PriorityHigh, "tenant-p")
	mid := makeStoreJob("prio", jobs.PriorityMedium, "tenant-p")
	low := makeStoreJob("prio", jobs.PriorityLow, "tenant-p")
	for _, j := range []*jobs.Job{high, mid, low} {
		if err := store.Save(ctx, j); err != nil {
			t.Fatalf("Save failed for %s: %v", j.Priority, err)
		}
	}

	expected := []*jobs.Job{high, mid, low}
	for i, want := range expected {
		got, err := store.Dequeue(ctx, "", "")
		if err != nil {
			t.Fatalf("Dequeue #%d failed: %v", i, err)
		}
		if got == nil || got.ID != want.ID {
			t.Fatalf("expected dequeue #%d to be %s (%s), got %#v", i, want.ID, want.Priority, got)
		}
		if got.Status != jobs.StatusProcessing {
			t.Fatalf("expected dequeued job to be processing, got %s", got.Status)
		}
	}

	empty, err := store.Dequeue(ctx, "", "")
	if err != nil {
		t.Fatalf("Dequeue on empty failed: %v", err)
	}
	if empty != nil {
		t.Fatalf("expected nil after draining, got %#v", empty)
	}
}

func TestPostgresStore_DequeueSkipsScheduled(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	future := makeStoreJob("sched", jobs.PriorityHigh, "tenant-s")
	future.RunAt = time.Now().Add(time.Hour)
	if err := store.Save(ctx, future); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := store.Dequeue(ctx, "", "")
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for future-scheduled job, got %#v", got)
	}
}

func TestPostgresStore_DequeueTenantFilter(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	a := makeStoreJob("tenant", jobs.PriorityHigh, "tenant-a")
	b := makeStoreJob("tenant", jobs.PriorityHigh, "tenant-b")
	for _, j := range []*jobs.Job{a, b} {
		if err := store.Save(ctx, j); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	got, err := store.Dequeue(ctx, "tenant-a", "")
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if got == nil || got.ID != a.ID {
		t.Fatalf("expected tenant-a job, got %#v", got)
	}
	if got.TenantID != "tenant-a" {
		t.Fatalf("dequeued wrong tenant: %s", got.TenantID)
	}
}

func TestPostgresStore_DequeueShardFilter(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	a := makeStoreJob("shard", jobs.PriorityHigh, "tenant-sh")
	a.ShardKey = "shard-a"
	b := makeStoreJob("shard", jobs.PriorityHigh, "tenant-sh")
	b.ShardKey = "shard-b"
	for _, j := range []*jobs.Job{a, b} {
		if err := store.Save(ctx, j); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	got, err := store.Dequeue(ctx, "", "shard-a")
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if got == nil || got.ID != a.ID || got.ShardKey != "shard-a" {
		t.Fatalf("expected shard-a job, got %#v", got)
	}
}

func TestPostgresStore_DequeueBlocksUnmetDependencies(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	job := makeStoreJob("dag", jobs.PriorityHigh, "tenant-dag")
	job.Dependencies = []string{"nonexistent-dep"}
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := store.Dequeue(ctx, "", "")
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for job with unmet dependency, got %#v", got)
	}
}

func TestPostgresStore_DequeueReleasesAfterDependenciesComplete(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	dep := makeStoreJob("dag", jobs.PriorityHigh, "tenant-dag")
	child := makeStoreJob("dag", jobs.PriorityMedium, "tenant-dag")
	child.Dependencies = []string{dep.ID}
	for _, j := range []*jobs.Job{dep, child} {
		if err := store.Save(ctx, j); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	first, err := store.Dequeue(ctx, "", "")
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if first == nil || first.ID != dep.ID {
		t.Fatalf("expected dependency to dequeue first, got %#v", first)
	}

	if err := store.Complete(ctx, dep.ID, "ok"); err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	second, err := store.Dequeue(ctx, "", "")
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if second == nil || second.ID != child.ID {
		t.Fatalf("expected child to dequeue after dep completion, got %#v", second)
	}
}

func TestPostgresStore_CompleteAndFail(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	ok := makeStoreJob("complete", jobs.PriorityMedium, "tenant-c")
	if err := store.Save(ctx, ok); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := store.Complete(ctx, ok.ID, "finished"); err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	got, err := store.GetByID(ctx, ok.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != jobs.StatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}

	failNoRequeue := makeStoreJob("fail", jobs.PriorityMedium, "tenant-c")
	if err := store.Save(ctx, failNoRequeue); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := store.Fail(ctx, failNoRequeue.ID, errors.New("permanent"), false); err != nil {
		t.Fatalf("Fail(requeue=false) failed: %v", err)
	}
	got, err = store.GetByID(ctx, failNoRequeue.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != jobs.StatusFailed {
		t.Fatalf("expected failed, got %s", got.Status)
	}

	failRequeue := makeStoreJob("fail", jobs.PriorityMedium, "tenant-c")
	if err := store.Save(ctx, failRequeue); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := store.Fail(ctx, failRequeue.ID, errors.New("transient"), true); err != nil {
		t.Fatalf("Fail(requeue=true) failed: %v", err)
	}
	got, err = store.GetByID(ctx, failRequeue.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != jobs.StatusPending {
		t.Fatalf("expected pending after requeue, got %s", got.Status)
	}
}

func TestPostgresStore_Heartbeat(t *testing.T) {
	store, pool, ctx := setupPostgresStore(t)

	job := makeStoreJob("hb", jobs.PriorityMedium, "tenant-hb")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := store.UpdateStatus(ctx, job.ID, jobs.StatusProcessing, "worker-hb"); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}
	if err := store.Heartbeat(ctx, job.ID); err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	var heartbeat *time.Time
	if err := pool.QueryRow(ctx, `SELECT last_heartbeat_at FROM jobs WHERE id = $1`, job.ID).Scan(&heartbeat); err != nil {
		t.Fatalf("query heartbeat failed: %v", err)
	}
	if heartbeat == nil {
		t.Fatal("expected last_heartbeat_at to be set after Heartbeat")
	}
}

func TestPostgresStore_ListJobsFiltersAndPagination(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	for i := 0; i < 3; i++ {
		j := makeStoreJob("email", jobs.PriorityMedium, "tenant-l")
		if i == 2 {
			j.Status = jobs.StatusCompleted
		}
		if err := store.Save(ctx, j); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}
	other := makeStoreJob("image", jobs.PriorityMedium, "tenant-m")
	if err := store.Save(ctx, other); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	all, err := store.ListJobs(ctx, "tenant-l", "", "email", 10, 0)
	if err != nil {
		t.Fatalf("ListJobs failed: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 email jobs for tenant-l, got %d", len(all))
	}

	pending, err := store.ListJobs(ctx, "tenant-l", "pending", "email", 10, 0)
	if err != nil {
		t.Fatalf("ListJobs(pending) failed: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending email jobs, got %d", len(pending))
	}

	page1, err := store.ListJobs(ctx, "tenant-l", "", "email", 2, 0)
	if err != nil {
		t.Fatalf("ListJobs(page1) failed: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 jobs on page 1, got %d", len(page1))
	}
	page2, err := store.ListJobs(ctx, "tenant-l", "", "email", 2, 2)
	if err != nil {
		t.Fatalf("ListJobs(page2) failed: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("expected 1 job on page 2, got %d", len(page2))
	}

	none, err := store.ListJobs(ctx, "tenant-zzz", "", "", 10, 0)
	if err != nil {
		t.Fatalf("ListJobs(no tenant) failed: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected no jobs for tenant-zzz, got %d", len(none))
	}
}

func TestPostgresStore_SearchJobs(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	pending := makeStoreJob("email", jobs.PriorityMedium, "tenant-s")
	completed := makeStoreJob("email", jobs.PriorityMedium, "tenant-s")
	completed.Status = jobs.StatusCompleted
	for _, j := range []*jobs.Job{pending, completed} {
		if err := store.Save(ctx, j); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	filtered, err := store.SearchJobs(ctx, models.JobFilter{
		TenantID: "tenant-s", Status: "pending", Type: "email", Limit: 10, Offset: 0,
	})
	if err != nil {
		t.Fatalf("SearchJobs failed: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != pending.ID {
		t.Fatalf("expected 1 pending email job, got %d", len(filtered))
	}
}

func TestPostgresStore_RecoverOrphans(t *testing.T) {
	store, pool, ctx := setupPostgresStore(t)

	job := makeStoreJob("orphan", jobs.PriorityMedium, "tenant-o")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := store.UpdateStatus(ctx, job.ID, jobs.StatusProcessing, "worker-orphan"); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET last_heartbeat_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("backdate heartbeat failed: %v", err)
	}

	count, err := store.RecoverOrphans(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("RecoverOrphans failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 orphan recovered, got %d", count)
	}

	got, err := store.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != jobs.StatusPending || got.ProcessedBy != "" {
		t.Fatalf("expected recovered job to be pending/unassigned, got %s/%q", got.Status, got.ProcessedBy)
	}
}

func TestPostgresStore_DeleteJobAndTTLCleanup(t *testing.T) {
	store, pool, ctx := setupPostgresStore(t)

	job := makeStoreJob("dlq", jobs.PriorityMedium, "tenant-dlq")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := store.DeleteJob(ctx, job.ID); err != nil {
		t.Fatalf("DeleteJob failed: %v", err)
	}
	if _, err := store.GetByID(ctx, job.ID); !errors.Is(err, models.ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound after DeleteJob, got %v", err)
	}

	old := makeStoreJob("dlq", jobs.PriorityMedium, "tenant-dlq")
	if err := store.Save(ctx, old); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET created_at = NOW() - INTERVAL '48 hours' WHERE id = $1`, old.ID); err != nil {
		t.Fatalf("backdate created_at failed: %v", err)
	}
	fresh := makeStoreJob("dlq", jobs.PriorityMedium, "tenant-dlq")
	if err := store.Save(ctx, fresh); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	count, err := store.DeleteJobsBefore(ctx, "", "pending", "dlq", time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("DeleteJobsBefore failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 job deleted (TTL cleanup), got %d", count)
	}
	if _, err := store.GetByID(ctx, fresh.ID); err != nil {
		t.Fatalf("fresh job should survive TTL cleanup: %v", err)
	}
}

func TestPostgresStore_IsDedupKeyTaken(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	job := makeStoreJob("dedup", jobs.PriorityMedium, "tenant-dedup")
	job.DedupKey = "dedup-1"
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	taken, err := store.IsDedupKeyTaken(ctx, "dedup-1", "tenant-dedup")
	if err != nil {
		t.Fatalf("IsDedupKeyTaken failed: %v", err)
	}
	if !taken {
		t.Fatal("expected dedup key to be taken for same tenant")
	}

	otherTenant, err := store.IsDedupKeyTaken(ctx, "dedup-1", "other-tenant")
	if err != nil {
		t.Fatalf("IsDedupKeyTaken(other tenant) failed: %v", err)
	}
	if otherTenant {
		t.Fatal("dedup key should be scoped per tenant")
	}

	empty, err := store.IsDedupKeyTaken(ctx, "", "tenant-dedup")
	if err != nil {
		t.Fatalf("IsDedupKeyTaken(empty) failed: %v", err)
	}
	if empty {
		t.Fatal("empty dedup key should never be taken")
	}
}

func TestPostgresStore_GetByIDs(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	a := makeStoreJob("ids", jobs.PriorityMedium, "tenant-id")
	b := makeStoreJob("ids", jobs.PriorityMedium, "tenant-id")
	if err := store.Save(ctx, a); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := store.Save(ctx, b); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := store.GetByIDs(ctx, []string{a.ID, b.ID, "missing"})
	if err != nil {
		t.Fatalf("GetByIDs failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(got))
	}
	ids := map[string]bool{}
	for _, j := range got {
		ids[j.ID] = true
	}
	if !ids[a.ID] || !ids[b.ID] {
		t.Fatalf("expected jobs %s and %s, got %#v", a.ID, b.ID, ids)
	}
}

func TestPostgresStore_GetQueueLengths(t *testing.T) {
	store, _, ctx := setupPostgresStore(t)

	pending1 := makeStoreJob("email", jobs.PriorityMedium, "tenant-q")
	pending2 := makeStoreJob("email", jobs.PriorityMedium, "tenant-q")
	done := makeStoreJob("email", jobs.PriorityMedium, "tenant-q")
	done.Status = jobs.StatusCompleted
	image := makeStoreJob("image", jobs.PriorityMedium, "tenant-q2")
	for _, j := range []*jobs.Job{pending1, pending2, done, image} {
		if err := store.Save(ctx, j); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	lengths, err := store.GetQueueLengths(ctx)
	if err != nil {
		t.Fatalf("GetQueueLengths failed: %v", err)
	}
	if lengths["email"]["tenant-q"] != 2 {
		t.Fatalf("expected 2 pending email jobs for tenant-q, got %d", lengths["email"]["tenant-q"])
	}
	if lengths["image"]["tenant-q2"] != 1 {
		t.Fatalf("expected 1 pending image job for tenant-q2, got %d", lengths["image"]["tenant-q2"])
	}
	if _, ok := lengths["email"]["tenant-q2"]; ok {
		t.Fatal("did not expect email jobs for tenant-q2")
	}
}
