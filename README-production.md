# Production Deployment Guide

This document describes the current production-oriented deployment path for the task queue system.

## What Gets Deployed

The system runs as four processes:

- API server
- worker pool
- scheduler
- Redis queue/backing store

PostgreSQL is optional, but required if you want durable job storage with `STORE_BACKEND=postgres` or `STORE_BACKEND=dual`.

## Operator Terminal

There is no in-app terminal. Day-to-day operator work happens from your shell:

- `kubectl` for cluster operations
- `make` for local tasks
- `go run ./cmd/cli ...` for migration and maintenance jobs
- `./scripts/chaos.sh ...` for chaos reporting

The browser UI is for job and DLQ operations, but it is not a shell replacement.

## Namespace

Create a dedicated namespace:

```bash
kubectl create namespace task-queue
```

Then apply manifests into that namespace or patch the YAML `metadata.namespace` fields to `task-queue`.

## Secrets

### API key

Store the API key in a Kubernetes Secret:

```bash
kubectl -n task-queue create secret generic task-queue-secrets \
  --from-literal=api-key='replace-me'
```

The API deployment reads it as `API_KEY`.

### Postgres connection string

Store the DSN in a separate secret:

```bash
kubectl -n task-queue create secret generic task-queue-postgres \
  --from-literal=conn-str='postgres://user:pass@host:5432/dbname?sslmode=require'
```

Then inject it as `POSTGRES_CONN_STR` for the API, worker, or any pod using Postgres.



## Storage Initialization

### Redis

Apply:

- `deploy/k8s/redis-service.yaml`
- `deploy/k8s/redis-statefulset.yaml`

### Postgres schema

Before using `STORE_BACKEND=postgres` or `STORE_BACKEND=dual`, apply the schema:

```bash
make migrate-schema
```

This uses the built-in versioned SQL migration runner and tracks applied versions in `schema_migrations`.

## Application Deployments

Apply:

- `deploy/k8s/rbac.yaml`
- `deploy/k8s/api-deployment.yaml`
- `deploy/k8s/api-service.yaml`
- `deploy/k8s/worker-deployment.yaml`
- `deploy/k8s/scheduler-deployment.yaml`
- `deploy/k8s/hpa.yaml`
- `deploy/k8s/prometheus-adapter-configmap.yaml`
- `deploy/monitoring/prometheus-rules.yaml`
- `deploy/grafana/dashboard.json`
- `deploy/grafana/provisioning/dashboards/dashboards.yaml`
- `deploy/grafana/provisioning/datasources/datasource.yaml`

## Ingress and TLS

The repository does not ship a full ingress manifest yet. In production:

- terminate TLS at your ingress controller or load balancer
- route the public host to `task-queue-api`
- keep Redis and Postgres private inside the cluster

Typical setup:

- NGINX Ingress or Traefik
- cert-manager for TLS certificates
- a `ClusterIP` service for the API

## RBAC and Service Accounts

The current manifests do not include dedicated service accounts or RBAC rules.

Recommended production hardening:

- create a service account per workload
- give the API only the permissions it needs
- keep worker and scheduler permissions minimal
- mount secrets through Kubernetes Secrets or a CSI driver

## Readiness and Liveness

The services expose:

- `GET /healthz`
- `GET /readyz`

The worker also exposes:

- `POST /healthz/shutdown`
- `GET /metrics`

Use those endpoints for probes and graceful shutdown hooks.

## Postgres Integration Tests

The Postgres store path is covered by a dedicated integration suite that runs
against a real PostgreSQL instance — no mocking, no build tags. The tests are
opt-in and gated on the `POSTGRES_CONN_STR` environment variable, so the fast
unit suite (`go test ./...`) never needs a database.

### Locally

The easiest way is the one-command Makefile target, which starts a Postgres
container from `deploy/test/docker-compose.yml`, runs the suite, and tears the
container down:

```bash
make test-postgres
```

Under the hood this runs:

```bash
POSTGRES_CONN_STR='postgres://tq:tq@localhost:5433/tq?sslmode=disable' \
POSTGRES_MIGRATIONS_DIR='<repo>/db/migrations' \
go test -count=1 ./test/... ./internal/storage/...
```

If you want the database to stay up between runs:

```bash
make test-postgres-up     # start the test DB only
make test-postgres        # run against the running container
make test-postgres-down   # stop it
```

Or point the suite at any Postgres you already run:

```bash
POSTGRES_CONN_STR='postgres://user:pass@host:5432/db?sslmode=disable' \
POSTGRES_MIGRATIONS_DIR='$(pwd)/db/migrations' \
go test -count=1 ./test/... ./internal/storage/...
```

`POSTGRES_MIGRATIONS_DIR` is optional when the suite can find `db/migrations`
from the repository root; it is always required to be set from CI because Go
runs each test package from its own directory.

What the suite covers against the real Postgres backend:

- job save/get/enqueue round-trips (payload, priority, dedup key, shard key,
  webhook config, DAG dependencies, progress)
- update status/progress/result, error history accumulation
- dequeue with SKIP LOCKED: priority tiers (high > medium > low), scheduled
  gating, tenant and shard filtering, DAG dependency blocking/release
- heartbeats, orphan recovery, completion/failure/requeue
- list/search filters and pagination, `GetByIDs`, dedup idempotency markers,
  queue length stats, job deletion and TTL cleanup

The `jobs` table is truncated before and after every test so runs are isolated
and repeatable.

### In CI

`test.yml` runs a dedicated `test-postgres` job that spins up a `postgres:16`
service container and executes the same suite with `-race`. The Postgres tests
are intentionally excluded from the fast test job so the default CI run needs
no database beyond Redis.

## Webhook Integration Tests

The webhook delivery suite lives in `internal/webhooks/` and is split in two:

- **Default-run delivery tests** (`delivery_integration_test.go`) — fully
  self-contained using `httptest` receivers and in-memory Redis, so they run
  as part of `go test ./...` with no external services:
  - HMAC-SHA256 signature verification on the receiving end (valid secret
    accepted, wrong secret rejected as a 4xx without retry)
  - payload integrity across the event types the system emits (`completed`,
    `failed`) — exact body, method, content-type, and timestamp
  - exponential backoff schedule with jitter bounds, give-up after `MaxRetries`
  - retry on 5xx and on client timeout; no retry on 4xx
  - 50-way concurrent deliveries with a goroutine-leak check
- **Redis-pipeline test** (`dispatcher_integration_test.go`) — gated behind
  `RUN_QUEUE_INTEGRATION=1` and drives the real stream → consumer-group →
  dispatcher path, verifying the delivered webhook signature end-to-end.

### Locally

```bash
make test-webhooks
```

This starts a dedicated Redis container (`task_queue_redis_test`) on host port
`:6380`, runs `RUN_QUEUE_INTEGRATION=1 go test -race ./internal/webhooks/`, and
removes the container afterwards. It uses a separate port so it never collides
with a running `make dev`/compose stack on `:6379`, and the gated test reads
`REDIS_ADDR` (default `localhost:6379`) to find Redis. If Redis is
unreachable the gated tests skip.

### In CI

`test.yml` runs a dedicated `test-webhooks` job with a `redis:7` service
container and `RUN_QUEUE_INTEGRATION=1`. The default-run delivery tests are
already covered by the fast `test` job.

## Vault Integration Tests

The Vault suite lives in `internal/secrets/vault_integration_test.go`. It tests
the `VaultSecretsProvider`'s TTL caching contract against a real Vault dev
server (AppRole auth + KV v2), using a counting HTTP transport so assertions are
made on actual Vault calls rather than mocks:

- login via AppRole (`role-id` + `secret-id`) and KV v2 secret retrieval
- value served from cache until the TTL expires (confirmed by changing the
  underlying secret and observing the old value is still returned, with only 1
  Vault read)
- re-fetch after TTL expiry picks up the new value
- stale-while-revalidate: a failed background refresh inside the pre-expiry
  window still serves the stale cached value; after full expiry the provider
  returns an error instead of stale data
- single-flight coalescing: 20 concurrent cold-miss readers cause exactly 1 Vault
  read, and 20 concurrent in-window readers trigger a single coalesced refresh
  (no stampede)

If no Vault server is reachable the tests skip, so `go test ./...` needs no
extra services.

### Locally

```bash
make test-vault
```

This starts a Vault dev server (`hashicorp/vault:1.18` in dev mode, root token
`root`) from `deploy/test/docker-compose.vault.yml` on host port `:8200`, runs
`go test -race ./internal/secrets/`, and tears the container down. Override the
endpoint with `VAULT_ADDR` / `VAULT_DEV_ROOT_TOKEN_ID` to run against an
existing dev server.

### In CI

`test.yml` runs a dedicated `test-vault` job with a `hashicorp/vault:1.18`
service container (dev mode, unsealed) on `:8200`.

## Chaos CLI

`cmd/chaos` is a standalone binary that runs failure-injection scenarios
against a **live** deployment and writes a structured JSON report with an
SLO-based pass/fail verdict. It complements the build-tagged in-process suite
in `chaos/` (which boots its own ephemeral stack via `go test -tags chaos`).

### Build

```sh
go build -o bin/chaos ./cmd/chaos/
```

### Scenarios

| Scenario         | Fault injected                                                   |
| ---------------- | ---------------------------------------------------------------- |
| `redis-crash`    | Stop (SIGTERM) or kill (SIGKILL) the Redis container, restart it after a fault window |
| `worker-kill`    | Stop a worker container mid-processing; the rest of the pool and the scheduler's in-flight reclaim finish the jobs |
| `redis-partition`| Isolate Redis from the compose network (or drop packets via iptables on a root host), restore after a fault window |
| `orphan-reclaim` | Forge stale (`now - 2h`) and fresh (`now + 30s`) in-flight visibility scores; the scheduler must reclaim only the stale job |

### Usage

```sh
chaos list                                   # list scenarios
chaos run <scenario> [flags]                 # run one scenario
chaos run <scenario> --dry-run               # validate config + reachability, no fault
chaos run <scenario> --output report.json    # write report to a file (default: stdout)
```

Every option is a flag with a `CHAOS_*` environment fallback; run
`chaos run --help` for the full list. Common ones:

| Flag                | Env                    | Default                     |
| ------------------- | ---------------------- | --------------------------- |
| `--api-url`         | `CHAOS_API_URL`        | `http://localhost:8080`     |
| `--api-key`         | `CHAOS_API_KEY`        | `secret-api-key`            |
| `--redis-addr`      | `CHAOS_REDIS_ADDR`     | `localhost:6379`            |
| `--redis-container` | `CHAOS_REDIS_CONTAINER`| `task_queue_redis` (auto-detect workers via compose labels) |
| `--worker-container`| `CHAOS_WORKER_CONTAINER`| empty (first compose worker auto-detected) |
| `--jobs`            |                        | `20`                        |
| `--fault-duration`  |                        | `5s`                        |
| `--sla-timeout`     |                        | `120s`                      |
| `--slo-success`     |                        | `1.0`                       |
| `--slo-max-dlq`     |                        | `0`                         |

Examples:

```sh
# sanity-check reachability against the deployed stack, no fault injected
bin/chaos run redis-crash --dry-run

# crash Redis hard for 10s and require 95% of jobs to survive
bin/chaos run redis-crash --crash-kind kill --fault-duration 10s \
  --slo-success 0.95 --output /tmp/redis-crash.json

# stop one worker and confirm the pool + reclaim finish the batch
bin/chaos run worker-kill --output /tmp/worker-kill.json

# forge an orphan and confirm the scheduler reclaims it within the SLO
bin/chaos run orphan-reclaim --output /tmp/reclaim.json
```

`--dry-run` probes the API, Redis, and container targets without injecting any
fault; run it before every scenario against a new deployment.

### Report format

The report is a single JSON object (indented, written to `--output` or stdout):

```json
{
  "scenario": "orphan-reclaim",
  "started_at": "2026-07-31T14:56:43Z",
  "ended_at": "2026-07-31T14:56:47Z",
  "duration_ms": 4369,
  "config": { "api_url": "...", "redis_addr": "...", "jobs": 20, "...": "..." },
  "fault": { "type": "forge-in-flight-visibility", "target": "...", "injected_at": "...", "cleared_at": "..." },
  "observations": { "jobs_enqueued": 20, "jobs_completed": 20, "recovery_time_ms": 3025, "...": "..." },
  "slo": { "job_success_ratio": 1, "recovery_timeout_ms": 120000, "max_dlq_growth": 0 },
  "verdict": "pass",
  "failures": ["..."]
}
```

Key fields:

- `config` — the flags/env that produced the run (reproducibility).
- `fault` — what was injected, the target container, and inject/clear timestamps.
- `observations` — enqueued/completed/failed/dropped counts, DLQ growth,
  recovery time, API unavailability, queue lengths before/after, stale
  reclaimed, worker endpoint health.
- `slo` — the thresholds the run was measured against.
- `verdict` — `pass` when no SLO check failed, `fail` otherwise.
- `failures` — human-readable SLO violations (present only on `fail`).

Notes:

- `redis-partition --partition-method iptables` requires a root host with
  `iptables`; the `docker` method (default) uses the compose network instead.
- The CLI never changes job payloads or queue keys beyond the scenario's own
  enqueue and the forged in-flight scores; each scenario waits for jobs to
  reach a terminal state before finishing.
- Exit code is `0` for completed runs (pass or fail) and non-zero only for
  execution errors (config, unreachable targets, docker failures). Use the
  report `verdict` to drive CI gates.

## Deploy Order

1. Create namespace.
2. Create secrets.
3. Apply RBAC.
4. Apply Redis.
5. Apply Postgres schema if needed.
6. Apply API, worker, and scheduler deployments.
7. Apply service and ingress resources.
8. Confirm `/readyz` returns `200 OK`.

## What Is Still Missing

- a complete ingress manifest
- a production `values.yaml` if you want Helm
- full RBAC/service account definitions
- distributed tracing export

Tracing note:

- the repo now carries a lightweight trace ID hook in request logging
- OTEL/Jaeger can be added later without changing the handler call sites
