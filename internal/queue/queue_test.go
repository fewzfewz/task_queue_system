package queue

import (
	"context"
	"testing"

	"task-queue-system/internal/jobs"
)

func TestMockQueueEnqueueDequeue(t *testing.T) {
	mq := NewMockQueue()
	ctx := context.Background()

	job := &jobs.Job{ID: "test-1", Type: "email", TenantID: "tenant-a"}
	if err := mq.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	size, err := mq.Size(ctx)
	if err != nil {
		t.Fatalf("Size failed: %v", err)
	}
	if size != 1 {
		t.Fatalf("expected size 1, got %d", size)
	}

	got, err := mq.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected a job, got nil")
	}
	if got.ID != "test-1" {
		t.Fatalf("expected ID test-1, got %s", got.ID)
	}

	size, _ = mq.Size(ctx)
	if size != 0 {
		t.Fatalf("expected size 0 after dequeue, got %d", size)
	}
}

func TestMockQueueDequeueBlocked(t *testing.T) {
	mq := NewMockQueue()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mq.Dequeue(ctx)
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

func TestMockQueueAckAndFail(t *testing.T) {
	mq := NewMockQueue()
	ctx := context.Background()

	if err := mq.Ack(ctx, "job-1"); err != nil {
		t.Fatalf("Ack failed: %v", err)
	}

	if err := mq.Fail(ctx, "job-1", nil); err != nil {
		t.Fatalf("Fail failed: %v", err)
	}

	failed, err := mq.GetFailedJobs(ctx)
	if err != nil {
		t.Fatalf("GetFailedJobs failed: %v", err)
	}
	if len(failed) != 1 {
		t.Fatalf("expected 1 failed job, got %d", len(failed))
	}
	if failed[0].ID != "job-1" {
		t.Fatalf("expected job-1, got %s", failed[0].ID)
	}

	// Verify drain
	failed2, _ := mq.GetFailedJobs(ctx)
	if len(failed2) != 0 {
		t.Fatal("expected GetFailedJobs to drain after read")
	}
}

func TestMockQueueGetMetrics(t *testing.T) {
	mq := NewMockQueue()
	ctx := context.Background()

	metrics, err := mq.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}
	if metrics.TotalJobs != 0 {
		t.Fatalf("expected TotalJobs 0, got %d", metrics.TotalJobs)
	}
}

func TestMockQueueHeartbeatAndWorkers(t *testing.T) {
	mq := NewMockQueue()
	ctx := context.Background()

	if err := mq.RegisterHeartbeat(ctx, "worker-1"); err != nil {
		t.Fatalf("RegisterHeartbeat failed: %v", err)
	}

	workers, err := mq.GetActiveWorkers(ctx)
	if err != nil {
		t.Fatalf("GetActiveWorkers failed: %v", err)
	}
	if len(workers) != 1 {
		t.Fatalf("expected 1 worker, got %d", len(workers))
	}
	if workers[0].ID != "worker-1" {
		t.Fatalf("expected worker-1, got %s", workers[0].ID)
	}
}

func TestMockQueueIdempotency(t *testing.T) {
	mq := NewMockQueue()
	ctx := context.Background()

	processed, err := mq.IsProcessed(ctx, "job-1")
	if err != nil {
		t.Fatalf("IsProcessed failed: %v", err)
	}
	if processed {
		t.Fatal("expected job-1 not processed yet")
	}

	if err := mq.MarkProcessed(ctx, "job-1"); err != nil {
		t.Fatalf("MarkProcessed failed: %v", err)
	}

	processed, err = mq.IsProcessed(ctx, "job-1")
	if err != nil {
		t.Fatalf("IsProcessed failed: %v", err)
	}
	if !processed {
		t.Fatal("expected job-1 to be processed")
	}
}

func TestMockQueueMaintenance(t *testing.T) {
	mq := NewMockQueue()
	ctx := context.Background()

	n, err := mq.PromoteScheduledJobs(ctx)
	if err != nil {
		t.Fatalf("PromoteScheduledJobs failed: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}

	n, err = mq.ReclaimTimedOutJobs(ctx)
	if err != nil {
		t.Fatalf("ReclaimTimedOutJobs failed: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

func TestMockQueueMultiTenancy(t *testing.T) {
	mq := NewMockQueue()
	ctx := context.Background()

	allowed, err := mq.IsAllowed(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("IsAllowed failed: %v", err)
	}
	if !allowed {
		t.Fatal("expected tenant-a to be allowed")
	}
}

func TestMockQueuePublishWebhookEvent(t *testing.T) {
	mq := NewMockQueue()
	ctx := context.Background()

	event := map[string]string{"type": "job.completed", "job_id": "job-1"}
	if err := mq.PublishWebhookEvent(ctx, event); err != nil {
		t.Fatalf("PublishWebhookEvent failed: %v", err)
	}

	// Invalid JSON should fail
	if err := mq.PublishWebhookEvent(ctx, make(chan int)); err == nil {
		t.Fatal("expected error for non-serializable event")
	}
}

func TestMockQueueImplementsQueue(t *testing.T) {
	var _ Queue = (*MockQueue)(nil)
}
