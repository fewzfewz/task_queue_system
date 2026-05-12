package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"task-queue-system/internal/config"
	"task-queue-system/internal/logger"
	_ "task-queue-system/internal/worker/plugins/standard" // Dynamic plugin auto-loading
	redisqueue "task-queue-system/internal/queue/redis"
	"task-queue-system/internal/service"
	"task-queue-system/internal/storage/models"
	"task-queue-system/internal/worker/executor"
	"task-queue-system/internal/worker/pool"
)

func main() {
	// ── 1. Setup structured logging ───────────────────────────────────────────
	log := logger.Setup()

	// ── 2. Load configuration ─────────────────────────────────────────────────
	cfg := config.Load()

	// Get a unique ID for this worker instance (container hostname or random UUID)
	instanceID, err := os.Hostname()
	if err != nil || instanceID == "" {
		instanceID = "worker-" + time.Now().Format("05.000") // simple fallback
	}

	log = log.With("instance_id", instanceID)
	log.Info("starting worker service", "workers", 50)

	// ── 3. Connect to Redis ───────────────────────────────────────────────────
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisHost,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	// PING Redis to ensure the connection is alive
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	log.Info("connected to redis", "host", cfg.RedisHost)

	// ── 4. Initialise Queue, Store and Service ────────────────────────────────
	// Both API and Worker use the same queue name and store backend.
	q := redisqueue.New(redisClient, "jobs")
	store := models.NewRedisStore(redisClient)
	svc := service.New(q, store, log, 0) // Workers don't enforce ingestion backpressure

	// ── 5. Initialise Job Executor ──────────────────────────────────────────
	// The executor automatically picks up plugins registered via init() calls
	// in the imported packages.
	jobExec := executor.NewJobExecutor(log)

	// ── 6. Setup Worker Pool ──────────────────────────────────────────────────
	// Number of concurrent workers. We use 50 as a default for massive load testing.
	poolCfg := pool.Config{
		NumWorkers:    50,
		JobsPerSecond: cfg.JobRateLimit,
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

	// ── 8. Start Processing with Graceful Shutdown ────────────────────────────
	// Start is non-blocking. It spins up the goroutines.
	workerPool.Start(context.Background())

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutdown signal received, stopping workers...")

	// Stop triggers the pool context cancellation, making workers exit cleanly
	// after finishing their current in-flight job, and blocks until they finish.
	workerPool.Stop()

	// Clean up connections
	_ = redisClient.Close()

	log.Info("worker service stopped")
}
