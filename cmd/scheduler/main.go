package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"task-queue-system/internal/config"
	"task-queue-system/internal/health"
	"task-queue-system/internal/logger"
	queue_redis "task-queue-system/internal/queue/redis"
	"task-queue-system/internal/redisutil"
	"task-queue-system/internal/service"
	"task-queue-system/internal/storage"
	"task-queue-system/internal/tracing"
)

var (
	// renewLeaseScript atomically renews the lease TTL if the caller currently owns the key.
	renewLeaseScript = redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("PEXPIRE", KEYS[1], tonumber(ARGV[2]))
		else
			return 0
		end
	`)

	// releaseLeaseScript atomically releases the lease only if the caller currently owns the key.
	releaseLeaseScript = redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`)
)

// LeaderElectorConfig defines parameters for distributed scheduler leader election.
type LeaderElectorConfig struct {
	Client        redis.UniversalClient
	LeaderKey     string
	InstanceID    string
	LeaseTTL      time.Duration
	RenewInterval time.Duration
	Logger        *slog.Logger
}

// LeaderElector manages distributed leader election via a Redis lease.
type LeaderElector struct {
	cfg            LeaderElectorConfig
	mu             sync.RWMutex
	isLeader       bool
	leaseExpiresAt time.Time
	fencingToken   int64
	stopCh         chan struct{}
	doneCh         chan struct{}
}

// NewLeaderElector initializes a new LeaderElector.
func NewLeaderElector(cfg LeaderElectorConfig) *LeaderElector {
	if cfg.LeaderKey == "" {
		cfg.LeaderKey = "task_queue:scheduler:leader"
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = 5 * time.Second
	}
	if cfg.RenewInterval <= 0 {
		cfg.RenewInterval = cfg.LeaseTTL / 3
		if cfg.RenewInterval <= 0 {
			cfg.RenewInterval = 500 * time.Millisecond
		}
	}
	if cfg.InstanceID == "" {
		cfg.InstanceID = fmt.Sprintf("scheduler-%s-%d", uuid.New().String()[:8], os.Getpid())
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &LeaderElector{
		cfg:    cfg,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start begins leader election and lease renewal in a background goroutine.
func (le *LeaderElector) Start(ctx context.Context) {
	// Immediate initial acquisition attempt
	initCtx, cancel := context.WithTimeout(ctx, le.cfg.RenewInterval)
	le.tryAcquireOrRenew(initCtx)
	cancel()

	go le.runLoop(ctx)
}

func (le *LeaderElector) runLoop(ctx context.Context) {
	defer close(le.doneCh)

	ticker := time.NewTicker(le.cfg.RenewInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-le.stopCh:
			return
		case <-ticker.C:
			checkCtx, cancel := context.WithTimeout(ctx, le.cfg.RenewInterval)
			le.tryAcquireOrRenew(checkCtx)
			cancel()
		}
	}
}

func (le *LeaderElector) tryAcquireOrRenew(ctx context.Context) {
	select {
	case <-le.stopCh:
		return
	default:
	}

	le.mu.Lock()
	currentLeader := le.isLeader
	validUntil := le.leaseExpiresAt
	le.mu.Unlock()

	now := time.Now()

	// 1. If currently leader and lease has not yet expired, renew existing lease
	if currentLeader && now.Before(validUntil) {
		res, err := renewLeaseScript.Run(ctx, le.cfg.Client, []string{le.cfg.LeaderKey}, le.cfg.InstanceID, le.cfg.LeaseTTL.Milliseconds()).Int()
		if err == nil && res == 1 {
			le.mu.Lock()
			le.leaseExpiresAt = time.Now().Add(le.cfg.LeaseTTL)
			le.mu.Unlock()
			return
		}

		// Renewal failed or ownership lost
		le.mu.Lock()
		wasLeader := le.isLeader
		le.isLeader = false
		le.leaseExpiresAt = time.Time{}
		le.mu.Unlock()

		if wasLeader && le.cfg.Logger != nil {
			le.cfg.Logger.Warn("scheduler lost leader lease; transitioning to standby",
				"instance_id", le.cfg.InstanceID,
				"error", err,
			)
		}
		return
	}

	// 2. Not currently leader (or lease expired): attempt atomic lease acquisition
	acquired, err := le.cfg.Client.SetNX(ctx, le.cfg.LeaderKey, le.cfg.InstanceID, le.cfg.LeaseTTL).Result()
	if err != nil {
		le.mu.Lock()
		wasLeader := le.isLeader
		le.isLeader = false
		le.leaseExpiresAt = time.Time{}
		le.mu.Unlock()

		if wasLeader && le.cfg.Logger != nil {
			le.cfg.Logger.Warn("scheduler lost leader lease due to redis error; transitioning to standby",
				"instance_id", le.cfg.InstanceID,
				"error", err,
			)
		}
		return
	}

	le.mu.Lock()
	defer le.mu.Unlock()

	if acquired {
		if !le.isLeader {
			le.fencingToken++
			if le.cfg.Logger != nil {
				le.cfg.Logger.Info("scheduler acquired leader lease; transitioning to active leader",
					"instance_id", le.cfg.InstanceID,
					"epoch", le.fencingToken,
					"ttl", le.cfg.LeaseTTL,
				)
			}
		}
		le.isLeader = true
		le.leaseExpiresAt = time.Now().Add(le.cfg.LeaseTTL)
	} else {
		if le.isLeader {
			if le.cfg.Logger != nil {
				le.cfg.Logger.Warn("scheduler leader lease taken by another instance; transitioning to standby",
					"instance_id", le.cfg.InstanceID,
				)
			}
		}
		le.isLeader = false
		le.leaseExpiresAt = time.Time{}
	}
}

// IsLeader reports whether this instance is currently holding a valid, non-expired lease.
func (le *LeaderElector) IsLeader() bool {
	le.mu.RLock()
	defer le.mu.RUnlock()
	return le.isLeader && time.Now().Before(le.leaseExpiresAt)
}

// FencingToken returns the monotonic leadership generation epoch for this instance.
func (le *LeaderElector) FencingToken() int64 {
	le.mu.RLock()
	defer le.mu.RUnlock()
	return le.fencingToken
}

// InstanceID returns the unique identifier of this scheduler instance.
func (le *LeaderElector) InstanceID() string {
	return le.cfg.InstanceID
}

// Release relinquishes leadership safely using an atomic compare-and-delete.
// It stops the elector loop and guarantees another instance's lease is never deleted.
func (le *LeaderElector) Release(ctx context.Context) error {
	le.mu.Lock()
	wasLeader := le.isLeader
	le.isLeader = false
	le.leaseExpiresAt = time.Time{}
	le.mu.Unlock()

	select {
	case <-le.stopCh:
	default:
		close(le.stopCh)
	}

	if !wasLeader {
		return nil
	}

	res, err := releaseLeaseScript.Run(ctx, le.cfg.Client, []string{le.cfg.LeaderKey}, le.cfg.InstanceID).Int()
	if err != nil {
		if le.cfg.Logger != nil {
			le.cfg.Logger.Warn("failed to release leader lease in redis",
				"instance_id", le.cfg.InstanceID,
				"error", err,
			)
		}
		return err
	}
	if res == 1 && le.cfg.Logger != nil {
		le.cfg.Logger.Info("scheduler released leader lease", "instance_id", le.cfg.InstanceID)
	}
	return nil
}

// MaintenanceExecutor abstracts the underlying queue and service routines protected by leadership.
type MaintenanceExecutor interface {
	PromoteScheduledJobs(ctx context.Context) (int, error)
	ReclaimTimedOutJobs(ctx context.Context) (int, error)
	ReconcileDeferredJobs(ctx context.Context) (int, error)
	RecurringJobsDue(ctx context.Context) (int, error)
	ArchiveOldJobs(ctx context.Context, age time.Duration) (int64, error)
}

type defaultMaintenanceExecutor struct {
	q   *queue_redis.RedisQueue
	svc *service.JobService
}

func (d *defaultMaintenanceExecutor) PromoteScheduledJobs(ctx context.Context) (int, error) {
	return d.q.PromoteScheduledJobs(ctx)
}

func (d *defaultMaintenanceExecutor) ReclaimTimedOutJobs(ctx context.Context) (int, error) {
	return d.q.ReclaimTimedOutJobs(ctx)
}

func (d *defaultMaintenanceExecutor) ReconcileDeferredJobs(ctx context.Context) (int, error) {
	return d.svc.ReconcileDeferredJobs(ctx)
}

func (d *defaultMaintenanceExecutor) RecurringJobsDue(ctx context.Context) (int, error) {
	return d.svc.RecurringJobsDue(ctx)
}

func (d *defaultMaintenanceExecutor) ArchiveOldJobs(ctx context.Context, age time.Duration) (int64, error) {
	return d.svc.ArchiveOldJobs(ctx, age)
}

// runMaintenanceTick executes the 5 maintenance routines only if the elector holds active leadership.
// It verifies leadership before and between routines, returning true if routines were executed.
func runMaintenanceTick(ctx context.Context, exec MaintenanceExecutor, elector *LeaderElector, cfg *config.Config, log *slog.Logger, tickCount int) (int, bool) {
	if !elector.IsLeader() {
		return tickCount, false
	}

	// 1. Promote scheduled jobs
	count, err := exec.PromoteScheduledJobs(ctx)
	if err != nil {
		log.Error("promotion failed", "error", err)
	} else if count > 0 {
		log.Info("scheduled jobs promoted", "count", count)
	}

	if !elector.IsLeader() {
		log.Warn("scheduler lost leadership during maintenance tick; halting remaining routines")
		return tickCount, true
	}

	// 2. Reclaim timed-out jobs
	reclaimed, err := exec.ReclaimTimedOutJobs(ctx)
	if err != nil {
		log.Error("reclamation failed", "error", err)
	} else if reclaimed > 0 {
		log.Info("stalled jobs reclaimed", "count", reclaimed)
	}

	if !elector.IsLeader() {
		log.Warn("scheduler lost leadership during maintenance tick; halting remaining routines")
		return tickCount, true
	}

	// 3. Reconcile deferred jobs (concurrency limit freed up)
	reconciled, err := exec.ReconcileDeferredJobs(ctx)
	if err != nil {
		log.Error("deferral reconciliation failed", "error", err)
	} else if reconciled > 0 {
		log.Info("deferred jobs returned to main queue", "count", reconciled)
	}

	if !elector.IsLeader() {
		log.Warn("scheduler lost leadership during maintenance tick; halting remaining routines")
		return tickCount, true
	}

	// 4. Process recurring jobs
	recurring, err := exec.RecurringJobsDue(ctx)
	if err != nil {
		log.Error("recurring job check failed", "error", err)
	} else if recurring > 0 {
		log.Info("recurring job instances created", "count", recurring)
	}

	if !elector.IsLeader() {
		log.Warn("scheduler lost leadership during maintenance tick; halting remaining routines")
		return tickCount, true
	}

	// 5. Job Archiving (run every ~1 hour / 2400 ticks)
	tickCount++
	if cfg.ArchiveAgeDays > 0 && tickCount%2400 == 0 {
		archived, err := exec.ArchiveOldJobs(ctx, time.Duration(cfg.ArchiveAgeDays)*24*time.Hour)
		if err != nil {
			log.Error("job archiving failed", "error", err)
		} else if archived > 0 {
			log.Info("old jobs archived/purged", "count", archived, "age_days", cfg.ArchiveAgeDays)
		}
	}

	return tickCount, true
}

func main() {
	if err := run(); err != nil {
		logger.Setup().Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// ── 1. Load Config ────────────────────────────────────────────────────────
	cfg := config.Load()

	// ── 2. Initialize Logger ──────────────────────────────────────────────────
	log := logger.Setup()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	log.Info("starting scheduler service")

	// ── 2b. Initialize OpenTelemetry ───────────────────────────────────────────
	otelShutdown, err := tracing.Init(context.Background(), cfg.OTELExporterOTLPEndpoint)
	if err != nil {
		return fmt.Errorf("failed to init tracing: %w", err)
	}
	if cfg.OTELExporterOTLPEndpoint != "" {
		log.Info("opentelemetry tracing enabled", "endpoint", cfg.OTELExporterOTLPEndpoint)
	}
	defer func() {
		_ = otelShutdown(context.Background())
	}()

	// ── 3. Connect to Redis ───────────────────────────────────────────────────
	redisClient := redisutil.NewClient(cfg)

	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("failed to connect to redis: %w", err)
	}

	// ── 4. Initialize Queue and Store ────────────────────────────────────────
	q := queue_redis.New(redisClient, "")

	schedulerCtx, schedulerCancel := context.WithCancel(context.Background())
	defer schedulerCancel()

	store, err := storage.InitStore(schedulerCtx, cfg, redisClient)
	if err != nil {
		return fmt.Errorf("failed to initialise storage: %w", err)
	}
	svc := service.New(q, store, log, cfg.MaxQueueSize)

	// ── 5. Start Health/Metrics HTTP Server ───────────────────────────────────
	checker := health.NewChecker("scheduler", health.AdaptRedis(redisClient))
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", checker.Live)
	mux.HandleFunc("/readyz", checker.Ready)
	mux.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.SchedulerPort),
		Handler: mux,
	}

	go func() {
		log.Info("starting scheduler HTTP server", "port", cfg.SchedulerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("scheduler HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	// ── 6. Setup Context & Shutdown ───────────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Info("received shutdown signal", "signal", sig)
		cancel()
	}()

	// ── 6b. Start Distributed Leader Election ─────────────────────────────────
	instanceID := fmt.Sprintf("scheduler-%s-%d", uuid.New().String()[:8], os.Getpid())
	elector := NewLeaderElector(LeaderElectorConfig{
		Client:        redisClient,
		LeaderKey:     "task_queue:scheduler:leader",
		InstanceID:    instanceID,
		LeaseTTL:      5 * time.Second,
		RenewInterval: 1500 * time.Millisecond,
		Logger:        log,
	})
	elector.Start(ctx)
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		_ = elector.Release(shutdownCtx)
	}()

	// ── 7. Run Maintenance Loop ───────────────────────────────────────────────
	ticker := time.NewTicker(1500 * time.Millisecond) // 1.5s interval
	defer ticker.Stop()

	log.Info("scheduler maintenance loop active", "interval_ms", 1500, "instance_id", instanceID)

	exec := &defaultMaintenanceExecutor{q: q, svc: svc}
	tickCount := 0

	for {
		select {
		case <-ctx.Done():
			log.Info("scheduler shutting down gracefully")
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = elector.Release(shutdownCtx)
			_ = srv.Shutdown(shutdownCtx)
			_ = redisClient.Close()
			return nil
		case <-ticker.C:
			tickCount, _ = runMaintenanceTick(ctx, exec, elector, cfg, log, tickCount)
		}
	}
}
