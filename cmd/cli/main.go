package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"task-queue-system/internal/config"
	"task-queue-system/internal/jobs"
	"task-queue-system/internal/logger"
	"task-queue-system/internal/storage/postgres"
)

func main() {
	log := logger.Setup()
	if err := run(); err != nil {
		log.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	migrateCmd := flag.NewFlagSet("migrate-jobs", flag.ExitOnError)
	from := migrateCmd.String("from", "redis", "Source backend")
	to := migrateCmd.String("to", "postgres", "Destination backend")
	batch := migrateCmd.Int("batch", 500, "Batch size for migration")

	schemaCmd := flag.NewFlagSet("migrate-schema", flag.ExitOnError)
	schemaDir := schemaCmd.String("dir", "db/migrations", "Directory containing versioned SQL migrations")

	downSchemaCmd := flag.NewFlagSet("migrate-down-schema", flag.ExitOnError)
	downDir := downSchemaCmd.String("dir", "db/migrations", "Directory containing versioned SQL migrations")

	if len(os.Args) < 2 {
		return fmt.Errorf("expected 'migrate-jobs', 'migrate-schema', 'migrate-down', or 'migrate-down-schema' subcommand")
	}

	log := logger.Setup()

	switch os.Args[1] {
	case "migrate-jobs":
		_ = migrateCmd.Parse(os.Args[2:])
		if *from != "redis" || *to != "postgres" {
			return fmt.Errorf("currently only redis -> postgres migration is supported")
		}

		ctx := context.Background()

		rdb := redis.NewClient(&redis.Options{
			Addr:     cfg.RedisHost,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		})

		pgStore, err := postgres.New(ctx, cfg.PostgresConnStr)
		if err != nil {
			return fmt.Errorf("failed to connect to postgres: %w", err)
		}
		defer pgStore.Close()

		log.Info("starting data migration", "from", *from, "to", *to, "batch_size", *batch)

		const jobStoreKey = "task_queue:store:jobs"
		var cursor uint64
		totalMigrated := 0
		start := time.Now()

		for {
			keys, nextCursor, err := rdb.HScan(ctx, jobStoreKey, cursor, "", int64(*batch)).Result()
			if err != nil {
				return fmt.Errorf("failed to scan redis: %w", err)
			}

			for i := 1; i < len(keys); i += 2 {
				val := keys[i]
				var job jobs.Job
				if err := json.Unmarshal([]byte(val), &job); err != nil {
					log.Warn("failed to unmarshal job, skipping", "val", val)
					continue
				}

				if err := pgStore.Save(ctx, &job); err != nil {
					log.Error("failed to migrate job", "id", job.ID, "error", err)
					continue
				}
				totalMigrated++
			}

			log.Info("progress report", "migrated", totalMigrated, "cursor", nextCursor)

			cursor = nextCursor
			if cursor == 0 {
				break
			}
		}

		log.Info("migration completed successfully",
			"total", totalMigrated,
			"duration", time.Since(start).String())

	case "migrate-schema":
		_ = schemaCmd.Parse(os.Args[2:])

		if cfg.PostgresConnStr == "" {
			return fmt.Errorf("POSTGRES_CONN_STR is required for schema migration")
		}

		ctx := context.Background()
		pool, err := postgres.CreatePool(ctx, cfg.PostgresConnStr)
		if err != nil {
			return fmt.Errorf("failed to connect to postgres: %w", err)
		}
		defer pool.Close()

		if err := postgres.RunMigrations(ctx, pool, *schemaDir, log); err != nil {
			return fmt.Errorf("schema migration failed: %w", err)
		}

		log.Info("schema migration completed successfully")

	case "migrate-down":
		log.Warn("data migration (redis -> postgres) is one-way and cannot be rolled back automatically")
		log.Warn("to revert, run the application with STORE_BACKEND=redis and keep Postgres as a standby")
		log.Info("no action taken")

	case "migrate-down-schema":
		_ = downSchemaCmd.Parse(os.Args[2:])

		if cfg.PostgresConnStr == "" {
			return fmt.Errorf("POSTGRES_CONN_STR is required for schema rollback")
		}

		ctx := context.Background()
		if err := postgres.RollbackLastMigration(ctx, cfg.PostgresConnStr, *downDir, log); err != nil {
			return fmt.Errorf("schema rollback failed: %w", err)
		}
		log.Info("schema rollback completed successfully")

	default:
		return fmt.Errorf("unknown command: %s", os.Args[1])
	}

	return nil
}
