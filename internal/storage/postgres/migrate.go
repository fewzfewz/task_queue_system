package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ensureSchemaMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	return err
}

func appliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	versions := make(map[string]bool)
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
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
	return versions, nil
}

func RunMigrations(ctx context.Context, pool *pgxpool.Pool, dir string, log *slog.Logger) error {
	if err := ensureSchemaMigrationsTable(ctx, pool); err != nil {
		return fmt.Errorf("migrate: create schema_migrations table: %w", err)
	}

	applied, err := appliedVersions(ctx, pool)
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

		tx, err := pool.Begin(ctx)
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

	if err := ensureSchemaMigrationsTable(ctx, pool); err != nil {
		return fmt.Errorf("rollback: create schema_migrations table: %w", err)
	}

	var lastVersion string
	err = pool.QueryRow(ctx, `SELECT version FROM schema_migrations ORDER BY applied_at DESC LIMIT 1`).Scan(&lastVersion)
	if err != nil {
		if err == pgx.ErrNoRows {
			if log != nil {
				log.Info("no migrations applied; nothing to roll back")
			}
			return nil
		}
		return err
	}

	rollbackFile := filepath.Join(dir, lastVersion+"_rollback.sql")
	if _, err := os.Stat(rollbackFile); os.IsNotExist(err) {
		return fmt.Errorf("rollback file not found: %s", rollbackFile)
	}

	sqlBytes, err := os.ReadFile(rollbackFile)
	if err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
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
		return err
	}

	if log != nil {
		log.Info("rolled back migration", "version", lastVersion)
	}
	return nil
}
