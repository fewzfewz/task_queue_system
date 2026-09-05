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

- `GET /` — public landing page and client registration
- `GET /ui` — browser-based operator UI (session login required)
- `POST /api/v1/login` — authenticate with username/password, starts a session
- `POST /api/v1/logout` — revoke the current session
- `GET /api/v1/session` — current session state (role + CSRF token)
- `GET /api/v1/circuit-breakers` — worker circuit-breaker status (proxied)
- `POST /api/v1/circuit-breakers/reset/{type}` — reset a circuit breaker
- `POST /api/v1/register` — register a tenant and receive a one-time API key
- `GET /api/v1/client/me` — return the authenticated client's tenant ID
- `GET /api/v1/clients` — list registered tenants (operator)
- `DELETE /api/v1/clients/{tenant_id}` — revoke a tenant API key (operator)
- `POST /api/v1/clients/{tenant_id}/rotate` — rotate a tenant API key (operator)
- `GET /api/v1/job-types` — list registered job types
- `POST /api/v1/job-types` — register a custom job type (operator)
- `DELETE /api/v1/job-types/{name}` — remove a custom job type (operator)
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
| `ADMIN_USERNAME` | `admin` | UI login username (admin role) |
| `ADMIN_PASSWORD` | `admin123` | UI login password |
| `READONLY_USERNAME` | `` | Optional viewer-only UI login |
| `READONLY_PASSWORD` | `` | Password for the viewer login |
| `SESSION_TTL_SECONDS` | `28800` | Operator UI session lifetime (seconds) |
| `LOGIN_RATE_LIMIT` | `5` | Login attempts per IP per minute |
| `REGISTER_RATE_LIMIT` | `10` | Client registration attempts per IP per minute |
| `WORKER_ADDR` | `localhost:8081` | Worker address for the circuit-breaker proxy |
| `SLA_TARGET_SECONDS` | `300` | SLA target in seconds |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `` | OpenTelemetry gRPC endpoint |
| `POSTGRES_MIGRATIONS_DIR` | `db/migrations` | Postgres SQL migrations directory |
| `SMTP_HOST` | `` | SMTP server for real email delivery (worker). When unset, email jobs are simulated. |
| `SMTP_PORT` | `587` | SMTP port |
| `SMTP_USER` | `` | SMTP username (optional) |
| `SMTP_PASSWORD` | `` | SMTP password (optional) |
| `SMTP_FROM` | `` | From address for outbound email |

Auth:
- Machine clients authenticate with the `X-API-Key` header. Each registered tenant receives its own key via `POST /api/v1/register`.
- Registered clients are scoped to their `tenant_id`: they can only read or mutate their own jobs, DLQ entries, and webhooks. Operator sessions and the static `API_KEY` are unrestricted.
- Browsers authenticate via a server-side session: `POST /api/v1/login` returns an `HttpOnly` session cookie and a per-session CSRF token. The API key is never exposed to the browser.
- Write endpoints (job control, DLQ replay/purge, webhooks, CB reset, logout) additionally require an admin role and the `X-CSRF-Token` header.
- `GET /events` (SSE) and all job/DLQ/stats/worker endpoints require authentication.

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

The system has three browser-based interfaces, all accessed via the API server (port `8080`).

### Main Operator Dashboard — `GET /ui`

A full-featured sidebar layout with pages for every operation:

| Sidebar Page | Description |
|-------------|-------------|
| **Dashboard** | Stats cards (pending jobs, workers, DLQ, circuit breakers) + quick actions |
| **Create Jobs** | Submit new jobs with type, priority, tenant, dedup key, shard key, payload, scheduling |
| **Search Jobs** | Filter by type/status/tenant with pagination |
| **Stats** | Queue breakdown and system statistics |
| **DAG Dependencies** | Lookup upstream/downstream dependency chains |
| **Workers & Health** | Active workers, Prometheus metrics, health check status |
| **Circuit Breaker** | Per-plugin breaker states with reset action |
| **Dead Letter Queue** | Browse, search, filter, export, replay, purge, bulk purge |
| **Clients** | View registered tenants; revoke or rotate API keys |
| **Webhooks** | Register and list event-driven HTTP callbacks |
| **Job Types** | Register custom job types (delegates to http/email/image handlers) |

### DLQ Admin Console — `GET /admin/dlq`

A focused dead-letter queue management console:

| Sidebar Page | Description |
|-------------|-------------|
| **Overview** | Stats cards (failed count, queues, tenants, workers) |
| **Workers** | Active worker heartbeat cards |
| **DLQ Table** | Sortable/filterable table with replay/purge per row, auto-refresh toggle |

### Client Portal — `/` (Landing), `/client/login`, `/client/dashboard`

A dedicated, styled single-page application for tenants to register, view their API keys, and manage their specific jobs. The public landing page at `/` provides inline registration.

| Page | Description |
|-------------|-------------|
| **Register** | Register a new tenant to generate a one-time API key |
| **Login** | Log in with the API key (stored in `localStorage`) |
| **Dashboard** | Tenant-scoped stats (pending, processing, completed, failed, DLQ, total) + live SSE feed |
| **Submit Job** | Submit jobs with type, priority, JSON payload, optional webhook |
| **My Jobs** | Paginated job list with detail modal |
| **Webhooks** | Register HTTP callbacks for `created`, `completed`, and `failed` events |
| **API Key** | View/copy key and quick-start curl snippet |
| **Docs** | Full API reference with JSON payload examples |

### Auth flow

For operator pages, they show a login overlay on first load. `POST /api/v1/login` validates the credentials and starts a server-side session (an `HttpOnly` session cookie plus a per-session CSRF token). The API key is never returned to the browser and nothing is stored in `localStorage`; the session cookie is sent with every request and the CSRF token rides in the `X-CSRF-Token` header on mutating calls. Sessions expire after `SESSION_TTL_SECONDS` and are revoked by `POST /api/v1/logout` or after 15 minutes of idle.

For the **Client Portal**, authentication is stateless. The client registers their tenant, receives a one-time API key, and that key is stored purely on the client-side (in `localStorage`). It is sent in the `X-API-Key` header for all dashboard requests.

### Built-in job types

| Type | Handler | Description |
|------|---------|-------------|
| `email` | email | Sends mail via SMTP when `SMTP_HOST` is set on the worker; otherwise simulates delivery (logs only). |
| `image` | image | Image processing pipeline (resize, transform) |
| `http` | http | Makes an HTTP request to any URL. Payload: `url` (required), `method`, `headers`, `body`. |

Admins can register additional job types via `POST /api/v1/job-types` or the operator UI **Job Types** page. Custom types specify a handler (`http`, `email`, or `image`) that the worker uses at execution time.

### Webhook events

Use canonical event names: `created`, `completed`, and `failed`. The dispatcher POSTs JSON to your URL:

```json
{
  "job_id": "abc-123",
  "tenant_id": "my-tenant",
  "status": "completed",
  "result": "...",
  "error": "",
  "timestamp": "2026-08-12T10:00:00Z"
}
```

Verify the HMAC signature in the `X-Webhook-Signature: sha256=<hex>` header. The webhook secret is **never** included in the POST body.

## Terminal Examples

Set an API key (machine clients):

```bash
export API_KEY=secret-api-key
```

Browser-style login (creates a session cookie, returns a CSRF token):

```bash
curl -X POST http://localhost:8080/api/v1/login \
  -c /tmp/tq-cookies.txt \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'
# 200 → {"authenticated":true,"role":"admin","csrf_token":"<token>"}
```

All subsequent calls use the session cookie; mutating calls also send `X-CSRF-Token`:

```bash
curl -X POST http://localhost:8080/jobs \
  -b /tmp/tq-cookies.txt \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: <token>" \
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
curl http://localhost:8080/workers -H "X-API-Key: secret-api-key"
```

## Admin vs View-Only Roles

- Sessions are granted a role: `admin` (from `ADMIN_USERNAME`/`ADMIN_PASSWORD`) or `readonly` (from `READONLY_USERNAME`/`READONLY_PASSWORD`, optional).
- Read endpoints (jobs, stats, workers, DLQ GET, SSE, circuit-breaker status) are available to any authenticated session.
- Mutating endpoints (job control, DLQ replay/purge, webhooks, circuit-breaker reset) require the `admin` role plus a valid CSRF token.
- API-key requests bypass role checks — the `X-API-Key` header is the privileged machine identity, so keep it out of browsers.

## Storage and Dependencies

- Redis is required for queueing.
- PostgreSQL is optional unless `STORE_BACKEND=postgres` or `STORE_BACKEND=dual`.
- Vault is optional and only used when configured.
- Prometheus is optional but required if you want to scrape metrics.

## Job Types

The built-in worker plugins currently support:

- `email` (SMTP when configured, simulated otherwise)
- `image`
- `http`

Admins can register additional types via the operator UI or `POST /api/v1/job-types`.

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
- webhook tests (unit + integration: HMAC verification, exponential backoff,
  retry/give-up behaviour, payload integrity, concurrent deliveries)
- a build-tagged chaos suite
- a Postgres integration suite (every `Store` interface method against a real Postgres, run in CI)
- a Vault integration suite (TTL caching, expiry/refetch, stale-while-revalidate, and
  single-flight coalescing against a real Vault dev server, run in CI)

Known gaps:

- Chaos tests are present, but they are opt-in via build tags and Docker

Optional integration workflows:

```bash
RUN_QUEUE_INTEGRATION=1 go test ./test
make test-postgres
make test-webhooks
make test-vault
go test ./internal/webhooks -run DispatcherSendIntegration
```

The Postgres store suite is no longer gated on `POSTGRES_CONN_STR` alone — it
has a dedicated runner:

```bash
make test-postgres
```

This starts the Postgres container from `deploy/test/docker-compose.yml`,
executes `go test ./test/... ./internal/storage/...` against it, and tears it
down. The `jobs` table is truncated before/after each test. See
[README-production.md](/home/fewzan/Projects/task-queue-system/README-production.md#postgres-integration-tests)
for local and CI details.

Webhook delivery tests run in the default suite and need no external services
(they use `httptest` and in-memory Redis). To additionally exercise the full
Redis stream → dispatcher pipeline, run:

```bash
make test-webhooks
```

This starts a dedicated Redis container on `:6380` (so it never collides with a
running `make dev`/compose stack on `:6379`), runs
`RUN_QUEUE_INTEGRATION=1 go test -race ./internal/webhooks/`, and tears the
container down. CI runs this in a dedicated `test-webhooks` job against a
Redis service. See
[README-production.md](/home/fewzan/Projects/task-queue-system/README-production.md#webhook-integration-tests)
for details.

Vault secret integration tests run in the default suite too, but skip unless a
Vault dev server is reachable. To exercise them against a real Vault:

```bash
make test-vault
```

This starts a Vault dev server container from `deploy/test/docker-compose.vault.yml`
on `:8200`, runs `go test -race ./internal/secrets/`, and tears the container
down. CI runs this in a dedicated `test-vault` job against a Vault service. See
[README-production.md](/home/fewzan/Projects/task-queue-system/README-production.md#vault-integration-tests)
for details.

## Production Deployment

What exists:

- Local Docker Compose and per-component Dockerfiles under [deploy/local/](/home/fewzan/Projects/task-queue-system/deploy/local)
- Kubernetes manifests under [deploy/k8s](/home/fewzan/Projects/task-queue-system/deploy/k8s):
  - `api-deployment.yaml` — API service
  - `worker-deployment.yaml` — worker service
  - `scheduler-deployment.yaml` — scheduler service
  - `redis-statefulset.yaml` + `redis-service.yaml` — Redis cluster
  - `postgres-statefulset.yaml` + `postgres-service.yaml` — optional PostgreSQL
  - `secrets.yaml` — Opaque Secret template for API key + admin password (empty values, fails closed)
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

## Client Integration Guide (How to Use This System)

If you are a developer integrating a separate application with this Task Queue, you can either interact exclusively with the HTTP API using your machine client, or use the **Client Portal UI**. Each application gets its own unique API key tied to its own `tenant_id`.

### Option A: Use the Client Portal UI (Recommended for Users)

1. Navigate to `http://localhost:8080/`.
2. Enter your desired tenant name in the registration section.
3. The system will provide your **API Key**. Save this immediately! It will be automatically saved to your browser's `localStorage` for the current session.
4. You will be redirected to the **Client Dashboard** (`/client/dashboard`) where you can submit jobs, view your task queue, and check statuses using a rich, modern UI.
5. In the future, you can return and access the dashboard by entering your API key at `http://localhost:8080/client/login`.

### Option B: Use the HTTP API (For Application Backends)

#### Step 1: Register and get your API key

Call the open `/api/v1/register` endpoint with your service's name. No prior credentials are needed.

```bash
curl -X POST http://localhost:8080/api/v1/register \
  -H "Content-Type: application/json" \
  -d '{"tenant_id": "my-backend-service"}'
```

You will get a response containing your unique, one-time API key:

```json
{
  "tenant_id": "my-backend-service",
  "api_key": "tq_live_a3f...c8d",
  "message": "Store this API key safely. It will not be shown again."
}
```

> ⚠️ **Save this key immediately.** Only the hash is stored on the server. The raw key cannot be retrieved again.

### Step 2: Authenticate every request

Include your key in the `X-API-Key` header on every request:

```http
X-API-Key: tq_live_a3f...c8d
```

### Step 3: Submit a job

Hand off a background task by posting to `/jobs`:

```bash
curl -X POST http://localhost:8080/jobs \
  -H "X-API-Key: tq_live_a3f...c8d" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "email",
    "payload": {"to": "user@example.com", "subject": "Welcome!"},
    "priority": "high",
    "tenant_id": "my-backend-service"
  }'
```

The system returns a `job_id`. Your application can immediately return a response to your user — the work happens in the background.

### Step 4: Get notified when the job finishes (Webhooks)

Instead of polling, register a webhook. The Task Queue will call your application when a job succeeds or fails:

```bash
curl -X POST http://localhost:8080/api/v1/webhooks \
  -H "X-API-Key: tq_live_a3f...c8d" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://my-backend-service.com/hooks/task-queue",
    "events": ["created", "completed", "failed"]
  }'
```

Verify the HMAC signature on the incoming webhook payload to ensure it genuinely came from the Task Queue.

## Project Roadmap & Remaining Work

Core functionality is production-ready. Remaining items are optional hardening and polish:

- **Deployment & Infrastructure:**
  - Document TLS / Ingress setup for Kubernetes deployments.
  - Finalize production-grade Namespace, Service Account, and RBAC hardening.
- **Testing & Tooling:**
  - Integration tests for HashiCorp Vault tenant-level secret rotation.

See [docs/ROADMAP.md](/home/fewzan/Projects/task-queue-system/docs/ROADMAP.md) for the full completed-feature history.
