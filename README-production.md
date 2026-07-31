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
