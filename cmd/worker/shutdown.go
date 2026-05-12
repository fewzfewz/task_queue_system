package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"task-queue-system/internal/metrics"
)

type workerPool interface {
	InitiateDrain() bool
	Stop()
}

type shutdownCoordinator struct {
	pool    workerPool
	logger  *slog.Logger
	timeout time.Duration
	done    chan struct{}
	started int32
	once    sync.Once
}

func newShutdownCoordinator(pool workerPool, timeout time.Duration, logger *slog.Logger) *shutdownCoordinator {
	return &shutdownCoordinator{
		pool:    pool,
		logger:  logger,
		timeout: timeout,
		done:    make(chan struct{}),
	}
}

func (c *shutdownCoordinator) Initiate() bool {
	if !atomic.CompareAndSwapInt32(&c.started, 0, 1) {
		return false
	}
	c.logger.Info("shutdown initiated")
	c.pool.InitiateDrain()
	go c.waitForDrain()
	return true
}

func (c *shutdownCoordinator) waitForDrain() {
	defer c.once.Do(func() { close(c.done) })
	done := make(chan struct{})
	go func() {
		c.pool.Stop()
		close(done)
	}()

	select {
	case <-done:
		metrics.WorkerGracefulShutdownTotal.Inc()
		c.logger.Info("graceful shutdown completed")
	case <-time.After(c.timeout):
		c.logger.Warn("drain timeout exceeded, forcing exit")
		os.Exit(1)
	}
}

func (c *shutdownCoordinator) Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	started := c.Initiate()
	if started {
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, "shutdown initiated")
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "shutdown already in progress")
}

func (c *shutdownCoordinator) Wait() {
	<-c.done
}
