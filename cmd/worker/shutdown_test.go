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

func TestShutdownEndpoint_GETMethod(t *testing.T) {
	log := logger.Setup()
	pool := &fakePool{
		jobCompleted: make(chan struct{}),
		drainStarted: make(chan struct{}, 1),
		stopDone:     make(chan struct{}),
	}
	coord := newShutdownCoordinator(pool, 10*time.Second, log)

	go func() {
		time.Sleep(50 * time.Millisecond)
		close(pool.jobCompleted)
	}()

	req := httptest.NewRequest(http.MethodGet, "/healthz/shutdown", nil)
	rr := httptest.NewRecorder()
	coord.Handler(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for GET, got %d", rr.Code)
	}
}

func TestShutdownEndpoint_DoubleInitiate(t *testing.T) {
	log := logger.Setup()
	pool := &fakePool{
		jobCompleted: make(chan struct{}),
		drainStarted: make(chan struct{}, 1),
		stopDone:     make(chan struct{}),
	}
	coord := newShutdownCoordinator(pool, 10*time.Second, log)

	go func() {
		time.Sleep(50 * time.Millisecond)
		close(pool.jobCompleted)
	}()

	// First call
	req1 := httptest.NewRequest(http.MethodPost, "/healthz/shutdown", nil)
	rr1 := httptest.NewRecorder()
	coord.Handler(rr1, req1)
	if rr1.Code != http.StatusAccepted {
		t.Fatalf("first call: expected 202, got %d", rr1.Code)
	}

	// Second call — should return 200 (already in progress)
	req2 := httptest.NewRequest(http.MethodPost, "/healthz/shutdown", nil)
	rr2 := httptest.NewRecorder()
	coord.Handler(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("second call: expected 200, got %d", rr2.Code)
	}
}

func TestShutdownEndpoint_InitiateReturnsFalseOnSecondCall(t *testing.T) {
	pool := &fakePool{
		jobCompleted: make(chan struct{}),
		drainStarted: make(chan struct{}, 1),
		stopDone:     make(chan struct{}),
	}
	coord := newShutdownCoordinator(pool, 10*time.Second, logger.Setup())

	if !coord.Initiate() {
		t.Fatal("expected Initiate to return true on first call")
	}
	if coord.Initiate() {
		t.Fatal("expected Initiate to return false on second call")
	}
}

func TestShutdownEndpoint_MethodNotAllowed(t *testing.T) {
	coord := newShutdownCoordinator(&fakePool{
		jobCompleted: make(chan struct{}),
		drainStarted: make(chan struct{}, 1),
		stopDone:     make(chan struct{}),
	}, 10*time.Second, logger.Setup())

	req := httptest.NewRequest(http.MethodPut, "/healthz/shutdown", nil)
	rr := httptest.NewRecorder()
	coord.Handler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for PUT, got %d", rr.Code)
	}
}

func TestShutdownEndpoint_WaitsForDone(t *testing.T) {
	pool := &fakePool{
		jobCompleted: make(chan struct{}),
		drainStarted: make(chan struct{}, 1),
		stopDone:     make(chan struct{}),
	}
	coord := newShutdownCoordinator(pool, 10*time.Second, logger.Setup())

	go func() {
		time.Sleep(100 * time.Millisecond)
		close(pool.jobCompleted)
	}()

	coord.Initiate()

	select {
	case <-coord.done:
	case <-time.After(2 * time.Second):
		t.Fatal("expected coordinator done channel to close after job completed")
	}
}
