package pool

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"task-queue-system/internal/jobs"
	redisqueue "task-queue-system/internal/queue/redis"
	"task-queue-system/internal/service"
	"task-queue-system/internal/storage/models"
	"task-queue-system/internal/worker/executor"
	"task-queue-system/internal/worker/plugin"
)

type benchFastPlugin struct {
	onExecute func()
}

func (p *benchFastPlugin) Type() string {
	return "bench-fast"
}

func (p *benchFastPlugin) Execute(_ context.Context, _ *jobs.Job) (interface{}, error) {
	if p.onExecute != nil {
		p.onExecute()
	}
	return "bench-ok", nil
}

func BenchmarkWorkerPoolThroughput(b *testing.B) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Skip("skipping: TCP listener not available")
	}
	_ = ln.Close()

	mr, err := miniredis.Run()
	if err != nil {
		b.Skip("skipping: miniredis could not start")
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	q := redisqueue.New(rdb, "bench-pool")
	store := models.NewRedisStore(rdb)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := service.New(q, store, logger, 0)

	var completed atomic.Int64
	doneCh := make(chan struct{}, 1)
	target := int64(b.N)

	fastPlugin := &benchFastPlugin{
		onExecute: func() {
			if completed.Add(1) >= target {
				select {
				case doneCh <- struct{}{}:
				default:
				}
			}
		},
	}

	pluginReg := plugin.NewRegistry()
	_ = pluginReg.Register(fastPlugin)
	cb := plugin.NewCircuitBreaker(100, 10*time.Second)
	je := executor.NewJobExecutorWithRegistry(pluginReg, cb, logger)

	// Pre-enqueue b.N jobs
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		job := &jobs.Job{
			ID:         fmt.Sprintf("pool-bench-%d", i),
			Type:       "bench-fast",
			TenantID:   "tenant-bench",
			Status:     jobs.StatusPending,
			Priority:   jobs.PriorityHigh,
			MaxRetries: 1,
			CreatedAt:  time.Now().UTC(),
			RunAt:      time.Now().UTC(),
		}
		if err := store.Save(ctx, job); err != nil {
			b.Fatalf("failed to save job: %v", err)
		}
		if err := q.Enqueue(ctx, job); err != nil {
			b.Fatalf("failed to enqueue job: %v", err)
		}
	}

	p, err := New(Config{
		NumWorkers: 4,
		SLATarget:  5 * time.Second,
	}, "bench-pool", svc, je, logger)
	if err != nil {
		b.Fatalf("failed to create pool: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	p.Start(ctx)

	// Wait for all jobs to be executed or benchmark timeout
	select {
	case <-doneCh:
	case <-time.After(30 * time.Second):
		b.Fatalf("timeout waiting for %d jobs to complete (completed: %d)", target, completed.Load())
	}

	b.StopTimer()
	p.Stop()
}
