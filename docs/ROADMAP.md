# Roadmap & Known Gaps

## Legend

- **Priority:** Critical / High / Medium / Low
- **Effort:** Small (< 1h) / Medium (1-4h) / Large (4-16h) / X-Large (> 16h)

---

## Still Open

### High Priority

| # | Item | Location | Effort | Notes |
|---|------|----------|--------|-------|
| 1 | **Webhook dispatcher Start() untested** | `internal/webhooks/dispatcher.go` | Medium | `send()` and `sign()` tested, but `Start()` loops on Redis Streams — needs integration test |
| 2 | **Chaos tests not CI-integrated** | `chaos/` | Medium | Requires Docker + root. CI pipeline step documented in `chaos/README.md` but not wired into any CI config |

### Low Priority / Nice-to-Have

| # | Item | Location | Effort | Notes |
|---|------|----------|--------|-------|
| 3 | **API HPA with custom metrics (request rate)** | `deploy/helm/task-queue/` | Medium | HPA currently uses CPU/memory only — could scale on request rate via Prometheus adapter |
| 4 | **OpenTelemetry context propagation across queue boundaries** | `internal/tracing/` | Medium | OTel SDK + OTLP exporter wired in. Trace IDs flow through request logging but not yet propagated across Enqueue/Dequeue boundaries |
| 5 | **Helm chart production hardening** | `deploy/helm/task-queue/` | Medium | Chart skeleton exists. Needs: CI-tested install/upgrade, pod disruption budgets, topology spread constraints |
| 6 | **Postgres migration improvements** | `cmd/cli/` | Small | Migrations work but have no rollback or version tracking |

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
