# Roadmap & Known Gaps

## Legend

- **Priority:** Critical / High / Medium / Low
- **Effort:** Small (< 1h) / Medium (1-4h) / Large (4-16h) / X-Large (> 16h)

---

## Still Open

### High Priority

| # | Item | Location | Effort | Notes |
|---|------|----------|--------|-------|
| 1 | **Webhook dispatcher Start() untested** | `internal/webhooks/dispatcher.go` | Medium | `Start()` reads from Redis Streams — needs a mock or integration test |
| 2 | **shutdown_test coverage gaps** | `cmd/worker/shutdown_test.go` | Small | Only 1 test. Missing: timeout scenario, double-initiate, GET vs POST method handling |
| 3 | **Chaos tests not CI-integrated** | `chaos/` | Medium | Requires Docker + root. No CI pipeline runs these |

### Medium Priority

| # | Item | Location | Effort | Notes |
|---|------|----------|--------|-------|
| 4 | **No NetworkPolicy** | `deploy/k8s/` | Medium | No pod-level network isolation |
| 5 | **No cert-manager resources** | `deploy/k8s/ingress.yaml` | Small | Ingress references `letsencrypt-prod` issuer but no Certificate resource is included |
| 6 | **No HPA for API** | `deploy/k8s/` | Small | Only worker has autoscaling — API is a single replica |
| 7 | **Missing chaos test docs** | `chaos/` | Small | No README explaining setup (Docker, root, iptables) |
| 8 | **benchmark.sh assumes `ab` + `jq`** | `scripts/benchmark.sh` | Small | Not portable. Consider adding a check or fallback to curl |
| 9 | **load_test.sh uses `bc`** | `scripts/load_test.sh` | Small | Not available on all systems |
| 10 | **PostgreSQL connection retry** | `internal/storage/postgres/postgres.go` | Small | Store fails immediately if Postgres is unavailable on startup. No retry/backoff |

### Low Priority / Nice-to-Have

| # | Item | Location | Effort | Notes |
|---|------|----------|--------|-------|
| 11 | **OpenTelemetry / Jaeger exporter** | `internal/tracing/` | Large | Trace ID is already in request logging. Need OTEL SDK + exporter + context propagation across queue boundaries |
| 12 | **Helm chart** | `deploy/helm/` | X-Large | Package all K8s manifests into a versioned chart with values.yaml |
| 13 | **Ingress manifest** | `deploy/k8s/ingress.yaml` | Medium | Exists as a skeleton but needs TLS cert configuration and path routing |
| 14 | **RBAC per workload** | `deploy/k8s/rbac.yaml` | Medium | Single set of ServiceAccounts/Roles. Should be one per workload (api, worker, scheduler) |
| 15 | **Secrets management workflow** | `docs/` | Medium | Document production secret handling for K8s, Vault, and environment variables |
| 16 | **Rate limit per-tenant from UI** | `internal/queue/redis/redis.go` | Small | Rate limit is now configurable via env var but not exposed via any API endpoint |
| 17 | **SLA target configurable** | `internal/worker/executor/worker_processor.go` | Small | SLA target is hardcoded at 5 seconds |
| 18 | **API HPA with custom metrics** | `deploy/k8s/` | Medium | Only workers autoscale. API could scale based on request rate |
| 19 | **Cleanup expired processed IDs** | `internal/queue/redis/redis.go` | Medium | The `processed` set grows unboundedly — needs periodic cleanup |

---

## Recently Completed

| # | Item | Tests Added | Lines |
|---|------|-------------|-------|
| ✅ | Removed empty `internal/utils/` and `pkg/middleware/` packages | — | — |
| ✅ | Fixed API/worker port conflict (added `WORKER_PORT`, `SCHEDULER_PORT`) | — | — |
| ✅ | Added HTTP server (health/metrics) to scheduler | — | — |
| ✅ | Fixed Postgres `UpdateResult` JSONB race condition (atomic `jsonb` append) | — | — |
| ✅ | Made per-tenant rate limit configurable via `TENANT_RATE_LIMIT` env var | — | — |
| ✅ | Fixed `GetJobStatus` returning INTERNAL_ERROR instead of NOT_FOUND | — | — |
| ✅ | Removed empty `pkg/` directory | — | — |
| ✅ | Created `deploy/k8s/namespace.yaml` | — | — |
| ✅ | Added `.gitignore` with `bin/`, `.env`, IDE files | — | — |
| ✅ | Removed pre-built binaries from `bin/` | — | — |
| ✅ | Implemented `RecoverOrphans` for `InMemoryStore` and `RedisStore` | — | — |
| ✅ | Fixed K8s namespace inconsistency (`default` → `task-queue` in all manifests) | — | — |
| ✅ | Fixed scheduler K8s deployment: added `SCHEDULER_PORT`, HTTP liveness/readiness probes | — | — |
| ✅ | Fixed worker K8s deployment: updated port to 8081, correct probes | — | — |
| ✅ | Fixed Redis FQDN references (`redis.default` → `redis.task-queue`) | — | — |
| ✅ | **Worker pool tests** | 7 tests | pool_test.go |
| ✅ | **Redis queue tests** | 20 tests | redis/redis_test.go |
| ✅ | **Plugin tests (email + image)** | 12 tests | plugins/standard/plugins_test.go |
| ✅ | **Route tests** | 3 tests | routes/routes_test.go |
| ✅ | **Postgres store integration tests** | 10 tests (gated behind `POSTGRES_CONN_STR`) | test/postgres_store_integration_test.go |
| ✅ | Created `internal/queue/mock.go` (blocking `MockQueue` with channel-based `Dequeue`) | — | — |
| ✅ | Updated `deploy/k8s/rbac.yaml` apply order comment | — | — |
