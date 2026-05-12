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

	migrateCmd := flag.NewFlagSet("migrate-jobs", flag.ExitOnError)
	from := migrateCmd.String("from", "redis", "Source backend")
	to := migrateCmd.String("to", "postgres", "Destination backend")
	batch := migrateCmd.Int("batch", 500, "Batch size for migration")

	if len(os.Args) < 2 {
		fmt.Println("expected 'migrate-jobs' subcommand")
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

		// 1. Connect to Redis
		rdb := redis.NewClient(&redis.Options{
			Addr:     cfg.RedisHost,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		})

		// 2. Connect to Postgres
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

			// HScan returns [key1, val1, key2, val2, ...]
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

	default:
		fmt.Println("unknown command:", os.Args[1])
		os.Exit(1)
	}
}
