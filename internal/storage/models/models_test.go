package models

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"task-queue-system/internal/jobs"
)

func TestRedisStoreListJobsAndQueueLengths(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("skipping Redis-backed store test: local TCP listeners are not permitted in this environment")
	}
	_ = ln.Close()

	mr, err := miniredis.Run()
	if err != nil {
		t.Skip("skipping Redis-backed store test: miniredis could not start in this environment")
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := NewRedisStore(rdb)
	ctx := context.Background()

	now := time.Now().UTC()
	jobsToSave := []*jobs.Job{
		{
			ID:         "job-1",
			Type:       "email",
			Status:     jobs.StatusPending,
			Priority:   jobs.PriorityMedium,
			TenantID:   "tenant-a",
			CreatedAt:   now.Add(-3 * time.Minute),
			UpdatedAt:   now.Add(-3 * time.Minute),
			RunAt:      now,
			MaxRetries: 3,
		},
		{
			ID:         "job-2",
			Type:       "email",
			Status:     jobs.StatusCompleted,
			Priority:   jobs.PriorityMedium,
			TenantID:   "tenant-a",
			CreatedAt:   now.Add(-2 * time.Minute),
			UpdatedAt:   now.Add(-2 * time.Minute),
			RunAt:      now,
			MaxRetries: 3,
		},
		{
			ID:         "job-3",
			Type:       "image",
			Status:     jobs.StatusPending,
			Priority:   jobs.PriorityHigh,
			TenantID:   "tenant-b",
			CreatedAt:   now.Add(-1 * time.Minute),
			UpdatedAt:   now.Add(-1 * time.Minute),
			RunAt:      now,
			MaxRetries: 3,
		},
	}

	for _, j := range jobsToSave {
		if err := store.Save(ctx, j); err != nil {
			t.Fatalf("failed to save job %s: %v", j.ID, err)
		}
	}

	list, err := store.ListJobs(ctx, "tenant-a", "", "email", 10, 0)
	if err != nil {
		t.Fatalf("ListJobs failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 jobs for tenant-a/email, got %d", len(list))
	}
	if list[0].ID != "job-2" || list[1].ID != "job-1" {
		t.Fatalf("expected newest-first ordering, got %s then %s", list[0].ID, list[1].ID)
	}

	paged, err := store.ListJobs(ctx, "", "", "", 1, 1)
	if err != nil {
		t.Fatalf("paged ListJobs failed: %v", err)
	}
	if len(paged) != 1 {
		t.Fatalf("expected 1 job in paged result, got %d", len(paged))
	}

	lengths, err := store.GetQueueLengths(ctx)
	if err != nil {
		t.Fatalf("GetQueueLengths failed: %v", err)
	}
	if lengths["email"]["tenant-a"] != 1 {
		t.Fatalf("expected 1 pending email job for tenant-a, got %d", lengths["email"]["tenant-a"])
	}
	if lengths["image"]["tenant-b"] != 1 {
		t.Fatalf("expected 1 pending image job for tenant-b, got %d", lengths["image"]["tenant-b"])
	}
	if _, ok := lengths["email"]["tenant-b"]; ok {
		t.Fatalf("did not expect pending email jobs for tenant-b")
	}
}
