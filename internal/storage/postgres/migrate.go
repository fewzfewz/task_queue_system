package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MigrationAdvisoryLockID is a deterministic 64-bit advisory lock key derived from hash of "task_queue_migrations".
const MigrationAdvisoryLockID = int64(0x7461736b71756575) // 8386121408848782709

type dbExecQuerier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type legacyMapping struct {
	Legacy    string
	Canonical string
}

var legacyMigrationMappings = []legacyMapping{
	{Legacy: "002_create_clients", Canonical: "003_create_clients"},
	{Legacy: "003_add_progress", Canonical: "004_add_progress"},
	{Legacy: "004_add_cron_expr", Canonical: "005_add_cron_expr"},
	{Legacy: "005_create_jobs_archive", Canonical: "006_create_jobs_archive"},
	{Legacy: "006_dedup_unique", Canonical: "007_dedup_unique"},
	{Legacy: "007_add_expires_at", Canonical: "008_add_expires_at"},
	{Legacy: "008_create_dead_letter_jobs", Canonical: "009_create_dead_letter_jobs"},
	{Legacy: "009_create_audit_logs", Canonical: "010_create_audit_logs"},
}

var legacyToCanonicalRollback = map[string]string{
	"002_create_clients":          "003_create_clients",
	"003_add_progress":            "004_add_progress",
	"004_add_cron_expr":           "005_add_cron_expr",
	"005_create_jobs_archive":     "006_create_jobs_archive",
	"006_dedup_unique":            "007_dedup_unique",
	"007_add_expires_at":          "008_add_expires_at",
	"008_create_dead_letter_jobs": "009_create_dead_letter_jobs",
	"009_create_audit_logs":       "010_create_audit_logs",
}

func destroyConn(conn *pgxpool.Conn) {
	if conn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = conn.Conn().Close(ctx)
	conn.Release()
}

func checkTableExists(ctx context.Context, q dbExecQuerier, table string) error {
	var exists bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_name = $1
		)
	`, table).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check table %s: %w", table, err)
	}
	if !exists {
		return fmt.Errorf("table %s does not exist", table)
	}
	return nil
}

func checkColumnExists(ctx context.Context, q dbExecQuerier, table, column, expectedType string) error {
	var dataType string
	err := q.QueryRow(ctx, `
		SELECT data_type FROM information_schema.columns 
		WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
	`, table, column).Scan(&dataType)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("column %s on table %s does not exist", column, table)
		}
		return fmt.Errorf("check column %s on table %s: %w", column, table, err)
	}
	if expectedType != "" && !strings.EqualFold(dataType, expectedType) {
		return fmt.Errorf("column %s on table %s has type %s, expected %s", column, table, dataType, expectedType)
	}
	return nil
}

func checkIndexExists(ctx context.Context, q dbExecQuerier, table, index string, requireUnique bool, predicateSubstrs ...string) error {
	var isUnique bool
	var pred string
	err := q.QueryRow(ctx, `
		SELECT i.indisunique, COALESCE(pg_get_expr(i.indpred, i.indrelid), '') AS predicate
		FROM pg_index i
		JOIN pg_class c ON c.oid = i.indexrelid
		JOIN pg_class t ON t.oid = i.indrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		WHERE n.nspname = 'public' AND t.relname = $1 AND c.relname = $2
	`, table, index).Scan(&isUnique, &pred)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("index %s on table %s does not exist", index, table)
		}
		return fmt.Errorf("check index %s on table %s: %w", index, table, err)
	}
	if requireUnique && !isUnique {
		return fmt.Errorf("index %s on table %s is not unique", index, table)
	}
	for _, sub := range predicateSubstrs {
		if !strings.Contains(pred, sub) {
			return fmt.Errorf("index %s on table %s predicate %q does not contain %q", index, table, pred, sub)
		}
	}
	return nil
}

type schemaValidatorFunc func(ctx context.Context, q dbExecQuerier) error

var migrationValidators = map[string]schemaValidatorFunc{
	"003_create_clients": func(ctx context.Context, q dbExecQuerier) error {
		if err := checkTableExists(ctx, q, "clients"); err != nil {
			return err
		}
		for col, typ := range map[string]string{
			"id":           "uuid",
			"tenant_id":    "character varying",
			"api_key_hash": "character varying",
			"created_at":   "timestamp with time zone",
		} {
			if err := checkColumnExists(ctx, q, "clients", col, typ); err != nil {
				return err
			}
		}
		return checkIndexExists(ctx, q, "clients", "idx_clients_api_key_hash", false)
	},
	"004_add_progress": func(ctx context.Context, q dbExecQuerier) error {
		return checkColumnExists(ctx, q, "jobs", "progress", "double precision")
	},
	"005_add_cron_expr": func(ctx context.Context, q dbExecQuerier) error {
		return checkColumnExists(ctx, q, "jobs", "cron_expr", "text")
	},
	"006_create_jobs_archive": func(ctx context.Context, q dbExecQuerier) error {
		if err := checkTableExists(ctx, q, "jobs_archive"); err != nil {
			return err
		}
		for col, typ := range map[string]string{
			"id":              "text",
			"tenant_id":       "text",
			"type":            "text",
			"payload":         "jsonb",
			"status":          "text",
			"priority":        "text",
			"attempts":        "integer",
			"max_attempts":    "integer",
			"timeout_seconds": "integer",
			"version":         "integer",
			"cron_expr":       "text",
		} {
			if err := checkColumnExists(ctx, q, "jobs_archive", col, typ); err != nil {
				return err
			}
		}
		if err := checkIndexExists(ctx, q, "jobs_archive", "idx_jobs_archive_tenant_status", false); err != nil {
			return err
		}
		return checkIndexExists(ctx, q, "jobs_archive", "idx_jobs_archive_worker_status", false)
	},
	"007_dedup_unique": func(ctx context.Context, q dbExecQuerier) error {
		return checkIndexExists(ctx, q, "jobs", "idx_jobs_dedup_key_unique", true, "dedup_key IS NOT NULL")
	},
	"008_add_expires_at": func(ctx context.Context, q dbExecQuerier) error {
		if err := checkColumnExists(ctx, q, "jobs", "expires_at", "timestamp with time zone"); err != nil {
			return err
		}
		return checkIndexExists(ctx, q, "jobs", "idx_jobs_expires_at", false, "expires_at IS NOT NULL")
	},
	"009_create_dead_letter_jobs": func(ctx context.Context, q dbExecQuerier) error {
		if err := checkTableExists(ctx, q, "dead_letter_jobs"); err != nil {
			return err
		}
		for col, typ := range map[string]string{
			"id":                 "text",
			"type":               "text",
			"payload":            "jsonb",
			"labels":             "jsonb",
			"priority":           "text",
			"status":             "text",
			"retries":            "integer",
			"max_retries":        "integer",
			"created_at":         "timestamp with time zone",
			"updated_at":         "timestamp with time zone",
			"run_at":             "timestamp with time zone",
			"tenant_id":          "text",
			"dead_letter_reason": "text",
			"error_history":      "jsonb",
		} {
			if err := checkColumnExists(ctx, q, "dead_letter_jobs", col, typ); err != nil {
				return err
			}
		}
		if err := checkIndexExists(ctx, q, "dead_letter_jobs", "idx_dlq_tenant", false); err != nil {
			return err
		}
		return checkIndexExists(ctx, q, "dead_letter_jobs", "idx_dlq_created", false)
	},
	"010_create_audit_logs": func(ctx context.Context, q dbExecQuerier) error {
		if err := checkTableExists(ctx, q, "audit_logs"); err != nil {
			return err
		}
		for col, typ := range map[string]string{
			"id":          "uuid",
			"timestamp":   "timestamp with time zone",
			"actor":       "character varying",
			"action":      "character varying",
			"target_type": "character varying",
			"target_id":   "character varying",
			"details":     "jsonb",
		} {
			if err := checkColumnExists(ctx, q, "audit_logs", col, typ); err != nil {
				return err
			}
		}
		if err := checkIndexExists(ctx, q, "audit_logs", "idx_audit_logs_timestamp", false); err != nil {
			return err
		}
		return checkIndexExists(ctx, q, "audit_logs", "idx_audit_logs_actor", false)
	},
}

func ensureSchemaMigrationsTable(ctx context.Context, q dbExecQuerier) error {
	_, err := q.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	return err
}

func appliedVersions(ctx context.Context, q dbExecQuerier) (map[string]bool, error) {
	versions := make(map[string]bool)
	rows, err := q.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		versions[v] = true
	}
	return versions, rows.Err()
}

func reconcileLegacyMigrationLedger(ctx context.Context, conn *pgxpool.Conn, log *slog.Logger) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin reconciliation tx: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `SELECT version, applied_at FROM schema_migrations FOR UPDATE`)
	if err != nil {
		return fmt.Errorf("query migrations for update: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]time.Time)
	for rows.Next() {
		var v string
		var t time.Time
		if err := rows.Scan(&v, &t); err != nil {
			return fmt.Errorf("scan migration row: %w", err)
		}
		applied[v] = t
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, pair := range legacyMigrationMappings {
		legacyTime, hasLegacy := applied[pair.Legacy]
		canonicalTime, hasCanonical := applied[pair.Canonical]

		if !hasLegacy {
			continue
		}

		validator, ok := migrationValidators[pair.Canonical]
		if !ok {
			return fmt.Errorf("missing validator for %s", pair.Canonical)
		}
		if err := validator(ctx, tx); err != nil {
			return fmt.Errorf("reconciliation blocked: schema validation failed for %s (legacy %s): %w", pair.Canonical, pair.Legacy, err)
		}

		if hasLegacy && !hasCanonical {
			// Case A: Sole legacy record -> Rename in-place, preserving exact timestamp
			if _, err := tx.Exec(ctx, `UPDATE schema_migrations SET version = $1 WHERE version = $2`, pair.Canonical, pair.Legacy); err != nil {
				return fmt.Errorf("rename legacy migration %s to %s: %w", pair.Legacy, pair.Canonical, err)
			}
			if log != nil {
				log.Info("reconciled legacy migration by rename", "legacy", pair.Legacy, "canonical", pair.Canonical, "applied_at", legacyTime)
			}
		} else if hasLegacy && hasCanonical {
			// Case B: Both records present -> Preserve earliest timestamp, delete legacy duplicate
			earliest := legacyTime
			if canonicalTime.Before(earliest) {
				earliest = canonicalTime
			}

			if _, err := tx.Exec(ctx, `UPDATE schema_migrations SET applied_at = $1 WHERE version = $2`, earliest, pair.Canonical); err != nil {
				return fmt.Errorf("update canonical migration timestamp for %s: %w", pair.Canonical, err)
			}
			if _, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, pair.Legacy); err != nil {
				return fmt.Errorf("delete legacy migration duplicate %s: %w", pair.Legacy, err)
			}
			if log != nil {
				log.Info("reconciled duplicate migration ledger", "canonical", pair.Canonical, "purged_legacy", pair.Legacy, "preserved_applied_at", earliest)
			}
		}
	}

	return tx.Commit(ctx)
}

func RunMigrations(ctx context.Context, pool *pgxpool.Pool, dir string, log *slog.Logger) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("migrate: acquire connection: %w", err)
	}

	var lockAcquired bool
	var cleanedUp bool
	cleanup := func(destroy bool) {
		if cleanedUp {
			return
		}
		cleanedUp = true
		if destroy {
			destroyConn(conn)
			return
		}
		if lockAcquired {
			unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, unlockErr := conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", MigrationAdvisoryLockID); unlockErr != nil {
				if log != nil {
					log.Error("migrate: failed to release advisory lock; destroying connection", "error", unlockErr)
				}
				destroyConn(conn)
				return
			}
		}
		conn.Release()
	}
	defer func() { cleanup(false) }()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", MigrationAdvisoryLockID); err != nil {
		cleanup(true)
		return fmt.Errorf("migrate: acquire advisory lock: %w", err)
	}
	lockAcquired = true

	if err := ensureSchemaMigrationsTable(ctx, conn); err != nil {
		return fmt.Errorf("migrate: create schema_migrations table: %w", err)
	}

	if err := reconcileLegacyMigrationLedger(ctx, conn, log); err != nil {
		return fmt.Errorf("migrate: ledger reconciliation failed: %w", err)
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return fmt.Errorf("migrate: read applied versions: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("migrate: read dir %s: %w", dir, err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".sql") && !strings.HasSuffix(entry.Name(), "_rollback.sql") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		version := strings.TrimSuffix(filepath.Base(name), ".sql")
		if applied[version] {
			continue
		}

		sqlBytes, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("migrate: read %s: %w", name, err)
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("migrate: begin tx for %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migrate: apply %s failed: %w", version, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES ($1)`, version); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			cleanup(true)
			return fmt.Errorf("migrate: commit %s: %w", version, err)
		}
		if log != nil {
			log.Info("applied migration", "version", version)
		}
	}

	return nil
}

func RollbackLastMigration(ctx context.Context, connStr, dir string, log *slog.Logger) error {
	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("rollback: acquire connection: %w", err)
	}

	var lockAcquired bool
	var cleanedUp bool
	cleanup := func(destroy bool) {
		if cleanedUp {
			return
		}
		cleanedUp = true
		if destroy {
			destroyConn(conn)
			return
		}
		if lockAcquired {
			unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, unlockErr := conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", MigrationAdvisoryLockID); unlockErr != nil {
				if log != nil {
					log.Error("rollback: failed to release advisory lock; destroying connection", "error", unlockErr)
				}
				destroyConn(conn)
				return
			}
		}
		conn.Release()
	}
	defer func() { cleanup(false) }()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", MigrationAdvisoryLockID); err != nil {
		cleanup(true)
		return fmt.Errorf("rollback: acquire advisory lock: %w", err)
	}
	lockAcquired = true

	if err := ensureSchemaMigrationsTable(ctx, conn); err != nil {
		return fmt.Errorf("rollback: create schema_migrations table: %w", err)
	}

	if err := reconcileLegacyMigrationLedger(ctx, conn, log); err != nil {
		return fmt.Errorf("rollback: ledger reconciliation failed: %w", err)
	}

	var lastVersion string
	err = conn.QueryRow(ctx, `SELECT version FROM schema_migrations ORDER BY applied_at DESC, version DESC LIMIT 1`).Scan(&lastVersion)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if log != nil {
				log.Info("no migrations applied; nothing to roll back")
			}
			return nil
		}
		return err
	}

	rollbackFile := filepath.Join(dir, lastVersion+"_rollback.sql")
	if _, err := os.Stat(rollbackFile); os.IsNotExist(err) {
		if canonical, ok := legacyToCanonicalRollback[lastVersion]; ok {
			candidate := filepath.Join(dir, canonical+"_rollback.sql")
			if _, statErr := os.Stat(candidate); statErr == nil {
				rollbackFile = candidate
			}
		}
	}

	if _, err := os.Stat(rollbackFile); os.IsNotExist(err) {
		return fmt.Errorf("rollback file not found: %s", rollbackFile)
	}

	sqlBytes, err := os.ReadFile(rollbackFile)
	if err != nil {
		return err
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("rollback %s failed: %w", lastVersion, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, lastVersion); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		cleanup(true)
		return err
	}

	if log != nil {
		log.Info("rolled back migration", "version", lastVersion)
	}
	return nil
}
