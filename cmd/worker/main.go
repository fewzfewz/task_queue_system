package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"task-queue-system/internal/config"
	redisqueue "task-queue-system/internal/queue/redis"
	"task-queue-system/internal/worker/executor"
	"task-queue-system/internal/worker/pool"
)

func main() {
	// ── 1. Setup structured logging ───────────────────────────────────────────
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// ── 2. Load configuration ─────────────────────────────────────────────────
	cfg := config.Load()
	logger.Info("starting worker service", "workers", 5) // Hardcoded 5 for now, could be in config.

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
		logger.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	logger.Info("connected to redis", "host", cfg.RedisHost)

	// ── 4. Initialise Queue and Executors ─────────────────────────────────────
	// Using "jobs" as the default queue name to match the API.
	q := redisqueue.New(redisClient, "jobs")

	// JobExecutor pre-registers the "email" and "image" handlers.
	jobExec := executor.NewJobExecutor(logger)

	// ── 5. Setup Worker Pool ──────────────────────────────────────────────────
	// Number of concurrent workers. We use 5 as a default.
	poolCfg := pool.Config{NumWorkers: 5}

	workerPool, err := pool.New(poolCfg, q, jobExec, logger)
	if err != nil {
		logger.Error("failed to initialise worker pool", "error", err)
		os.Exit(1)
	}

	// ── 6. Start Processing with Graceful Shutdown ────────────────────────────
	// Start is non-blocking. It spins up the goroutines.
	workerPool.Start(context.Background())

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutdown signal received, stopping workers...")

	// Stop triggers the pool context cancellation, making workers exit cleanly
	// after finishing their current in-flight job, and blocks until they finish.
	workerPool.Stop()

	// Clean up connections
	_ = redisClient.Close()

	logger.Info("worker service stopped")
}
