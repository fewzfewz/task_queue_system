package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"task-queue-system/internal/api/routes"
	"task-queue-system/internal/config"
	redisqueue "task-queue-system/internal/queue/redis"
	"task-queue-system/internal/storage/models"
)

func main() {
	// ── 1. Setup structured logging ───────────────────────────────────────────
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// ── 2. Load configuration ─────────────────────────────────────────────────
	cfg := config.Load()
	logger.Info("starting api service", "port", cfg.ServerPort)

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

	// ── 4. Initialise Queue and Store ─────────────────────────────────────────
	// Using "jobs" as the default queue name.
	q := redisqueue.New(redisClient, "jobs")

	// Using the in-memory store for now.
	// In production, you would drop in postgres.New(dbPool) here.
	store := models.NewInMemoryStore()

	// ── 5. Setup HTTP Server & Routes ─────────────────────────────────────────
	router := routes.NewRouter(q, store, logger)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.ServerPort),
		Handler: router,
		// Sane timeouts for a production server
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// ── 6. Start Server with Graceful Shutdown ────────────────────────────────
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down HTTP server...")

	// The context is used to inform the server it has 10 seconds to finish
	// the request it is currently handling
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server forced to shutdown", "error", err)
	}

	// Also cleanly close the redis connection
	_ = redisClient.Close()

	logger.Info("API service stopped")
}
