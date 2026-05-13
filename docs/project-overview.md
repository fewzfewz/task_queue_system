# Task Queue System Overview

This project is a Go-based distributed task queue. It provides an HTTP API for creating and inspecting jobs, a worker process that executes jobs through pluggable handlers, and a scheduler that promotes delayed jobs and reclaims stalled ones.

## What the system does

At a high level, the system lets a client submit background work and then handles the rest asynchronously:

1. A client sends a job to the API.
2. The API validates the request, checks auth, and stores the job.
3. The job is pushed into Redis for queueing.
4. Worker processes pull jobs from Redis and execute the matching plugin.
5. The result is saved back to the store.
6. Failed jobs go to a dead-letter queue.
7. A scheduler promotes delayed jobs and requeues timed-out ones.
8. Metrics and worker health are exposed for operational monitoring.

## Main pieces

- `cmd/api`
  - Runs the HTTP API server.
  - Exposes job creation, status lookup, metrics, worker health, DLQ, and Swagger.
- `cmd/worker`
  - Runs the job processor.
  - Starts worker goroutines, heartbeats, metrics, webhook dispatching, and graceful shutdown handling.
- `cmd/scheduler`
  - Runs the maintenance loop.
  - Promotes delayed jobs and reclaims jobs that exceeded their visibility timeout.
- `cmd/cli`
  - Provides migration tooling.
  - Currently supports Redis to PostgreSQL job migration.

## Core data flow

### Job submission

When a client submits a job:

1. The request hits `POST /jobs`.
2. `internal/api/middleware/auth.go` validates either a JWT Bearer token or the legacy `X-API-Key`.
3. `internal/api/dto/dto.go` validates the request body.
4. `internal/service/job_service.go` checks allowed job types, tenant rate limits, queue capacity, and scheduling rules.
5. The job is built in `internal/jobs/job.go`.
6. The job is saved to the configured store through `internal/storage`.
7. The job is enqueued into Redis through `internal/queue/redis/redis.go`.
8. The API returns the created job payload.

### Job execution

When a worker processes a job:

1. `internal/worker/pool/pool.go` starts many worker goroutines.
2. `internal/worker/executor/worker_processor.go` dequeues a job.
3. The processor checks for duplicates and marks the job as processing.
4. `internal/worker/executor/job_executor.go` looks up the plugin for the job type.
5. The plugin performs the work.
6. On success, the job is marked completed and the result is saved.
7. On failure, the worker retries the job or sends it to the DLQ.

### Scheduled and stalled jobs

- Delayed jobs are stored in a Redis sorted set until their run time arrives.
- The scheduler moves them into the active queue when they are due.
- Stalled jobs are reclaimed after the visibility timeout and put back into the queue.

## Storage model

The system has two distinct data layers:

- Queue state in Redis
  - Pending jobs
  - In-flight jobs
  - Delayed jobs
  - Processed markers
  - Worker heartbeats
  - Metrics counters
  - Webhook event stream
- Persistent job records
  - Redis store
  - PostgreSQL store
  - Dual mode that writes to both

## Supported job types

Built-in worker plugins currently support:

- `email`
- `image`

The test suite also uses:

- `test`
- `test-success`
- `test-fail`
- `test-scheduled`

## Interfaces

### HTTP endpoints

- `GET /`
  - Main browser UI with tabs for create/search jobs, workers/health, and DLQ/admin actions
- `GET /ui`
  - Alias of the main browser UI
- `GET /admin/dlq`
  - DLQ-focused management console
- `POST /jobs`
  - Create a new job
- `GET /jobs/{id}`
  - Fetch job status
- `GET /metrics`
  - Prometheus metrics
- `GET /workers`
  - Active worker list
- `GET /healthz`
  - Liveness check for API and worker processes
- `GET /readyz`
  - Readiness check that verifies Redis connectivity
- `GET /admin/dlq`
  - DLQ web UI
- `GET /api/v1/dlq`
  - List failed jobs
- `GET /api/v1/dlq/{id}`
  - Inspect one failed job
- `POST /api/v1/dlq/{id}/replay`
  - Re-enqueue a failed job
- `DELETE /api/v1/dlq/{id}`
  - Delete one failed job
- `DELETE /api/v1/dlq`
  - Bulk purge failed jobs
- `GET /swagger/`
  - Swagger UI

### Background processes

- Worker pool
- Scheduler loop
- Webhook dispatcher

### CLI

- `migrate-jobs --from redis --to postgres --batch 500`

## Configuration summary

Important environment variables:

- `PORT`
- `REDIS_HOST`
- `REDIS_PASSWORD`
- `REDIS_DB`
- `API_KEY`
- `JOB_RATE_LIMIT`
- `LOG_LEVEL`
- `MAX_QUEUE_SIZE`
- `STORE_BACKEND`
- `POSTGRES_CONN_STR`
- `JWT_PUBLIC_KEY`
- `JWT_PUBLIC_KEY_PATH`
- `VAULT_ADDR`
- `VAULT_ROLE_ID`
- `VAULT_SECRET_ID`
- `DRAIN_TIMEOUT`

## External systems

- Redis
  - Required for the queue and runtime coordination
- PostgreSQL
  - Optional durable store
- Vault
  - Optional secret provider
- Prometheus
  - Optional metrics consumer
- Webhook targets
  - Optional per job

## Repository structure

- `cmd`
  - Entrypoints for API, worker, scheduler, and CLI
- `internal`
  - Core business logic and infrastructure code
- `db/migrations`
  - Database schema
- `deploy`
  - Kubernetes manifests
- `deployments`
  - Docker build files and compose setup
- `scripts`
  - Load testing and migration helpers
- `test`
  - Integration tests
- `.env.example`
  - Example environment variables for local development
- `Makefile`
  - Common developer commands for tests, builds, and local startup

## New developer quick mental model

- The API creates jobs.
- Redis moves jobs around.
- Workers execute jobs.
- PostgreSQL stores job history when enabled.
- The scheduler keeps delayed and stalled jobs moving.
