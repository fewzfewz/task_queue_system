package models

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"sync/atomic"
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
			CreatedAt:  now.Add(-3 * time.Minute),
			UpdatedAt:  now.Add(-3 * time.Minute),
			RunAt:      now,
			MaxRetries: 3,
		},
		{
			ID:         "job-2",
			Type:       "email",
			Status:     jobs.StatusCompleted,
			Priority:   jobs.PriorityMedium,
			TenantID:   "tenant-a",
			CreatedAt:  now.Add(-2 * time.Minute),
			UpdatedAt:  now.Add(-2 * time.Minute),
			RunAt:      now,
			MaxRetries: 3,
		},
		{
			ID:         "job-3",
			Type:       "image",
			Status:     jobs.StatusPending,
			Priority:   jobs.PriorityHigh,
			TenantID:   "tenant-b",
			CreatedAt:  now.Add(-1 * time.Minute),
			UpdatedAt:  now.Add(-1 * time.Minute),
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

func TestRedisStore_IsDedupKeyTaken(t *testing.T) {
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
	defer rdb.Close()
	store := NewRedisStore(rdb)
	ctx := context.Background()

	t.Run("Empty dedup key is never taken", func(t *testing.T) {
		taken, err := store.IsDedupKeyTaken(ctx, "", "tenant-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if taken {
			t.Fatalf("expected empty dedup key to not be taken")
		}
	})

	t.Run("First submission succeeds and repeated submission is rejected", func(t *testing.T) {
		dedupKey := "order-123"
		tenant := "tenant-a"

		// 1. First submission: must not be taken (claims key)
		taken, err := store.IsDedupKeyTaken(ctx, dedupKey, tenant)
		if err != nil {
			t.Fatalf("first submission failed: %v", err)
		}
		if taken {
			t.Fatalf("expected first submission to be accepted (taken=false), got taken=true")
		}

		// 2. Second submission: must be taken (rejected duplicate)
		taken, err = store.IsDedupKeyTaken(ctx, dedupKey, tenant)
		if err != nil {
			t.Fatalf("second submission failed: %v", err)
		}
		if !taken {
			t.Fatalf("expected duplicate submission to be rejected (taken=true), got taken=false")
		}
	})

	t.Run("Expiration behavior resets deduplication after TTL", func(t *testing.T) {
		dedupKey := "expiring-key"
		tenant := "tenant-a"

		taken, err := store.IsDedupKeyTaken(ctx, dedupKey, tenant)
		if err != nil || taken {
			t.Fatalf("initial submission failed: taken=%v, err=%v", taken, err)
		}

		// Verify TTL is configured (within ~24 hours)
		key := "dedup:" + tenant + ":" + dedupKey
		ttl := mr.TTL(key)
		if ttl <= 0 || ttl > 24*time.Hour {
			t.Fatalf("expected TTL around 24h, got %v", ttl)
		}

		// Fast forward past TTL
		mr.FastForward(25 * time.Hour)

		// After expiration, should be available again
		taken, err = store.IsDedupKeyTaken(ctx, dedupKey, tenant)
		if err != nil {
			t.Fatalf("post-expiration submission failed: %v", err)
		}
		if taken {
			t.Fatalf("expected key to be available again after TTL expiration, got taken=true")
		}
	})

	t.Run("Different deduplication keys and tenant isolation", func(t *testing.T) {
		// Different keys for same tenant
		taken1, err := store.IsDedupKeyTaken(ctx, "key-alpha", "tenant-x")
		if err != nil || taken1 {
			t.Fatalf("key-alpha failed: taken=%v, err=%v", taken1, err)
		}

		taken2, err := store.IsDedupKeyTaken(ctx, "key-beta", "tenant-x")
		if err != nil || taken2 {
			t.Fatalf("key-beta failed: taken=%v, err=%v", taken2, err)
		}

		// Same key for different tenants must not collide
		takenOtherTenant, err := store.IsDedupKeyTaken(ctx, "key-alpha", "tenant-y")
		if err != nil || takenOtherTenant {
			t.Fatalf("key-alpha for tenant-y failed: taken=%v, err=%v", takenOtherTenant, err)
		}

		// Verify repeated submissions for each are rejected
		takenAlphaRepeat, _ := store.IsDedupKeyTaken(ctx, "key-alpha", "tenant-x")
		if !takenAlphaRepeat {
			t.Fatalf("expected key-alpha for tenant-x to be taken")
		}
		takenBetaRepeat, _ := store.IsDedupKeyTaken(ctx, "key-beta", "tenant-x")
		if !takenBetaRepeat {
			t.Fatalf("expected key-beta for tenant-x to be taken")
		}
		takenOtherTenantRepeat, _ := store.IsDedupKeyTaken(ctx, "key-alpha", "tenant-y")
		if !takenOtherTenantRepeat {
			t.Fatalf("expected key-alpha for tenant-y to be taken")
		}
	})

	t.Run("Concurrent submissions allow only one winner", func(t *testing.T) {
		dedupKey := "concurrent-key"
		tenant := "tenant-c"
		const concurrency = 25

		var wg sync.WaitGroup
		var successCount int64
		var duplicateCount int64
		var errCount int64

		start := make(chan struct{})

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				taken, err := store.IsDedupKeyTaken(ctx, dedupKey, tenant)
				if err != nil {
					atomic.AddInt64(&errCount, 1)
					return
				}
				if !taken {
					atomic.AddInt64(&successCount, 1)
				} else {
					atomic.AddInt64(&duplicateCount, 1)
				}
			}()
		}

		close(start)
		wg.Wait()

		if errCount != 0 {
			t.Fatalf("expected 0 errors, got %d", errCount)
		}
		if successCount != 1 {
			t.Fatalf("expected exactly 1 successful claim, got %d", successCount)
		}
		if duplicateCount != concurrency-1 {
			t.Fatalf("expected %d duplicates rejected, got %d", concurrency-1, duplicateCount)
		}
	})

	t.Run("Propagates Redis errors", func(t *testing.T) {
		// Create client pointing to invalid address
		badClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
		badStore := NewRedisStore(badClient)

		// Use short timeout context so test fails fast
		errCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()

		taken, err := badStore.IsDedupKeyTaken(errCtx, "err-key", "tenant-err")
		if err == nil {
			t.Fatalf("expected error from bad Redis client, got nil")
		}
		if taken {
			t.Fatalf("expected taken=false on error, got true")
		}
		_ = badClient.Close()
	})
}

func TestRedisStore_DLQ(t *testing.T) {
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
	defer rdb.Close()
	store := NewRedisStore(rdb)
	ctx := context.Background()

	t.Run("MoveToDLQ transitions job from active store to DLQ", func(t *testing.T) {
		job := &jobs.Job{
			ID:        "job-dlq-1",
			Type:      "email",
			Status:    jobs.StatusProcessing,
			TenantID:  "tenant-a",
			CreatedAt: time.Now().UTC().Add(-10 * time.Minute),
			UpdatedAt: time.Now().UTC().Add(-5 * time.Minute),
		}
		if err := store.Save(ctx, job); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		// Move to DLQ
		err := store.MoveToDLQ(ctx, job.ID, "database timeout")
		if err != nil {
			t.Fatalf("MoveToDLQ failed: %v", err)
		}

		// Must no longer exist in active store
		_, err = store.GetByID(ctx, job.ID)
		if !errors.Is(err, ErrJobNotFound) {
			t.Fatalf("expected ErrJobNotFound in active store, got %v", err)
		}

		// Must exist in DLQ with failed status and error reason
		dlqJob, err := store.GetFromDLQ(ctx, job.ID)
		if err != nil {
			t.Fatalf("GetFromDLQ failed: %v", err)
		}
		if dlqJob.Status != jobs.StatusFailed {
			t.Fatalf("expected status %s, got %s", jobs.StatusFailed, dlqJob.Status)
		}
		if dlqJob.Payload["_error_reason"] != "database timeout" {
			t.Fatalf("expected error reason 'database timeout', got %v", dlqJob.Payload["_error_reason"])
		}
		if len(dlqJob.ErrorHistory) == 0 || dlqJob.ErrorHistory[0].Error != "database timeout" {
			t.Fatalf("expected error in ErrorHistory, got %v", dlqJob.ErrorHistory)
		}

		// Idempotent: moving again should succeed
		if err := store.MoveToDLQ(ctx, job.ID, "repeat"); err != nil {
			t.Fatalf("idempotent MoveToDLQ failed: %v", err)
		}
	})

	t.Run("MoveToDLQ returns ErrJobNotFound for non-existent job", func(t *testing.T) {
		err := store.MoveToDLQ(ctx, "non-existent-id", "error")
		if !errors.Is(err, ErrJobNotFound) {
			t.Fatalf("expected ErrJobNotFound, got %v", err)
		}
	})

	t.Run("Count and List with tenant and type filtering", func(t *testing.T) {
		now := time.Now().UTC()
		jobsToAdd := []*jobs.Job{
			{ID: "j-1", Type: "email", TenantID: "tenant-a", CreatedAt: now.Add(-3 * time.Minute), UpdatedAt: now.Add(-3 * time.Minute)},
			{ID: "j-2", Type: "image", TenantID: "tenant-a", CreatedAt: now.Add(-2 * time.Minute), UpdatedAt: now.Add(-2 * time.Minute)},
			{ID: "j-3", Type: "email", TenantID: "tenant-b", CreatedAt: now.Add(-1 * time.Minute), UpdatedAt: now.Add(-1 * time.Minute)},
		}
		for _, j := range jobsToAdd {
			_ = store.Save(ctx, j)
			if err := store.MoveToDLQ(ctx, j.ID, "fail"); err != nil {
				t.Fatalf("MoveToDLQ failed: %v", err)
			}
		}

		// Total count (including job-dlq-1 = 4 total)
		totalCount, err := store.CountDLQ(ctx, "", "")
		if err != nil {
			t.Fatalf("CountDLQ failed: %v", err)
		}
		if totalCount != 4 {
			t.Fatalf("expected total count 4, got %d", totalCount)
		}

		// Filter by tenant-a
		tenantACount, err := store.CountDLQ(ctx, "tenant-a", "")
		if err != nil || tenantACount != 3 {
			t.Fatalf("expected tenant-a count 3, got %d, err: %v", tenantACount, err)
		}

		// Filter by tenant-b
		tenantBCount, err := store.CountDLQ(ctx, "tenant-b", "")
		if err != nil || tenantBCount != 1 {
			t.Fatalf("expected tenant-b count 1, got %d, err: %v", tenantBCount, err)
		}

		// Filter by tenant-a and type email
		tenantAEmailCount, err := store.CountDLQ(ctx, "tenant-a", "email")
		if err != nil || tenantAEmailCount != 2 {
			t.Fatalf("expected tenant-a email count 2, got %d, err: %v", tenantAEmailCount, err)
		}

		// List with ordering (newest first)
		listTenantA, err := store.ListDLQ(ctx, "tenant-a", "", 10, 0)
		if err != nil {
			t.Fatalf("ListDLQ failed: %v", err)
		}
		if len(listTenantA) != 3 {
			t.Fatalf("expected 3 jobs in list, got %d", len(listTenantA))
		}
		if listTenantA[0].ID != "j-2" || listTenantA[1].ID != "j-1" {
			t.Fatalf("expected newest-first ordering (j-2 then j-1), got %s, %s", listTenantA[0].ID, listTenantA[1].ID)
		}

		// Pagination: limit 1, offset 1
		paged, err := store.ListDLQ(ctx, "tenant-a", "", 1, 1)
		if err != nil {
			t.Fatalf("ListDLQ paged failed: %v", err)
		}
		if len(paged) != 1 || paged[0].ID != "j-1" {
			t.Fatalf("expected j-1 in paged result, got %v", paged)
		}
	})

	t.Run("DeleteFromDLQ removes specific job", func(t *testing.T) {
		err := store.DeleteFromDLQ(ctx, "j-3")
		if err != nil {
			t.Fatalf("DeleteFromDLQ failed: %v", err)
		}

		// Confirm it's gone
		_, err = store.GetFromDLQ(ctx, "j-3")
		if !errors.Is(err, ErrJobNotFound) {
			t.Fatalf("expected ErrJobNotFound after delete, got %v", err)
		}

		// Deleting again returns ErrJobNotFound
		err = store.DeleteFromDLQ(ctx, "j-3")
		if !errors.Is(err, ErrJobNotFound) {
			t.Fatalf("expected ErrJobNotFound on second delete, got %v", err)
		}
	})

	t.Run("DeleteDLQBefore bulk purges expired DLQ entries", func(t *testing.T) {
		// Populate DLQ with 1 old job (updated 10m ago)
		oldTime := time.Now().UTC().Add(-10 * time.Minute)
		oldJob := &jobs.Job{
			ID:        "old-dlq-job",
			Type:      "email",
			TenantID:  "tenant-a",
			Status:    jobs.StatusFailed,
			CreatedAt: oldTime,
			UpdatedAt: oldTime,
		}
		dataOld, _ := json.Marshal(oldJob)
		_ = rdb.HSet(ctx, dlqStoreKey, oldJob.ID, dataOld)

		cutoff := time.Now().UTC().Add(-4 * time.Minute)
		deleted, err := store.DeleteDLQBefore(ctx, "tenant-a", "", cutoff)
		if err != nil {
			t.Fatalf("DeleteDLQBefore failed: %v", err)
		}
		if deleted != 1 {
			t.Fatalf("expected 1 job deleted before cutoff, got %d", deleted)
		}

		// Verify old-dlq-job is gone
		_, err = store.GetFromDLQ(ctx, "old-dlq-job")
		if !errors.Is(err, ErrJobNotFound) {
			t.Fatalf("expected old-dlq-job to be deleted")
		}

		// Verify j-1 and j-2 still exist
		_, err = store.GetFromDLQ(ctx, "j-1")
		if err != nil {
			t.Fatalf("expected j-1 to still exist: %v", err)
		}
		_, err = store.GetFromDLQ(ctx, "j-2")
		if err != nil {
			t.Fatalf("expected j-2 to still exist: %v", err)
		}
	})

	t.Run("Propagates Redis errors on DLQ operations", func(t *testing.T) {
		badClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
		badStore := NewRedisStore(badClient)
		errCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()

		if err := badStore.MoveToDLQ(errCtx, "err-id", "err"); err == nil {
			t.Fatal("expected error on MoveToDLQ with bad client")
		}
		if _, err := badStore.ListDLQ(errCtx, "", "", 10, 0); err == nil {
			t.Fatal("expected error on ListDLQ with bad client")
		}
		if _, err := badStore.CountDLQ(errCtx, "", ""); err == nil {
			t.Fatal("expected error on CountDLQ with bad client")
		}
		if _, err := badStore.GetFromDLQ(errCtx, "err-id"); err == nil {
			t.Fatal("expected error on GetFromDLQ with bad client")
		}
		if err := badStore.DeleteFromDLQ(errCtx, "err-id"); err == nil {
			t.Fatal("expected error on DeleteFromDLQ with bad client")
		}
		if _, err := badStore.DeleteDLQBefore(errCtx, "", "", time.Now()); err == nil {
			t.Fatal("expected error on DeleteDLQBefore with bad client")
		}
		_ = badClient.Close()
	})
}
