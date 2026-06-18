package pool

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"task-queue-system/internal/queue"
	"task-queue-system/internal/service"
	"task-queue-system/internal/storage/models"
	"task-queue-system/internal/worker/executor"
)

func newTestDeps(t *testing.T) (*service.JobService, *executor.JobExecutor) {
	t.Helper()
	store := models.NewInMemoryStore()
	mq := queue.NewMockQueue()
	svc := service.New(mq, store, slog.Default(), 0)
	je := executor.NewJobExecutor(slog.Default())
	return svc, je
}

func TestPool_New_InvalidConfig(t *testing.T) {
	svc, je := newTestDeps(t)
	_, err := New(Config{NumWorkers: 0}, "test", svc, je, slog.Default())
	if err == nil {
		t.Fatal("expected error for NumWorkers=0")
	}
}

func TestPool_New_ValidConfig(t *testing.T) {
	svc, je := newTestDeps(t)
	p, err := New(Config{NumWorkers: 1}, "test", svc, je, slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil pool")
	}
}

func TestPool_StartStop(t *testing.T) {
	svc, je := newTestDeps(t)
	p, err := New(Config{NumWorkers: 2}, "pool-test", svc, je, slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p.Start(context.Background())
	time.Sleep(50 * time.Millisecond)

	p.Stop()
}

func TestPool_InitiateDrain(t *testing.T) {
	svc, je := newTestDeps(t)
	p, err := New(Config{NumWorkers: 1}, "drain-test", svc, je, slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p.Start(context.Background())

	if !p.InitiateDrain() {
		t.Fatal("expected InitiateDrain to return true on first call")
	}
	if p.InitiateDrain() {
		t.Fatal("expected InitiateDrain to return false on second call")
	}

	p.Stop()
}

func TestPool_WithRateLimit(t *testing.T) {
	svc, je := newTestDeps(t)
	p, err := New(Config{NumWorkers: 1, JobsPerSecond: 10.0}, "rate-test", svc, je, slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.limiter == nil {
		t.Fatal("expected non-nil limiter when JobsPerSecond > 0")
	}

	p.Start(context.Background())
	p.Stop()
}

func TestPool_StopMultiple(t *testing.T) {
	svc, je := newTestDeps(t)
	p, err := New(Config{NumWorkers: 1}, "multi-stop", svc, je, slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p.Start(context.Background())
	p.Stop()
	p.Stop()
}

func TestPool_BusyCountStartsAtZero(t *testing.T) {
	svc, je := newTestDeps(t)
	p, err := New(Config{NumWorkers: 3}, "busy-test", svc, je, slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.busyCount.Load() != 0 {
		t.Fatalf("expected busyCount=0, got %d", p.busyCount.Load())
	}

	p.Start(context.Background())
	p.Stop()
}
