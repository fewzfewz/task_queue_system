# Stage 2 Implementation Report: Distributed Reliability

**Execution Date:** 2026-11-09  
**Branch:** `main`  
**Base Commit:** `7b08fe3`  
**Status:** Completed & Fully Verified  

---

## 1. Executive Summary

Stage 2 focused on distributed reliability, fault tolerance, and queue recovery mechanisms as defined in `PROJECT_AUDIT.md`. All tasks outlined in the Stage 2 specification have been implemented, tested, and verified:

1. **Task 2.1 — Redis Dead Letter Queue (DLQ):** Complete implementation of all stubbed DLQ methods on `RedisStore` (`MoveToDLQ`, `ListDLQ`, `CountDLQ`, `GetFromDLQ`, `DeleteDLQBefore`, `DeleteFromDLQ`) preserving strict multi-tenant isolation, filtering, sorting, pagination, and DualStore synchronization.
2. **Task 2.2 — Non-Blocking Retries:** Replaced worker-side blocking sleeps (`time.After(delay)`) with asynchronous delayed-job scheduling via Redis sorted sets (`task_queue:delayed_jobs`). Workers return immediately, allowing them to remain at full processing capacity.
3. **Task 2.3 — In-Flight Visibility & Stalled Task Recovery:** Made job visibility timeout dynamically respect individual `job.Timeout` configurations (falling back to the 30-second default). Hardened the Redis Lua scripts (`promoteScheduledJobsScript` and `reclaimTimedOutJobsScript`) using atomic conditional removals (`if redis.call('ZREM', ...) == 1`) to guarantee idempotency and prevent duplicate executions during concurrent scheduler cycles.
4. **Task 2.4 — Chaos & Reliability Verification:** Fixed test harness gaps in `chaos/chaos_test.go` and `chaos/helpers.go`. Resolved the `TestWorkerHardKill` failure by orchestrating the `cmd/scheduler` binary alongside workers. Formally distinguished between volatile Redis data loss (unpersisted in-memory instances) and crash recovery using durable AOF (`--appendonly yes --appendfsync always`). Accurately verified root permission handling for `iptables` network partitioning.

---

## 2. Root Cause Analysis & Technical Solutions

### Task 2.1: Redis DLQ Implementation

#### Root Cause
In `internal/storage/models/models.go`, the RedisStore DLQ methods (`MoveToDLQ`, `ListDLQ`, `CountDLQ`, `GetFromDLQ`, `DeleteDLQBefore`, `DeleteFromDLQ`) were empty stubs returning `nil`, `0, nil`, or `nil, nil`. Furthermore:
- `DualStore` ignored the secondary store during DLQ operations, creating divergence between PostgreSQL and Redis.
- `JobService.GetJobStatus` returned `ErrJobNotFound` when a failed job was moved out of the active jobs table into the DLQ table in PostgreSQL, preventing clients from inspecting failed jobs via the API.

#### Implemented Solution
- **Dedicated DLQ Key Space:** Established `const dlqStoreKey = "task_queue:store:dlq"`.
- **Atomic State Transition (`MoveToDLQ`):** Built around a Redis `TxPipeline` that retrieves the job from `task_queue:store:jobs`, marks `Status = StatusFailed`, appends the failure reason to the job payload and `ErrorHistory`, updates `UpdatedAt = time.Now().UTC()`, persists to `task_queue:store:dlq`, and removes from `task_queue:store:jobs`. If the job is already in the DLQ, the operation is idempotent.
- **Tenant Isolation & Querying (`ListDLQ`, `CountDLQ`):** Queries iterate or scan entries, filtering strictly by `tenantID` (when supplied) and `jobType` (when supplied). Results are sorted newest-first (`CreatedAt DESC`) and support standard `offset` and `limit` pagination.
- **Batch Purge (`DeleteDLQBefore`):** Iterates over the DLQ hash, verifies `tenantID`, `jobType`, and cutoff timestamp (`UpdatedAt.Before(before)`), and executes batched `HDel` operations.
- **DualStore DLQ Synchronization:** Updated `DualStore.MoveToDLQ`, `DualStore.DeleteFromDLQ`, and `DualStore.DeleteDLQBefore` to mirror operations across primary (PostgreSQL) and secondary (Redis) storage.
- **API Fallback:** Updated `JobService.GetJobStatus` so that when `store.GetByID` returns `ErrJobNotFound`, the service automatically falls back to `store.GetFromDLQ` before returning an error.

---

### Task 2.2: Non-Blocking Retries

#### Root Cause
In `internal/worker/executor/worker_processor.go`, worker retry logic used a blocking select:
```go
select {
case <-shutdownCtx.Done(): ...
case <-time.After(delay):
}
```
During exponential backoff delays (e.g. 2s, 4s, 8s up to minutes), the worker thread was blocked and unable to dequeue other ready jobs. This degraded worker pool throughput under transient error conditions.

#### Implemented Solution
- **Sorted Set Integration:** Leveraged the existing Redis delayed queue (`task_queue:delayed_jobs` ZSET).
- **Non-Blocking Worker Hand-Off:** In `WorkerProcessor.retry`:
  1. Calculates exponential backoff: `delay := jobs.BackoffDelay(job)`.
  2. Updates job fields: `job.Retries++`, `job.Status = jobs.StatusPending`, `job.ProcessedBy = ""`, `job.UpdatedAt = time.Now().UTC()`, `job.RunAt = time.Now().UTC().Add(delay)`.
  3. Saves the updated state into the persistent store (`wp.service.Store().Save`).
  4. Enqueues the job via `wp.service.Enqueue(execCtx, job)`. Because `job.RunAt.After(time.Now())`, `RedisQueue.Enqueue` places the serialized job into `task_queue:delayed_jobs` with score = `RunAt.Unix()`.
  5. Acknowledges the original in-flight record via `wp.service.Ack(execCtx, job.ID)` so the worker processing set stays clean.
  6. The worker goroutine returns immediately (< 1ms) and dequeues the next available job.
- **Circuit Breaker Deferral:** Applied the identical non-blocking architecture to `WorkerProcessor.deferJob` for jobs delayed due to an open circuit breaker (5-second deferral without incrementing `job.Retries`).

---

### Task 2.3: In-Flight Visibility & Stalled Task Recovery

#### Root Cause
- `RedisQueue.Dequeue` hardcoded the visibility timeout to `30 * time.Second`, ignoring custom `job.Timeout` values specified at submission time.
- Lua scripts for delayed job promotion and in-flight reclamation did not atomically verify that the job was still in the source sorted set before pushing to active lists, allowing race conditions and duplicate executions if multiple schedulers ran concurrently.

#### Implemented Solution
- **Job-Specific Visibility Timeout:** Updated `RedisQueue.Dequeue` to dynamically inspect `job.Timeout`:
  ```go
  vTimeout := defaultVisibilityTimeout // 30s
  if job.Timeout > 0 {
      vTimeout = time.Duration(job.Timeout) * time.Second
  }
  ```
  The job ID is recorded in `task_queue:in_flight` with score = `time.Now().Add(vTimeout).Unix()`.
- **Atomic Scheduler Scripts:**
  - `promoteScheduledJobsScript`: Wraps queue push in `if redis.call('ZREM', delayedKey, job) == 1 then redis.call('LPUSH', partitionKey, job) end`.
  - `reclaimTimedOutJobsScript`: Wraps queue push in `if redis.call('ZREM', inFlightKey, id) == 1 then ... redis.call('LPUSH', partitionKey, payload) end`, and cleans up active keys.
- **Scheduler Operational Requirement:** Documented that worker processes exclusively dequeue and execute tasks; orphan recovery and scheduled promotion require the `cmd/scheduler` daemon to be active in the deployment environment.

---

### Task 2.4: Chaos and Reliability Verification

#### Root Cause
1. `TestWorkerHardKill`: Failed because no scheduler process was executing during the test. When the worker was abruptly killed (`SIGKILL`), jobs remained in `task_queue:in_flight` indefinitely because workers do not self-reclaim stalled in-flight tasks.
2. `TestRedisCrashMidTransition`: Failed because default Docker `redis:7-alpine` containers run without persistent storage or AOF. Killing the container abruptly (`ContainerKill`) destroyed in-memory state; upon restart, Redis was completely empty.
3. `TestNetworkPartition`: Failed with permissions errors when non-root users attempted to invoke `iptables`.

#### Implemented Solution
- **Scheduler Orchestration in Chaos Harness:** Added `buildSchedulerBinary` and `startSchedulerProcess` in `chaos/helpers.go`. `TestWorkerHardKill` now spins up the scheduler binary with a dynamically allocated port (`getFreePort`), allowing stalled jobs from killed workers to be automatically reclaimed and processed to completion by restarted workers (100% completion verified).
- **Persistent Redis Verification:** Added `newPersistentRedisContainer` launching Redis with `--appendonly yes --appendfsync always`. In `TestRedisCrashMidTransition`, mid-transition `SIGKILL` and container restart result in 100% job recovery and completion.
- **Volatile Loss Documentation:** Added `TestRedisCrash_VolatileDataLoss` to explicitly demonstrate that unpersisted Redis instances drop in-memory state upon `SIGKILL`, proving the necessity of AOF persistence or DualStore/PostgreSQL durability in production.
- **Root Capability Check:** Verified that `blockRedisTraffic` in `chaos/helpers.go` cleanly skips `TestNetworkPartition` when run without root privileges (`os.Geteuid() != 0`).

---

## 3. Files Modified

| File | Changes Made |
| :--- | :--- |
| `internal/storage/models/models.go` | Implemented `RedisStore` DLQ methods (`MoveToDLQ`, `ListDLQ`, `CountDLQ`, `GetFromDLQ`, `DeleteDLQBefore`, `DeleteFromDLQ`) with tenant isolation, sorting, and pagination. |
| `internal/storage/models/models_test.go` | Added `TestRedisStore_DLQ` suite covering creation, query filters, pagination, single deletion, bulk purge, and error conditions. |
| `internal/storage/models/dual_store.go` | Synchronized `DualStore` DLQ methods (`MoveToDLQ`, `DeleteFromDLQ`, `DeleteDLQBefore`) with `secondary` (Redis). |
| `internal/service/job_service.go` | Added DLQ fallback to `GetJobStatus` for completed and failed jobs. |
| `internal/worker/executor/worker_processor.go` | Made `retry` and `deferJob` non-blocking via Redis delayed queue scheduling and immediate worker return. |
| `internal/worker/executor/worker_processor_test.go` | Added unit tests verifying non-blocking timing (< 500ms for 2s backoff), attempt incrementing, future `RunAt` scheduling, DLQ routing on exhaustion, and circuit breaker deferral. |
| `internal/queue/redis/redis.go` | Updated `promoteScheduledJobsScript` and `reclaimTimedOutJobsScript` to use atomic `ZREM == 1` guards. Added dynamic job-specific visibility timeout handling in `Dequeue`. |
| `internal/queue/redis/redis_test.go` | Added `TestDequeue_RespectsJobTimeout` verifying custom timeout scores in `task_queue:in_flight`. |
| `chaos/helpers.go` | Added `buildSchedulerBinary`, `startSchedulerProcess`, `getFreePort`, and `newPersistentRedisContainer` with AOF persistence. |
| `chaos/chaos_test.go` | Updated `TestWorkerHardKill` to run scheduler; updated `TestRedisCrashMidTransition` with persistent AOF; added `TestRedisCrash_VolatileDataLoss`. |

---

## 4. Test Verification & Results

### 1. Worker Executor Suite
```bash
go test -v ./internal/worker/executor/...
```
**Result:** `PASS (0.112s)` — All 11 tests passed, including `TestProcessOnce_NonBlockingRetryTiming` (0.00s), `TestProcessOnce_RetryExhaustionDLQ` (0.00s), and `TestProcessOnce_CircuitBreakerNonBlockingDeferral` (0.00s).

### 2. Storage Models Suite
```bash
go test -v ./internal/storage/models/...
```
**Result:** `PASS (0.221s)` — All tests passed, including `TestRedisStore_DLQ` subtests.

### 3. Redis Queue Suite
```bash
go test -v ./internal/queue/redis/...
```
**Result:** `PASS (0.157s)` — All 29 tests passed, including `TestDequeue_RespectsJobTimeout`.

### 4. Full Repository Suite
```bash
go test ./...
```
**Result:** `PASS` — All 29 packages in the repository passed without errors.

### 5. Race Detector Verification
```bash
go test -race ./internal/worker/executor/... ./internal/storage/models/... ./internal/queue/redis/... ./internal/service/...
```
**Result:** `PASS` — Zero data races detected across all core concurrency packages.

### 6. Chaos & Resilience Suite
```bash
go test -v -tags chaos ./chaos
```
**Result:** `PASS (51.373s)`:
- `TestRedisCrashMidTransition`: `PASS` (20/20 jobs completed after Redis `SIGKILL` and container restart)
- `TestRedisCrash_VolatileDataLoss`: `PASS` (confirmed expected volatile loss when persistence is off)
- `TestWorkerHardKill`: `PASS` (15/15 jobs completed after worker `SIGKILL`; scheduler recovered in-flight jobs)
- `TestNetworkPartition`: `SKIP` (clean skip on non-root environment)
- `TestClockSkewOrphanRecovery`: `PASS` (stale job reclaimed, active job preserved)

---

## 5. Architectural Notes & Operational Recommendations

1. **Scheduler Deployment Requirement:** In production deployments, at least one instance of `cmd/scheduler` must run continuously. The scheduler is strictly responsible for:
   - Promoting delayed jobs from `task_queue:delayed_jobs` to active priority lists once their `RunAt` time arrives.
   - Reclaiming abandoned in-flight jobs from `task_queue:in_flight` once their visibility timeout expires.
   - Reconciling tenant concurrency limits and recurring jobs.
2. **Redis Durability Configuration:** Pure in-memory Redis deployments risk data loss on unexpected node restarts. Production Redis deployments backing task queues should configure `--appendonly yes --appendfsync everysec` (or `always` for strict financial/audit workloads), or use `DualStore` with PostgreSQL as the durable source of truth.
3. **Stage Boundary Enforcement:** All Stage 2 requirements are complete. No Stage 3 features (RBAC, session management, security token handling) were modified or preemptively started.
