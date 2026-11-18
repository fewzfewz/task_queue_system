package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"task-queue-system/internal/config"
	"task-queue-system/internal/health"
)

type mockMaintenanceExecutor struct {
	mu           sync.Mutex
	promoteCalls int
	reclaimCalls int
	deferCalls   int
	recurCalls   int
	archiveCalls int
	onPromote    func()
}

func (m *mockMaintenanceExecutor) PromoteScheduledJobs(ctx context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.promoteCalls++
	if m.onPromote != nil {
		m.onPromote()
	}
	return 1, nil
}

func (m *mockMaintenanceExecutor) ReclaimTimedOutJobs(ctx context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reclaimCalls++
	return 1, nil
}

func (m *mockMaintenanceExecutor) ReconcileDeferredJobs(ctx context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deferCalls++
	return 1, nil
}

func (m *mockMaintenanceExecutor) RecurringJobsDue(ctx context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recurCalls++
	return 1, nil
}

func (m *mockMaintenanceExecutor) ArchiveOldJobs(ctx context.Context, age time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.archiveCalls++
	return 1, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestLeaderElector_FirstSchedulerAcquiresLeadership verifies that a single scheduler immediately acquires leadership.
func TestLeaderElector_FirstSchedulerAcquiresLeadership(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	elector := NewLeaderElector(LeaderElectorConfig{
		Client:        rdb,
		LeaderKey:     "test:scheduler:leader",
		InstanceID:    "node-1",
		LeaseTTL:      2 * time.Second,
		RenewInterval: 200 * time.Millisecond,
		Logger:        discardLogger(),
	})
	elector.Start(ctx)
	defer elector.Release(context.Background())

	if !elector.IsLeader() {
		t.Fatal("expected node-1 to acquire leadership immediately")
	}

	val, err := rdb.Get(ctx, "test:scheduler:leader").Result()
	if err != nil {
		t.Fatalf("failed to query leader key: %v", err)
	}
	if val != "node-1" {
		t.Fatalf("expected leader key to hold node-1, got %s", val)
	}
}

// TestLeaderElector_SecondSchedulerRemainsStandby verifies that a second instance does not steal leadership.
func TestLeaderElector_SecondSchedulerRemainsStandby(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	elector1 := NewLeaderElector(LeaderElectorConfig{
		Client:        rdb,
		LeaderKey:     "test:scheduler:leader",
		InstanceID:    "node-1",
		LeaseTTL:      2 * time.Second,
		RenewInterval: 200 * time.Millisecond,
		Logger:        discardLogger(),
	})
	elector1.Start(ctx)
	defer elector1.Release(context.Background())

	elector2 := NewLeaderElector(LeaderElectorConfig{
		Client:        rdb,
		LeaderKey:     "test:scheduler:leader",
		InstanceID:    "node-2",
		LeaseTTL:      2 * time.Second,
		RenewInterval: 200 * time.Millisecond,
		Logger:        discardLogger(),
	})
	elector2.Start(ctx)
	defer elector2.Release(context.Background())

	if !elector1.IsLeader() {
		t.Fatal("expected node-1 to be leader")
	}
	if elector2.IsLeader() {
		t.Fatal("expected node-2 to be standby, but was leader")
	}

	mockExec2 := &mockMaintenanceExecutor{}
	cfg := &config.Config{}
	_, executed := runMaintenanceTick(ctx, mockExec2, elector2, cfg, discardLogger(), 0)
	if executed {
		t.Fatal("expected standby node-2 to skip maintenance tick")
	}
	if mockExec2.promoteCalls > 0 {
		t.Fatalf("expected 0 routine calls on standby, got %d", mockExec2.promoteCalls)
	}
}

// TestLeaderElector_LeaseRenewalWorks verifies that the leader periodically extends its TTL.
func TestLeaderElector_LeaseRenewalWorks(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	elector := NewLeaderElector(LeaderElectorConfig{
		Client:        rdb,
		LeaderKey:     "test:scheduler:leader",
		InstanceID:    "node-1",
		LeaseTTL:      500 * time.Millisecond,
		RenewInterval: 100 * time.Millisecond,
		Logger:        discardLogger(),
	})
	elector.Start(ctx)
	defer elector.Release(context.Background())

	// Wait for several renewal intervals
	time.Sleep(350 * time.Millisecond)

	if !elector.IsLeader() {
		t.Fatal("expected node-1 to remain leader after renewals")
	}

	ttl := mr.TTL("test:scheduler:leader")
	if ttl <= 0 {
		t.Fatalf("expected key to have positive TTL, got %v", ttl)
	}
}

// TestLeaderElector_LeaseExpirationAllowsFailover verifies that standby takes over when leader shuts down or expires.
func TestLeaderElector_LeaseExpirationAllowsFailover(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	elector1 := NewLeaderElector(LeaderElectorConfig{
		Client:        rdb,
		LeaderKey:     "test:scheduler:leader",
		InstanceID:    "node-1",
		LeaseTTL:      500 * time.Millisecond,
		RenewInterval: 100 * time.Millisecond,
		Logger:        discardLogger(),
	})
	elector1.Start(ctx)

	elector2 := NewLeaderElector(LeaderElectorConfig{
		Client:        rdb,
		LeaderKey:     "test:scheduler:leader",
		InstanceID:    "node-2",
		LeaseTTL:      500 * time.Millisecond,
		RenewInterval: 50 * time.Millisecond,
		Logger:        discardLogger(),
	})
	elector2.Start(ctx)
	defer elector2.Release(context.Background())

	if !elector1.IsLeader() {
		t.Fatal("expected node-1 to be initial leader")
	}

	// Release node-1 cleanly
	if err := elector1.Release(ctx); err != nil {
		t.Fatalf("failed releasing node-1: %v", err)
	}

	// Wait for node-2 renewal loop to acquire
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if elector2.IsLeader() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if !elector2.IsLeader() {
		t.Fatal("expected node-2 to take over leadership after node-1 release")
	}

	val, err := rdb.Get(ctx, "test:scheduler:leader").Result()
	if err != nil || val != "node-2" {
		t.Fatalf("expected leader key to hold node-2, got %s (err: %v)", val, err)
	}
}

// TestLeaderElector_PreviousLeaderCannotReleaseNewLeadersLease verifies atomic compare-and-delete.
func TestLeaderElector_PreviousLeaderCannotReleaseNewLeadersLease(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ctx := context.Background()

	elector1 := NewLeaderElector(LeaderElectorConfig{
		Client:        rdb,
		LeaderKey:     "test:scheduler:leader",
		InstanceID:    "node-1",
		LeaseTTL:      2 * time.Second,
		RenewInterval: 200 * time.Millisecond,
		Logger:        discardLogger(),
	})
	elector1.Start(ctx)

	// Simulate node-1 losing leadership: key manually overwritten with node-2
	_ = rdb.Set(ctx, "test:scheduler:leader", "node-2", 2*time.Second).Err()

	// Old node-1 tries to call Release()
	err := elector1.Release(ctx)
	if err != nil {
		t.Fatalf("unexpected error from release: %v", err)
	}

	// Verify key STILL exists and belongs to node-2
	val, err := rdb.Get(ctx, "test:scheduler:leader").Result()
	if err != nil {
		t.Fatalf("failed to query leader key: %v", err)
	}
	if val != "node-2" {
		t.Fatalf("expected leader key to still hold node-2, got %s", val)
	}
}

// TestLeaderElector_RedisFailureStopsMaintenance verifies that connection loss stops maintenance immediately.
func TestLeaderElector_RedisFailureStopsMaintenance(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	elector := NewLeaderElector(LeaderElectorConfig{
		Client:        rdb,
		LeaderKey:     "test:scheduler:leader",
		InstanceID:    "node-1",
		LeaseTTL:      100 * time.Millisecond,
		RenewInterval: 20 * time.Millisecond,
		Logger:        discardLogger(),
	})
	elector.Start(ctx)

	if !elector.IsLeader() {
		t.Fatal("expected node-1 to be leader")
	}

	// Terminate Redis server
	mr.Close()
	rdb.Close()

	// Wait for lease to expire
	time.Sleep(150 * time.Millisecond)

	if elector.IsLeader() {
		t.Fatal("expected elector.IsLeader() to be false after Redis failure and lease expiration")
	}

	mockExec := &mockMaintenanceExecutor{}
	cfg := &config.Config{}
	_, executed := runMaintenanceTick(ctx, mockExec, elector, cfg, discardLogger(), 0)
	if executed {
		t.Fatal("expected maintenance tick to be skipped when Redis fails")
	}
	if mockExec.promoteCalls > 0 {
		t.Fatalf("expected 0 routine calls, got %d", mockExec.promoteCalls)
	}
}

// TestLeaderElector_CancelledSchedulerShutsDownCleanly verifies graceful shutdown.
func TestLeaderElector_CancelledSchedulerShutsDownCleanly(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ctx, cancel := context.WithCancel(context.Background())

	elector := NewLeaderElector(LeaderElectorConfig{
		Client:        rdb,
		LeaderKey:     "test:scheduler:leader",
		InstanceID:    "node-1",
		LeaseTTL:      2 * time.Second,
		RenewInterval: 200 * time.Millisecond,
		Logger:        discardLogger(),
	})
	elector.Start(ctx)

	if !elector.IsLeader() {
		t.Fatal("expected leader to be active")
	}

	// Trigger cancellation and release
	cancel()
	_ = elector.Release(context.Background())

	select {
	case <-elector.doneCh:
		// Clean exit
	case <-time.After(1 * time.Second):
		t.Fatal("elector did not stop within timeout")
	}

	exists := mr.Exists("test:scheduler:leader")
	if exists {
		t.Fatal("expected leader key to be deleted upon clean release")
	}
}

// TestLeaderElector_OnlyOneSchedulerExecutesMaintenance verifies mutual exclusion between replicas.
func TestLeaderElector_OnlyOneSchedulerExecutesMaintenance(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	elector1 := NewLeaderElector(LeaderElectorConfig{
		Client:        rdb,
		LeaderKey:     "test:scheduler:leader",
		InstanceID:    "node-1",
		LeaseTTL:      2 * time.Second,
		RenewInterval: 100 * time.Millisecond,
		Logger:        discardLogger(),
	})
	elector1.Start(ctx)
	defer elector1.Release(context.Background())

	elector2 := NewLeaderElector(LeaderElectorConfig{
		Client:        rdb,
		LeaderKey:     "test:scheduler:leader",
		InstanceID:    "node-2",
		LeaseTTL:      2 * time.Second,
		RenewInterval: 100 * time.Millisecond,
		Logger:        discardLogger(),
	})
	elector2.Start(ctx)
	defer elector2.Release(context.Background())

	sharedExec := &mockMaintenanceExecutor{}
	cfg := &config.Config{}

	// Run 5 ticks on both instances
	for i := 0; i < 5; i++ {
		runMaintenanceTick(ctx, sharedExec, elector1, cfg, discardLogger(), i)
		runMaintenanceTick(ctx, sharedExec, elector2, cfg, discardLogger(), i)
	}

	// Only node-1 should have executed routines (5 times, not 10)
	if sharedExec.promoteCalls != 5 {
		t.Fatalf("expected exactly 5 promote calls from leader, got %d", sharedExec.promoteCalls)
	}
	if sharedExec.reclaimCalls != 5 {
		t.Fatalf("expected exactly 5 reclaim calls from leader, got %d", sharedExec.reclaimCalls)
	}
}

// TestLeaderElector_MidTickLeadershipLossHaltsExecution verifies defense-in-depth during a slow tick.
func TestLeaderElector_MidTickLeadershipLossHaltsExecution(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ctx := context.Background()

	elector := NewLeaderElector(LeaderElectorConfig{
		Client:        rdb,
		LeaderKey:     "test:scheduler:leader",
		InstanceID:    "node-1",
		LeaseTTL:      2 * time.Second,
		RenewInterval: 200 * time.Millisecond,
		Logger:        discardLogger(),
	})
	elector.Start(ctx)

	mockExec := &mockMaintenanceExecutor{
		onPromote: func() {
			// Simulate leadership revocation mid-tick
			elector.mu.Lock()
			elector.isLeader = false
			elector.leaseExpiresAt = time.Time{}
			elector.mu.Unlock()
		},
	}

	cfg := &config.Config{}
	_, executed := runMaintenanceTick(ctx, mockExec, elector, cfg, discardLogger(), 0)
	if !executed {
		t.Fatal("expected tick to start")
	}

	// Promote ran once, but subsequent steps must have halted immediately!
	if mockExec.promoteCalls != 1 {
		t.Fatalf("expected 1 promote call, got %d", mockExec.promoteCalls)
	}
	if mockExec.reclaimCalls != 0 {
		t.Fatalf("expected 0 reclaim calls after mid-tick leadership loss, got %d", mockExec.reclaimCalls)
	}
	if mockExec.deferCalls != 0 {
		t.Fatalf("expected 0 defer calls after mid-tick leadership loss, got %d", mockExec.deferCalls)
	}
	if mockExec.recurCalls != 0 {
		t.Fatalf("expected 0 recur calls after mid-tick leadership loss, got %d", mockExec.recurCalls)
	}
}

// TestStandbyHealthEndpointsPreserved verifies readiness and liveness for standby instances.
func TestStandbyHealthEndpointsPreserved(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	checker := health.NewChecker("scheduler", health.AdaptRedis(rdb))

	reqLive := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recLive := httptest.NewRecorder()
	checker.Live(recLive, reqLive)
	if recLive.Code != http.StatusOK {
		t.Fatalf("expected 200 on /healthz, got %d", recLive.Code)
	}

	reqReady := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recReady := httptest.NewRecorder()
	checker.Ready(recReady, reqReady)
	if recReady.Code != http.StatusOK {
		t.Fatalf("expected 200 on /readyz, got %d", recReady.Code)
	}
}
