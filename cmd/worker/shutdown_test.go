package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"task-queue-system/internal/logger"
)

type fakePool struct {
	jobCompleted      chan struct{}
	drainStarted      chan struct{}
	stopDone          chan struct{}
	initiated         int32
	jobCompletedFirst bool
}

func (f *fakePool) InitiateDrain() bool {
	if !atomic.CompareAndSwapInt32(&f.initiated, 0, 1) {
		return false
	}
	select {
	case f.drainStarted <- struct{}{}:
	default:
	}
	return true
}

func (f *fakePool) Stop() {
	<-f.jobCompleted
	f.jobCompletedFirst = true
	close(f.stopDone)
}

func TestShutdownEndpointDrainsInFlightJob(t *testing.T) {
	log := logger.Setup()
	pool := &fakePool{
		jobCompleted: make(chan struct{}),
		drainStarted: make(chan struct{}, 1),
		stopDone:     make(chan struct{}),
	}

	coord := newShutdownCoordinator(pool, 10*time.Second, log)

	// Simulate a long-running job.
	go func() {
		time.Sleep(3 * time.Second)
		close(pool.jobCompleted)
	}()

	req := httptest.NewRequest(http.MethodPost, "/healthz/shutdown", nil)
	rr := httptest.NewRecorder()
	coord.Handler(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status 202 Accepted, got %d", rr.Code)
	}

	select {
	case <-pool.drainStarted:
	case <-time.After(1 * time.Second):
		t.Fatal("expected drain to start after shutdown request")
	}

	select {
	case <-pool.stopDone:
	case <-time.After(6 * time.Second):
		t.Fatal("expected shutdown to complete after job finished")
	}

	if !pool.jobCompletedFirst {
		t.Fatal("expected in-flight job to complete before shutdown blocked")
	}
}
