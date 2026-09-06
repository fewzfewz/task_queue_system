# Roadmap & Known Gaps

## Legend

- **Priority:** Critical / High / Medium / Low
- **Effort:** Small (< 1h) / Medium (1-4h) / Large (4-16h) / X-Large (> 16h)

---

## Still Open

### High Priority

| # | Item | Location | Effort | Notes |
|---|------|----------|--------|-------|
None — all items completed.

---

## Recently Completed

| # | Item | Tests Added | Lines |
|---|------|-------------|-------|
| ✅ | Removed empty packages (`internal/utils/`, `pkg/middleware/`, `pkg/`) | — | — |
| ✅ | Fixed API/worker port conflict (added `WORKER_PORT`, `SCHEDULER_PORT`) | — | — |
| ✅ | Added HTTP server (health/metrics) to scheduler | — | — |
| ✅ | Fixed Postgres `UpdateResult` JSONB race condition (atomic `jsonb` append) | — | — |
| ✅ | Made per-tenant rate limit configurable via `TENANT_RATE_LIMIT` env var | — | — |
| ✅ | Fixed `GetJobStatus` returning INTERNAL_ERROR instead of NOT_FOUND | — | — |
| ✅ | Created `deploy/k8s/namespace.yaml` | — | — |
| ✅ | Added `.gitignore` | — | — |
| ✅ | Removed pre-built binaries from `bin/` | — | — |
| ✅ | Implemented `RecoverOrphans` for `InMemoryStore` and `RedisStore` | — | — |
| ✅ | Fixed K8s namespace inconsistency (`default` → `task-queue` in all manifests) | — | — |
| ✅ | Fixed scheduler K8s deployment: `SCHEDULER_PORT`, HTTP probes | — | — |
| ✅ | Fixed worker K8s deployment: port 8081, correct probes | — | — |
| ✅ | Fixed Redis FQDN references (`redis.default` → `redis.task-queue`) | — | — |
| ✅ | **PostgreSQL connection retry** with exponential backoff (1s, 2s, 4s, … ×5) | — | `postgres.go` |
| ✅ | **Processed ID cleanup** (TTL-backed keys, bounded memory) | — | `redis.go` |
| ✅ | **Configurable SLA target** via `SLA_TARGET_SECONDS` env | — | `worker_processor.go` |
| ✅ | **Complete shutdown_test coverage** | 5 new tests (GET, double-initiate, method-not-allowed, done-waits, shutdown flow) | `shutdown_test.go` |
| ✅ | **Ingress TLS cert-manager Certificate** | — | `deploy/k8s/certificate.yaml` |
| ✅ | **NetworkPolicy manifests** | — | `deploy/k8s/network-policy.yaml` |
| ✅ | **API HPA manifest** | — | `deploy/k8s/api-hpa.yaml` |
| ✅ | **RBAC per workload** (separate Role/RoleBinding for api, worker, scheduler) | — | `deploy/k8s/rbac.yaml` |
| ✅ | **Script portability fixes** (benchmark.sh fallback for `ab`+`jq`, load_test.sh fallback for `bc`) | — | `scripts/` |
| ✅ | **Chaos test docs** (CHAOS.md with prerequisites, scenarios, CI snippet) | — | `chaos/README.md` |
| ✅ | **Secrets management docs** (env vars, K8s Secrets, Vault tiers) | — | `docs/secrets-management.md` |
| ✅ | **OpenTelemetry/Jaeger exporter** (OTel SDK + OTLP/HTTP, no-op fallback, graceful shutdown) | — | `internal/tracing/tracing.go` |
| ✅ | **Helm chart skeleton** (Chart.yaml, values.yaml, all templates) | — | `deploy/helm/task-queue/` |
| ✅ | **Worker pool tests** | 7 tests | `pool_test.go` |
| ✅ | **Redis queue tests** | 20 tests (miniredis) | `redis/redis_test.go` |
| ✅ | **Plugin tests (email + image)** | 12 tests | `plugins/standard/plugins_test.go` |
| ✅ | **Route tests** | 3 tests | `routes/routes_test.go` |
| ✅ | **Postgres store integration tests** | 10 tests (skip without `POSTGRES_CONN_STR`) | `test/postgres_store_integration_test.go` |
| ✅ | Created `internal/queue/mock.go` | — | — |
| ✅ | Updated `deploy/k8s/rbac.yaml` apply order comment | — | — |
| ✅ | **DAG dependency checks** in dequeue (SQL subquery filters blocked jobs) | — | `postgres.go` |
| ✅ | **Shard routing** via `Store.Dequeue(ctx, tenantID, shardKey)` parameter | — | `models.go`, `postgres.go` |
| ✅ | **TTL auto-cleanup** with 7-day retention, ~90s loop in scheduler | — | `cmd/scheduler/main.go` |
| ✅ | **Pagination metadata** in `ListJobs` (`total`, `page`, `limit`) | — | `handler.go` |
| ✅ | **Stats API** (`GET /api/v1/stats`) with queue/worker breakdown | — | `handler.go` |
| ✅ | **Circuit breaker reset** via worker HTTP (port 8081) | — | `circuitbreaker.go`, `cmd/worker/main.go` |
| ✅ | **DAG visualization** (`GET /api/v1/jobs/{id}/deps`) with upstream + downstream | — | `handler.go` |
| ✅ | **Progress reporting** via `plugin.WithProgressCallback` / `plugin.ReportProgress` | — | `plugin/interface.go`, `worker_processor.go` |
| ✅ | **Full HTML UI rewrite** with 7 tabs, pagination, toast notifications, circuit breaker controls | — | `ui_handler.go` |
| ✅ | **Webhook dispatcher Start() integration test** | `TestDispatcherStartIntegration` | `dispatcher_integration_test.go` |
| ✅ | **Chaos tests CI-integrated** (GitHub Actions workflow) | — | `.github/workflows/chaos.yml` |
| ✅ | **API HPA request rate metric** (`task_queue_api_request_total`) | — | `metrics.go`, `api-hpa.yaml` |
| ✅ | **OTel context propagation** (trace ID across Enqueue/Dequeue) | — | `tracing.go`, `redis.go`, `worker_processor.go` |
| ✅ | **Helm chart hardening** (PDBs + topology spread constraints) | — | `helm/task-queue/templates/pdb-*.yaml`, deployment templates |
| ✅ | **Postgres migration rollback** (rollback SQL + CLI commands) | — | `cmd/cli/main.go`, `db/migrations/*_rollback.sql` |
| ✅ | **Tenant isolation** on job/DLQ endpoints for registered clients | `access_test.go` | `handler/access.go` |
| ✅ | **Client API key revoke/rotate** (API + operator UI) | — | `client_handler.go`, `job_service.go` |
| ✅ | **Efficient job counts** via `CountJobs` (no full-table scans) | `access_test.go` | `models.go`, store backends |
| ✅ | **Registration rate limit** via `REGISTER_RATE_LIMIT` | — | `client_handler.go`, `config.go` |
| ✅ | **Real SMTP email plugin** when `SMTP_HOST` is set | `plugins_test.go` | `plugins/standard/email.go` |
| ✅ | **`created` webhook event** on job submission | — | `handler.go`, `job_service.go` |
| ✅ | **Custom job types registry** (`email`, `image`, `http` handlers) | `jobtypes/store_test.go` | `internal/jobtypes/` |
| ✅ | **Webhook payload hardening** (secret excluded from POST body) | `dispatcher_test.go` | `webhooks/dispatcher.go` |
| ✅ | **Job Deduplication** (Postgres unique constraint + 409 Conflict) | — | `job_service.go`, `migrations` |
| ✅ | **High-Performance Batching** (True bulk `INSERT` for `/jobs/batch`) | — | `job_service.go`, `postgres.go` |
| ✅ | **Tenant Rate Limiting & QoS** (Redis `active_tenant` tracking + deferred queue) | — | `redis.go`, `client_handler.go` |
| ✅ | **Worker Circuit Breaker Backpressure** (Defers instead of burning retries) | — | `worker_processor.go`, `job_executor.go` |
| ✅ | **DLQ Management Hardening** (Bounded Redis `LTRIM` to prevent OOM) | — | `redis.go` |
| ✅ | **Swagger docs regenerated** | — | `docs/swagger.yaml` |

## Next-Generation (Enterprise) Roadmap
*(Future capabilities to compete with Temporal/Step Functions)*
- [x] Multi-Language Worker SDKs (Python/Node.js integration via HTTP Push)
- [x] Map-Reduce (Fan-out / Fan-in) Primitives
- [x] Middleware / Interceptor Pipeline
- [x] Operator "Panic Buttons" (Manual Queue Pausing)
- [ ] Full-Text Payload Search in the UI
- [ ] Job-Level TTL (Time-To-Live)
