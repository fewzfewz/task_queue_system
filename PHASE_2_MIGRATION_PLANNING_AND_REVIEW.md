# Phase 2 — Database Migration Compatibility Architecture & Controlled Approval Gate (Revision 2.2)

**Document:** `PHASE_2_MIGRATION_PLANNING_AND_REVIEW.md`  
**Revision:** 2.2 (Verified Physical Catalog Contracts, Safe Pool Lifecycle, Deterministic Rollback Tie-Breaker & Concurrency Hardening)  
**Status:** **PLANNING AND REVIEW ONLY (STRICTLY NO CODE MODIFIED)**  
**Author:** Principal Go Backend Engineer, PostgreSQL Database Architect & Distributed Systems Reliability Engineer  
**Issue Reference:** [HIGH-01] Migration Renumbering Ledger Incompatibility  
**Target Package:** `internal/storage/postgres` (`migrate.go`, `migrate_test.go`)  

---

## 1. Executive Summary & Root Cause Analysis

### 1.1 The Ledger Desynchronization Problem
In Stage 6 (commit `b762762`), migration filenames in `db/migrations/` were standardized to enforce strictly sequential numeric prefixes (`001_` through `010_`) and paired rollback scripts. Prior to Stage 6, two migration files shared the prefix `002_`:
1. `002_add_dedup_deps_partition.sql` (introduced in commit `30db763`)
2. `002_create_clients.sql` (introduced in commit `d098d73`)

Renumbering `002_create_clients.sql` to `003_create_clients.sql` shifted all eight subsequent migration filenames by $+1$:
- `002_create_clients.sql` $\rightarrow$ `003_create_clients.sql`
- `003_add_progress.sql` $\rightarrow$ `004_add_progress.sql`
- `004_add_cron_expr.sql` $\rightarrow$ `005_add_cron_expr.sql`
- `005_create_jobs_archive.sql` $\rightarrow$ `006_create_jobs_archive.sql`
- `006_dedup_unique.sql` $\rightarrow$ `007_dedup_unique.sql`
- `007_add_expires_at.sql` $\rightarrow$ `008_add_expires_at.sql`
- `008_create_dead_letter_jobs.sql` $\rightarrow$ `009_create_dead_letter_jobs.sql`
- `009_create_audit_logs.sql` $\rightarrow$ `010_create_audit_logs.sql`

Because the migration runner in `internal/storage/postgres/migrate.go` records verbatim base filenames (e.g. `'003_create_clients'`) in `schema_migrations.version`, an existing database that executed migrations prior to Stage 6 has a ledger containing legacy keys (`002_create_clients`, `003_add_progress`, ..., `009_create_audit_logs`).

When upgraded with the current runner:
1. **Redundant Execution:** The runner checks `applied["003_create_clients"]`, finds it missing, and re-executes all migrations from `003_` through `010_`.
2. **Ledger Duplication:** The ledger accumulates 18 rows instead of 10.
3. **Fatal Rollback Failure:** Once new migrations are rolled back, the runner encounters `009_create_audit_logs`. It attempts to read `db/migrations/009_create_audit_logs_rollback.sql`, which does not exist on disk (renamed to `010_create_audit_logs_rollback.sql`). Rollback crashes with `os.IsNotExist: rollback file not found`, leaving the database in an unrecoverable state.

### 1.2 Why SQL `IF NOT EXISTS` is Insufficient Proof of Equivalence
Previous assumptions relied on SQL `IF NOT EXISTS` clauses to prevent errors during re-execution. **This is fundamentally unsafe for production systems:**
- **Silently Masked Schema Drift:** `CREATE TABLE IF NOT EXISTS clients (...)` is a complete no-op if a table named `clients` already exists—even if that table is missing critical columns (`api_key_hash`) or has mismatched column types.
- **Index Misconfiguration:** `CREATE INDEX IF NOT EXISTS` does not verify index columns, sort order, or partial predicates (`WHERE dedup_key IS NOT NULL AND dedup_key != ''`). If a non-unique index already exists with that name, a `CREATE UNIQUE INDEX IF NOT EXISTS` statement silently succeeds without enforcing uniqueness.
- **Type Incompatibilities:** `ALTER TABLE jobs ADD COLUMN IF NOT EXISTS progress DOUBLE PRECISION` does nothing if a `progress` column already exists with an incompatible type (e.g., `INTEGER` or `TEXT`).
- **Ledger Inconsistency:** `IF NOT EXISTS` allows the runner to blindly write new version rows to `schema_migrations` without validating that the database actually matches the expected structural contract of that version.

Therefore, safe reconciliation **must prove physical schema equivalence** using PostgreSQL system catalogs before altering or consolidating ledger records.

---

## 2. Separation of Concerns & Implementation Scope

> [!IMPORTANT]
> **Strict Isolation from [SEC-WEBHOOK-01]:**  
> Remediation for [SEC-WEBHOOK-01] (cross-tenant webhook management authorization) is **strictly excluded** from Phase 2. Webhook authorization will be tracked and remediated as an independent task so that schema and migration runner changes remain entirely decoupled from API handler authorization logic.
>
> **Strict Implementation Scope:**
> Phase 2 is strictly limited to:
> - `internal/storage/postgres/migrate.go`
> - `internal/storage/postgres/migrate_test.go`
>
> **Zero modifications** will be made to SQL migration or rollback files in `db/migrations/`.

---

## 3. Dedicated Connection & PostgreSQL Advisory Locking Architecture

### 3.1 Connection Affinity & Advisory Lock Invariants
To prevent race conditions, concurrent migration executions, or interleaving DDL statements between multiple application instances, we implement PostgreSQL session-level advisory locking:

```go
// Deterministic 64-bit advisory lock key derived from hash of "task_queue_migrations"
const MigrationAdvisoryLockID = int64(0x7461736b71756575) // 8386121408848782709
```

#### Dedicated Connection Rule
Because `pgxpool.Pool` distributes queries across arbitrary pooled TCP connections, an advisory lock acquired on `pool.Exec()` would be tied to one connection while subsequent queries or transactions might run on a different connection (which would NOT hold the lock).

To guarantee strict connection affinity:
1. **Single Acquired Connection:** Both `RunMigrations` and `RollbackLastMigration` check out a dedicated connection via `conn, err := pool.Acquire(ctx)`.
2. **Session Lock Execution:** `SELECT pg_advisory_lock($1)` is executed directly on `conn`.
3. **Internal Function Refactoring:** All migration helper functions (`ensureSchemaMigrationsTable`, `reconcileLegacyMigrationLedger`, `appliedVersions`) and transaction operations (`conn.Begin(ctx)`) are executed **exclusively on `conn`**, never on the pool.
4. **Guaranteed Release with Detached Timeout Context:** A deferred function on `conn` executes `SELECT pg_advisory_unlock($1)` using a detached context (`context.WithTimeout(context.Background(), 5*time.Second)`). This ensures the lock is cleanly released even if the parent migration context was cancelled (`ctx.Done()`).
5. **Connection Destruction on Error [MED-REV-01]:** If an error occurs during migration execution or if the unlock query fails, `conn.Destroy()` is invoked instead of `conn.Release()`. `Destroy()` immediately closes the underlying TCP socket, guaranteeing that the PostgreSQL server terminates the backend and drops the session lock, preventing any tainted connection from returning to the pool.
6. **Caller Pool Preservation [HIGH-REV-01]:** `RunMigrations` and `RollbackLastMigration` **never call `pool.Close()`**. The caller's connection pool remains fully functional for health checks and outer retry backoff loops in `NewPostgresStore`.

### 3.2 Lifecyle Integration in `RunMigrations` and `RollbackLastMigration`

```go
func RunMigrations(ctx context.Context, pool *pgxpool.Pool, dir string, log *slog.Logger) error {
	// 1. Acquire dedicated connection from the pool
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("migrate: acquire connection: %w", err)
	}

	var lockAcquired bool
	defer func() {
		if lockAcquired {
			unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, unlockErr := conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", MigrationAdvisoryLockID); unlockErr != nil {
				if log != nil {
					log.Error("migrate: failed to release advisory lock; destroying connection", "error", unlockErr)
				}
				conn.Destroy()
				return
			}
		}
		conn.Release()
	}()

	// 2. Acquire PostgreSQL session advisory lock on the dedicated connection
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", MigrationAdvisoryLockID); err != nil {
		conn.Destroy()
		return fmt.Errorf("migrate: acquire advisory lock: %w", err)
	}
	lockAcquired = true

	// 3. Ensure ledger table exists on this connection
	if err := ensureSchemaMigrationsTable(ctx, conn); err != nil {
		return fmt.Errorf("migrate: create schema_migrations table: %w", err)
	}

	// 4. Pre-flight schema validation and ledger reconciliation on this connection
	if err := reconcileLegacyMigrationLedger(ctx, conn, log); err != nil {
		return fmt.Errorf("migrate: ledger reconciliation failed: %w", err)
	}

	// 5. Query applied versions on this connection
	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return fmt.Errorf("migrate: read applied versions: %w", err)
	}

	// 6. Discover files and execute unapplied migrations sequentially on this connection
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

		// Begin transaction on the locked connection
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
			conn.Destroy() // Destroy connection to handle potential commit ambiguity safely
			return fmt.Errorf("migrate: commit %s: %w", version, err)
		}
		if log != nil {
			log.Info("applied migration", "version", version)
		}
	}

	return nil
}
```

---

## 4. Physical Schema Verification Contracts (All 8 Renamed Migrations)

Before any legacy migration record is renamed, reconciled, or consolidated in `schema_migrations`, the reconciliation engine executes an explicit structural verification against PostgreSQL system catalogs (`information_schema.tables`, `information_schema.columns`, `pg_catalog.pg_index`, `pg_catalog.pg_class`).

If physical verification fails for any migration, **the engine halts immediately and rolls back the reconciliation transaction**. No ledger records are deleted or modified.

### 4.1 Schema Verification Catalog Queries

```sql
-- Check table existence
SELECT EXISTS (
    SELECT 1 FROM information_schema.tables 
    WHERE table_schema = 'public' AND table_name = $1
);

-- Check column existence and data type
SELECT data_type FROM information_schema.columns 
WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2;

-- Check index existence, uniqueness, and predicate expression
SELECT i.indisunique, COALESCE(pg_get_expr(i.indpred, i.indrelid), '') AS predicate
FROM pg_index i
JOIN pg_class c ON c.oid = i.indexrelid
JOIN pg_class t ON t.oid = i.indrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
WHERE n.nspname = 'public' AND t.relname = $1 AND c.relname = $2;
```

### 4.2 Exact Schema Contracts per Migration (Verified Against DDL)

Each contract matches the **exact physical DDL** defined in `db/migrations/`:

| Migration Version | Target Physical Objects | Verification Rules & Catalog Expectations | Failure Action |
| :--- | :--- | :--- | :--- |
| **003_create_clients**<br>*(Legacy `002_create_clients`)* | Table `clients`, Index `idx_clients_api_key_hash` | 1. Table `clients` exists in `public`.<br>2. Column `id` exists with data type `uuid`.<br>3. Column `tenant_id` exists with data type `character varying`.<br>4. Column `api_key_hash` exists with data type `character varying`.<br>5. Column `created_at` exists with data type `timestamp with time zone`.<br>6. Index `idx_clients_api_key_hash` exists on `clients(api_key_hash)`. | Halt; report missing table, column, or index |
| **004_add_progress**<br>*(Legacy `003_add_progress`)* | Column `jobs.progress` | Column `progress` exists on table `jobs` with data type `double precision` (`float8`). | Halt; report missing or wrong column type |
| **005_add_cron_expr**<br>*(Legacy `004_add_cron_expr`)* | Column `jobs.cron_expr` | Column `cron_expr` exists on table `jobs` with data type `text`. | Halt; report missing or wrong column type |
| **006_create_jobs_archive**<br>*(Legacy `005_create_jobs_archive`)* | Table `jobs_archive`, Indexes | 1. Table `jobs_archive` exists in `public`.<br>2. Columns `id` (`text`), `tenant_id` (`text`), `type` (`text`), `payload` (`jsonb`), `status` (`text`), `priority` (`text`), `attempts` (`integer`), `max_attempts` (`integer`), `timeout_seconds` (`integer`), `version` (`integer`), `cron_expr` (`text`) exist.<br>3. Index `idx_jobs_archive_tenant_status` exists on `jobs_archive(tenant_id, status)`.<br>4. Index `idx_jobs_archive_worker_status` exists on `jobs_archive(processed_by, status)`. | Halt; report missing table, columns, or indexes |
| **007_dedup_unique**<br>*(Legacy `006_dedup_unique`)* | Index `idx_jobs_dedup_key_unique` | Index `idx_jobs_dedup_key_unique` exists on table `jobs`, `indisunique = true`, and predicate expression contains `dedup_key IS NOT NULL` and `dedup_key != ''`. | Halt; report non-unique or missing index |
| **008_add_expires_at**<br>*(Legacy `007_add_expires_at`)* | Column `jobs.expires_at`, Index `idx_jobs_expires_at` | 1. Column `expires_at` exists on table `jobs` with data type `timestamp with time zone`.<br>2. Index `idx_jobs_expires_at` exists on `jobs(expires_at)` with predicate `expires_at IS NOT NULL`. | Halt; report missing column or index |
| **009_create_dead_letter_jobs**<br>*(Legacy `008_create_dead_letter_jobs`)* | Table `dead_letter_jobs`, Indexes | 1. Table `dead_letter_jobs` exists in `public`.<br>2. Columns `id` (`text`), `type` (`text`), `payload` (`jsonb`), `labels` (`jsonb`), `priority` (`text`), `status` (`text`), `retries` (`integer`), `max_retries` (`integer`), `created_at` (`timestamp with time zone`), `updated_at` (`timestamp with time zone`), `run_at` (`timestamp with time zone`), `tenant_id` (`text`), `dead_letter_reason` (`text`), `error_history` (`jsonb`) exist.<br>3. Index `idx_dlq_tenant` exists on `dead_letter_jobs(tenant_id)`.<br>4. Index `idx_dlq_created` exists on `dead_letter_jobs(created_at)`. | Halt; report missing table, columns, or indexes |
| **010_create_audit_logs**<br>*(Legacy `009_create_audit_logs`)* | Table `audit_logs`, Indexes | 1. Table `audit_logs` exists in `public`.<br>2. Columns `id` (`uuid`), `timestamp` (`timestamp with time zone`), `actor` (`character varying`), `action` (`character varying`), `target_type` (`character varying`), `target_id` (`character varying`), `details` (`jsonb`) exist.<br>3. Index `idx_audit_logs_timestamp` exists on `audit_logs(timestamp)`.<br>4. Index `idx_audit_logs_actor` exists on `audit_logs(actor)`. | Halt; report missing table, columns, or indexes |

### 4.3 Handling Inconsistent or Ambiguous States
- **Ambiguous State:** Both `legacy` and `canonical` exist, but physical verification fails for any object in the contract.
  - **Action:** The reconciliation transaction is aborted via `tx.Rollback()`.
  - **Output:** Returns `ErrSchemaInconsistent`:
    `"migrate: reconciliation blocked: schema validation failed for %s (legacy %s): %w. Ledger remains unchanged; manual operator review required."`
  - **Guarantee:** Not a single ledger entry is deleted, updated, or inserted.
- **Sole Legacy State:** Only `legacy` exists, but physical verification fails.
  - **Action:** Abort immediately. In-place renaming is refused.
- **Unknown Version:** A row exists with an unrecognized name (e.g. `'003_custom_patch'`).
  - **Action:** The runner ignores unrecognized rows during reconciliation. They are never modified or removed.

---

## 5. Safe Duplicate Handling & Earliest Timestamp Preservation

When both a legacy record and a canonical record exist in `schema_migrations`, the runner never assumes they are equivalent without proof.

### 5.1 Reconciliation Algorithm
```go
func reconcileLegacyMigrationLedger(ctx context.Context, conn *pgxpool.Conn, log *slog.Logger) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin reconciliation tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Read all current versions and their applied_at timestamps within tx
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
		hasLegacy := false
		var legacyTime time.Time
		if t, ok := applied[pair.Legacy]; ok {
			hasLegacy = true
			legacyTime = t
		}

		hasCanonical := false
		var canonicalTime time.Time
		if t, ok := applied[pair.Canonical]; ok {
			hasCanonical = true
			canonicalTime = t
		}

		if !hasLegacy {
			continue // No legacy migration to reconcile
		}

		// Step 1: Prove physical schema equivalence BEFORE touching ledger
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
```

---

## 6. Transaction Recovery, Failure Semantics & Rollback Safety

### 6.1 Transaction Boundaries & Failure Points in Rollback
In `RollbackLastMigration`, rollback DDL and ledger deletion are unified into a single atomic transaction block on the dedicated locked connection:

```mermaid
sequenceDiagram
    participant App as Migration Runner
    participant PG as PostgreSQL (Dedicated Conn)
    App->>PG: Acquire Advisory Lock
    App->>PG: Reconcile Ledger
    App->>PG: Query lastVersion (ORDER BY applied_at DESC, version DESC LIMIT 1)
    App->>App: Pre-check rollback file on disk (with fallback map)
    alt Rollback file missing or unknown version
        App->>App: Abort safely (ledger intact, 0 DDL executed)
    else Rollback file exists
        App->>PG: BEGIN TX
        App->>PG: Exec(rollbackSQL)
        alt Rollback SQL execution fails
            App->>PG: ROLLBACK TX (Schema and Ledger reverted)
        else Rollback SQL succeeds
            App->>PG: DELETE FROM schema_migrations WHERE version = lastVersion
            alt Delete fails / 0 rows affected
                App->>PG: ROLLBACK TX (Schema and Ledger reverted)
            else Delete succeeds
                App->>PG: COMMIT TX
                alt Commit fails or connection drops (Commit Ambiguity)
                    App->>App: Destroy Conn; Reconnection checks catalog
                else Commit succeeds
                    App->>PG: Release Advisory Lock
                end
            end
        end
    end
```

### 6.2 Deterministic Rollback Tie-Breaker [HIGH-REV-02]
When multiple migrations are applied in batch during automated deployment, they often share identical sub-second timestamps (`applied_at`). In PostgreSQL, `ORDER BY applied_at DESC LIMIT 1` is non-deterministic under identical timestamps.

**Enforced Resolution:** All rollback version selection queries use a deterministic secondary tie-breaker:
```sql
SELECT version FROM schema_migrations ORDER BY applied_at DESC, version DESC LIMIT 1
```
This guarantees strict LIFO (Last-In, First-Out) ordering even when multiple migrations commit within the exact same millisecond.

### 6.3 Handling Commit Ambiguity (Connection Loss during Commit) [HIGH-REV-01]
- **The Problem:** If a network disconnection occurs while `tx.Commit()` is in flight, the client cannot know whether PostgreSQL committed or aborted the transaction.
- **The Solution:**
  1. If `tx.Commit()` returns an error:
     - The migration runner invokes `conn.Destroy()` to ensure the backend session and advisory lock are terminated on PostgreSQL.
     - **The runner does NOT close `pool`**. The caller's `*pgxpool.Pool` remains intact.
     - The runner returns an error to the caller.
  2. On retry / recovery run:
     - The caller's outer retry loop or restarted runner checks out a fresh connection.
     - The pre-flight reconciliation engine queries `schema_migrations` and physical catalogs.
     - Because PostgreSQL DDL is transactional, both the DDL statements and the `INSERT INTO schema_migrations` row commit together or roll back together.
     - If the commit succeeded, the version is in `schema_migrations`, so `RunMigrations` skips it.
     - If the commit failed, the version is absent, so `RunMigrations` cleanly reapplies it.

---

## 7. Historical Migration Verification (Git Commit Evidence)

All 8 migration renamings in Stage 6 were independently verified against Git commit history:

```bash
$ git log --oneline -- db/migrations
b762762 feat: implement Stage 6 production hardening, docker assets, and migration rollbacks
570c95c feat(security): implement Audit Logging and UI dashboard
bdb4d36 feat(dlq): implement true Dead Letter Queue with isolated storage
f0608dd feat: implement job-level TTL
33f8c4e feat: implement strong exactly-once deduplication with unique constraint
e2eb5c2 feat: implement global concurrency limits and job archiving
2232120 feat: implement scalable recurring cron jobs
d098d73 feat: adding the client usage withe the ui
56ea5ba Add Postgres integration tests and fix store bugs they exposed
3fc6e86 feat: complete remaining roadmap items - webhook Start() test, chaos CI, HPA metrics, OTel propagation, Helm hardening, migration rollback
30db763 feat: add DAG deps, shard routing, TTL cleanup, stats API, circuit breaker, progress reporting, and HTML UI rewrite
ace532f Add DLQ management API with error history tracking and bulk purge support
40e4c8d Support webhook callbacks in task queue with SQLite/Postgres persistence and API ingestion
2ffe1a7 Finalize PostgresStore with transactional Dequeue and SLA-aware RecoverOrphans
93bbe36 Implement PostgresStore with versioned migrations and connection pooling via pgx
```

### 7.1 Detailed 8-Entry Mapping Table & Content Verification

| Seq | Legacy Filename (Pre-b762762) | Canonical Filename (Current) | Introducing Commit | Renaming Commit | Content Comparison | Rollback Script & Content |
| :---: | :--- | :--- | :--- | :--- | :--- | :--- |
| **1** | `001_create_jobs.sql` | `001_create_jobs.sql` | `93bbe36` | `b762762` | Added `IF NOT EXISTS` to indexes | `001_create_jobs_rollback.sql`: `DROP TABLE IF EXISTS jobs;` |
| **2** | `002_add_dedup_deps_partition.sql` | `002_add_dedup_deps_partition.sql` | `30db763` | `b762762` | 100% Identical | `002_add_dedup_deps_partition_rollback.sql`: `DROP INDEX ...; ALTER TABLE jobs DROP COLUMN ...;` |
| **3** | `002_create_clients.sql` *(duplicate 002)* | `003_create_clients.sql` | `d098d73` | `b762762` | 100% Identical (`CREATE TABLE IF NOT EXISTS clients (...)`) | `003_create_clients_rollback.sql`: `DROP TABLE IF EXISTS clients CASCADE;` |
| **4** | `003_add_progress.sql` | `004_add_progress.sql` | `30db763` | `b762762` | 100% Identical (`ALTER TABLE jobs ADD COLUMN IF NOT EXISTS progress ...`) | `004_add_progress_rollback.sql`: `ALTER TABLE jobs DROP COLUMN IF EXISTS progress;` |
| **5** | `004_add_cron_expr.sql` | `005_add_cron_expr.sql` | `2232120` | `b762762` | Added `IF NOT EXISTS` to column `cron_expr` | `005_add_cron_expr_rollback.sql`: `ALTER TABLE jobs DROP COLUMN IF EXISTS cron_expr;` |
| **6** | `005_create_jobs_archive.sql` | `006_create_jobs_archive.sql` | `e2eb5c2` | `b762762` | Added `IF NOT EXISTS` to indexes | `006_create_jobs_archive_rollback.sql`: `DROP TABLE IF EXISTS jobs_archive CASCADE;` |
| **7** | `006_dedup_unique.sql` | `007_dedup_unique.sql` | `33f8c4e` | `b762762` | Added `IF NOT EXISTS` to unique index `idx_jobs_dedup_key_unique` | `007_dedup_unique_rollback.sql`: `DROP INDEX ...; CREATE INDEX ...;` |
| **8** | `007_add_expires_at.sql` | `008_add_expires_at.sql` | `f0608dd` | `b762762` | Added `IF NOT EXISTS` to column `expires_at` | `008_add_expires_at_rollback.sql`: `DROP INDEX ...; ALTER TABLE jobs DROP COLUMN ...;` |
| **9** | `008_create_dead_letter_jobs.sql` | `009_create_dead_letter_jobs.sql` | `bdb4d36` | `b762762` | 100% Identical (`CREATE TABLE IF NOT EXISTS dead_letter_jobs ...`) | `009_create_dead_letter_jobs_rollback.sql`: `DROP TABLE IF EXISTS dead_letter_jobs;` |
| **10** | `009_create_audit_logs.sql` | `010_create_audit_logs.sql` | `570c95c` | `b762762` | 100% Identical (`CREATE TABLE IF NOT EXISTS audit_logs ...`) | `010_create_audit_logs_rollback.sql`: `DROP TABLE IF EXISTS audit_logs;` |

### 7.2 Rollback Script Transaction Compatibility
We inspected all 10 rollback scripts in `db/migrations/*_rollback.sql`:
- None contain non-transactional statements (`CREATE INDEX CONCURRENTLY`, `DROP INDEX CONCURRENTLY`, `VACUUM`).
- All 10 files contain strictly transactional DDL statements (`DROP TABLE IF EXISTS`, `ALTER TABLE ... DROP COLUMN IF EXISTS`, `CREATE INDEX IF NOT EXISTS`).
- **Conclusion:** 100% compatible with PostgreSQL ACID transaction boundaries.

---

## 8. Complete Expanded Test Matrix (13 Scenarios)

The test suite in `internal/storage/postgres/migrate_test.go` will cover the following 13 scenarios:

### 1. `TestMigration_FreshInstall`
- **Initial State:** Empty database without prior schema.
- **Operation:** `RunMigrations`.
- **Expected Schema:** All tables (`jobs`, `clients`, `jobs_archive`, `dead_letter_jobs`, `audit_logs`) and indexes created matching physical DDL.
- **Expected Ledger:** Exactly 10 canonical rows (`001_` through `010_`).
- **Expected Result:** Success (0 errors).

### 2. `TestMigration_LegacyFullUpgrade`
- **Initial State:** Pre-seeded with 10 legacy ledger entries (`001_create_jobs` .. `009_create_audit_logs`, including duplicate `002_` prefix). Schema tables created matching physical DDL.
- **Operation:** `RunMigrations`.
- **Expected Schema:** Fully verified via `pg_catalog`. **Zero DDL statements re-executed.**
- **Expected Ledger:** Exactly 10 canonical rows (`001_create_jobs` to `010_create_audit_logs`).
- **Expected Result:** Success. Followed by a clean 10-step sequential rollback down to 0.

### 3. `TestMigration_LegacyPartialUpgrade`
- **Initial State:** Pre-seeded with migrations up to `004_add_cron_expr`.
- **Operation:** `RunMigrations`.
- **Expected Schema:** Migrations 006–010 applied; archive, DLQ, audit tables created.
- **Expected Ledger:** Exactly 10 canonical rows.
- **Expected Result:** Success.

### 4. `TestMigration_DuplicatedLedgerRepair`
- **Initial State:** Pre-seeded with 18 rows (both legacy and canonical keys present). Schema verified intact.
- **Operation:** `RunMigrations`.
- **Expected Schema:** Unchanged.
- **Expected Ledger:** Normalized from 18 rows down to 10 canonical rows.
- **Expected Result:** Success.

### 5. `TestMigration_DirectLegacyRollback`
- **Initial State:** Database with a legacy key at the top of the stack.
- **Operation:** `RollbackLastMigration`.
- **Expected Schema:** Table rolled back cleanly via fallback map.
- **Expected Ledger:** Decremented by 1.
- **Expected Result:** Success.

### 6. `TestMigration_InconsistentDuplicateLedger_StopsSafely`
- **Initial State:** Ledger contains both `002_create_clients` and `003_create_clients`, but `clients` table is intentionally missing required column `api_key_hash`.
- **Operation:** `RunMigrations`.
- **Expected Schema:** Unmodified.
- **Expected Ledger:** Unmodified (both records preserved).
- **Expected Result:** **Fatal Error** (`schema validation failed: column api_key_hash missing on table clients`). Runner halts immediately without modifying ledger.

### 7. `TestMigration_ConflictingTimestamps_PreservesEarliest`
- **Initial State:** Legacy `002_create_clients` applied at `2026-01-01 10:00:00`. Canonical `003_create_clients` applied at `2026-06-01 12:00:00`.
- **Operation:** `RunMigrations`.
- **Expected Schema:** Validated intact.
- **Expected Ledger:** Canonical `003_create_clients` retained with `applied_at = 2026-01-01 10:00:00`. Legacy row deleted.
- **Expected Result:** Success; earliest timestamp preserved.

### 8. `TestMigration_ConcurrentRunners_AdvisoryLockSerialization`
- **Initial State:** Empty database. Two independent goroutines concurrently invoke `RunMigrations`.
- **Operation:** Concurrent execution under PostgreSQL advisory lock.
- **Expected Schema:** All tables created exactly once.
- **Expected Ledger:** Exactly 10 canonical rows (zero duplicate key errors).
- **Expected Result:** Both goroutines succeed (one performs DDL; second serializes, reads ledger, and finds 0 pending).

### 9. `TestMigration_FailedMigration_LedgerConsistent`
- **Initial State:** Database with migration 001 applied. A malformed migration `002_broken.sql` (syntax error) is introduced in test directory.
- **Operation:** `RunMigrations`.
- **Expected Schema:** Migration 002 changes rolled back.
- **Expected Ledger:** Contains only migration 001. No entry for `002_broken`.
- **Expected Result:** Error returned; transaction rolled back.

### 10. `TestMigration_FailedRollback_SchemaAndLedgerAtomic`
- **Initial State:** Database with migrations 001 and 002 applied. Rollback script for 002 contains invalid SQL (`DROP NONEXISTENT_SYNTAX`).
- **Operation:** `RollbackLastMigration`.
- **Expected Schema:** Unchanged (jobs table and columns intact).
- **Expected Ledger:** Migration 002 record **retained in schema_migrations**.
- **Expected Result:** Error returned; transaction rolled back; schema and ledger did not diverge.

### 11. `TestMigration_UnknownMigrationVersion_FailsGracefully`
- **Initial State:** `schema_migrations` contains phantom version `'999_unknown_feature'`.
- **Operation:** `RollbackLastMigration`.
- **Expected Schema:** Unchanged.
- **Expected Ledger:** Record `'999_unknown_feature'` remains in `schema_migrations`.
- **Expected Result:** Clear error: `"rollback aborted: rollback script ... not found: ledger unchanged"`.

### 12. `TestMigration_Rollback_TimestampTieBreaker_Deterministic`
- **Initial State:** Multiple migrations inserted with identical sub-second `applied_at` timestamps.
- **Operation:** `RollbackLastMigration` executed sequentially.
- **Expected Schema & Ledger:** Migrations are rolled back in strict numeric reverse order (`version DESC`), never out of order.
- **Expected Result:** Deterministic LIFO rollback order guaranteed.

### 13. `TestMigration_CommitError_CallerPoolRemainsUsable`
- **Initial State:** Migration execution encounters simulated connection termination during `tx.Commit()`.
- **Operation:** Runner catches error, calls `conn.Destroy()`, and returns error.
- **Expected Behavior:** Specific connection is closed; caller's `pool` remains open and usable for subsequent queries.
- **Expected Result:** Subsequent `pool.Ping()` succeeds.

---

## 9. Implementation Files & Preservation of Existing Work

### Files to be Modified during Phase 2 Implementation
1. `internal/storage/postgres/migrate.go`:
   - Add dedicated connection checkout (`pool.Acquire(ctx)`).
   - Add PostgreSQL advisory lock acquisition and release with detached context (`SELECT pg_advisory_lock(...)`).
   - Add `conn.Destroy()` on connection error or unlock failure.
   - Add physical schema verification helper functions matching exact DDL.
   - Add `reconcileLegacyMigrationLedger` with timestamp preservation and schema gate.
   - Update `RollbackLastMigration` to use `ORDER BY applied_at DESC, version DESC LIMIT 1` and atomic rollback transaction.
2. `internal/storage/postgres/migrate_test.go`:
   - Add test implementations for all 13 scenarios in Section 8 using real schema definitions.

### Existing Work Preservation Guarantee
- `internal/api/handler/handler.go`, `internal/api/handler/access_test.go`, `internal/service/job_service.go`, and `internal/service/job_service_test.go` from Phase 1 remain completely untouched.
- `db/migrations/` SQL files remain completely untouched.
- No database schemas or live environments are modified.
