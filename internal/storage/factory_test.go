package storage

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"task-queue-system/internal/config"
)

func TestInitStoreRedis(t *testing.T) {
	cfg := &config.Config{
		StoreBackend:  "redis",
		ApiKey:        "test-key",
		ServerPort:    "8080",
		RedisHost:     "localhost:6379",
		LogLevel:      "info",
	}

	store, err := InitStore(context.Background(), cfg, &redis.Client{})
	if err != nil {
		t.Fatalf("InitStore(redis) failed: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestInitStoreUnknown(t *testing.T) {
	cfg := &config.Config{
		StoreBackend: "nonexistent",
	}

	_, err := InitStore(context.Background(), cfg, &redis.Client{})
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}

func TestInitStorePostgresFailsWithoutConnStr(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()

	cfg := &config.Config{
		StoreBackend:    "postgres",
		PostgresConnStr: "",
	}

	_, err := InitStore(ctx, cfg, &redis.Client{})
	if err == nil {
		t.Fatal("expected error for missing postgres conn string")
	}
}

func TestInitStoreDualFailsWithoutConnStr(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()

	cfg := &config.Config{
		StoreBackend:    "dual",
		PostgresConnStr: "",
	}

	_, err := InitStore(ctx, cfg, &redis.Client{})
	if err == nil {
		t.Fatal("expected error for missing postgres conn string in dual mode")
	}
}
