// Package pool manages a configurable pool of worker executors.
package pool

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"task-queue-system/internal/metrics"
	"task-queue-system/internal/service"
	"task-queue-system/internal/worker/executor"
	"task-queue-system/internal/worker/limiter"
)

// Config holds the configuration for the worker pool.
type Config struct {
	// NumWorkers is the number of concurrent worker goroutines.
	// Must be at least 1.
	NumWorkers int
	// JobsPerSecond limits the global processing rate across all workers.
	// A value of 0 means no limit.
	JobsPerSecond float64
}

// Pool manages a fixed number of worker processors and their lifecycle.
type Pool struct {
	cfg        Config
	instanceID string
	service    *service.JobService
	executor   *executor.JobExecutor
	limiter    limiter.RateLimiter
	logger     *slog.Logger

	cancel context.CancelFunc
	wg     sync.WaitGroup

	busyCount atomic.Int32
	shuttingDown int32
}


// New creates a new Pool. Call Start to begin processing.
func New(cfg Config, instanceID string, svc *service.JobService, je *executor.JobExecutor, logger *slog.Logger) (*Pool, error) {
	if cfg.NumWorkers < 1 {
		return nil, fmt.Errorf("pool: NumWorkers must be at least 1, got %d", cfg.NumWorkers)
	}

	var l limiter.RateLimiter
	if cfg.JobsPerSecond > 0 {
		l = limiter.NewTokenBucketLimiter(cfg.JobsPerSecond)
	}

	return &Pool{
		cfg:        cfg,
		instanceID: instanceID,
		service:    svc,
		executor:   je,
		limiter:    l,
		logger:     logger,
	}, nil
}

// Start launches NumWorkers goroutines. It is non-blocking; workers run in
// the background. Call Stop to initiate graceful shutdown.
func (p *Pool) Start(ctx context.Context) {
	// Derive a cancellable child context so Stop can signal all workers
	// without cancelling the caller's context.
	workerCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	p.logger.Info("starting worker pool", "num_workers", p.cfg.NumWorkers)

	// Start heartbeat goroutine for this instance
	p.wg.Add(1)
	go p.heartbeatLoop(workerCtx)

	// Start metrics reporting goroutine
	p.wg.Add(1)
	go p.metricsLoop(workerCtx)

	for i := 0; i < p.cfg.NumWorkers; i++ {
		name := fmt.Sprintf("%s:worker-%d", p.instanceID, i+1)
		w := executor.NewWorkerProcessor(name, p.service, p.executor, p.limiter, p.logger)
		
		w.SetHooks(
			func() { p.busyCount.Add(1) },
			func() { p.busyCount.Add(-1) },
		)

		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			w.Run(workerCtx)
		}()
	}

}

// InitiateDrain marks the worker pool as draining and cancels the worker context.
// It returns true if the drain was started by this call, false if already draining.
func (p *Pool) InitiateDrain() bool {
	if !atomic.CompareAndSwapInt32(&p.shuttingDown, 0, 1) {
		return false
	}
	p.logger.Info("worker pool drain initiated")
	if p.cancel != nil {
		p.cancel()
	}
	return true
}

func (p *Pool) metricsLoop(ctx context.Context) {
	defer p.wg.Done()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 1. Worker Busy Ratio
			busy := float64(p.busyCount.Load())
			ratio := busy / float64(p.cfg.NumWorkers)
			metrics.WorkerBusyRatio.Set(ratio)
			
			// 2. Queue Lengths (Segmented)

			lengths, err := p.service.ListQueueLengths(ctx)
			if err == nil {
				// Clear old values to avoid stale metrics
				metrics.QueueLength.Reset()
				for qType, tenants := range lengths {
					for tenantID, count := range tenants {
						metrics.QueueLength.WithLabelValues(qType, tenantID).Set(float64(count))
					}
				}
			}
		}
	}
}

func (p *Pool) heartbeatLoop(ctx context.Context) {
	defer p.wg.Done()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Register initial heartbeat
	if err := p.service.RegisterHeartbeat(ctx, p.instanceID); err != nil {
		p.logger.Error("failed to register heartbeat", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.service.RegisterHeartbeat(ctx, p.instanceID); err != nil {
				p.logger.Error("failed to update heartbeat", "error", err)
			}
		}
	}
}

// Stop signals all workers to stop and blocks until they have all exited.
// It is safe to call Stop more than once.
func (p *Pool) Stop() {
	p.InitiateDrain()
	p.wg.Wait()
	p.logger.Info("worker pool shut down")
}
