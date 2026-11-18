# Final Production Readiness Audit Report

**Date of Audit:** 2026-11-09  
**Branch Audited:** `main`  
**Commit Hash:** `b762762`  
**Auditor:** Senior Go Backend Engineer, Distributed Systems Architect & Security Reviewer  

---

## 1. Executive Verdict

### Final Classification: **STAGING READY**

> [!WARNING]
> **VERDICT RATIONALE:**  
> The Go Task Queue System has achieved exceptional implementation maturity across core queueing, concurrency, non-blocking retries, native benchmarks, SDK client tooling, and containerization. All automated test suites (unit, race detection, benchmarks, live PostgreSQL integration, and chaos crash recovery) pass with 100% success.
>
> However, **the system cannot be certified as PRODUCTION READY** due to two critical issues:
> 1. A **Critical Security Vulnerability** in the job submission API ([internal/api/handler/handler.go#L136-L139](file:///home/fewzan/Projects/task-queue-system/internal/api/handler/handler.go#L136-L139)) allowing authenticated tenants to spoof `tenant_id` and inject jobs into other tenants' namespaces.
> 2. An **Upgrade & Rollback Incompatibility** in the database migration ledger ([internal/storage/postgres/migrate.go#L70](file:///home/fewzan/Projects/task-queue-system/internal/storage/postgres/migrate.go#L70)) caused by renumbering migration files, which breaks rollback chains for databases with existing schemas.
>
> The system is classified as **STAGING READY** for controlled pre-production environments, integration testing, and staging deployments while the prioritized remediations are implemented.

---

## 2. Verified Implementation by Stage

| Stage | Claimed Implementation | Codebase Verification | Status |
| :--- | :--- | :--- | :--- |
| **Stage 1: Foundation** | Restored test suite; inverted Redis deduplication logic (`SetNX` check); multi-partition queue depth aggregation (`Size()`). | Verified in [internal/storage/models/models.go#L1095](file:///home/fewzan/Projects/task-queue-system/internal/storage/models/models.go#L1095), [internal/queue/redis/redis.go#L745](file:///home/fewzan/Projects/task-queue-system/internal/queue/redis/redis.go#L745), and test suites. | **VERIFIED** |
| **Stage 2: Distributed Reliability** | Redis DLQ primitives (`MoveToDLQ`, `ListDLQ`, `CountDLQ`, `GetFromDLQ`, `DeleteDLQBefore`); non-blocking retries via sorted set (`delayed_jobs`); visibility timeout recovery (`ReclaimTimedOutJobs`). | Verified in [internal/storage/models/models.go#L1150](file:///home/fewzan/Projects/task-queue-system/internal/storage/models/models.go#L1150), [internal/worker/executor/worker_processor.go#L320](file:///home/fewzan/Projects/task-queue-system/internal/worker/executor/worker_processor.go#L320), [internal/queue/redis/redis.go#L673](file:///home/fewzan/Projects/task-queue-system/internal/queue/redis/redis.go#L673). Chaos tests pass. | **VERIFIED** |
| **Stage 3: Security & Multi-Tenancy** | Distributed Redis/PostgreSQL session tokens with CSRF; dynamic API key hashing (SHA-256); RBAC middleware (`RequireAuth`, `RequireRole`); tenant access checks. | Verified in [internal/api/session/session.go](file:///home/fewzan/Projects/task-queue-system/internal/api/session/session.go), [internal/api/middleware/session.go](file:///home/fewzan/Projects/task-queue-system/internal/api/middleware/session.go), [internal/api/handler/access.go](file:///home/fewzan/Projects/task-queue-system/internal/api/handler/access.go). | **VERIFIED** |
| **Stage 4: Performance & Observability** | O(1) Redis queue metrics counter (`task_queue:stats:queue_lengths`); OpenTelemetry spans for HTTP and job execution; Prometheus metrics. | Verified in [internal/storage/models/models.go#L985](file:///home/fewzan/Projects/task-queue-system/internal/storage/models/models.go#L985), [internal/worker/executor/worker_processor.go#L229](file:///home/fewzan/Projects/task-queue-system/internal/worker/executor/worker_processor.go#L229), [internal/tracing/tracing.go](file:///home/fewzan/Projects/task-queue-system/internal/tracing/tracing.go). | **VERIFIED** |
| **Stage 5: Benchmarks & SDKs** | Go `testing.B` benchmarks (`make bench`); zero-dependency Python client (`sdk/python/taskqueue.py`); TypeScript client (`sdk/typescript/TaskQueueClient.ts`). | Verified in [internal/queue/redis/queue_benchmark_test.go](file:///home/fewzan/Projects/task-queue-system/internal/queue/redis/queue_benchmark_test.go), [internal/worker/pool/pool_benchmark_test.go](file:///home/fewzan/Projects/task-queue-system/internal/worker/pool/pool_benchmark_test.go). SDK test suites pass. | **VERIFIED** |
| **Stage 6: Hardening & Packaging** | Multi-stage Docker builds with CLI packaging; sequential migrations (`001_`–`010_`); paired rollback SQL; two-way migration CI tests; Postgres scan field alignment. | Verified in [Dockerfile](file:///home/fewzan/Projects/task-queue-system/Dockerfile), [deploy/local/docker/](file:///home/fewzan/Projects/task-queue-system/deploy/local/docker/), [internal/storage/postgres/migrate_test.go](file:///home/fewzan/Projects/task-queue-system/internal/storage/postgres/migrate_test.go), [internal/storage/postgres/postgres.go](file:///home/fewzan/Projects/task-queue-system/internal/storage/postgres/postgres.go). | **VERIFIED** |

---

## 3. Unverified Claims & Implementation Gaps

1. **Redis Audit Logging (Unverified Claim in Stage 3):**
   - The documentation asserts full audit logging support across storage backends.
   - **Actual Implementation:** In [internal/storage/models/models.go#L1402-L1405](file:///home/fewzan/Projects/task-queue-system/internal/storage/models/models.go#L1402-L1405), `RedisStore.LogAuditAction` and `RedisStore.ListAuditLogs` are no-op stubs (`return nil`). When running in a Redis-only deployment, all audit events are silently discarded.
2. **End-to-End Distributed Tracing Spans (Partial Gap in Stage 4):**
   - The documentation claims tracing covers the entire distributed job lifecycle.
   - **Actual Implementation:** `tracing.Start` is only called for HTTP request ingestion ([internal/api/middleware/logging.go#L37](file:///home/fewzan/Projects/task-queue-system/internal/api/middleware/logging.go#L37)) and worker execution ([internal/worker/executor/worker_processor.go#L229](file:///home/fewzan/Projects/task-queue-system/internal/worker/executor/worker_processor.go#L229)). Queue enqueue, scheduler promotion, delayed scheduling, and webhook delivery do not generate intermediate spans.
3. **Database-Wide O(1) Queue Depth Tracking (Partial Gap in Stage 4):**
   - While Redis uses the $O(1)$ counter hash, [internal/storage/postgres/postgres.go#L721-L727](file:///home/fewzan/Projects/task-queue-system/internal/storage/postgres/postgres.go#L721-L727) still issues a full table aggregation query (`SELECT type, tenant_id, COUNT(*) FROM jobs WHERE status = 'pending' GROUP BY type, tenant_id`) for every Prometheus metrics scrape.

---

## 4. Findings Categorized by Severity

### Critical Severity

#### [CRIT-01] Multi-Tenant Spoofing in Job Creation API
* **Location:** [internal/api/handler/handler.go#L136-L139](file:///home/fewzan/Projects/task-queue-system/internal/api/handler/handler.go#L136-L139), [internal/api/handler/handler.go#L220-L222](file:///home/fewzan/Projects/task-queue-system/internal/api/handler/handler.go#L220-L222)
* **Description:**  
  In `CreateJob`, the handler inspects the authenticated tenant ID:
  ```go
  ctxTenant := middleware.TenantIDFromContext(r.Context())
  if req.TenantID == "" && middleware.IsClientTenant(ctxTenant) {
      req.TenantID = ctxTenant
  }
  ```
  If a client tenant provides an explicit `tenant_id` in the JSON request body (e.g., `"tenant_id": "victim-tenant"`), `req.TenantID == ""` evaluates to `false`. The user-supplied tenant ID is **never verified or overwritten**.
* **Impact:** Any authenticated tenant can inject tasks into any other tenant's namespace, consume that tenant's concurrency limits, bypass rate limits, or execute jobs under another tenant's billing identity.
* **Remediation:** Enforce that if `middleware.IsClientTenant(ctxTenant)` is true, `req.TenantID` must either equal `ctxTenant` or be forcibly overridden to `ctxTenant`. If `req.TenantID != "" && req.TenantID != ctxTenant`, return `403 Forbidden`.

---

### High Severity

#### [HIGH-01] Migration Renumbering Breaks Rollback Chain on Existing Databases
* **Location:** [internal/storage/postgres/migrate.go#L70](file:///home/fewzan/Projects/task-queue-system/internal/storage/postgres/migrate.go#L70), [internal/storage/postgres/migrate.go#L119-L133](file:///home/fewzan/Projects/task-queue-system/internal/storage/postgres/migrate.go#L119-L133)
* **Description:**  
  `schema_migrations` stores the raw filename without `.sql` as its primary key. Stage 6 renumbered migrations (`002_create_clients.sql` $\rightarrow$ `003_create_clients.sql`, up to `010_create_audit_logs.sql`).  
  When run against an existing production or staging database that previously executed migrations:
  1. The migration runner treats `003_create_clients` through `010_create_audit_logs` as unapplied because the recorded versions in `schema_migrations` were `002_create_clients` through `009_create_audit_logs`.
  2. The runner executes the entire sequence again.
  3. Subsequent invocations of `RollbackLastMigration` query `schema_migrations` ordered by `applied_at DESC`. Once the rollback reaches an older entry (e.g. `009_create_audit_logs`), it attempts to load `009_create_audit_logs_rollback.sql`. Because that file was renamed to `010_create_audit_logs_rollback.sql`, the file lookup fails with `os.IsNotExist(err)` and the rollback process crashes.
* **Impact:** Inability to perform automated deployment rollbacks on existing databases; duplicate ledger state.
* **Remediation:** Store a canonical integer migration version in `schema_migrations` rather than relying on mutable filenames, or provide an upgrade migration that reconciles legacy filename entries.

#### [HIGH-02] Lack of Distributed Leader Election in Scheduler
* **Location:** [cmd/scheduler/main.go#L106-L167](file:///home/fewzan/Projects/task-queue-system/cmd/scheduler/main.go#L106-L167), [internal/service/job_service.go#L638-L690](file:///home/fewzan/Projects/task-queue-system/internal/service/job_service.go#L638-L690)
* **Description:**  
  The scheduler maintenance loop executes periodic maintenance every 1,500ms without acquiring a distributed lease or leader lock. If two scheduler instances are started for High Availability (HA):
  - In `RecurringJobsDue`, both schedulers execute `SearchJobs` concurrently, detect due recurring templates, generate distinct job IDs via `jobs.NewJob(...)`, and enqueue duplicate instances.
* **Impact:** Duplicate recurring job generation; race conditions during concurrent maintenance loops.
* **Remediation:** Implement a Redis-backed distributed lease lock (e.g., `SET task_queue:scheduler:leader <instance_id> NX EX 5`) so only the active leader executes maintenance loops.

---

### Medium Severity

#### [MED-01] Non-Atomic Dequeue in Redis Store Can Drop Jobs on Worker Hard-Kill
* **Location:** [internal/queue/redis/redis.go#L294-L405](file:///home/fewzan/Projects/task-queue-system/internal/queue/redis/redis.go#L294-L405)
* **Description:**  
  `RedisQueue.Dequeue` pops a job from the priority list using `q.client.BRPop(...)` (line 294), and only tracks the in-flight state via `TxPipeline` (lines 384–405) after JSON unmarshaling and rate-limit checks. If a worker process is terminated (`SIGKILL`, OOM killer, power disruption) between `BRPop` and `pipe.Exec`, the job has already been removed from the Redis list but is not yet recorded in `task_queue:payloads` or `processingSetKey`.
* **Impact:** Permanent job loss under ungraceful worker termination during the dequeue window.
* **Remediation:** Use `RPOPLPUSH` or `LMOVE` to atomically move popped items to an in-flight reservation queue in Redis before worker processing.

#### [MED-02] O(N) Network and Memory Overhead in Redis DLQ Operations
* **Location:** [internal/storage/models/models.go#L1206](file:///home/fewzan/Projects/task-queue-system/internal/storage/models/models.go#L1206), [internal/storage/models/models.go#L1244](file:///home/fewzan/Projects/task-queue-system/internal/storage/models/models.go#L1244), [internal/storage/models/models.go#L1263](file:///home/fewzan/Projects/task-queue-system/internal/storage/models/models.go#L1263)
* **Description:**  
  `ListDLQ`, `CountDLQ`, and `DeleteDLQBefore` fetch all hash values from Redis using `s.client.HVals(ctx, dlqStoreKey)`. If the Dead Letter Queue contains 100,000+ items, this pulls the entire hash into Go memory across the network and unmarshals every JSON payload to perform in-memory pagination and filtering.
* **Impact:** Memory spikes, network saturation, and Redis latency degradation during DLQ management.
* **Remediation:** Maintain tenant and timestamp index sorted sets for DLQ items (e.g., `task_queue:dlq:index:<tenant>`), or use `HScan` cursor-based iteration.

#### [MED-03] Full Table Scan on Metrics Scrape in PostgreSQL Store
* **Location:** [internal/storage/postgres/postgres.go#L721-L727](file:///home/fewzan/Projects/task-queue-system/internal/storage/postgres/postgres.go#L721-L727)
* **Description:**  
  `PostgresStore.GetQueueLengths` executes `SELECT type, tenant_id, COUNT(*) FROM jobs WHERE status = 'pending' GROUP BY type, tenant_id` on every scrape. There is no partial composite index on `(status, type, tenant_id)`.
* **Impact:** High database I/O and query latency under large queue volumes.
* **Remediation:** Add a composite index `CREATE INDEX idx_jobs_pending_metrics ON jobs(type, tenant_id) WHERE status = 'pending'`.

#### [MED-04] DualStore Silent Divergence on Secondary Write Failure
* **Location:** [internal/storage/models/dual_store.go#L36](file:///home/fewzan/Projects/task-queue-system/internal/storage/models/dual_store.go#L36), [internal/storage/models/dual_store.go#L46-L94](file:///home/fewzan/Projects/task-queue-system/internal/storage/models/dual_store.go#L46-L94)
* **Description:**  
  All mutations to the secondary store in `DualStore` discard errors using `_ = s.Secondary.<Method>(...)`. If the secondary store becomes unavailable or rejects writes, the primary and secondary stores diverge silently with no telemetry or error logs.
* **Impact:** Inconsistent secondary store state; potential data loss if switching backends.
* **Remediation:** Log secondary write failures and expose a metrics counter for DualStore synchronization errors.

---

### Low Severity

#### [LOW-01] Missing CLI Binary in Scheduler Docker Image
* **Location:** [deploy/local/docker/scheduler.Dockerfile#L1-L17](file:///home/fewzan/Projects/task-queue-system/deploy/local/docker/scheduler.Dockerfile#L1-L17)
* **Description:**  
  While `Dockerfile`, `api.Dockerfile`, and `worker.Dockerfile` build and copy `/cli` into `/app/cli`, `scheduler.Dockerfile` omits the CLI compilation step.
* **Impact:** Operators cannot run management CLI commands from inside scheduler container instances.
* **Remediation:** Add `RUN ... go build -o /cli ./cmd/cli` and `COPY --from=builder /cli /app/cli` to `scheduler.Dockerfile`.

#### [LOW-02] Structured Logger Lacks Field Masking Filter
* **Location:** [internal/logger/logger.go#L10-L27](file:///home/fewzan/Projects/task-queue-system/internal/logger/logger.go#L10-L27)
* **Description:**  
  The `slog.JSONHandler` does not implement a `ReplaceAttr` function to redact sensitive field keys (such as `api_key`, `secret`, `password`, `token`) if accidentally passed to logger attributes.
* **Impact:** Potential accidental credential leakage in log aggregation systems.
* **Remediation:** Configure `slog.HandlerOptions.ReplaceAttr` to mask sensitive keys with `[REDACTED]`.

---

## 5. Migration Compatibility Assessment

### Filename & Version Mapping
| Old Filename | Current Filename | Current Rollback File |
| :--- | :--- | :--- |
| `001_create_jobs.sql` | `001_create_jobs.sql` | `001_create_jobs_rollback.sql` |
| `002_add_dedup_deps_partition.sql` | `002_add_dedup_deps_partition.sql` | `002_add_dedup_deps_partition_rollback.sql` |
| `002_create_clients.sql` *(duplicate prefix)* | `003_create_clients.sql` | `003_create_clients_rollback.sql` |
| `003_add_progress.sql` | `004_add_progress.sql` | `004_add_progress_rollback.sql` |
| `004_add_cron_expr.sql` | `005_add_cron_expr.sql` | `005_add_cron_expr_rollback.sql` |
| `005_create_jobs_archive.sql` | `006_create_jobs_archive.sql` | `006_create_jobs_archive_rollback.sql` |
| `006_dedup_unique.sql` | `007_dedup_unique.sql` | `007_dedup_unique_rollback.sql` |
| `007_add_expires_at.sql` | `008_add_expires_at.sql` | `008_add_expires_at_rollback.sql` |
| `008_create_dead_letter_jobs.sql` | `009_create_dead_letter_jobs.sql` | `009_create_dead_letter_jobs_rollback.sql` |
| `009_create_audit_logs.sql` | `010_create_audit_logs.sql` | `010_create_audit_logs_rollback.sql` |

### Backward Compatibility Assessment
- **Fresh Databases:** Flawless. Migrations apply `001_` through `010_` sequentially and pass the full UP $\rightarrow$ DOWN $\rightarrow$ UP cycle in 0.42 seconds.
- **Existing Pre-Migrated Databases:** **HIGH RISK**. Databases with historical `schema_migrations` containing `002_create_clients` through `009_create_audit_logs` will treat new files `003_` through `010_` as unapplied, re-executing schema statements and breaking rollback resolution.
- **Idempotency Safeguards:** `IF NOT EXISTS` clauses on all tables and indexes prevent runtime syntax errors during re-execution, but cannot update mismatched column types if previous migrations were modified.

---

## 6. Docker Build & Runtime Verification

All four container images were independently built and inspected from scratch:

1. **Root `Dockerfile` (`taskqueue:root`):**
   - Built successfully (`271.8s`).
   - Binaries verified: `/app/api`, `/app/worker`, `/app/scheduler`, `/app/cli`.
   - Migration directory verified: `/app/db/migrations` (20 files, 10 up + 10 rollback).
   - Non-root user verified: `appuser`.
2. **API Image (`deploy/local/docker/api.Dockerfile` $\rightarrow$ `taskqueue:api`):**
   - Built successfully (`165.9s`).
   - Binaries verified: `/app/api`, `/app/cli`.
   - Migration directory verified: `/app/db/migrations` (10 up + 10 rollback).
3. **Worker Image (`deploy/local/docker/worker.Dockerfile` $\rightarrow$ `taskqueue:worker`):**
   - Built successfully (`58.6s`).
   - Binaries verified: `/app/worker`, `/app/scheduler`, `/app/cli`.
   - Migration directory verified: `/app/db/migrations` (10 up + 10 rollback).
4. **Scheduler Image (`deploy/local/docker/scheduler.Dockerfile` $\rightarrow$ `taskqueue:scheduler`):**
   - Built successfully (`125.7s`).
   - Binaries verified: `/app/scheduler`.
   - Migration directory verified: `/app/db/migrations` (10 up + 10 rollback).
   - Finding: `/app/cli` omitted (see [LOW-01]).

---

## 7. Security Assessment

1. **Authentication & Session Security:**
   - Static operator key provides immediate admin access without database dependency.
   - Dynamic client API keys are hashed with SHA-256 before database lookup and cached with a 60-second in-memory TTL.
   - Distributed sessions are stored in Redis/PostgreSQL with secure `httpOnly` cookie delivery and cryptographically strong CSRF validation on all state-changing endpoints.
2. **Role-Based Access Control:**
   - Strict segregation between `RoleAdmin`, `RoleTenantClient`, and `RoleViewer`.
   - Administrative endpoints (`/api/v1/clients`, `/api/v1/operator/queue/*`, `/api/v1/circuit-breakers/reset/*`, `/api/v1/audit-logs`) strictly enforce `RoleAdmin`.
3. **Tenant Isolation:**
   - Data read endpoints (`GetJobStatus`, `ListJobs`, `GetFailedJobDetail`) strictly validate that `job.TenantID == ctxTenant`.
   - Data modification endpoints (`CancelJob`, `PauseJob`, `ResumeJob`, `UpdateJobProgress`) strictly validate ownership before action.
   - **Vulnerability Identified:** Job ingestion (`CreateJob`, `CreateJobBatch`) permits client-side tenant spoofing if `tenant_id` is passed in request payload (see [CRIT-01]).

---

## 8. Distributed Reliability Assessment

1. **Job Delivery Guarantee:**
   - **At-least-once delivery** is maintained under normal operation, graceful worker shutdown, and scheduler-mediated recovery.
   - Under catastrophic worker termination in the microsecond window between `BRPop` and reservation pipeline execution, jobs can be lost (see [MED-01]).
2. **Crash & Partition Recovery:**
   - Worker termination triggers automatic in-flight reclamation by the scheduler after the visibility timeout expires.
   - Volatile Redis data loss on crash without AOF persistence is verified by chaos tests; AOF `appendfsync always` enables complete state recovery.
3. **Scheduler Single Point of Failure:**
   - The scheduler is currently an active-passive component. Running multiple scheduler replicas concurrently causes duplicate execution of recurring jobs (see [HIGH-02]).

---

## 9. Test Verification Matrix

All tests were executed honestly in the current environment:

| Test Suite | Command | Execution Time | Result | Notes |
| :--- | :--- | :--- | :--- | :--- |
| **All Unit Tests** | `go test ./...` | 3.14s | **100% PASS** | All 25 internal packages passed |
| **Race Detector** | `go test -race ./...` | 5.52s | **100% PASS** | 0 data races detected |
| **Postgres Tests** | `make test-postgres` | 33.36s | **100% PASS** | Integration, models, postgres, and 10-step migration cycle |
| **Go Benchmarks** | `make bench` | 11.94s | **PASS** | 19,170 enqueue ops/sec, 2,160 worker jobs/sec |
| **Python SDK** | `python3 sdk/python/test_taskqueue.py` | 0.55s | **5/5 PASS** | Complete client API coverage |
| **TypeScript SDK** | `node --experimental-strip-types sdk/typescript/test_client.ts` | 0.04s | **6/6 PASS** | Complete client API coverage |
| **Chaos Suite** | `go test -v -tags chaos ./chaos/...` | 65.18s | **PASS / 1 SKIP** | `TestRedisCrashMidTransition`: PASS (30.6s)<br>`TestRedisCrash_VolatileDataLoss`: PASS (9.3s)<br>`TestWorkerHardKill`: PASS<br>`TestClockSkewOrphanRecovery`: PASS (5.7s)<br>`TestNetworkPartition`: **SKIPPED** (requires root privileges for iptables) |

---

## 10. Remaining Blockers for Production Deployment

To achieve a **PRODUCTION READY** classification, the following two blockers must be resolved:

1. **Blocker 1 (Security):** Fix tenant ID spoofing in `CreateJob` and `CreateJobBatch` ([CRIT-01]). Client tenants must never be permitted to specify a different `tenant_id`.
2. **Blocker 2 (Operational):** Reconcile migration versions in `schema_migrations` ([HIGH-01]) so existing databases do not re-run migrations or fail during rollbacks.

---

## 11. Prioritized Remediation Plan

### Phase 1: Immediate Security & Upgrade Fixes (Pre-Production Gate)
1. **Patch Tenant Spoofing ([CRIT-01]):**
   - In `internal/api/handler/handler.go`, update `CreateJob` (lines 136–139) and `CreateJobBatch` (lines 220–222):
     ```go
     if middleware.IsClientTenant(ctxTenant) {
         if req.TenantID != "" && req.TenantID != ctxTenant {
             h.writeError(w, http.StatusForbidden, apperr.CodePermissionDenied, "cannot submit job for another tenant")
             return
         }
         req.TenantID = ctxTenant
     }
     ```
2. **Normalize Migration Ledger ([HIGH-01]):**
   - In `internal/storage/postgres/migrate.go`, parse migration prefixes (e.g. `003`) as integer versions, or provide a legacy migration reconciling historical names (`002_create_clients` $\rightarrow$ `003_create_clients`).

### Phase 2: High Availability & Data Durability (Production Readiness Gate)
3. **Add Distributed Lease to Scheduler ([HIGH-02]):**
   - Acquire a Redis lease lock (`task_queue:scheduler:leader`) before running maintenance ticks to support multi-replica HA scheduler deployments safely.
4. **Implement Atomic Dequeue (`LMOVE`/`RPOPLPUSH`) ([MED-01]):**
   - Eliminate the window between list pop and reservation tracking in `RedisQueue.Dequeue`.
5. **Optimize Redis DLQ Pagination ([MED-02]):**
   - Replace full `HVals` extraction with Redis sorted set indexing or cursor-based `HScan`.

### Phase 3: Observability & Operational Polish
6. **Add PostgreSQL Metrics Index ([MED-03]):**
   - Add `idx_jobs_pending_metrics` to eliminate table scans during metric scraping.
7. **Add Secret Masking in Logger ([LOW-02]):**
   - Add `slog.HandlerOptions.ReplaceAttr` to sanitize credentials and tokens from logs.
8. **Package CLI into Scheduler Image ([LOW-01]):**
   - Align `deploy/local/docker/scheduler.Dockerfile` with the other Dockerfiles.
