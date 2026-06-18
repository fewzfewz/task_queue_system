package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"task-queue-system/internal/config"
	"task-queue-system/internal/health"
	"task-queue-system/internal/logger"
	queue_redis "task-queue-system/internal/queue/redis"
	"task-queue-system/internal/tracing"
)

func main() {
	// ── 1. Load Config ────────────────────────────────────────────────────────
	cfg := config.Load()

	// ── 2. Initialize Logger ──────────────────────────────────────────────────
	log := logger.Setup()
	if err := cfg.Validate(); err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	log.Info("starting scheduler service")

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
	})

	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		log.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}

	// ── 4. Initialize Queue Backend ──────────────────────────────────────────
	q := queue_redis.New(redisClient, "")

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

	// ── 7. Run Promotion Loop ─────────────────────────────────────────────────
	ticker := time.NewTicker(1500 * time.Millisecond) // 1.5s interval
	defer ticker.Stop()

	log.Info("scheduler maintenance loop active", "interval_ms", 1500)

	for {
		select {
		case <-ctx.Done():
			log.Info("scheduler shutting down gracefully")
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = srv.Shutdown(shutdownCtx)
			_ = redisClient.Close()
			return
		case <-ticker.C:
			// 1. Promote scheduled jobs
			count, err := q.PromoteScheduledJobs(ctx)
			if err != nil {
				log.Error("promotion failed", "error", err)
			} else if count > 0 {
				log.Info("scheduled jobs promoted", "count", count)
			}

			// 2. Reclaim timed-out jobs
			reclaimed, err := q.ReclaimTimedOutJobs(ctx)
			if err != nil {
				log.Error("reclamation failed", "error", err)
			} else if reclaimed > 0 {
				log.Info("stalled jobs reclaimed", "count", reclaimed)
			}
		}
	}
}
