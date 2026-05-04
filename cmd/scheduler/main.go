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
	queue_redis "task-queue-system/internal/queue/redis"
)

func main() {
	// ── 1. Load Config ────────────────────────────────────────────────────────
	cfg := config.Load()

	// ── 2. Initialize Logger ──────────────────────────────────────────────────
	log := logger.Setup()
	log.Info("starting scheduler service")

	// ── 3. Connect to Redis ───────────────────────────────────────────────────
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisHost,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		log.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}

	// ── 4. Initialize Queue Backend ──────────────────────────────────────────
	q := queue_redis.New(redisClient, "")

	// ── 5. Setup Context & Shutdown ───────────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Info("received shutdown signal", "signal", sig)
		cancel()
	}()

	// ── 6. Run Promotion Loop ─────────────────────────────────────────────────
	ticker := time.NewTicker(1500 * time.Millisecond) // 1.5s interval
	defer ticker.Stop()

	log.Info("scheduler promotion loop active", "interval_ms", 1500)

	for {
		select {
		case <-ctx.Done():
			log.Info("scheduler shutting down gracefully")
			return
		case <-ticker.C:
			count, err := q.PromoteScheduledJobs(ctx)
			if err != nil {
				log.Error("promotion failed", "error", err)
				continue
			}

			if count > 0 {
				log.Info("scheduled jobs promoted", "count", count)
			}
		}
	}
}
