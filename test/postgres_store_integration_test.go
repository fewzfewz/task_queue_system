package test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"task-queue-system/internal/jobs"
	"task-queue-system/internal/storage/postgres"
)

// resolveMigrationsDir returns an absolute path to db/migrations, searching
// upward from the test's working directory. `go test` runs each package from
// its own source directory, so a relative "db/migrations" would not resolve.
func resolveMigrationsDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "db/migrations"
	}
	for {
		candidate := filepath.Join(dir, "db", "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "db/migrations"
		}
		dir = parent
	}
}

func newPostgresStore(t *testing.T) (*postgres.PostgresStore, context.Context) {
	t.Helper()
	conn := os.Getenv("POSTGRES_CONN_STR")
	if conn == "" {
		t.Skip("POSTGRES_CONN_STR not set, skipping Postgres integration test")
	}
	if os.Getenv("POSTGRES_MIGRATIONS_DIR") == "" {
		_ = os.Setenv("POSTGRES_MIGRATIONS_DIR", resolveMigrationsDir())
	}
	ctx := context.Background()
	// Ensure the jobs table reflects the latest migration even if it was
	// created by an older schema in a previous run against this database.
	pool, err := pgxpool.New(ctx, conn)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS jobs, schema_migrations CASCADE`)
	pool.Close()
	store, err := postgres.New(ctx, conn)
	if err != nil {
		t.Skipf("Postgres not available: %v", err)
	}
	return store, ctx
}

// TestPostgresStoreIntegrationWorkflow exercises the basic save/enqueue/get/
// list workflow end to end. Every individual Store method has a dedicated,
// stricter test in postgres_store_test.go.
func TestPostgresStoreIntegrationWorkflow(t *testing.T) {
	store, ctx := newPostgresStore(t)
	defer store.Close()

	job := jobs.NewJob("test-success", map[string]interface{}{"hello": "world"}, nil, jobs.PriorityMedium, 3, time.Time{}, "", 30, 1, "tenant-int")
	if err := store.Save(ctx, job); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if err := store.Enqueue(ctx, job); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	got, err := store.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("get by id failed: %v", err)
	}
	if got == nil || got.ID != job.ID {
		t.Fatalf("expected job %s, got %#v", job.ID, got)
	}

	list, err := store.ListJobs(ctx, "tenant-int", "", "test-success", 10, 0)
	if err != nil {
		t.Fatalf("list jobs failed: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("expected at least one job in list")
	}
}
