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
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
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
		fmt.Println("expected 'migrate-jobs', 'migrate-schema', 'migrate-down', or 'migrate-down-schema' subcommand")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "migrate-jobs":
		_ = migrateCmd.Parse(os.Args[2:])
		if *from != "redis" || *to != "postgres" {
			log.Error("currently only redis -> postgres migration is supported")
			os.Exit(1)
		}

		ctx := context.Background()

		rdb := redis.NewClient(&redis.Options{
			Addr:     cfg.RedisHost,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		})

		pgStore, err := postgres.New(ctx, cfg.PostgresConnStr)
		if err != nil {
			log.Error("failed to connect to postgres", "error", err)
			os.Exit(1)
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
				log.Error("failed to scan redis", "error", err)
				os.Exit(1)
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
			log.Error("POSTGRES_CONN_STR is required for schema migration")
			os.Exit(1)
		}

		ctx := context.Background()
		pool, err := postgres.CreatePool(ctx, cfg.PostgresConnStr)
		if err != nil {
			log.Error("failed to connect to postgres", "error", err)
			os.Exit(1)
		}
		defer pool.Close()

		if err := postgres.RunMigrations(ctx, pool, *schemaDir, log); err != nil {
			log.Error("schema migration failed", "error", err)
			os.Exit(1)
		}

		log.Info("schema migration completed successfully")

	case "migrate-down":
		log.Warn("data migration (redis -> postgres) is one-way and cannot be rolled back automatically")
		log.Warn("to revert, run the application with STORE_BACKEND=redis and keep Postgres as a standby")
		log.Info("no action taken")

	case "migrate-down-schema":
		_ = downSchemaCmd.Parse(os.Args[2:])

		if cfg.PostgresConnStr == "" {
			log.Error("POSTGRES_CONN_STR is required for schema rollback")
			os.Exit(1)
		}

		ctx := context.Background()
		if err := postgres.RollbackLastMigration(ctx, cfg.PostgresConnStr, *downDir, log); err != nil {
			log.Error("schema rollback failed", "error", err)
			os.Exit(1)
		}
		log.Info("schema rollback completed successfully")

	default:
		fmt.Println("unknown command:", os.Args[1])
		os.Exit(1)
	}
}
