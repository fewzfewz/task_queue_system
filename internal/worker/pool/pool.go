// Package pool manages a configurable pool of worker executors.
package pool

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

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

	for i := range p.cfg.NumWorkers {
		name := fmt.Sprintf("%s:worker-%d", p.instanceID, i+1)
		w := executor.NewWorkerProcessor(name, p.service, p.executor, p.limiter, p.logger)
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			w.Run(workerCtx)
		}()
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
	if p.cancel != nil {
		p.logger.Info("shutting down worker pool")
		p.cancel()
	}
	p.wg.Wait()
	p.logger.Info("worker pool shut down")
}
