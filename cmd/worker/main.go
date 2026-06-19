package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"task-queue-system/internal/config"
	"task-queue-system/internal/health"
	"task-queue-system/internal/logger"
	redisqueue "task-queue-system/internal/queue/redis"
	"task-queue-system/internal/service"
	"task-queue-system/internal/storage"
	"task-queue-system/internal/tracing"
	"task-queue-system/internal/webhooks"
	"task-queue-system/internal/worker/executor"
	_ "task-queue-system/internal/worker/plugins/standard" // Dynamic plugin auto-loading
	"task-queue-system/internal/worker/pool"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// cors wraps a handler with CORS headers so the UI (served on a different port) can
// reach the worker's circuit-breaker endpoints from the browser.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}


func main() {
	// ── 1. Setup structured logging ───────────────────────────────────────────
	log := logger.Setup()

	// ── 2. Load configuration ─────────────────────────────────────────────────
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	// Get a unique ID for this worker instance (container hostname or random UUID)
	instanceID, err := os.Hostname()
	if err != nil || instanceID == "" {
		instanceID = "worker-" + time.Now().Format("05.000") // simple fallback
	}

	log = log.With("instance_id", instanceID)
	log.Info("starting worker service", "workers", cfg.WorkerPoolSize)

	// ── 2b. Initialize OpenTelemetry ───────────────────────────────────────────
	otelShutdown, err := tracing.Init(context.Background(), cfg.OTELExporterOTLPEndpoint)
	if err != nil {
		log.Error("failed to init tracing", "error", err)
		os.Exit(1)
	}
	if cfg.OTELExporterOTLPEndpoint != "" {
		log.Info("opentelemetry tracing enabled", "endpoint", cfg.OTELExporterOTLPEndpoint)
	}
	defer func() {
		_ = otelShutdown(context.Background())
	}()

	// ── 3. Connect to Redis ───────────────────────────────────────────────────
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisHost,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
		PoolSize: 128,
	})

	// PING Redis to ensure the connection is alive
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	log.Info("connected to redis", "host", cfg.RedisHost)

	// ── 4. Initialise Queue and Store ─────────────────────────────────────────
	q := redisqueue.NewWithRateLimit(redisClient, "jobs", cfg.TenantRateLimit)

	// Initialise store based on configuration (Redis, Postgres, or Dual)
	store, err := storage.InitStore(ctx, cfg, redisClient)
	if err != nil {
		log.Error("failed to initialise storage", "error", err)
		os.Exit(1)
	}

	svc := service.New(q, store, log, 0) // Workers don't enforce ingestion backpressure

	// ── 5. Initialise Job Executor ──────────────────────────────────────────
	// The executor automatically picks up plugins registered via init() calls
	// in the imported packages.
	jobExec := executor.NewJobExecutor(log)

	// ── 6. Setup Worker Pool ──────────────────────────────────────────────────
	// Number of concurrent workers. We use 50 as a default for massive load testing.
	poolCfg := pool.Config{
		NumWorkers:    cfg.WorkerPoolSize,
		JobsPerSecond: cfg.JobRateLimit,
		SLATarget:     time.Duration(cfg.SLATargetSeconds) * time.Second,
	}

	workerPool, err := pool.New(poolCfg, instanceID, svc, jobExec, log)
	if err != nil {
		log.Error("failed to initialise worker pool", "error", err)
		os.Exit(1)
	}

	// ── 7. Reconcile Orphaned Jobs ──────────────────────────────────────────
	// Find jobs this specific instance was processing before a crash/restart.
	if count, err := svc.ReconcileOrphanedJobs(context.Background(), instanceID); err == nil && count > 0 {
		log.Info("orphaned jobs reconciled", "count", count)
	}

	// ── 8. Start Webhook Dispatcher ───────────────────────────────────────────
	webhookCtx, cancelWebhooks := context.WithCancel(context.Background())
	dispatcher := webhooks.NewDispatcher(redisClient, log)
	go dispatcher.Start(webhookCtx)

	// ── 9. Start Processing with Graceful Shutdown ────────────────────────────
	// Start is non-blocking. It spins up the goroutines.
	workerPool.Start(context.Background())

	shutdownCoordinator := newShutdownCoordinator(workerPool, time.Duration(cfg.DrainTimeoutSeconds)*time.Second, log)

	mux := http.NewServeMux()
	checker := health.NewChecker("worker", health.AdaptRedis(redisClient))
	mux.HandleFunc("/healthz", checker.Live)
	mux.HandleFunc("/readyz", checker.Ready)
	mux.HandleFunc("/healthz/shutdown", shutdownCoordinator.Handler)
	mux.Handle("/metrics", promhttp.Handler())

	cb := cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			status := jobExec.CircuitBreakerStatus()
			b, _ := json.Marshal(status)
			w.Write(b)
			return
		}
		if r.Method == http.MethodPost {
			jobType := r.PathValue("type")
			jobExec.ResetCircuitBreaker(jobType)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"status":"ok","type":%q}`, jobType)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	mux.Handle("GET /circuit-breaker", cb)
	mux.Handle("POST /circuit-breaker/reset/{type}", cb)
	mux.Handle("OPTIONS /circuit-breaker", cb)
	mux.Handle("OPTIONS /circuit-breaker/reset/{type}", cb)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.WorkerPort),
		Handler: mux,
	}

	go func() {
		log.Info("starting worker HTTP server", "port", cfg.WorkerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("worker HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutdown signal received, stopping workers...")
	shutdownCoordinator.Initiate()
	shutdownCoordinator.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)

	// Stop triggers the pool context cancellation, making workers exit cleanly
	// after finishing their current in-flight job, and blocks until they finish.
	// The pool has already been drained by shutdownCoordinator.Wait().
	cancelWebhooks()

	// Clean up connections
	_ = redisClient.Close()

	log.Info("worker service stopped")
}
