// Package pool manages a configurable pool of worker executors.
package pool

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"golang.org/x/time/rate"

	"task-queue-system/internal/service"
	"task-queue-system/internal/worker/executor"
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
	cfg      Config
	service  *service.JobService
	executor *executor.JobExecutor
	limiter  *rate.Limiter
	logger   *slog.Logger

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a new Pool. Call Start to begin processing.
func New(cfg Config, svc *service.JobService, je *executor.JobExecutor, logger *slog.Logger) (*Pool, error) {
	if cfg.NumWorkers < 1 {
		return nil, fmt.Errorf("pool: NumWorkers must be at least 1, got %d", cfg.NumWorkers)
	}

	var limiter *rate.Limiter
	if cfg.JobsPerSecond > 0 {
		limiter = rate.NewLimiter(rate.Limit(cfg.JobsPerSecond), 1)
	}

	return &Pool{
		cfg:      cfg,
		service:  svc,
		executor: je,
		limiter:  limiter,
		logger:   logger,
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

	for i := range p.cfg.NumWorkers {
		w := executor.NewWorkerProcessor(i+1, p.service, p.executor, p.limiter, p.logger)
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			w.Run(workerCtx)
		}()
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
