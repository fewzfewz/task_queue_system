package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"log/slog"

	"github.com/jackc/pgx/v5"
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
	migrationsDir := schemaCmd.String("dir", "db/migrations", "Directory containing versioned SQL migrations")

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

	case "migrate-schema":
		_ = schemaCmd.Parse(os.Args[2:])

		if cfg.PostgresConnStr == "" {
			log.Error("POSTGRES_CONN_STR is required for schema migration")
			os.Exit(1)
		}

		ctx := context.Background()
		pgStore, err := postgres.New(ctx, cfg.PostgresConnStr)
		if err != nil {
			log.Error("failed to connect to postgres", "error", err)
			os.Exit(1)
		}
		defer pgStore.Close()

		if err := applyMigrations(ctx, cfg.PostgresConnStr, *migrationsDir, log); err != nil {
			log.Error("schema migration failed", "error", err)
			os.Exit(1)
		}
		log.Info("schema migration completed successfully")

	default:
		fmt.Println("unknown command:", os.Args[1])
		os.Exit(1)
	}
}

func applyMigrations(ctx context.Context, connStr, dir string, log *slog.Logger) error {
	pool, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return err
	}
	defer pool.Close(ctx)

	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".sql") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		version := strings.TrimSuffix(filepath.Base(name), ".sql")
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&exists); err != nil {
			return err
		}
		if exists {
			log.Info("migration already applied", "version", version)
			continue
		}

		sqlBytes, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s failed: %w", version, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES ($1)`, version); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		log.Info("applied migration", "version", version)
	}

	return nil
}
