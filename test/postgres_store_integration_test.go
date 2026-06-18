package test

import (
	"context"
	"os"
	"testing"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/storage/postgres"
)

func newPostgresStore(t *testing.T) (*postgres.PostgresStore, context.Context) {
	t.Helper()
	conn := os.Getenv("POSTGRES_CONN_STR")
	if conn == "" {
		t.Skip("POSTGRES_CONN_STR not set, skipping Postgres integration test")
	}
	ctx := context.Background()
	store, err := postgres.New(ctx, conn)
	if err != nil {
		t.Skipf("Postgres not available: %v", err)
	}
	return store, ctx
}

func TestPostgresStoreIntegrationWorkflow(t *testing.T) {
	store, ctx := newPostgresStore(t)
	defer store.Close()

	job := jobs.NewJob("test-success", map[string]interface{}{"hello": "world"}, nil, jobs.PriorityMedium, 3, time.Time{}, "", 30, 1, "tenant-int")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if err := store.Enqueue(ctx, job); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	got, err := store.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("get by id failed: %v", err)
	}
	if got == nil || got.ID != job.ID {
		t.Fatalf("expected job %s, got %#v", job.ID, got)
	}

	list, err := store.ListJobs(ctx, "tenant-int", "", "test-success", 10, 0)
	if err != nil {
		t.Fatalf("list jobs failed: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("expected at least one job in list")
	}
}

func TestPostgresStore_UpdateStatus(t *testing.T) {
	store, ctx := newPostgresStore(t)
	defer store.Close()

	job := jobs.NewJob("update-status", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 30, 1, "tenant-upd")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	if err := store.UpdateStatus(ctx, job.ID, jobs.StatusProcessing, "worker-1"); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	got, err := store.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != jobs.StatusProcessing {
		t.Fatalf("expected StatusProcessing, got %v", got.Status)
	}
	if got.ProcessedBy != "worker-1" {
		t.Fatalf("expected ProcessedBy worker-1, got %s", got.ProcessedBy)
	}
}

func TestPostgresStore_UpdateResult(t *testing.T) {
	store, ctx := newPostgresStore(t)
	defer store.Close()

	job := jobs.NewJob("update-result", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 30, 1, "tenant-res")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	result := map[string]string{"output": "processed"}
	if err := store.UpdateResult(ctx, job.ID, jobs.StatusCompleted, "worker-1", result); err != nil {
		t.Fatalf("UpdateResult failed: %v", err)
	}

	got, err := store.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != jobs.StatusCompleted {
		t.Fatalf("expected StatusCompleted, got %v", got.Status)
	}
}

func TestPostgresStore_GetByWorkerAndStatus(t *testing.T) {
	store, ctx := newPostgresStore(t)
	defer store.Close()

	job := jobs.NewJob("get-by-worker", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 30, 1, "tenant-worker")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if err := store.UpdateStatus(ctx, job.ID, jobs.StatusProcessing, "worker-2"); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	jobs, err := store.GetByWorkerAndStatus(ctx, "worker-2", jobs.StatusProcessing)
	if err != nil {
		t.Fatalf("GetByWorkerAndStatus failed: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("expected at least one job for worker-2")
	}
}

func TestPostgresStore_Complete(t *testing.T) {
	store, ctx := newPostgresStore(t)
	defer store.Close()

	job := jobs.NewJob("complete-test", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 30, 1, "tenant-complete")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	if err := store.Complete(ctx, job.ID, "done"); err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	got, err := store.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != jobs.StatusCompleted {
		t.Fatalf("expected StatusCompleted, got %v", got.Status)
	}
}

func TestPostgresStore_Fail(t *testing.T) {
	store, ctx := newPostgresStore(t)
	defer store.Close()

	job := jobs.NewJob("fail-test", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 30, 1, "tenant-fail")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	if err := store.Fail(ctx, job.ID, nil, false); err != nil {
		t.Fatalf("Fail failed: %v", err)
	}

	got, err := store.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != jobs.StatusFailed {
		t.Fatalf("expected StatusFailed, got %v", got.Status)
	}
}

func TestPostgresStore_DeleteJob(t *testing.T) {
	store, ctx := newPostgresStore(t)
	defer store.Close()

	job := jobs.NewJob("delete-test", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 30, 1, "tenant-del")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	if err := store.DeleteJob(ctx, job.ID); err != nil {
		t.Fatalf("DeleteJob failed: %v", err)
	}

	_, err := store.GetByID(ctx, job.ID)
	if err == nil {
		t.Fatal("expected error after deleting job")
	}
}

func TestPostgresStore_GetQueueLengths(t *testing.T) {
	store, ctx := newPostgresStore(t)
	defer store.Close()

	job := jobs.NewJob("ql-test", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 30, 1, "tenant-ql")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if err := store.Enqueue(ctx, job); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	lengths, err := store.GetQueueLengths(ctx)
	if err != nil {
		t.Fatalf("GetQueueLengths failed: %v", err)
	}
	if len(lengths) == 0 {
		t.Log("GetQueueLengths returned empty (no jobs in pending status)")
	}
}

func TestPostgresStore_Heartbeat(t *testing.T) {
	store, ctx := newPostgresStore(t)
	defer store.Close()

	job := jobs.NewJob("hb-test", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 30, 1, "tenant-hb")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if err := store.UpdateStatus(ctx, job.ID, jobs.StatusProcessing, "worker-hb"); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	if err := store.Heartbeat(ctx, job.ID); err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
}

func TestPostgresStore_RecoverOrphans(t *testing.T) {
	store, ctx := newPostgresStore(t)
	defer store.Close()

	job := jobs.NewJob("orphan-test", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 30, 1, "tenant-orphan")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if err := store.UpdateStatus(ctx, job.ID, jobs.StatusProcessing, "worker-orphan"); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	count, err := store.RecoverOrphans(ctx, 1*time.Nanosecond)
	if err != nil {
		t.Fatalf("RecoverOrphans failed: %v", err)
	}
	t.Logf("RecoverOrphans recovered %d jobs", count)
}

func TestPostgresStore_ListJobsPagination(t *testing.T) {
	store, ctx := newPostgresStore(t)
	defer store.Close()

	for i := 0; i < 3; i++ {
		job := jobs.NewJob("list-paginate", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 30, 1, "tenant-page")
		if err := store.Save(ctx, job); err != nil {
			t.Fatalf("save failed for job %d: %v", i, err)
		}
	}

	page1, err := store.ListJobs(ctx, "tenant-page", "", "list-paginate", 2, 0)
	if err != nil {
		t.Fatalf("ListJobs page1 failed: %v", err)
	}
	if len(page1) > 2 {
		t.Fatalf("expected at most 2 jobs in page1, got %d", len(page1))
	}

	if len(page1) == 2 {
		page2, err := store.ListJobs(ctx, "tenant-page", "", "list-paginate", 2, 2)
		if err != nil {
			t.Fatalf("ListJobs page2 failed: %v", err)
		}
		t.Logf("page1=%d, page2=%d jobs", len(page1), len(page2))
	}
}

func TestPostgresStore_DeleteJobsBefore(t *testing.T) {
	store, ctx := newPostgresStore(t)
	defer store.Close()

	job := jobs.NewJob("delete-before", nil, nil, jobs.PriorityMedium, 3, time.Time{}, "", 30, 1, "tenant-db")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	count, err := store.DeleteJobsBefore(ctx, "tenant-db", "pending", "delete-before", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("DeleteJobsBefore failed: %v", err)
	}
	t.Logf("DeleteJobsBefore deleted %d jobs", count)
}
