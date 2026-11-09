package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupMigrationDirs(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	files := map[string]string{
		"001_create_users.sql":        "CREATE TABLE IF NOT EXISTS test_users (id TEXT PRIMARY KEY, name TEXT);",
		"002_add_email.sql":           "ALTER TABLE test_users ADD COLUMN IF NOT EXISTS email TEXT;",
		"002_add_email_rollback.sql":  "ALTER TABLE test_users DROP COLUMN IF EXISTS email;",
		"003_create_orders.sql":       "CREATE TABLE IF NOT EXISTS test_orders (id TEXT PRIMARY KEY, user_id TEXT);",
		"003_create_orders_rollback.sql": "DROP TABLE IF EXISTS test_orders;",
		"README.txt":                  "not a migration",
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	return dir
}

func connectPool(t *testing.T, connStr string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestRunMigrations(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)

	// Clean up any previous test artifacts
	_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS test_orders, test_users, schema_migrations CASCADE`)

	dir := setupMigrationDirs(t)

	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	// Verify tables exist
	var exists bool
	err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'test_users')`).Scan(&exists)
	if err != nil {
		t.Fatalf("check test_users: %v", err)
	}
	if !exists {
		t.Fatal("test_users table not created")
	}

	err = pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'test_orders')`).Scan(&exists)
	if err != nil {
		t.Fatalf("check test_orders: %v", err)
	}
	if !exists {
		t.Fatal("test_orders table not created")
	}

	// Verify migrations table
	err = pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'schema_migrations')`).Scan(&exists)
	if err != nil {
		t.Fatalf("check schema_migrations: %v", err)
	}
	if !exists {
		t.Fatal("schema_migrations table not created")
	}

	// Verify migration versions recorded
	var count int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	if err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 migrations, got %d", count)
	}
}

func TestRunMigrationsIdempotent(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS test_orders, test_users, schema_migrations CASCADE`)

	dir := setupMigrationDirs(t)

	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("first run failed: %v", err)
	}

	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("second run (idempotent) failed: %v", err)
	}

	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	if err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 migrations after idempotent run, got %d", count)
	}
}

func TestRollbackLastMigration(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS test_orders, test_users, schema_migrations CASCADE`)

	dir := setupMigrationDirs(t)

	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	if err := RollbackLastMigration(ctx, connStr, dir, nil); err != nil {
		t.Fatalf("RollbackLastMigration failed: %v", err)
	}

	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	if err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 migrations after rollback, got %d", count)
	}

	var exists bool
	err = pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'test_orders')`).Scan(&exists)
	if err != nil {
		t.Fatalf("check test_orders: %v", err)
	}
	if exists {
		t.Fatal("test_orders table should have been dropped by rollback")
	}
}

func TestRollbackLastMigrationNoMigrations(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS schema_migrations CASCADE`)

	dir := setupMigrationDirs(t)

	if err := RollbackLastMigration(ctx, connStr, dir, nil); err != nil {
		t.Fatalf("expected no error when no migrations applied, got: %v", err)
	}
}

func TestEnsureSchemaMigrationsTable(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS schema_migrations CASCADE`)

	if err := ensureSchemaMigrationsTable(ctx, pool); err != nil {
		t.Fatalf("ensureSchemaMigrationsTable failed: %v", err)
	}

	if err := ensureSchemaMigrationsTable(ctx, pool); err != nil {
		t.Fatalf("ensureSchemaMigrationsTable (idempotent) failed: %v", err)
	}
}

func resolveRepoMigrationsDir() string {
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

func TestMigrationsDirectoryIntegrity(t *testing.T) {
	dir := resolveRepoMigrationsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read migrations directory %s: %v", dir, err)
	}

	var upFiles []string
	rollbackMap := make(map[string]bool)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		if strings.HasSuffix(e.Name(), "_rollback.sql") {
			rollbackMap[e.Name()] = true
			info, err := e.Info()
			if err != nil || info.Size() == 0 {
				t.Fatalf("rollback file %s is empty or inaccessible", e.Name())
			}
		} else {
			upFiles = append(upFiles, e.Name())
			info, err := e.Info()
			if err != nil || info.Size() == 0 {
				t.Fatalf("migration file %s is empty or inaccessible", e.Name())
			}
		}
	}

	if len(upFiles) == 0 {
		t.Fatal("no migration files found in migrations directory")
	}

	sort.Strings(upFiles)

	for i, name := range upFiles {
		expectedPrefix := fmt.Sprintf("%03d_", i+1)
		if !strings.HasPrefix(name, expectedPrefix) {
			t.Fatalf("migration %s does not match sequential prefix %s", name, expectedPrefix)
		}

		baseVersion := strings.TrimSuffix(name, ".sql")
		expectedRollback := baseVersion + "_rollback.sql"
		if !rollbackMap[expectedRollback] {
			t.Fatalf("migration %s is missing its required rollback file %s", name, expectedRollback)
		}
	}
}

func TestFullMigrationCycle_Up_Down_Up(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR to run live Postgres migration test")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)

	// Clean all tables before test
	_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS jobs, clients, jobs_archive, dead_letter_jobs, audit_logs, schema_migrations CASCADE`)

	dir := resolveRepoMigrationsDir()

	// 1. Migrate UP to latest
	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("RunMigrations (UP) failed: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("failed to query schema_migrations: %v", err)
	}
	if count != 10 {
		t.Fatalf("expected 10 applied migrations, got %d", count)
	}

	// 2. Rollback DOWN to zero
	for i := 0; i < 10; i++ {
		if err := RollbackLastMigration(ctx, connStr, dir, nil); err != nil {
			t.Fatalf("RollbackLastMigration step %d failed: %v", i+1, err)
		}
	}

	// Verify schema_migrations is empty
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("failed to query schema_migrations after rollback: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 applied migrations after full rollback, got %d", count)
	}

	// 3. Migrate back UP to latest
	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("RunMigrations (re-apply UP) failed: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("failed to query schema_migrations after re-apply: %v", err)
	}
	if count != 10 {
		t.Fatalf("expected 10 applied migrations after re-apply, got %d", count)
	}
}
