package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupMigrationDirs(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	files := map[string]string{
		"001_create_users.sql":           "CREATE TABLE IF NOT EXISTS test_users (id TEXT PRIMARY KEY, name TEXT);",
		"002_add_email.sql":              "ALTER TABLE test_users ADD COLUMN IF NOT EXISTS email TEXT;",
		"002_add_email_rollback.sql":     "ALTER TABLE test_users DROP COLUMN IF EXISTS email;",
		"003_create_orders.sql":          "CREATE TABLE IF NOT EXISTS test_orders (id TEXT PRIMARY KEY, user_id TEXT);",
		"003_create_orders_rollback.sql": "DROP TABLE IF EXISTS test_orders;",
		"README.txt":                     "not a migration",
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

func cleanTestDB(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `DROP TABLE IF EXISTS jobs, clients, jobs_archive, dead_letter_jobs, audit_logs, test_orders, test_users, schema_migrations CASCADE`)
	if err != nil {
		t.Fatalf("cleanTestDB failed: %v", err)
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

func TestRunMigrations(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	cleanTestDB(t, ctx, pool)

	dir := setupMigrationDirs(t)

	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	var exists bool
	err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'test_users')`).Scan(&exists)
	if err != nil || !exists {
		t.Fatalf("test_users table not created: %v", err)
	}

	err = pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'test_orders')`).Scan(&exists)
	if err != nil || !exists {
		t.Fatalf("test_orders table not created: %v", err)
	}

	var count int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	if err != nil || count != 3 {
		t.Fatalf("expected 3 migrations, got %d (err: %v)", count, err)
	}
}

func TestRunMigrationsIdempotent(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	cleanTestDB(t, ctx, pool)

	dir := setupMigrationDirs(t)

	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("first run failed: %v", err)
	}

	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("second run (idempotent) failed: %v", err)
	}

	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	if err != nil || count != 3 {
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
	cleanTestDB(t, ctx, pool)

	dir := setupMigrationDirs(t)

	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	if err := RollbackLastMigration(ctx, connStr, dir, nil); err != nil {
		t.Fatalf("RollbackLastMigration failed: %v", err)
	}

	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	if err != nil || count != 2 {
		t.Fatalf("expected 2 migrations after rollback, got %d", count)
	}

	var exists bool
	err = pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'test_orders')`).Scan(&exists)
	if err != nil || exists {
		t.Fatalf("test_orders table should have been dropped by rollback (exists: %v, err: %v)", exists, err)
	}
}

func TestRollbackLastMigrationNoMigrations(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	cleanTestDB(t, ctx, pool)

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
	cleanTestDB(t, ctx, pool)

	if err := ensureSchemaMigrationsTable(ctx, pool); err != nil {
		t.Fatalf("ensureSchemaMigrationsTable failed: %v", err)
	}

	if err := ensureSchemaMigrationsTable(ctx, pool); err != nil {
		t.Fatalf("ensureSchemaMigrationsTable (idempotent) failed: %v", err)
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

	content001, err := os.ReadFile(filepath.Join(dir, "001_create_jobs.sql"))
	if err != nil {
		t.Fatalf("failed to read 001_create_jobs.sql: %v", err)
	}
	if !strings.Contains(string(content001), "idx_jobs_pending_metrics") {
		t.Fatal("001_create_jobs.sql is missing idx_jobs_pending_metrics index")
	}
}

func TestFullMigrationCycle_Up_Down_Up(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR to run live Postgres migration test")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	cleanTestDB(t, ctx, pool)

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

// ---------------------------------------------------------------------------
// Phase 2 Comprehensive Test Matrix Scenarios (13 Scenarios)
// ---------------------------------------------------------------------------

// Scenario 1: TestMigration_FreshInstall
func TestMigration_FreshInstall(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	cleanTestDB(t, ctx, pool)

	dir := resolveRepoMigrationsDir()
	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("RunMigrations failed on fresh install: %v", err)
	}

	for _, table := range []string{"jobs", "clients", "jobs_archive", "dead_letter_jobs", "audit_logs", "schema_migrations"} {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("table %s was not created on fresh install", table)
		}
	}

	if err := checkIndexExists(ctx, pool, "jobs", "idx_jobs_pending_metrics", false, "status = 'pending'"); err != nil {
		t.Fatalf("expected idx_jobs_pending_metrics index: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil || count != 10 {
		t.Fatalf("expected exactly 10 canonical rows in ledger, got %d", count)
	}
}

// Scenario 2: TestMigration_LegacyFullUpgrade
func TestMigration_LegacyFullUpgrade(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	cleanTestDB(t, ctx, pool)

	dir := resolveRepoMigrationsDir()

	// Step 1: Initial migration to create valid physical schema
	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("initial migration failed: %v", err)
	}

	// Step 2: Seed legacy version keys into schema_migrations
	legacyVersions := map[string]string{
		"003_create_clients":          "002_create_clients",
		"004_add_progress":            "003_add_progress",
		"005_add_cron_expr":           "004_add_cron_expr",
		"006_create_jobs_archive":     "005_create_jobs_archive",
		"007_dedup_unique":            "006_dedup_unique",
		"008_add_expires_at":          "007_add_expires_at",
		"009_create_dead_letter_jobs": "008_create_dead_letter_jobs",
		"010_create_audit_logs":       "009_create_audit_logs",
	}
	for canonical, legacy := range legacyVersions {
		if _, err := pool.Exec(ctx, `UPDATE schema_migrations SET version = $1 WHERE version = $2`, legacy, canonical); err != nil {
			t.Fatalf("failed seeding legacy version %s: %v", legacy, err)
		}
	}

	// Step 3: Run RunMigrations to trigger pre-flight reconciliation
	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("RunMigrations failed during legacy full upgrade: %v", err)
	}

	// Step 4: Verify ledger is fully reconciled to canonical names
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil || count != 10 {
		t.Fatalf("expected exactly 10 canonical migrations, got %d", count)
	}

	for canonical := range legacyVersions {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version=$1)`, canonical).Scan(&exists); err != nil || !exists {
			t.Fatalf("canonical version %s missing after reconciliation", canonical)
		}
	}

	// Step 5: Verify sequential rollback works cleanly down to 0
	for i := 0; i < 10; i++ {
		if err := RollbackLastMigration(ctx, connStr, dir, nil); err != nil {
			t.Fatalf("rollback step %d failed after legacy reconciliation: %v", i+1, err)
		}
	}
}

// Scenario 3: TestMigration_LegacyPartialUpgrade
func TestMigration_LegacyPartialUpgrade(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	cleanTestDB(t, ctx, pool)

	dir := resolveRepoMigrationsDir()

	// Apply all migrations first to establish initial state
	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("initial migration failed: %v", err)
	}

	// Roll back 5 migrations so only first 5 exist
	for i := 0; i < 5; i++ {
		if err := RollbackLastMigration(ctx, connStr, dir, nil); err != nil {
			t.Fatalf("rollback failed: %v", err)
		}
	}

	// Transform remaining applied migrations to legacy names
	_, _ = pool.Exec(ctx, `UPDATE schema_migrations SET version = '002_create_clients' WHERE version = '003_create_clients'`)
	_, _ = pool.Exec(ctx, `UPDATE schema_migrations SET version = '003_add_progress' WHERE version = '004_add_progress'`)
	_, _ = pool.Exec(ctx, `UPDATE schema_migrations SET version = '004_add_cron_expr' WHERE version = '005_add_cron_expr'`)

	// Re-run RunMigrations: should reconcile 002-004 and apply 006-010
	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("partial upgrade failed: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil || count != 10 {
		t.Fatalf("expected exactly 10 migrations after partial upgrade, got %d", count)
	}
}

// Scenario 4: TestMigration_DuplicatedLedgerRepair
func TestMigration_DuplicatedLedgerRepair(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	cleanTestDB(t, ctx, pool)

	dir := resolveRepoMigrationsDir()
	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("initial migration failed: %v", err)
	}

	// Insert duplicate legacy entries for all 8 mappings
	for _, m := range legacyMigrationMappings {
		_, err := pool.Exec(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES ($1, NOW() - INTERVAL '1 hour')`, m.Legacy)
		if err != nil {
			t.Fatalf("failed inserting duplicate %s: %v", m.Legacy, err)
		}
	}

	var preCount int
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&preCount)
	if preCount != 18 {
		t.Fatalf("expected 18 duplicate rows before repair, got %d", preCount)
	}

	// Run migration to trigger duplicate repair
	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("RunMigrations failed during duplicate repair: %v", err)
	}

	var postCount int
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&postCount)
	if postCount != 10 {
		t.Fatalf("expected 10 canonical rows after duplicate repair, got %d", postCount)
	}
}

// Scenario 5: TestMigration_DirectLegacyRollback
func TestMigration_DirectLegacyRollback(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	cleanTestDB(t, ctx, pool)

	dir := resolveRepoMigrationsDir()
	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("initial migration failed: %v", err)
	}

	// Change top migration to its legacy name: 010_create_audit_logs -> 009_create_audit_logs
	_, err := pool.Exec(ctx, `UPDATE schema_migrations SET version = '009_create_audit_logs' WHERE version = '010_create_audit_logs'`)
	if err != nil {
		t.Fatalf("failed setting top legacy version: %v", err)
	}

	// Direct rollback must succeed via fallback map
	if err := RollbackLastMigration(ctx, connStr, dir, nil); err != nil {
		t.Fatalf("direct legacy rollback failed: %v", err)
	}

	var count int
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	if count != 9 {
		t.Fatalf("expected 9 migrations after rollback, got %d", count)
	}
}

// Scenario 6: TestMigration_InconsistentDuplicateLedger_StopsSafely
func TestMigration_InconsistentDuplicateLedger_StopsSafely(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	cleanTestDB(t, ctx, pool)

	dir := resolveRepoMigrationsDir()
	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("initial migration failed: %v", err)
	}

	// Insert legacy duplicate
	_, _ = pool.Exec(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES ('002_create_clients', NOW() - INTERVAL '1 hour')`)

	// Break the physical schema: drop api_key_hash column
	_, err := pool.Exec(ctx, `ALTER TABLE clients DROP COLUMN api_key_hash`)
	if err != nil {
		t.Fatalf("failed altering clients table: %v", err)
	}

	// Run migration: must fail schema validation and stop safely
	err = RunMigrations(ctx, pool, dir, nil)
	if err == nil {
		t.Fatal("expected RunMigrations to fail due to schema inconsistency, but it succeeded")
	}
	if !strings.Contains(err.Error(), "schema validation failed") {
		t.Fatalf("expected error mentioning schema validation, got: %v", err)
	}

	// Ledger must remain untouched: both rows still exist
	var hasLegacy, hasCanonical bool
	_ = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version='002_create_clients')`).Scan(&hasLegacy)
	_ = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version='003_create_clients')`).Scan(&hasCanonical)
	if !hasLegacy || !hasCanonical {
		t.Fatal("ledger records were modified despite schema validation failure")
	}
}

// Scenario 7: TestMigration_ConflictingTimestamps_PreservesEarliest
func TestMigration_ConflictingTimestamps_PreservesEarliest(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	cleanTestDB(t, ctx, pool)

	dir := resolveRepoMigrationsDir()
	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("initial migration failed: %v", err)
	}

	earlierTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	laterTime := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Set canonical to later time
	_, _ = pool.Exec(ctx, `UPDATE schema_migrations SET applied_at = $1 WHERE version = '003_create_clients'`, laterTime)

	// Insert legacy with earlier time
	_, _ = pool.Exec(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES ('002_create_clients', $1)`, earlierTime)

	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	var preservedTime time.Time
	err := pool.QueryRow(ctx, `SELECT applied_at FROM schema_migrations WHERE version = '003_create_clients'`).Scan(&preservedTime)
	if err != nil {
		t.Fatalf("failed reading preserved timestamp: %v", err)
	}

	if !preservedTime.Equal(earlierTime) {
		t.Fatalf("expected preserved timestamp %v, got %v", earlierTime, preservedTime)
	}
}

// Scenario 8: TestMigration_ConcurrentRunners_AdvisoryLockSerialization
func TestMigration_ConcurrentRunners_AdvisoryLockSerialization(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	cleanTestDB(t, ctx, pool)

	dir := resolveRepoMigrationsDir()

	const concurrency = 2
	var wg sync.WaitGroup
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runnerPool := connectPool(t, connStr)
			if err := RunMigrations(ctx, runnerPool, dir, nil); err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent migration failed: %v", err)
		}
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil || count != 10 {
		t.Fatalf("expected exactly 10 migrations after concurrent execution, got %d", count)
	}
}

// Scenario 9: TestMigration_FailedMigration_LedgerConsistent
func TestMigration_FailedMigration_LedgerConsistent(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	cleanTestDB(t, ctx, pool)

	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "001_good.sql"), []byte("CREATE TABLE IF NOT EXISTS test_users (id TEXT PRIMARY KEY);"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "002_broken.sql"), []byte("MALFORMED SQL STATEMENT;"), 0644)

	err := RunMigrations(ctx, pool, dir, nil)
	if err == nil {
		t.Fatal("expected failure on malformed migration, got nil")
	}

	var count int
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	if count != 1 {
		t.Fatalf("expected ledger to contain only migration 001, got %d", count)
	}

	var version string
	_ = pool.QueryRow(ctx, `SELECT version FROM schema_migrations LIMIT 1`).Scan(&version)
	if version != "001_good" {
		t.Fatalf("expected version 001_good, got %s", version)
	}
}

// Scenario 10: TestMigration_FailedRollback_SchemaAndLedgerAtomic
func TestMigration_FailedRollback_SchemaAndLedgerAtomic(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	cleanTestDB(t, ctx, pool)

	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "001_good.sql"), []byte("CREATE TABLE IF NOT EXISTS test_users (id TEXT PRIMARY KEY);"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "001_good_rollback.sql"), []byte("DROP TABLE IF EXISTS test_users;"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "002_step.sql"), []byte("ALTER TABLE test_users ADD COLUMN val TEXT;"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "002_step_rollback.sql"), []byte("INVALID ROLLBACK SYNTAX;"), 0644)

	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	err := RollbackLastMigration(ctx, connStr, dir, nil)
	if err == nil {
		t.Fatal("expected rollback to fail on invalid SQL")
	}

	// Verify ledger still contains 002_step (transaction rolled back)
	var count int
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	if count != 2 {
		t.Fatalf("expected ledger to retain both migrations after failed rollback, got %d", count)
	}
}

// Scenario 11: TestMigration_UnknownMigrationVersion_FailsGracefully
func TestMigration_UnknownMigrationVersion_FailsGracefully(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	cleanTestDB(t, ctx, pool)

	_ = ensureSchemaMigrationsTable(ctx, pool)
	_, _ = pool.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ('999_unknown_feature')`)

	dir := resolveRepoMigrationsDir()
	err := RollbackLastMigration(ctx, connStr, dir, nil)
	if err == nil {
		t.Fatal("expected error on unknown migration version rollback")
	}

	// Verify ledger unchanged
	var exists bool
	_ = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version='999_unknown_feature')`).Scan(&exists)
	if !exists {
		t.Fatal("unknown version was unexpectedly deleted from ledger")
	}
}

// Scenario 12: TestMigration_Rollback_TimestampTieBreaker_Deterministic
func TestMigration_Rollback_TimestampTieBreaker_Deterministic(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)
	cleanTestDB(t, ctx, pool)

	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "001_step.sql"), []byte("CREATE TABLE IF NOT EXISTS test_t1 (id TEXT PRIMARY KEY);"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "001_step_rollback.sql"), []byte("DROP TABLE IF EXISTS test_t1;"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "002_step.sql"), []byte("CREATE TABLE IF NOT EXISTS test_t2 (id TEXT PRIMARY KEY);"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "002_step_rollback.sql"), []byte("DROP TABLE IF EXISTS test_t2;"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "003_step.sql"), []byte("CREATE TABLE IF NOT EXISTS test_t3 (id TEXT PRIMARY KEY);"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "003_step_rollback.sql"), []byte("DROP TABLE IF EXISTS test_t3;"), 0644)

	if err := RunMigrations(ctx, pool, dir, nil); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	// Force identical sub-second timestamps
	sharedTime := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	_, _ = pool.Exec(ctx, `UPDATE schema_migrations SET applied_at = $1`, sharedTime)

	// Rollback 1: must pick 003_step due to version DESC tie-breaker
	if err := RollbackLastMigration(ctx, connStr, dir, nil); err != nil {
		t.Fatalf("first rollback failed: %v", err)
	}
	var v string
	_ = pool.QueryRow(ctx, `SELECT version FROM schema_migrations ORDER BY applied_at DESC, version DESC LIMIT 1`).Scan(&v)
	if v != "002_step" {
		t.Fatalf("expected top version to be 002_step after rolling back 003, got %s", v)
	}

	// Rollback 2: must pick 002_step
	if err := RollbackLastMigration(ctx, connStr, dir, nil); err != nil {
		t.Fatalf("second rollback failed: %v", err)
	}
	_ = pool.QueryRow(ctx, `SELECT version FROM schema_migrations ORDER BY applied_at DESC, version DESC LIMIT 1`).Scan(&v)
	if v != "001_step" {
		t.Fatalf("expected top version to be 001_step after rolling back 002, got %s", v)
	}
}

// Scenario 13: TestMigration_CommitError_CallerPoolRemainsUsable
func TestMigration_CommitError_CallerPoolRemainsUsable(t *testing.T) {
	connStr := os.Getenv("POSTGRES_CONN_STR")
	if connStr == "" {
		t.Skip("skipping; set POSTGRES_CONN_STR")
	}

	ctx := context.Background()
	pool := connectPool(t, connStr)

	// Acquire connection and simulate connection destruction
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("failed acquiring connection: %v", err)
	}

	destroyConn(conn)

	// Caller's pool must remain completely usable
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("caller pool unusable after connection destruction: %v", err)
	}

	var res int
	if err := pool.QueryRow(ctx, `SELECT 1`).Scan(&res); err != nil || res != 1 {
		t.Fatalf("caller pool query failed: %v", err)
	}
}
