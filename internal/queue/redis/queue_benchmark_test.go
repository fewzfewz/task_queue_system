package redis

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"task-queue-system/internal/jobs"
)

func newBenchmarkQueue(b *testing.B) (*miniredis.Miniredis, *RedisQueue, context.Context) {
	b.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Skip("skipping: TCP listener not available")
	}
	_ = ln.Close()

	mr, err := miniredis.Run()
	if err != nil {
		b.Skip("skipping: miniredis could not start")
	}

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	q := New(rdb, "bench")
	ctx := context.Background()
	return mr, q, ctx
}

func BenchmarkEnqueue(b *testing.B) {
	mr, q, ctx := newBenchmarkQueue(b)
	defer mr.Close()

	job := &jobs.Job{
		Type:     "benchmark",
		TenantID: "tenant-bench",
		Priority: jobs.PriorityHigh,
		Payload:  map[string]interface{}{"data": "benchmark payload"},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		job.ID = fmt.Sprintf("bench-job-%d", i)
		if err := q.Enqueue(ctx, job); err != nil {
			b.Fatalf("Enqueue failed: %v", err)
		}
	}
}

func BenchmarkDequeue(b *testing.B) {
	mr, q, ctx := newBenchmarkQueue(b)
	defer mr.Close()

	// Pre-enqueue b.N jobs
	for i := 0; i < b.N; i++ {
		job := &jobs.Job{
			ID:       fmt.Sprintf("bench-job-%d", i),
			Type:     "benchmark",
			TenantID: "tenant-bench",
			Priority: jobs.PriorityHigh,
			Payload:  map[string]interface{}{"data": "benchmark payload"},
		}
		if err := q.Enqueue(ctx, job); err != nil {
			b.Fatalf("failed to pre-enqueue: %v", err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		j, err := q.Dequeue(ctx)
		if err != nil {
			b.Fatalf("Dequeue failed: %v", err)
		}
		if j == nil {
			b.Fatalf("expected job, got nil")
		}
	}
}

func BenchmarkEnqueueParallel(b *testing.B) {
	mr, q, ctx := newBenchmarkQueue(b)
	defer mr.Close()

	var counter int64
	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			id := atomic.AddInt64(&counter, 1)
			job := &jobs.Job{
				ID:       fmt.Sprintf("bench-par-%d", id),
				Type:     "benchmark",
				TenantID: "tenant-bench",
				Priority: jobs.PriorityMedium,
				Payload:  map[string]interface{}{"key": "val"},
			}
			if err := q.Enqueue(ctx, job); err != nil {
				b.Fatalf("Enqueue failed: %v", err)
			}
		}
	})
}
