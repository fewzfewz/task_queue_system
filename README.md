# Task Queue System

This repository contains a Go-based distributed task queue system with four binaries: an HTTP API, a worker, a scheduler, and a small CLI for migration tasks. Jobs are stored in Redis for queueing and state tracking, with optional PostgreSQL persistence for the job record store.

## What it does

- Accepts jobs over HTTP.
- Persists job state in Redis, PostgreSQL, or both.
- Processes jobs with pluggable worker handlers.
- Supports delayed jobs, retries, DLQ inspection, metrics, worker heartbeats, and webhook delivery.

## Architecture

The runtime is a small distributed system:

- `cmd/api` exposes the HTTP API.
- `cmd/worker` consumes queued jobs and executes plugins.
- `cmd/scheduler` promotes delayed jobs and reclaims timed-out jobs.
- `cmd/cli` provides a data migration command.

Redis is the queue broker and also stores queue state, worker heartbeats, processed markers, delayed jobs, webhook stream events, and queue metrics. PostgreSQL is an optional durable store for job records.

## Tech Stack

- Go `1.25.0` in `go.mod`
- Redis via `github.com/redis/go-redis/v9`
- PostgreSQL via `github.com/jackc/pgx/v5`
- Prometheus metrics via `github.com/prometheus/client_golang`
- API key auth via `X-API-Key` header
- Swagger UI via `github.com/swaggo/swag` and `github.com/swaggo/http-swagger/v2`

## Main Entry Points

- `cmd/api/main.go`
- `cmd/worker/main.go`
- `cmd/scheduler/main.go`
- `cmd/cli/main.go`

## Key Routes

- `GET /` or `GET /ui` — browser-based operator UI (login required)
- `POST /api/v1/login` — authenticate with username/password, returns api_key
- `POST /jobs` — create a job
- `POST /jobs/batch` — batch create up to 100 jobs
- `GET /jobs` — list/search jobs
- `GET /jobs/{id}` — get job status
- `PATCH /jobs/{id}/progress` — update job progress
- `POST /jobs/{id}/cancel` — cancel a job
- `POST /jobs/{id}/pause` — pause a job
- `POST /jobs/{id}/resume` — resume a job
- `GET /api/v1/jobs/{id}/deps` — get dependency graph
- `GET /api/v1/stats` — system statistics
- `GET /metrics` — Prometheus metrics
- `GET /workers` — active worker instances
- `GET /events` — SSE job event stream
- `GET /healthz` — liveness probe
- `GET /readyz` — readiness probe
- `GET /admin/dlq` — DLQ admin console
- `GET /api/v1/dlq` — list failed jobs
- `GET /api/v1/dlq/{id}` — failed job detail
- `POST /api/v1/dlq/{id}/replay` — re-enqueue a failed job
- `DELETE /api/v1/dlq/{id}` — purge a failed job
- `DELETE /api/v1/dlq` — bulk purge failed jobs
- `POST /api/v1/webhooks` — register webhook
- `GET /api/v1/webhooks` — list webhooks
- `GET /api/v1/webhooks/{id}` — get webhook
- `PUT /api/v1/webhooks/{id}` — update webhook
- `DELETE /api/v1/webhooks/{id}` — delete webhook
- `GET /swagger/` — Swagger UI

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | API HTTP port |
| `WORKER_PORT` | `8081` | Worker metrics/health port |
| `SCHEDULER_PORT` | `8082` | Scheduler metrics/health port |
| `REDIS_HOST` | `localhost:6379` | Redis address |
| `REDIS_PASSWORD` | `` | Redis password |
| `REDIS_DB` | `0` | Redis database number |
| `API_KEY` | `secret-api-key` | API key for protected endpoints (X-API-Key header) |
| `JOB_RATE_LIMIT` | `0` | Worker throughput limit (jobs/sec) |
| `TENANT_RATE_LIMIT` | `0` | Per-tenant API rate limit (req/sec) |
| `LOG_LEVEL` | `info` | Log level |
| `MAX_QUEUE_SIZE` | `10000` | Max pending jobs |
| `STORE_BACKEND` | `redis` | Store backend (redis, postgres, dual) |
| `POSTGRES_CONN_STR` | `` | PostgreSQL connection string |
| `DRAIN_TIMEOUT` | `60` | Worker drain timeout (seconds) |
| `WORKER_POOL_SIZE` | `50` | Worker goroutine count |
| `ADMIN_USERNAME` | `admin` | UI login username |
| `ADMIN_PASSWORD` | `admin123` | UI login password |
| `SLA_TARGET_SECONDS` | `300` | SLA target in seconds |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `` | OpenTelemetry gRPC endpoint |
| `POSTGRES_MIGRATIONS_DIR` | `db/migrations` | Postgres SQL migrations directory |

Auth:
- All protected endpoints require the `X-API-Key` header set to the value of `API_KEY`.
- The `/api/v1/login` endpoint accepts `{"username":"...","password":"..."}` and returns the API key.
- The UI prompts for credentials on first load, then stores the returned key in `localStorage`.

## Local Run

### 1. Install dependencies

```bash
go mod download
```

Or use the Makefile:

```bash
make deps
```

### 2. Start infrastructure

The repo provides Docker Compose files:

- `deploy/local/docker-compose.yml`

The local compose file starts Redis, API, worker, and scheduler.

```bash
docker compose -f deploy/local/docker-compose.yml up --build --scale worker=3
```

Or:

```bash
make docker-up
```

### 3. Minimum environment

The default Compose file already sets the basic Redis and API key values. For a local shell run, the minimum useful values are:

```bash
export PORT=8080
export REDIS_HOST=localhost:6379
export REDIS_PASSWORD=
export REDIS_DB=0
export API_KEY=secret-api-key
export STORE_BACKEND=redis
```

An example file is also provided at [`.env.example`](/home/fewzan/Projects/task-queue-system/.env.example).

### 4. Run in development

All-in-one (starts Redis + all 3 services):

```bash
make dev
```

Or run each binary separately:

```bash
go run ./cmd/api
go run ./cmd/worker
go run ./cmd/scheduler
```

Or use Makefile shortcuts:

```bash
make run-api
make run-worker
make run-scheduler
```

### 5. Run tests

```bash
go test ./...
```

Or:

```bash
make test
```

### 6. Build production binaries

```bash
go build -o bin/api ./cmd/api
go build -o bin/worker ./cmd/worker
go build -o bin/scheduler ./cmd/scheduler
go build -o bin/cli ./cmd/cli
```

Or:

```bash
make build
```

### 7. Docker build

```bash
docker build -t task-queue-system .
```

Per-component Dockerfiles are also available:

- `deploy/local/docker/api.Dockerfile`
- `deploy/local/docker/worker.Dockerfile`
- `deploy/local/docker/scheduler.Dockerfile`

Or:

```bash
make docker-build
```

## UI

Open the browser UI at `http://localhost:8080/`. The UI prompts for credentials (defaults: `admin` / `admin123`), then stores the returned API key in `localStorage` for subsequent requests.

Tabs:

- Create / Search Jobs
- Workers / Health
- DLQ / Admin

## Terminal Examples

Set a token first if you use Bearer or legacy API key auth:

```bash
export API_KEY=secret-api-key
```

Create a job:

```bash
curl -X POST http://localhost:8080/jobs \
  -H "Content-Type: application/json" \
  -H "X-API-Key: secret-api-key" \
  -d '{"type":"email","payload":{"to":"user@example.com","subject":"Hello"},"priority":"medium","tenant_id":"tenant-a"}'
```

Search a job by ID:

```bash
curl http://localhost:8080/jobs/<job-id> \
  -H "X-API-Key: secret-api-key"
```

Delete a failed job from the DLQ:

```bash
curl -X DELETE http://localhost:8080/api/v1/dlq/<job-id> \
  -H "X-API-Key: secret-api-key"
```

Replay a failed job:

```bash
curl -X POST http://localhost:8080/api/v1/dlq/<job-id>/replay \
  -H "X-API-Key: secret-api-key"
```

Bulk purge failed jobs older than a timestamp:

```bash
curl -X DELETE "http://localhost:8080/api/v1/dlq?older_than=2026-05-01T00:00:00Z&queue=email" \
  -H "X-API-Key: secret-api-key"
```

Check health and readiness:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

Get metrics and workers:

```bash
curl http://localhost:8080/metrics
curl http://localhost:8080/workers
```

## Admin vs Everyone Else

- The main UI at `/` and `/ui` is for general users and operators.
- The admin DLQ page at `/admin/dlq` is a DLQ-focused console with stronger operational emphasis.
- The API routes under `/api/v1/dlq` are the actual privileged backend actions both UIs use.
- In practice, the admin page is just a more DLQ-centric view, not a different security model by itself.

## Storage and Dependencies

- Redis is required for queueing.
- PostgreSQL is optional unless `STORE_BACKEND=postgres` or `STORE_BACKEND=dual`.
- Vault is optional and only used when configured.
- Prometheus is optional but required if you want to scrape metrics.

## Job Types

The built-in worker plugins currently support:

- `email`
- `image`

The API also accepts several test job types used by the test suite:

- `test`
- `test-success`
- `test-fail`
- `test-scheduled`

## Useful Scripts

- `scripts/migrate.sh`
- `scripts/load_test.sh`
- `scripts/benchmark.sh`

## Database Migrations

The repository includes versioned SQL schema files under [db/migrations](/home/fewzan/Projects/task-queue-system/db/migrations).

Current migration files:

- `db/migrations/001_create_jobs.sql`

How they run:

- **Auto-migration**: `postgres.New()` runs all SQL files in `POSTGRES_MIGRATIONS_DIR` (default `db/migrations`) in sorted order on every connect.
- **CLI**: `make migrate-schema` runs `go run ./cmd/cli migrate-schema -dir=db/migrations` to apply migrations independently.
- **Migration tool**: `cmd/cli migrate-jobs` moves job data from Redis to Postgres.

If you use Postgres locally, ensure the migrations directory is accessible before starting the API or worker with `STORE_BACKEND=postgres` or `STORE_BACKEND=dual`.

## Chaos Engineering Tests

The repository includes a build-tagged chaos test package in [chaos](/home/fewzan/Projects/task-queue-system/chaos).

What it currently does:

- Spins up Redis in Docker during tests
- Builds the worker binary
- Injects failure scenarios such as:
  - Redis crash mid-transition
  - worker hard kill
  - network partition
  - orphan recovery / timing skew
- Produces structured `Report` output in `chaos/report.go`

How to run it:

```bash
go test -tags chaos ./chaos
```

Or export a JSON report for CI dashboards:

```bash
./scripts/chaos.sh chaos-report.json
```

Notes:

- These tests require Docker access.
- They are separate from the normal `go test ./...` flow.
- There is not yet a dedicated standalone chaos CLI or JSON report exporter.

## Vault and Tenant Secrets

Vault support lives in [internal/secrets/vault.go](/home/fewzan/Projects/task-queue-system/internal/secrets/vault.go).

What it does:

- Authenticates with Vault AppRole
- Reads tenant secrets from:
  - `secret/data/taskqueue/<tenant_id>`
- Caches secrets in memory for a short TTL

Configuration:

- `VAULT_ADDR`
- `VAULT_ROLE_ID`
- `VAULT_SECRET_ID`

Notes:

- Vault is optional. If it is not configured, the API falls back to environment-based secrets.
- Tenant-level secret rotation and cache invalidation are implemented with a simple TTL cache, but the repo does not yet include a dedicated integration test for that behavior.

## Observability

What exists:

- Prometheus metrics from `internal/metrics`
- Health and readiness endpoints:
  - `GET /healthz`
  - `GET /readyz`
- Worker metrics endpoint:
  - `GET /metrics`
- HPA manifest for worker autoscaling:
  - [deploy/k8s/hpa.yaml](/home/fewzan/Projects/task-queue-system/deploy/k8s/hpa.yaml)
- Prometheus Adapter config for external metrics:
  - [deploy/k8s/prometheus-adapter-configmap.yaml](/home/fewzan/Projects/task-queue-system/deploy/k8s/prometheus-adapter-configmap.yaml)
- Grafana dashboard and provisioning:
  - [deploy/grafana/dashboard.json](/home/fewzan/Projects/task-queue-system/deploy/grafana/dashboard.json)
  - [deploy/grafana/provisioning/dashboards/dashboards.yaml](/home/fewzan/Projects/task-queue-system/deploy/grafana/provisioning/dashboards/dashboards.yaml)
  - [deploy/grafana/provisioning/datasources/datasource.yaml](/home/fewzan/Projects/task-queue-system/deploy/grafana/provisioning/datasources/datasource.yaml)
- Prometheus alerting rules:
  - [deploy/monitoring/prometheus-rules.yaml](/home/fewzan/Projects/task-queue-system/deploy/monitoring/prometheus-rules.yaml)
- Ingress:
  - [deploy/k8s/ingress.yaml](/home/fewzan/Projects/task-queue-system/deploy/k8s/ingress.yaml)
- RBAC:
  - [deploy/k8s/rbac.yaml](/home/fewzan/Projects/task-queue-system/deploy/k8s/rbac.yaml)

## Test Coverage

The repo includes:

- unit tests
- store tests
- health tests
- webhook tests
- a build-tagged chaos suite

Known gaps:

- The Postgres store integration workflow is opt-in and only runs when `POSTGRES_CONN_STR` is set
- The webhook delivery integration test is opt-in and uses a mock HTTP server
- Chaos tests are present, but they are opt-in via build tags and Docker

Optional integration workflows:

```bash
RUN_QUEUE_INTEGRATION=1 go test ./test
POSTGRES_CONN_STR='postgres://...' go test ./test -run PostgresStoreIntegrationWorkflow
go test ./internal/webhooks -run DispatcherSendIntegration
```

## Production Deployment

What exists:

- Local Docker Compose and per-component Dockerfiles under [deploy/local/](/home/fewzan/Projects/task-queue-system/deploy/local)
- Kubernetes manifests under [deploy/k8s](/home/fewzan/Projects/task-queue-system/deploy/k8s):
  - `api-deployment.yaml` — API service
  - `worker-deployment.yaml` — worker service
  - `scheduler-deployment.yaml` — scheduler service
  - `redis-statefulset.yaml` + `redis-service.yaml` — Redis cluster
  - `postgres-statefulset.yaml` + `postgres-service.yaml` — optional PostgreSQL
  - `secrets.yaml` — Opaque Secret for API key, admin password, postgres conn string
  - `hpa.yaml` — worker horizontal pod autoscaler
  - `ingress.yaml` — ingress rules
  - `rbac.yaml` — service accounts and roles
  - `prometheus-adapter-configmap.yaml` — external metrics adapter
- A production-focused guide at [README-production.md](/home/fewzan/Projects/task-queue-system/README-production.md)

What is not yet fully documented:

- TLS / ingress setup
- secret management workflow for production clusters
- production-grade namespace / service account / RBAC setup
- a single end-to-end production deployment guide

Practical note:

- The Kubernetes manifests are a starting point, not a full hardened production deployment package.
- See [README-production.md](/home/fewzan/Projects/task-queue-system/README-production.md) for the current production deployment workflow, secret handling, and deploy order.

## Deploy Folders

There are two deployment-related folders and they serve different purposes:

- [deploy/](/home/fewzan/Projects/task-queue-system/deploy)
  - production-oriented Kubernetes and monitoring assets
  - includes API, worker, scheduler, Redis, HPA, Prometheus rules, and Grafana files
- [deploy/local/](/home/fewzan/Projects/task-queue-system/deploy/local)
  - container build and local compose examples
  - useful for local development and image builds

What matters most for this system:

- `deploy/k8s` for Kubernetes runtime
- `deploy/monitoring` and `deploy/grafana` for observability
- `deploy/local/docker-compose.yml` for local bring-up

## Terminal Access

This repo does not include a hidden in-app shell or dedicated command terminal screen.
The “terminal” for this system is the real shell you already use on the project folder.

Use it to run:

- the API: `go run ./cmd/api`
- the worker: `go run ./cmd/worker`
- the scheduler: `go run ./cmd/scheduler`
- the migration CLI: `go run ./cmd/cli migrate-jobs ...`
- the schema migrator: `make migrate-schema`
- the chaos workflow: `make chaos`

If you want a quick local control loop, the simplest route is:

```bash
docker compose -f deploy/local/docker-compose.yml up --build --scale worker=3
```

Then use the browser UI at `http://localhost:8080/` or the terminal commands above.

## Swagger

The API is fully annotated with Swagger Go comments. Regenerate the spec with:

```bash
swag init -g cmd/api/main.go --output docs
```

The checked-in spec files (`docs/swagger.yaml`, `docs/swagger.json`, `docs/docs.go`) cover all 30+ endpoints including DAG dependencies, batch operations, progress updates, pause/resume, webhooks CRUD, SSE, stats, and DLQ management.

## Notes

- The DLQ admin page is served at `/admin/dlq`.
- Worker metrics are exposed on `/metrics` from the worker process.
- The scheduler is responsible for moving delayed jobs into active queues and reclaiming timed-out jobs.
- The `chaos/` package is build-tagged and intentionally excluded from normal `go test ./...` runs.
- Request logs now include a lightweight trace ID so the project is ready for a future OTEL exporter without changing the call sites.
