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
- JWT auth via `github.com/golang-jwt/jwt/v5`
- Vault integration via `github.com/hashicorp/vault/api`
- Swagger UI via `github.com/swaggo/swag` and `github.com/swaggo/http-swagger/v2`

## Main Entry Points

- `cmd/api/main.go`
- `cmd/worker/main.go`
- `cmd/scheduler/main.go`
- `cmd/cli/main.go`

## Key Routes

- `GET /` or `GET /ui`
  - Browser-based operator UI with tabs for Create/Search Jobs, Workers/Health, and DLQ/Admin actions
- `POST /jobs`
- `GET /jobs/{id}`
- `GET /metrics`
- `GET /workers`
- `GET /healthz`
- `GET /readyz`
- `GET /admin/dlq`
- `GET /api/v1/dlq`
- `GET /api/v1/dlq/{id}`
- `POST /api/v1/dlq/{id}/replay`
- `DELETE /api/v1/dlq/{id}`
- `DELETE /api/v1/dlq`
- `GET /swagger/`

## Configuration

Environment variables read by the code:

- `PORT` default `8080`
- `REDIS_HOST` default `localhost:6379`
- `REDIS_PASSWORD` default empty
- `REDIS_DB` default `0`
- `API_KEY` default `secret-api-key`
- `JOB_RATE_LIMIT` default `0`
- `LOG_LEVEL` default `info`
- `MAX_QUEUE_SIZE` default `10000`
- `STORE_BACKEND` default `redis`
- `POSTGRES_CONN_STR` default empty
- `JWT_PUBLIC_KEY` default empty
- `JWT_PUBLIC_KEY_PATH` default empty
- `VAULT_ADDR` default empty
- `VAULT_ROLE_ID` default empty
- `VAULT_SECRET_ID` default empty
- `DRAIN_TIMEOUT` default `60`

Notes:

- `API_KEY` is used for legacy X-API-Key auth.
- If `JWT_PUBLIC_KEY` or `JWT_PUBLIC_KEY_PATH` is set, Bearer-token auth is enabled.
- If `VAULT_ADDR` is set, the API will try Vault-backed tenant secrets before falling back to `API_KEY`.

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

- `docker-compose.yml`
- `deployments/docker-compose.yml`

The top-level `docker-compose.yml` starts Redis, API, worker, and scheduler.

```bash
docker compose up --build --scale worker=3
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

Run each binary separately:

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

Or:

```bash
make docker-build
```

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

## Swagger

Generate or refresh the Swagger docs with:

```bash
swag init -g cmd/api/main.go
```

The checked-in spec is [docs/swagger.yaml](/home/fewzan/Projects/task-queue-system/docs/swagger.yaml), and it now includes the runtime DLQ, workers, and admin console routes in addition to the core job endpoints.

## Notes

- The DLQ admin page is served at `/admin/dlq`.
- Worker metrics are exposed on `/metrics` from the worker process.
- The scheduler is responsible for moving delayed jobs into active queues and reclaiming timed-out jobs.
