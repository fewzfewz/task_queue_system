# STAGE 1 REPORT: CRITICAL FOUNDATION

## 1. Problems Discovered

During Stage 1 inspection and implementation, the following critical issues were audited, root-caused, and addressed:

1. **Test Suite Compilation & Signature Breakages (Task 1.1)**:
   - **`internal/worker/executor/worker_processor_test.go:292`**: Syntax error `func TestIsEmptyQueueErr(t *testing.T, 1)` caused by a regex/sed refactoring artifact that appended `, 1` to function signatures.
   - **`test/integration_test.go`**: Multiple compilation failures across lines 53, 62, 98, 107, 126, 154, and 164 where `, 1` was erroneously appended to calls to `service.New(...)`, `svc.CreateJob(...)`, and `time.Sleep(...)`.
   - **`chaos/chaos_test.go`**: Signature mismatch where calls to `svc.CreateJob(...)` (lines 50, 108, 164, 213, 222) were missing the 19th argument `ttlSeconds int` introduced in commit `f0608dd`.
   - **`internal/service/job_service_test.go:400-421` (`TestBulkPurgeDLQ`)**: Test fixture logical mismatch. `DeleteDLQBefore(ctx, tenant, type, before)` filters records based on `j.UpdatedAt.Before(before)`. The test fixture populated `CreatedAt` with an old timestamp but left `UpdatedAt` as `time.Now()`, causing 0 records to be deleted instead of 3.

2. **Inverted Redis Deduplication Logic (Task 1.2)**:
   - **`internal/storage/models/models.go:857` (`RedisStore.IsDedupKeyTaken`)**: Returned the raw boolean result of `s.client.SetNX(ctx, key, "1", 24*time.Hour).Result()`.
   - In Redis, `SetNX` returns `true` when a key **does not exist** and is newly created.
   - Returning `SetNX` directly resulted in `IsDedupKeyTaken` returning `true` (taken) on the initial submission, rejecting valid new jobs with `409 Conflict`. Conversely, subsequent identical submissions received `false` (not taken), allowing duplicate jobs through.
   - Hardcoded `24*time.Hour` instead of utilizing the package constant `dedupKeyTTL`.

3. **Partitioned Queue Size Calculation & Ingestion Backpressure Blindness (Task 1.3)**:
   - **`internal/queue/redis/redis.go:249` (`RedisQueue.Size`)**: Executed `LLen` against unpartitioned keys `q.qHigh`, `q.qMedium`, and `q.qLow`.
   - In production, jobs are sharded into hash partitions (`base:1`, `base:2`, ..., `base:q.numPartitions`). Unpartitioned base keys are never written to, causing `Size()` to permanently return `0`.
   - This broke `MAX_QUEUE_SIZE` ingestion backpressure in `JobService.CreateJob`, allowing unbounded queue growth.
   - `Size()` also ignored Redis errors by discarding them with blank identifiers `_`.

4. **Chaos Test Suite Flaws**:
   - **Rate Limiting**: `chaos/chaos_test.go` instantiated `q := queue_redis.New(rdb, "jobs")`, which applies a default rate limit of 10 req/s. The test scenarios enqueue 20–50 jobs in a tight loop under `tenant-1`, causing `CreateJob` to fail with `[TOO_MANY_REQUESTS] tenant rate limit exceeded`.
   - **Clock Skew Test Direction**: `TestClockSkewOrphanRecovery` set `queue_redis.Now` to `Now + 1 hour` while `freshJob` had a 30-second visibility timeout (`Now + 30s`). Advancing the clock by 1 hour caused both jobs to be in the past, reclaiming 2 jobs instead of 1.

---

## 2. Files Modified

| File | Changes Made |
|---|---|
| `internal/worker/executor/worker_processor_test.go` | Fixed syntax error on line 292 (`func TestIsEmptyQueueErr(t *testing.T)`). |
| `test/integration_test.go` | Stripped extraneous `, 1` arguments from calls to `service.New`, `svc.CreateJob`, and `time.Sleep`. |
| `chaos/chaos_test.go` | Added `ttlSeconds = 0` to `CreateJob` calls; set unlimited rate limit `NewWithRateLimit(rdb, "jobs", 0)`; fixed clock skew test to `Add(-1 * time.Hour)`. |
| `internal/service/job_service_test.go` | Set `UpdatedAt` in `TestBulkPurgeDLQ` fixture; added `TestCreateJob_Deduplication`. |
| `internal/storage/models/models.go` | Inverted `SetNX` logic in `RedisStore.IsDedupKeyTaken` (`return !set, nil`); used `dedupKeyTTL`; propagated Redis errors. |
| `internal/storage/models/models_test.go` | Added unit test suite `TestRedisStore_IsDedupKeyTaken` covering first submission, duplicates, TTL expiry, multi-tenant isolation, concurrency, and error handling. |
| `internal/queue/redis/redis.go` | Rewrote `RedisQueue.Size()` to aggregate `LLen` across all priority tiers and partitions `1..numPartitions`; properly propagated Redis errors. |
| `internal/queue/redis/redis_test.go` | Updated `TestSize_WithJobs` to assert `Size() == 3`; added `TestRedisQueue_Size` suite (empty queue, priority tiers, multi-partition distribution, in-flight exclusion, error propagation, backpressure integration). |

---

## 3. Exact Changes Implemented

### Task 1.1: Test Suite Restoration
- **`internal/worker/executor/worker_processor_test.go`**:
  ```go
  // Before:
  func TestIsEmptyQueueErr(t *testing.T, 1) {
  // After:
  func TestIsEmptyQueueErr(t *testing.T) {
  ```
- **`test/integration_test.go`**:
  Restored valid parameter counts across 7 call sites for `service.New`, `svc.CreateJob`, and `time.Sleep`.
- **`internal/service/job_service_test.go`**:
  Updated `TestBulkPurgeDLQ` to explicitly set `j.UpdatedAt = old` on the 3 cutoff jobs and `j.UpdatedAt = recent` on the 2 retained jobs, matching the semantics of `DeleteDLQBefore`.

### Task 1.2: Redis Deduplication Fix
- **`internal/storage/models/models.go`**:
  ```go
  func (s *RedisStore) IsDedupKeyTaken(ctx context.Context, dedupKey, tenantID string) (bool, error) {
      if dedupKey == "" {
          return false, nil
      }
      key := fmt.Sprintf("dedup:%s:%s", tenantID, dedupKey)
      set, err := s.client.SetNX(ctx, key, "1", dedupKeyTTL).Result()
      if err != nil {
          return false, fmt.Errorf("redis_store: SetNX failed for dedup key %s: %w", key, err)
      }
      return !set, nil
  }
  ```

### Task 1.3: Redis Queue Size Fix
- **`internal/queue/redis/redis.go`**:
  ```go
  // Size returns the total number of pending jobs across all priority queues and partitions.
  func (q *RedisQueue) Size(ctx context.Context) (int64, error) {
      numPartitions := q.numPartitions
      if numPartitions <= 0 {
          numPartitions = 1
      }

      var total int64
      for i := 1; i <= numPartitions; i++ {
          for _, base := range []string{q.qHigh, q.qMedium, q.qLow} {
              key := fmt.Sprintf("%s:%d", base, i)
              n, err := q.client.LLen(ctx, key).Result()
              if err != nil {
                  return 0, fmt.Errorf("queue: LLen failed for %s: %w", key, err)
              }
              total += n
          }
      }
      return total, nil
  }
  ```

---

## 4. Tests Added

1. **`internal/storage/models/models_test.go: TestRedisStore_IsDedupKeyTaken`**:
   - `Empty dedup key is never taken`: Verifies empty string returns `false, nil`.
   - `First submission succeeds and repeated submission is rejected`: Validates initial submission returns `false` (claimed) and duplicate returns `true` (rejected).
   - `Expiration behavior resets deduplication after TTL`: Verifies Redis TTL is 24h; fast-forwards miniredis by 25h to confirm key becomes claimable again.
   - `Different deduplication keys and tenant isolation`: Confirms distinct keys on the same tenant don't collide and identical keys across distinct tenants (`tenant-x`, `tenant-y`) remain isolated.
   - `Concurrent submissions allow only one winner`: Launches 25 concurrent goroutines competing for the same dedup key; asserts exactly 1 succeeds and 24 are rejected with 0 errors.
   - `Propagates Redis errors`: Confirms connection errors return `false, err`.

2. **`internal/service/job_service_test.go: TestCreateJob_Deduplication`**:
   - End-to-end service test verifying `svc.CreateJob` successfully enqueues on first submission, returns `errors.CodeConflict` (`409 Conflict`) on duplicate submission, and allows different keys.

3. **`internal/queue/redis/redis_test.go: TestRedisQueue_Size`**:
   - `Empty queue returns 0`: Verifies `Size() == 0` on an empty Redis instance.
   - `Only high-priority jobs across partitions`: Verifies high-priority jobs across partitions are correctly counted.
   - `Only medium-priority jobs across partitions`: Verifies medium-priority jobs across partitions are correctly counted.
   - `Only low-priority jobs across partitions`: Verifies low-priority jobs across partitions are correctly counted.
   - `Jobs distributed across multiple partitions and priorities`: Enqueues 15 jobs with mixed priorities via `Enqueue` and verifies `Size() == 15`.
   - `In-flight jobs are not counted as pending in Size`: Adds 2 pending queue jobs and 3 in-flight ZSET jobs; asserts `Size() == 2`.
   - `Redis error propagation`: Validates error return when Redis client is unreachable.
   - `Backpressure integration with JobService`: Configures `JobService` with `maxQueueSize = 2` on `RedisQueue`; verifies jobs 1 and 2 succeed, queue size reports 2, and job 3 fails with `errors.CodeQueueFull`.

---

## 5. Test Commands Executed

```bash
gofmt -w chaos/chaos_test.go internal/queue/redis/redis.go internal/queue/redis/redis_test.go internal/service/job_service_test.go internal/storage/models/models.go internal/storage/models/models_test.go internal/worker/executor/worker_processor_test.go test/integration_test.go
go test -count=1 ./internal/worker/executor/...
go test -count=1 ./internal/storage/models/...
go test -count=1 ./internal/queue/redis/...
go test -count=1 ./internal/service/...
go test ./...
go test -race ./...
go test -v -tags chaos -run TestClockSkewOrphanRecovery ./chaos
go test -v -tags chaos ./chaos
```

---

## 6. Test Results

| Command / Suite | Status | Execution Details |
|---|---|---|
| `gofmt -w` | **PASS** | Formatted all 8 modified files cleanly. |
| `go test ./internal/worker/executor/...` | **PASS** | Exit code 0 (2.113s). |
| `go test ./internal/storage/models/...` | **PASS** | Exit code 0 (0.115s). All 18 tests passed. |
| `go test ./internal/queue/redis/...` | **PASS** | Exit code 0 (0.157s). All 22 tests passed. |
| `go test ./internal/service/...` | **PASS** | Exit code 0 (0.005s). All 16 tests passed. |
| `go test ./...` | **PASS** | Exit code 0. All 38 packages passed. |
| `go test -race ./...` | **PASS** | Exit code 0. No data races detected across the entire codebase. |
| `go test -v -tags chaos -run TestClockSkewOrphanRecovery ./chaos` | **PASS** | Exit code 0 (6.49s). 1 stale job reclaimed, 1 fresh job preserved. |

### Chaos Suite Environmental / Architectural Findings
When running the full Docker-backed chaos test suite (`go test -v -tags chaos ./chaos`):
- `TestClockSkewOrphanRecovery`: **PASS** (6.18s).
- `TestNetworkPartition`: **SKIP** (`network partition scenario requires root privileges for iptables`).
- `TestRedisCrashMidTransition` & `TestWorkerHardKill`: **FAIL** due to underlying architectural characteristics:
  1. `TestRedisCrashMidTransition`: Tests `SIGKILL` against a Docker Redis container without AOF enabled (`redis:7-alpine` default is pure in-memory). Killing Redis with `SIGKILL` deletes all volatile state.
  2. `TestWorkerHardKill`: When a worker is hard-killed, in-flight jobs remain in `task_queue:in_flight`. In this architecture, in-flight job recovery is executed exclusively by `cmd/scheduler` (`ReclaimTimedOutJobs`). Because the test harness only starts a worker binary and does not start the scheduler daemon, timed-out jobs are never swept back into the pending queues.

---

## 7. Remaining Issues

1. **DLQ Persistence in RedisStore (Stage 2 scope)**:
   `RedisStore.MoveToDLQ`, `ListDLQ`, `CountDLQ`, and `DeleteDLQBefore` remain stubbed no-ops in `internal/storage/models/models.go:1067-1077`.
2. **Worker Exponential Backoff Sleep (Stage 2 scope)**:
   Worker exponential backoff still uses blocking `time.Sleep` within worker processor goroutines rather than requeuing to delayed ZSET with a future `run_at`.
3. **Stand-alone Scheduler Dependency for Worker Crash Recovery (Stage 2 scope)**:
   In-flight orphan reclamation is tightly coupled to running `cmd/scheduler`.

---

## 8. Known Risks

1. **Redis Non-Persistent Deployments**:
   If Redis is deployed without AOF (`appendonly yes`), a container kill or power loss will result in loss of pending and in-flight queue items unless mirrored in Postgres via `DualStore`.
2. **High-Partition Count Overhead**:
   `RedisQueue.Size()` queries `3 * numPartitions` keys via sequential `LLen`. For the default of 3 partitions (9 keys total), execution takes <0.1ms; if `numPartitions` is significantly increased, a Redis pipeline or Lua script will be preferable.

---

## 9. Stage 1 Acceptance Criteria Verification

- [x] All syntax errors and function signature mismatches in the test suite resolved.
- [x] All unit and integration test packages compile and pass cleanly without skips or weakens.
- [x] `go test -race ./...` completes with zero race condition warnings across all packages.
- [x] `RedisStore.IsDedupKeyTaken` correctly permits first submission, rejects duplicate submissions, preserves TTL, isolates tenants, and handles concurrency.
- [x] `RedisQueue.Size()` correctly aggregates pending jobs across all partitions and priority tiers, excludes in-flight jobs, handles errors, and enforces `MAX_QUEUE_SIZE` backpressure in `JobService`.
- [x] No modifications made beyond Stage 1 scope.
