package test

import (
	"context"
	"os"
	"testing"
	"time"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/storage/postgres"
)

func TestPostgresStoreIntegrationWorkflow(t *testing.T) {
	conn := os.Getenv("POSTGRES_CONN_STR")
	if conn == "" {
		t.Skip("POSTGRES_CONN_STR not set, skipping Postgres integration workflow")
	}

	ctx := context.Background()
	store, err := postgres.New(ctx, conn)
	if err != nil {
		t.Skipf("Postgres not available, skipping integration workflow: %v", err)
	}
	defer store.Close()

	job := jobs.NewJob("test-success", map[string]interface{}{"hello": "world"}, jobs.PriorityMedium, 3, time.Time{}, "", 30, 1, "tenant-int")
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
