package storage

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"task-queue-system/internal/config"
	"task-queue-system/internal/storage/models"
	"task-queue-system/internal/storage/postgres"
)

// InitStore creates the requested storage backend based on configuration.
func InitStore(ctx context.Context, cfg *config.Config, rdb *redis.Client) (models.Store, error) {
	redisStore := models.NewRedisStore(rdb)

	switch cfg.StoreBackend {
	case "redis":
		return redisStore, nil

	case "postgres":
		pgStore, err := postgres.New(ctx, cfg.PostgresConnStr)
		if err != nil {
			return nil, fmt.Errorf("factory: failed to init postgres: %w", err)
		}
		return pgStore, nil

	case "dual":
		pgStore, err := postgres.New(ctx, cfg.PostgresConnStr)
		if err != nil {
			return nil, fmt.Errorf("factory: failed to init postgres for dual: %w", err)
		}
		return models.NewDualStore(pgStore, redisStore), nil

	default:
		return nil, fmt.Errorf("factory: unknown store backend: %s", cfg.StoreBackend)
	}
}
